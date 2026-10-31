package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
)

// setEnv clears every variable Load reads, so values from the developer's
// environment (e.g. a loaded .env) don't leak in, sets a Z2M broker, then
// applies env.
func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range []string{
		"HL_HTTP_PORT",
		"HL_DB_PATH", "HL_DB_RETENTION_MAX_AGE", "HL_DB_RETENTION_MAX_PER_DEVICE", "HL_DB_RETENTION_INTERVAL",
		"HL_SCRIPTS_DIR", "HL_SCRIPTS_WATCHER", "HL_SCRIPTS_TIMEOUT",
	} {
		t.Setenv(key, "")
	}
	setMQTTEnv(t, map[string]string{"HL_Z2M_MQTT_BROKER": "tcp://localhost:1883"})
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, nil)

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, &config.Web{HTTPPort: config.DefaultHTTPPort}, cfg.Web)
	assert.Equal(t, &config.DB{
		Path: "data/homeland.db",
		Retention: config.DBRetention{
			MaxAge:   30 * 24 * time.Hour,
			Interval: time.Hour,
		},
	}, cfg.DB)
	assert.Equal(t, &config.Scripts{Dir: "scripts", Timeout: 5 * time.Second}, cfg.Scripts)
}

func TestLoadOverrides(t *testing.T) {
	setEnv(t, map[string]string{
		"HL_HTTP_PORT":                   "9090",
		"HL_DB_PATH":                     "/var/lib/homeland/events.db",
		"HL_DB_RETENTION_MAX_AGE":        "0",
		"HL_DB_RETENTION_MAX_PER_DEVICE": "500",
		"HL_DB_RETENTION_INTERVAL":       "15m",
		"HL_SCRIPTS_DIR":                 "/scripts",
		"HL_SCRIPTS_WATCHER":             "true",
		"HL_SCRIPTS_TIMEOUT":             "0",
	})

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.Web.HTTPPort)
	assert.Equal(t, &config.DB{
		Path:      "/var/lib/homeland/events.db",
		Retention: config.DBRetention{MaxPerDevice: 500, Interval: 15 * time.Minute},
	}, cfg.DB)
	assert.True(t, cfg.DB.Retention.Enabled(), "a per-device cap alone enables retention")
	assert.Equal(t, &config.Scripts{Dir: "/scripts", EnableWatcher: true}, cfg.Scripts,
		"a zero timeout disables it")
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"HL_HTTP_PORT", "0"},
		{"HL_HTTP_PORT", "65536"},
		{"HL_HTTP_PORT", "http"},
		{"HL_DB_RETENTION_MAX_AGE", "30d"},
		{"HL_DB_RETENTION_MAX_PER_DEVICE", "many"},
		{"HL_DB_RETENTION_INTERVAL", "hourly"},
		{"HL_SCRIPTS_WATCHER", "maybe"},
		{"HL_SCRIPTS_TIMEOUT", "5"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			setEnv(t, map[string]string{tc.key: tc.value})
			_, err := config.Load()
			assert.ErrorContains(t, err, tc.key, "the error names the variable")
		})
	}
}

func TestRetentionEnabled(t *testing.T) {
	assert.False(t, config.DBRetention{Interval: time.Hour}.Enabled(), "an interval alone prunes nothing")
	assert.True(t, config.DBRetention{MaxAge: time.Hour}.Enabled())
	assert.True(t, config.DBRetention{MaxPerDevice: 1}.Enabled())
}
