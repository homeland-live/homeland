package script_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/script"
)

func TestCtxZ2MPublish(t *testing.T) {
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    ctx.z2m.publish("living-room-lamp", {state: "ON", brightness: 200});
    ctx.z2m.publishProp("living-room-lamp", "state", "OFF");
}
module.exports = { handle };
`), z2m, nil, 0)
	require.NoError(t, err)

	require.NoError(t, h.Handle(map[string]any{"action": "toggle"}, nil))

	require.Len(t, z2m.publishCalls, 1)
	assert.Equal(t, "living-room-lamp", z2m.publishCalls[0].name)
	payload, ok := z2m.publishCalls[0].payload.(map[string]any)
	require.Truef(t, ok, "publish payload type = %T", z2m.publishCalls[0].payload)
	assert.Equal(t, "ON", payload["state"])

	require.Len(t, z2m.publishPropCalls, 1)
	got := z2m.publishPropCalls[0]
	assert.Equal(t, "living-room-lamp", got.name)
	assert.Equal(t, "state", got.prop)
	assert.Equal(t, "OFF", got.value)
}

func TestCtxZ2MGetState(t *testing.T) {
	z2m := &fakeZ2M{states: map[string]map[string]any{
		"hallway-sensor": {"occupancy": true, "linkquality": 120.0},
	}}
	// The handler reads another device's state and acts on it, then probes an
	// unknown device to confirm z2m.getState returns null for a miss.
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    var motion = ctx.z2m.getState("hallway-sensor");
    if (motion && motion.occupancy) {
        ctx.z2m.publishProp("living-room-lamp", "state", "ON");
    }
    if (ctx.z2m.getState("nope") === null) {
        ctx.z2m.publishProp("marker", "missing", true);
    }
}
module.exports = { handle };
`), z2m, nil, 0)
	require.NoError(t, err)

	require.NoError(t, h.Handle(map[string]any{"action": "any"}, nil))

	require.Len(t, z2m.publishPropCalls, 2)
	assert.Equal(t, "living-room-lamp", z2m.publishPropCalls[0].name)
	assert.Equal(t, "state", z2m.publishPropCalls[0].prop)
	assert.Equal(t, "ON", z2m.publishPropCalls[0].value)
	assert.Equal(t, "marker", z2m.publishPropCalls[1].name)
	assert.Equal(t, "missing", z2m.publishPropCalls[1].prop)
	assert.Equal(t, true, z2m.publishPropCalls[1].value)
}

func TestCtxZ2MGetStateUnavailableThrows(t *testing.T) {
	// With a nil z2m, ctx.z2m.getState must throw rather than silently return.
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    ctx.z2m.getState("anything");
}
module.exports = { handle };
`), nil, nil, 0)
	require.NoError(t, err)

	assert.Error(t, h.Handle(map[string]any{"action": "any"}, nil))
}

func TestCtxMQTTPublish(t *testing.T) {
	m := &fakeMQTT{}
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    if (state.action === "brightness_step_up") {
        ctx.mqtt.publish("awtrix/settings", {MATP: true});
    }
}
module.exports = { handle };
`), nil, m, 0)
	require.NoError(t, err)

	require.NoError(t, h.Handle(map[string]any{"action": "brightness_step_up"}, nil))

	require.Len(t, m.calls, 1)
	assert.Equal(t, "awtrix/settings", m.calls[0].topic)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(m.calls[0].payload, &payload), "raw payload not valid JSON")
	assert.Equal(t, true, payload["MATP"])
}

func TestCtxSleep(t *testing.T) {
	// ctx.sleep(ms) blocks the handler for the requested duration. The handler
	// sleeps a short, measurable interval; Handle must not return before it
	// elapses.
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    ctx.sleep(30);
}
module.exports = { handle };
`), nil, nil, 0)
	require.NoError(t, err)

	start := time.Now()
	require.NoError(t, h.Handle(map[string]any{"action": "any"}, nil))
	assert.GreaterOrEqual(t, time.Since(start), 30*time.Millisecond)
}

func TestCtxSleepIsCutShortByTimeout(t *testing.T) {
	// A sleep longer than the timeout must not outlive it: the run fails with
	// the timeout, and the next run is unaffected.
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    if (state.sleep) ctx.sleep(60000);
}
module.exports = { handle };
`), nil, nil, 50*time.Millisecond)
	require.NoError(t, err)

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- h.Handle(map[string]any{"sleep": true}, nil) }()
	select {
	case herr := <-done:
		require.ErrorContains(t, herr, "handler timed out after 50ms")
		assert.Less(t, time.Since(start), 5*time.Second)
	case <-time.After(5 * time.Second):
		t.Fatal("ctx.sleep was not cut short by the timeout")
	}

	assert.NoError(t, h.Handle(map[string]any{"sleep": false}, nil), "the timeout must not leak into the next run")
}

func TestCtxSleepInScheduledJobIsCutShortByTimeout(t *testing.T) {
	h, err := script.Compile(newScript(`module.exports = { jobs: [{ cron: "0 17 * * *", run(ctx) { ctx.sleep(60000); } }] };`),
		nil, nil, 50*time.Millisecond)
	require.NoError(t, err)
	require.Len(t, h.Jobs(), 1)

	done := make(chan error, 1)
	go func() { done <- h.RunJob(h.Jobs()[0]) }()
	select {
	case jerr := <-done:
		require.ErrorContains(t, jerr, "scheduled job timed out after 50ms")
	case <-time.After(5 * time.Second):
		t.Fatal("ctx.sleep in a scheduled job was not cut short by the timeout")
	}
}

func TestCtxSleepNonPositiveReturnsImmediately(t *testing.T) {
	// A non-positive argument is a no-op: the handler returns without blocking.
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    ctx.sleep(0);
    ctx.sleep(-100);
}
module.exports = { handle };
`), nil, nil, 0)
	require.NoError(t, err)

	start := time.Now()
	require.NoError(t, h.Handle(map[string]any{"action": "any"}, nil))
	assert.Less(t, time.Since(start), 100*time.Millisecond)
}

func TestJSONStringifyAvailable(t *testing.T) {
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    ctx.log("state json:", JSON.stringify(state));
    var round = JSON.parse(JSON.stringify(state));
    ctx.log("action again:", round.action);
}
module.exports = { handle };
`), nil, nil, 0)
	require.NoError(t, err)

	state := map[string]any{
		"action":      "off",
		"linkquality": 51,
		"device":      map[string]any{"friendlyName": "workshop-awtrix-switch"},
	}
	require.NoError(t, h.Handle(state, nil))
}

func TestHandlePassesPrevState(t *testing.T) {
	// handle's third argument is the prevState passed to Handle, as a plain
	// object. The handler acts only when the previous state differs from the
	// current one (a cheap change detector).
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
function handle(ctx, state, prevState) {
    if (prevState && prevState.state !== state.state) {
        ctx.z2m.publishProp("lamp", "changed", state.state);
    }
}
module.exports = { handle };
`), z2m, nil, 0)
	require.NoError(t, err)

	// No previous state: prevState is null, so the handler does nothing.
	require.NoError(t, h.Handle(map[string]any{"state": "ON"}, nil))
	require.Empty(t, z2m.publishPropCalls)

	// Previous state differs from the current one: the handler publishes.
	require.NoError(t, h.Handle(map[string]any{"state": "OFF"}, map[string]any{"state": "ON"}))
	require.Len(t, z2m.publishPropCalls, 1)
	assert.Equal(t, "changed", z2m.publishPropCalls[0].prop)
	assert.Equal(t, "OFF", z2m.publishPropCalls[0].value)

	// Previous state equals the current one: no publish.
	require.NoError(t, h.Handle(map[string]any{"state": "OFF"}, map[string]any{"state": "OFF"}))
	require.Len(t, z2m.publishPropCalls, 1, "an unchanged state must not publish")
}

func TestHandlePrevStateIsNullWithoutOne(t *testing.T) {
	// A nil prevState, untyped or a nil prop map as the event service passes
	// it, reaches handle as null, not undefined or an empty object.
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
function handle(ctx, state, prevState) {
    ctx.z2m.publishProp("lamp", "prevIsNull", prevState === null);
}
module.exports = { handle };
`), z2m, nil, 0)
	require.NoError(t, err)

	require.NoError(t, h.Handle(map[string]any{"state": "ON"}, nil))
	require.NoError(t, h.Handle(map[string]any{"state": "ON"}, map[string]any(nil)))
	require.Len(t, z2m.publishPropCalls, 2)
	assert.Equal(t, true, z2m.publishPropCalls[0].value)
	assert.Equal(t, true, z2m.publishPropCalls[1].value)
}

func TestCtxHasNoGetPrevState(t *testing.T) {
	// The previous state is handle's third argument; ctx no longer carries it.
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
function handle(ctx) {
    ctx.z2m.publishProp("lamp", "hasGetPrevState", "getPrevState" in ctx);
}
module.exports = { handle };
`), z2m, nil, 0)
	require.NoError(t, err)

	require.NoError(t, h.Handle(map[string]any{}, nil))
	require.Len(t, z2m.publishPropCalls, 1)
	assert.Equal(t, false, z2m.publishPropCalls[0].value)
}
