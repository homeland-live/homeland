package z2m_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
	"github.com/homeland-live/homeland/internal/z2m"
)

// TestFirstStateSkipsNotify verifies that a device's first state is stored but
// not fanned out, while later changed states are.
func TestFirstStateSkipsNotify(t *testing.T) {
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	var calls int
	c.Subscribe(func(*z2m.Device) { calls++ })

	// Roster carries a definition so the device has a StateSchema: "state" is
	// a primary (regular-usage) prop that every valid state must report.
	c.Handle("bridge/devices", []byte(`[{
		"ieee_address":"0x01",
		"friendly_name":"bulb",
		"definition":{"exposes":[
			{"type":"binary","name":"state","property":"state","access":7},
			{"type":"numeric","name":"linkquality","property":"linkquality","access":1,"category":"diagnostic"}
		]}
	}]`))

	// First state: stored (device ready) but not fanned out.
	c.Handle("bulb", []byte(`{"state":"ON"}`))
	assert.NotNil(t, c.DeviceState("bulb"), "device should have recorded state after first update")
	assert.Equal(t, 0, calls, "subscriber calls after first state")

	// A subsequent valid state: fanned out.
	c.Handle("bulb", []byte(`{"state":"OFF"}`))
	assert.Equal(t, 1, calls, "subscriber calls after a subsequent state")

	// linkquality is ignored when comparing, even when it first appears.
	c.Handle("bulb", []byte(`{"state":"OFF","linkquality":99}`))
	assert.Equal(t, 1, calls, "subscriber calls after a state that only adds linkquality")

	// A new attribute makes it a change, so it is fanned out.
	c.Handle("bulb", []byte(`{"state":"OFF","linkquality":99,"power_on_behavior":"on"}`))
	assert.Equal(t, 2, calls, "subscriber calls after a state with a new attribute")
}

// TestDevicesSortedByFriendlyName verifies Devices is deterministic: sorted by
// friendly name regardless of roster order or map iteration.
func TestDevicesSortedByFriendlyName(t *testing.T) {
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	c.Handle("bridge/devices", []byte(`[
		{"ieee_address":"0x05","friendly_name":"kitchen"},
		{"ieee_address":"0x01","friendly_name":"zone"},
		{"ieee_address":"0x03","friendly_name":"attic"},
		{"ieee_address":"0x04","friendly_name":"hall"},
		{"ieee_address":"0x02","friendly_name":"bedroom"}
	]`))

	want := []string{"attic", "bedroom", "hall", "kitchen", "zone"}
	// Map iteration order varies per call, so repeat to catch an unsorted result.
	for range 20 {
		devices := c.Devices()
		names := make([]string, 0, len(devices))
		for _, d := range devices {
			names = append(names, d.Spec.FriendlyName)
		}
		require.Equal(t, want, names)
	}
}
