package context_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
	webcontext "github.com/homeland-live/homeland/internal/web/context"
)

// newConfig returns a config over a temp DB and scripts dir, with the watcher
// on and the z2m broker at an address nothing listens on.
func newConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		DB: &config.DB{Path: filepath.Join(t.TempDir(), "homeland.db")},
		Z2MMQTT: &config.MQTTConn{
			Broker:    "tcp://" + mqtttest.FreeAddr(t),
			ClientID:  "test",
			BaseTopic: config.DefaultZ2MBaseTopic,
		},
		Scripts: &config.Scripts{Dir: t.TempDir(), EnableWatcher: true},
	}
}

// assertDBClosed checks the database was opened and then closed: SQLite
// removes the WAL file when the last connection closes.
func assertDBClosed(t *testing.T, dbPath string) {
	t.Helper()
	assert.FileExists(t, dbPath)
	_, err := os.Stat(dbPath + "-wal")
	assert.ErrorIs(t, err, os.ErrNotExist, "the database must be closed")
}

func TestNewStartsWhileBrokerIsDown(t *testing.T) {
	cfg := newConfig(t)

	ctx, err := webcontext.New(cfg)
	require.NoError(t, err, "an unreachable broker must not stop startup")
	assert.False(t, ctx.HealthCheck().IsHealthy, "the broker connection is reported as down")

	ctx.Shutdown()
	assertDBClosed(t, cfg.DB.Path)
}

func TestNewShutsDownWhatItBuiltWhenAStepFails(t *testing.T) {
	// The external broker URL doesn't parse, so svc/scripts.Start fails after
	// the DB, the z2m client and the event service exist.
	cfg := newConfig(t)
	cfg.ExtMQTT = &config.MQTTConn{Broker: "://127.0.0.1:1", ClientID: "test-ext"}

	ctx, err := webcontext.New(cfg)
	require.ErrorContains(t, err, "invalid broker URL")
	assert.Nil(t, ctx)
	assertDBClosed(t, cfg.DB.Path)
}
