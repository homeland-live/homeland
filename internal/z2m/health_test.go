package z2m_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
	"github.com/homeland-live/homeland/internal/z2m"
)

// setBridgeOnline drives the client's observed bridge state through the real
// bridge/state message path, as the broker would deliver it.
func setBridgeOnline(c *z2m.Client, online bool) {
	state := "offline"
	if online {
		state = "online"
	}
	c.Handle("bridge/state", []byte(fmt.Sprintf(`{"state":%q}`, state)))
}

// setBridgeHealthMQTT drives a bridge/health message reporting the bridge's own
// MQTT link state through the real message path.
func setBridgeHealthMQTT(c *z2m.Client, connected bool) {
	c.Handle("bridge/health", []byte(fmt.Sprintf(`{"mqtt":{"connected":%t}}`, connected)))
}

func find(hc healthcheck.HealthCheck, name string) (healthcheck.HealthCheck, bool) {
	for _, c := range hc.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return healthcheck.HealthCheck{}, false
}

func TestHealthMQTTDown(t *testing.T) {
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: false}))
	require.NoError(t, err)
	setBridgeOnline(c, true)

	hc := c.HealthCheck()
	assert.False(t, hc.IsHealthy)
	mqttCheck, ok := find(hc, "mqtt")
	require.Truef(t, ok, "expected mqtt child, got %+v", hc.Checks)
	assert.False(t, mqttCheck.IsHealthy)
}

func TestHealthBridgeOffline(t *testing.T) {
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	setBridgeOnline(c, false)

	hc := c.HealthCheck()
	assert.False(t, hc.IsHealthy)
	bridge, ok := find(hc, "bridge")
	require.Truef(t, ok, "expected bridge child, got %+v", hc.Checks)
	assert.False(t, bridge.IsHealthy)
}

func TestHealthAllUp(t *testing.T) {
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	setBridgeOnline(c, true)

	hc := c.HealthCheck()
	assert.True(t, hc.IsHealthy)
	assert.Len(t, hc.Checks, 2)
}

func TestHealthBridgeReportsMQTTDown(t *testing.T) {
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	setBridgeOnline(c, true)
	setBridgeHealthMQTT(c, false)

	hc := c.HealthCheck()
	assert.False(t, hc.IsHealthy)
	bridge, ok := find(hc, "bridge")
	require.Truef(t, ok, "expected bridge child, got %+v", hc.Checks)
	assert.False(t, bridge.IsHealthy)
}

func TestHealthBridgeHealthMissingIsSoft(t *testing.T) {
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	setBridgeOnline(c, true)
	// bridge/health stays nil: never received.

	hc := c.HealthCheck()
	assert.True(t, hc.IsHealthy, "missing bridge/health should be soft")
}
