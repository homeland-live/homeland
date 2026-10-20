package healthcheck_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/healthcheck"
)

func TestNew(t *testing.T) {
	healthy := healthcheck.New("mqtt", nil)
	assert.True(t, healthy.IsHealthy)
	assert.Empty(t, healthy.Error)
	assert.Equal(t, "mqtt", healthy.Name)

	unhealthy := healthcheck.New("mqtt", fmt.Errorf("connection is down"))
	assert.False(t, unhealthy.IsHealthy)
	assert.Equal(t, "connection is down", unhealthy.Error)
}

func TestComposeAllHealthy(t *testing.T) {
	hc := healthcheck.Compose("runner", healthcheck.New("z2m", nil), healthcheck.New("ext-mqtt", nil))
	assert.True(t, hc.IsHealthy)
	assert.Empty(t, hc.Error)
	assert.Len(t, hc.Checks, 2)
}

func TestComposeAggregatesFailures(t *testing.T) {
	hc := healthcheck.Compose("runner",
		healthcheck.New("z2m", fmt.Errorf("bridge offline")),
		healthcheck.New("ext-mqtt", nil),
	)
	assert.False(t, hc.IsHealthy)
	assert.Equal(t, "unhealthy: z2m", hc.Error)
}

func TestComposeNested(t *testing.T) {
	z2m := healthcheck.Compose("z2m",
		healthcheck.New("mqtt", nil),
		healthcheck.New("bridge", fmt.Errorf("bridge is offline")),
	)
	root := healthcheck.Compose("runner", z2m, healthcheck.New("ext-mqtt", nil))

	assert.False(t, root.IsHealthy)
	assert.Equal(t, "unhealthy: z2m", root.Error)

	// The failing leaf must be reachable through the tree.
	require.Len(t, root.Checks[0].Checks, 2)
	assert.Equal(t, "bridge", root.Checks[0].Checks[1].Name)
	assert.False(t, root.Checks[0].Checks[1].IsHealthy)
}

func TestJSONShape(t *testing.T) {
	hc := healthcheck.Compose("z2m", healthcheck.New("mqtt", nil), healthcheck.New("bridge", fmt.Errorf("offline")))
	b, err := json.Marshal(hc)
	require.NoError(t, err)

	var round healthcheck.HealthCheck
	require.NoError(t, json.Unmarshal(b, &round))
	assert.Equal(t, "z2m", round.Name)
	assert.False(t, round.IsHealthy)
	require.Len(t, round.Checks, 2)
	// Healthy leaf must omit the error field.
	assert.Empty(t, round.Checks[0].Error)
}
