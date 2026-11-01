package script_test

import (
	"log/slog"
	"testing"
	"time"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/script"
)

// overflowMsg starts the error of a run that exceeds MaxCallDepth.
const overflowMsg = "call stack exceeded 64 nested calls; recursion is not supported at "

func TestHandleFailsOnRecursion(t *testing.T) {
	// Recursion fails the run at MaxCallDepth, naming the frame, instead of
	// growing memory until the timeout; the handler keeps working afterwards.
	h, err := script.Compile(newScript(`
function f(n) { return f(n + 1); }
function handle(ctx, state) {
    if (state.recurse) f(0);
}
module.exports = { handle };
`), nil, nil, time.Second)
	require.NoError(t, err)
	require.NoError(t, h.Error)

	require.ErrorContains(t, h.Handle(map[string]any{"recurse": true}, nil), "script: handle: "+overflowMsg+"f (test.js:2:")
	assert.NoError(t, h.Handle(map[string]any{"recurse": false}, nil))
}

func TestRecursionThroughBuiltinFails(t *testing.T) {
	// Recursion through a built-in that calls back recurses on the Go stack,
	// so it must hit the same limit.
	h, err := script.Compile(newScript(`
function g() { [1].forEach(g); }
module.exports = { handle() { g(); } };
`), nil, nil, time.Second)
	require.NoError(t, err)
	require.NoError(t, h.Error)

	assert.ErrorContains(t, h.Handle(map[string]any{}, nil), overflowMsg+"g (test.js:2:")
}

func TestRecursionCannotBeCaught(t *testing.T) {
	// The overflow is uncatchable, so a catch can't swallow it and carry on.
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
function f() { return f(); }
module.exports = { handle(ctx) {
    try { f(); } catch (e) { ctx.z2m.publishProp("lamp", "state", "caught"); }
} };
`), z2m, nil, time.Second)
	require.NoError(t, err)
	require.NoError(t, h.Error)

	require.ErrorContains(t, h.Handle(map[string]any{}, nil), overflowMsg)
	assert.Empty(t, z2m.publishPropCalls)
}

func TestAsyncRecursionFails(t *testing.T) {
	h, err := script.Compile(newScript(`
function f() { return f(); }
module.exports = { async handle() { await null; f(); } };
`), nil, nil, time.Second)
	require.NoError(t, err)
	require.NoError(t, h.Error)

	assert.ErrorContains(t, h.Handle(map[string]any{}, nil), overflowMsg)
}

func TestRunJobFailsOnRecursion(t *testing.T) {
	h, err := script.Compile(newScript(`
function f() { return f(); }
module.exports = { jobs: [{ cron: "0 17 * * *", run() { f(); } }] };
`), nil, nil, time.Second)
	require.NoError(t, err)
	require.Len(t, h.Jobs(), 1)

	assert.ErrorContains(t, h.RunJob(h.Jobs()[0]), "script: run scheduled job: "+overflowMsg)
}

func TestTopLevelRecursionBreaksScript(t *testing.T) {
	h, err := script.Compile(newScript(`
function f() { return f(); }
f();
module.exports = { handle() {} };
`), nil, nil, time.Second)
	require.NoError(t, err, "a script-author mistake is not a Compile error")
	assert.ErrorContains(t, h.Error, "script: compile: "+overflowMsg)
}

func TestRecursionInScriptRunByHostIsNotAPanic(t *testing.T) {
	// Script code the host runs itself, outside a call (an exports getter,
	// rendering a rejection reason), overflows as the script's error, not as
	// a recovered host panic.
	buf := slogtest.Capture(t, slog.LevelError)

	h, err := script.Compile(newScript(`
function f() { return f(); }
module.exports = { get handle() { return f(); } };
`), nil, nil, time.Second)
	require.NoError(t, err)
	require.ErrorContains(t, h.Error, "script: compile: "+overflowMsg)

	h, err = script.Compile(newScript(`
function f() { return f(); }
module.exports = { handle() { return Promise.reject({ toString: f }); } };
`), nil, nil, time.Second)
	require.NoError(t, err)
	require.NoError(t, h.Error)
	require.ErrorContains(t, h.Handle(map[string]any{}, nil), overflowMsg)

	assert.NotContains(t, buf.String(), "recovered panic")
}

func TestShallowNestingRuns(t *testing.T) {
	// Helpers calling helpers through callbacks, as a typical script does,
	// stay well within MaxCallDepth.
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
function name(room, kind) { return [room, kind].map(function (s) { return s.toLowerCase(); }).join("-"); }
function lampsIn(room) { return ["Ceiling", "Desk"].map(function (k) { return name(room, k); }); }
function setAll(ctx, room, on) {
    lampsIn(room).forEach(function (lamp) {
        ctx.z2m.publish(lamp, JSON.parse(JSON.stringify({ state: on ? "ON" : "OFF" })));
    });
}
module.exports = { handle(ctx, state) {
    [state.room].filter(Boolean).forEach(function (room) { setAll(ctx, room, state.occupancy); });
} };
`), z2m, nil, time.Second)
	require.NoError(t, err)
	require.NoError(t, h.Error)

	require.NoError(t, h.Handle(map[string]any{"room": "Office", "occupancy": true}, nil))
	assert.Len(t, z2m.publishCalls, 2)
}
