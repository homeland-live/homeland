package z2m_test

import (
	"log/slog"
	"testing"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
	"github.com/homeland-live/homeland/internal/z2m"
)

// contactRoster lists one contact sensor, "door" (0x01), whose schema
// requires the "contact" prop.
var contactRoster = []byte(`[{"ieee_address":"0x01","friendly_name":"door","definition":{"exposes":[
	{"type":"binary","name":"contact","property":"contact","access":1}
]}}]`)

// newCountingClient returns a client and a pointer to the number of device
// updates its subscriber has received.
func newCountingClient(t *testing.T) (*z2m.Client, *int) {
	t.Helper()
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	calls := new(int)
	c.Subscribe(func(*z2m.Device) { *calls++ })
	return c, calls
}

// TestStateBeforeRosterIsKept covers retained state the broker delivers
// before bridge/devices: it must survive the roster and count as the device's
// initial state, so the next update is an event.
func TestStateBeforeRosterIsKept(t *testing.T) {
	c, calls := newCountingClient(t)

	c.Handle("door", []byte(`{"contact":true}`))
	c.Handle("bridge/devices", contactRoster)

	state := c.DeviceState("door")
	require.NotNil(t, state, "the pre-roster state must move to the device")
	assert.Equal(t, map[string]any{"contact": true}, state.Props())
	assert.Equal(t, state, c.DeviceState("0x01"), "the state is keyed by IEEE address")
	assert.Zero(t, *calls, "the initial state is not an event")

	c.Handle("door", []byte(`{"contact":false}`))
	assert.Equal(t, 1, *calls, "the first update after the roster is an event")
}

// TestStateBeforeRosterKeepsLatest covers several pre-roster messages: the
// latest one is the device's state.
func TestStateBeforeRosterKeepsLatest(t *testing.T) {
	c, _ := newCountingClient(t)

	c.Handle("door", []byte(`{"contact":true}`))
	c.Handle("door", []byte(`{"contact":false}`))
	c.Handle("bridge/devices", contactRoster)

	assert.Equal(t, map[string]any{"contact": false}, c.DeviceState("door").Props())
}

// TestRosterKeepsNewerStateByAddress covers a device that reported under both
// its friendly name and its IEEE address before the roster: the older
// friendly-name state must not overwrite the newer address one.
func TestRosterKeepsNewerStateByAddress(t *testing.T) {
	c, _ := newCountingClient(t)

	c.Handle("door", []byte(`{"contact":true}`))
	c.Handle("0x01", []byte(`{"contact":false}`))
	c.Handle("bridge/devices", contactRoster)

	assert.Equal(t, map[string]any{"contact": false}, c.DeviceState("door").Props())
}

// TestRosterDropsStateOfRemovedDevice covers a device that leaves the roster:
// its state goes with it.
func TestRosterDropsStateOfRemovedDevice(t *testing.T) {
	c, _ := newCountingClient(t)
	c.Handle("bridge/devices", contactRoster)
	c.Handle("door", []byte(`{"contact":true}`))

	c.Handle("bridge/devices", []byte(`[]`))
	c.Handle("bridge/devices", contactRoster)

	assert.Nil(t, c.DeviceState("door"))
}

// TestDeviceWithoutDefinitionPropagates covers unsupported devices, which
// z2m lists with a null definition: with no schema to check, their updates
// are events like any other.
func TestDeviceWithoutDefinitionPropagates(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", []byte(`[{"ieee_address":"0x02","friendly_name":"diy","definition":null}]`))

	c.Handle("diy", []byte(`{"state":"ON"}`))
	c.Handle("diy", []byte(`{"state":"OFF"}`))

	assert.Equal(t, 1, *calls)
	assert.Equal(t, map[string]any{"state": "OFF"}, c.DeviceState("diy").Props())
}

// TestIsStateTopic covers which base-topic-relative topics are device state:
// a friendly name, which may contain slashes, but not the base topic itself,
// bridge, command or availability topics.
func TestIsStateTopic(t *testing.T) {
	for _, topic := range []string{
		"lamp", "room/lamp", "0x00158d0001a2b3c4",
		// Only whole segments count.
		"settings", "lamp/setup", "lamp/getter", "budget", "bridges", "bridge-lamp", "availability-sensor",
	} {
		assert.True(t, z2m.IsStateTopic(topic), topic)
	}
	for _, topic := range []string{
		"", "bridge", "bridge/state", "bridge/devices", "bridge/logging", "bridge/request/permit_join",
		"set", "lamp/set", "lamp/set/state", "room/lamp/set",
		"get", "lamp/get", "lamp/get/state",
		"availability", "lamp/availability", "room/lamp/availability",
	} {
		assert.False(t, z2m.IsStateTopic(topic), topic)
	}
}

// TestHandleIgnoresNonStateTopics covers routing what the "#" subscription
// delivers besides device state: a command, availability or bridge topic of a
// known device must neither change its state nor notify. Which topics are
// state is covered by TestIsStateTopic.
func TestHandleIgnoresNonStateTopics(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", []byte(`[
		{"ieee_address":"0x01","friendly_name":"lamp"},
		{"ieee_address":"0x02","friendly_name":"room/lamp"}
	]`))
	c.Handle("lamp", []byte(`{"state":"ON"}`))
	c.Handle("room/lamp", []byte(`{"state":"ON"}`))

	for _, topic := range []string{
		"lamp/set", "lamp/set/state", "lamp/get", "lamp/get/state", "lamp/availability",
		"room/lamp/set", "room/lamp/availability",
		"", "set", "get", "availability",
		"bridge", "bridge/logging", "bridge/event", "bridge/unknown-topic",
		"bridge/request/permit_join", "bridge/response/permit_join",
	} {
		c.Handle(topic, []byte(`{"state":"OFF"}`))
	}

	assert.Zero(t, *calls)
	for _, id := range []string{"lamp", "room/lamp"} {
		assert.Equal(t, map[string]any{"state": "ON"}, c.DeviceState(id).Props(), id)
	}
	assert.Len(t, c.Devices(), 2, "no topic may add a device")
}

// TestHandleStripsDeviceAndUpdateKeys covers the device info and OTA update
// objects z2m may embed in state: they are not device props.
func TestHandleStripsDeviceAndUpdateKeys(t *testing.T) {
	c, _ := newCountingClient(t)
	var got *z2m.Device
	c.Subscribe(func(d *z2m.Device) { got = d })
	c.Handle("bridge/devices", []byte(`[{"ieee_address":"0x01","friendly_name":"lamp"}]`))

	c.Handle("lamp", []byte(`{"state":"ON","device":{"friendlyName":"lamp"},"update":{"state":"idle"}}`))
	c.Handle("lamp", []byte(`{"state":"OFF","device":{"friendlyName":"lamp"},"update":{"state":"idle"}}`))

	require.NotNil(t, got)
	assert.Equal(t, map[string]any{"state": "OFF"}, got.State.Props())
}

// TestHandleIgnoresMalformedState covers a payload that is not a JSON object:
// it is logged and the stored state is unchanged.
func TestHandleIgnoresMalformedState(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", []byte(`[{"ieee_address":"0x01","friendly_name":"lamp"}]`))
	c.Handle("lamp", []byte(`{"state":"ON"}`))

	for _, payload := range []string{`not json`, `"OFF"`, `[1,2]`} {
		c.Handle("lamp", []byte(payload))
	}

	assert.Zero(t, *calls)
	assert.Equal(t, map[string]any{"state": "ON"}, c.DeviceState("lamp").Props())
}

// TestHandleIgnoresEmptyPayload covers z2m clearing a retained topic on a
// device rename or removal: it is not a malformed state, so nothing is
// logged above Debug and the stored state is unchanged.
func TestHandleIgnoresEmptyPayload(t *testing.T) {
	logs := slogtest.Capture(t, slog.LevelInfo)
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", []byte(`[{"ieee_address":"0x01","friendly_name":"lamp"}]`))
	c.Handle("lamp", []byte(`{"state":"ON"}`))

	c.HandleRetained("lamp", nil)
	c.Handle("lamp", []byte{})

	assert.Zero(t, *calls)
	assert.Equal(t, map[string]any{"state": "ON"}, c.DeviceState("lamp").Props())
	assert.NotContains(t, logs.String(), "failed to parse device state")
}

// TestHandleDropsStateMissingPrimaryProp covers an incomplete state: it is
// neither stored nor an event.
func TestHandleDropsStateMissingPrimaryProp(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", contactRoster)
	c.Handle("door", []byte(`{"contact":true}`))

	c.Handle("door", []byte(`{"battery":90}`))

	assert.Zero(t, *calls)
	assert.Equal(t, map[string]any{"contact": true}, c.DeviceState("door").Props())
}

// TestHandleDropsDuplicateState covers z2m republishing an unchanged state
// (only last_seen moves): it is not an event.
func TestHandleDropsDuplicateState(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", contactRoster)
	c.Handle("door", []byte(`{"contact":true,"last_seen":"2026-10-19T10:00:00Z"}`))

	c.Handle("door", []byte(`{"contact":true,"last_seen":"2026-10-19T10:05:00Z"}`))
	assert.Zero(t, *calls)

	c.Handle("door", []byte(`{"contact":false,"last_seen":"2026-10-19T10:06:00Z"}`))
	assert.Equal(t, 1, *calls)
}

// switchRoster lists a wall switch, "wall" (0x03), with a relay (state_left)
// and buttons (action); remoteRoster lists a remote, "remote" (0x04), whose
// only primary prop is action.
var (
	switchRoster = []byte(`[{"ieee_address":"0x03","friendly_name":"wall","definition":{"exposes":[
		{"type":"switch","features":[{"type":"binary","name":"state","property":"state_left","access":7}]},
		{"type":"enum","name":"action","property":"action","access":1,"category":"diagnostic"}
	]}}]`)
	remoteRoster = []byte(`[{"ieee_address":"0x04","friendly_name":"remote","definition":{"exposes":[
		{"type":"enum","name":"action","property":"action","access":1,"category":"diagnostic"},
		{"type":"numeric","name":"battery","property":"battery","access":1,"category":"diagnostic"}
	]}}]`)
)

// TestSwitchWithActionPropagatesRelayToggles covers a device exposing action
// next to other primary props: z2m never caches action, so a relay toggle
// arrives without it and must still be an event, as must every press.
func TestSwitchWithActionPropagatesRelayToggles(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", switchRoster)
	c.Handle("wall", []byte(`{"state_left":"ON"}`))

	c.Handle("wall", []byte(`{"state_left":"OFF"}`))
	assert.Equal(t, 1, *calls, "a relay toggle without action is an event")

	c.Handle("wall", []byte(`{"state_left":"OFF","action":"single_left"}`))
	c.Handle("wall", []byte(`{"state_left":"OFF","action":"single_left"}`))
	assert.Equal(t, 3, *calls, "every press is an event")

	c.Handle("wall", []byte(`{"state_left":"OFF"}`))
	assert.Equal(t, 3, *calls, "the state after a press, without action, repeats it")
}

// TestRemoteOnlyPressesPropagate covers a device whose only primary prop is
// action: a report without a press is not an event.
func TestRemoteOnlyPressesPropagate(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", remoteRoster)
	c.Handle("remote", []byte(`{"battery":90}`))

	c.Handle("remote", []byte(`{"battery":85}`))
	assert.Zero(t, *calls)

	c.Handle("remote", []byte(`{"battery":85,"action":"on"}`))
	assert.Equal(t, 1, *calls)
}

// TestRetainedReplayIsNotAPress covers the retained copies the broker resends
// on every reconnect: a press they carry already happened, so it must not
// reach subscribers again, while a state change made during the outage still
// does, without the stale action.
func TestRetainedReplayIsNotAPress(t *testing.T) {
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	var got []*z2m.Device
	c.Subscribe(func(d *z2m.Device) { got = append(got, d) })
	c.Handle("bridge/devices", switchRoster)
	c.Handle("wall", []byte(`{"state_left":"ON"}`))
	c.Handle("wall", []byte(`{"state_left":"ON","action":"single_left"}`))
	require.Len(t, got, 1)

	// Reconnect: the broker resends the last message, which carried the press.
	c.HandleRetained("wall", []byte(`{"state_left":"ON","action":"single_left"}`))
	assert.Len(t, got, 1, "a replayed press is not an event")

	// Reconnect after the relay was toggled (and pressed) during an outage.
	c.HandleRetained("wall", []byte(`{"state_left":"OFF","action":"double_left"}`))
	require.Len(t, got, 2, "a state change missed during an outage is an event")
	assert.Equal(t, map[string]any{"state_left": "OFF"}, got[1].State.Props(), "without the stale press")
}

// TestRetainedReplayOfRemoteIsDropped covers a remote: with the stale press
// removed, its replayed state has no event to report.
func TestRetainedReplayOfRemoteIsDropped(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", remoteRoster)
	c.Handle("remote", []byte(`{"battery":90}`))
	c.Handle("remote", []byte(`{"battery":90,"action":"on"}`))
	require.Equal(t, 1, *calls)

	c.HandleRetained("remote", []byte(`{"battery":90,"action":"on"}`))
	assert.Equal(t, 1, *calls)
}

// TestRenamedDeviceKeepsState covers a rename in z2m: state is keyed by IEEE
// address, so it survives the new roster, the old name stops resolving, and
// the next update under the new name is an event.
func TestRenamedDeviceKeepsState(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", contactRoster)
	c.Handle("door", []byte(`{"contact":true}`))

	c.Handle("bridge/devices", []byte(`[{"ieee_address":"0x01","friendly_name":"front-door","definition":{"exposes":[
		{"type":"binary","name":"contact","property":"contact","access":1}
	]}}]`))

	assert.Nil(t, c.Device("door"), "the old name no longer resolves")
	assert.Equal(t, map[string]any{"contact": true}, c.DeviceState("front-door").Props())

	c.Handle("front-door", []byte(`{"contact":false}`))
	assert.Equal(t, 1, *calls)
}
