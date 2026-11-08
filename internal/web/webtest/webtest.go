// Package webtest builds the JSON API over test dependencies for HTTP-level
// tests. It is imported by tests only.
package webtest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/db"
	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
	"github.com/homeland-live/homeland/internal/svc/event"
	"github.com/homeland-live/homeland/internal/svc/scripts"
	"github.com/homeland-live/homeland/internal/web"
	"github.com/homeland-live/homeland/internal/web/api"
	webcontext "github.com/homeland-live/homeland/internal/web/context"
	"github.com/homeland-live/homeland/internal/z2m"
)

// stubScript is a working script with an empty handle.
const stubScript = `module.exports = { handle(ctx, state) {} };`

// Server is the API wired over a Ctx with a temp DB, a broker-less z2m
// client fed through HandleZ2M, a started script service, an unstarted event
// service and no scheduler. It serves requests through fiber's App.Test,
// without a listener or the server's middleware.
type Server struct {
	App *fiber.App
	Ctx *webcontext.Context
}

type options struct {
	// scripts maps a file name in the scripts dir to its source.
	scripts map[string]string
}

// Option configures NewServer.
type Option func(*options)

// WithScript writes a script file with the given source to the scripts dir
// before the script service starts. A later option for the same file name
// replaces its source.
func WithScript(fileName, src string) Option {
	return func(o *options) {
		if o.scripts == nil {
			o.scripts = make(map[string]string)
		}
		o.scripts[fileName] = src
	}
}

// WithScripts writes a working script with an empty handle for each of the
// given file names, as WithScript does.
func WithScripts(fileNames ...string) Option {
	return func(o *options) {
		for _, name := range fileNames {
			WithScript(name, stubScript)(o)
		}
	}
}

// NewServer returns a Server whose Ctx is shut down when the test ends,
// before its temp dirs are removed.
//
// It builds the Ctx itself rather than through webcontext.New, which would
// connect a real broker and start the event service and the scheduler.
func NewServer(t *testing.T, opts ...Option) *Server {
	t.Helper()

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	// Cleanups run last-in first-out, so both dirs must exist before
	// ctx.Shutdown is registered for the DB to close before they go.
	scriptsDir, dbDir := t.TempDir(), t.TempDir()
	for name, src := range o.scripts {
		require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, name), []byte(src), 0o600))
	}

	// Shutdown skips nil fields, so a failed step releases what exists so far.
	ctx := &webcontext.Context{}
	t.Cleanup(ctx.Shutdown)

	var err error
	ctx.DB, err = db.New(&config.DB{Path: filepath.Join(dbDir, "test.db")})
	require.NoError(t, err)

	ctx.Z2M, err = z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)

	ctx.Scripts = scripts.New(&config.Config{Scripts: &config.Scripts{Dir: scriptsDir}}, ctx.Z2M)
	require.NoError(t, ctx.Scripts.Start())

	ctx.Events = event.New(ctx.Scripts, ctx.Z2M, ctx.DB)

	app := fiber.New(fiber.Config{ErrorHandler: web.ErrorHandler})
	api.New(ctx).Routes(app)

	return &Server{App: app, Ctx: ctx}
}

// HandleZ2M feeds a live z2m message on a base-topic-relative topic, as
// z2m.Client.Handle does.
func (s *Server) HandleZ2M(topic string, payload []byte) {
	s.Ctx.Z2M.Handle(topic, payload)
}

// GetJSON sends a GET request for path, requires the response to have
// wantStatus, and decodes its JSON body into v.
func (s *Server) GetJSON(t *testing.T, path string, wantStatus int, v any) {
	t.Helper()

	status, body := s.get(t, path)
	require.Equal(t, wantStatus, status, "GET %s", path)
	require.NoError(t, json.Unmarshal(body, v), "GET %s", path)
}

// GetStatus sends a GET request for path and asserts the response has
// wantStatus, ignoring its body.
func (s *Server) GetStatus(t *testing.T, path string, wantStatus int) {
	t.Helper()

	status, _ := s.get(t, path)
	assert.Equal(t, wantStatus, status, "GET %s", path)
}

// get sends a GET request for path and returns the response's status and
// body.
func (s *Server) get(t *testing.T, path string) (int, []byte) {
	t.Helper()

	resp, err := s.App.Test(httptest.NewRequest(http.MethodGet, path, nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "GET %s", path)
	return resp.StatusCode, body
}
