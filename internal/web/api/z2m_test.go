package api_test

import (
	"encoding/json"
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
	"github.com/homeland-live/homeland/internal/web/api"
	webcontext "github.com/homeland-live/homeland/internal/web/context"
	"github.com/homeland-live/homeland/internal/z2m"
)

// lampRoster is a bridge/devices payload with a single device, "lamp" at
// 0x01.
var lampRoster = []byte(`[{"ieee_address":"0x01","friendly_name":"lamp"}]`)

// newApp wires the API over a real (temp) DB and a z2m client fed through its
// message path, and returns the fiber app plus the client and DB.
func newApp(t *testing.T) (*fiber.App, *z2m.Client, *db.Repo) {
	t.Helper()
	return newAppWithScripts(t)
}

// newAppWithScripts is newApp with a started script service that has loaded
// a working script for each of the given file names.
func newAppWithScripts(t *testing.T, fileNames ...string) (*fiber.App, *z2m.Client, *db.Repo) {
	t.Helper()

	repo, err := db.New(&config.DB{Path: filepath.Join(t.TempDir(), "test.db")})
	require.NoError(t, err)
	t.Cleanup(repo.Close)

	client, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)

	dir := t.TempDir()
	for _, name := range fileNames {
		src := []byte(`module.exports = { handle(ctx, state) {} };`)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), src, 0o600))
	}
	svc := scripts.New(&config.Config{Scripts: &config.Scripts{Dir: dir}}, client)
	require.NoError(t, svc.Start())
	t.Cleanup(svc.Shutdown)

	ctx := &webcontext.Context{
		DB:      repo,
		Z2M:     client,
		Scripts: svc,
		Events:  event.New(svc, client, repo),
	}

	app := fiber.New()
	api.New(ctx).Routes(app)
	return app, client, repo
}

func TestListDeviceEventsDeviceWithoutState(t *testing.T) {
	app, client, _ := newApp(t)
	// Known to the bridge, but has not reported any state yet.
	client.Handle("bridge/devices", lampRoster)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/devices/0x01/events", nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.JSONEq(t, `null`, string(body["state"]), "a device without state must serialize state as null")
	assert.JSONEq(t, `[]`, string(body["events"]))
}

func TestListDeviceEventsDeviceWithState(t *testing.T) {
	app, client, _ := newApp(t)
	client.Handle("bridge/devices", lampRoster)
	client.Handle("lamp", []byte(`{"state":"ON"}`))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/devices/0x01/events", nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		State *struct {
			ID    string         `json:"id"`
			Props map[string]any `json:"props"`
		} `json:"state"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotNil(t, body.State)
	assert.NotEmpty(t, body.State.ID)
	assert.Equal(t, map[string]any{"state": "ON"}, body.State.Props)
}

func TestListDeviceEventsByFriendlyNameOrAddress(t *testing.T) {
	app, client, repo := newApp(t)
	client.Handle("bridge/devices", lampRoster)
	// Events are stored by IEEE address.
	require.NoError(t, repo.CreateEvent(&db.Event{
		IEEEAddress: "0x01",
		Payload:     map[string]json.RawMessage{"state": json.RawMessage(`"ON"`)},
	}))

	// Looking the device up by either id must return the same events.
	for _, id := range []string{"0x01", "lamp"} {
		t.Run(id, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/devices/"+id+"/events", nil))
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var body struct {
				Events []db.Event `json:"events"`
				Page   db.Page    `json:"page"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			require.Len(t, body.Events, 1)
			assert.Equal(t, "0x01", body.Events[0].IEEEAddress)
			assert.Equal(t, 1, body.Page.Count)
		})
	}
}

func TestListDeviceEventsUnknownDevice(t *testing.T) {
	app, _, _ := newApp(t)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/devices/nope/events", nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetDeviceScriptByFriendlyNameOrAddress(t *testing.T) {
	app, client, _ := newApp(t)
	client.Handle("bridge/devices", lampRoster)

	// The device has no script, so both fields are null for either id.
	for _, id := range []string{"0x01", "lamp"} {
		t.Run(id, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/devices/"+id+"/script", nil))
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var body map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			assert.JSONEq(t, `null`, string(body["script"]))
			assert.JSONEq(t, `null`, string(body["handler"]))
		})
	}
}

func TestGetDeviceScriptUnknownDevice(t *testing.T) {
	app, _, _ := newApp(t)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/devices/nope/script", nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDeviceRoutesDecodeEncodedFriendlyName(t *testing.T) {
	app, client, _ := newApp(t)
	// Friendly names may hold spaces and slashes; clients percent-encode them.
	client.Handle("bridge/devices", []byte(
		`[{"ieee_address":"0x01","friendly_name":"Living Room"},{"ieee_address":"0x02","friendly_name":"kitchen/light"}]`))

	for _, id := range []string{"Living%20Room", "kitchen%2Flight"} {
		for _, route := range []string{"events", "script"} {
			t.Run(id+"/"+route, func(t *testing.T) {
				resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/devices/"+id+"/"+route, nil))
				require.NoError(t, err)
				t.Cleanup(func() { _ = resp.Body.Close() })
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			})
		}
	}
}

func TestGetDeviceScriptReportsTwoScriptsConflict(t *testing.T) {
	app, client, _ := newAppWithScripts(t, "0x01.js", "lamp.js")
	client.Handle("bridge/devices", lampRoster)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/devices/lamp/script", nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Script struct {
			FileName string `json:"filename"`
		} `json:"script"`
		Handler struct {
			HasHandle bool `json:"hasHandle"`
			Error     struct {
				Msg string `json:"msg"`
			} `json:"error"`
		} `json:"handler"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body.Handler.Error.Msg, "has two scripts, 0x01.js and lamp.js")
	assert.False(t, body.Handler.HasHandle)
	assert.Equal(t, "0x01.js", body.Script.FileName)
}

// TestDeviceRoutesReturnOptions covers the options both device routes return:
// the options z2m applies to the device, null until bridge/info arrives and
// while none are set.
func TestDeviceRoutesReturnOptions(t *testing.T) {
	app, client, _ := newApp(t)
	client.Handle("bridge/devices", lampRoster)

	options := func(t *testing.T) (list, events json.RawMessage) {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/bridge/devices", nil))
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		var listBody struct {
			Devices []map[string]json.RawMessage `json:"devices"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&listBody))
		require.Len(t, listBody.Devices, 1)

		resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/z2m/devices/lamp/events", nil))
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		var eventsBody map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&eventsBody))
		return listBody.Devices[0]["options"], eventsBody["options"]
	}

	list, events := options(t)
	assert.JSONEq(t, `null`, string(list))
	assert.JSONEq(t, `null`, string(events))

	client.Handle("bridge/info", []byte(`{"version":"2.6.0","config":{"devices":{"0x01":{"friendly_name":"lamp"}}}}`))

	list, events = options(t)
	assert.JSONEq(t, `null`, string(list), "no options set")
	assert.JSONEq(t, `null`, string(events), "no options set")

	client.Handle("bridge/info", []byte(`{"version":"2.6.0","config":{
		"device_options":{"filtered_attributes":["battery"]},
		"devices":{"0x01":{"filtered_attributes":["linkquality"]}}
	}}`))

	list, events = options(t)
	assert.JSONEq(t, `{"filtered_attributes":["linkquality"]}`, string(list))
	assert.JSONEq(t, `{"filtered_attributes":["linkquality"]}`, string(events))
}
