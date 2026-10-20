package script_test

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/script"
)

func TestHandleRunsAndLogs(t *testing.T) {
	buf := slogtest.Capture(t, slog.LevelInfo)
	h, err := script.Compile(&script.Script{FileName: "switch.js", Content: `
function handle(state, ctx) {
    ctx.log("event for", state.device.friendlyName, "action=", state.action, state.linkquality,
        state.device, [1, "a"], null, undefined, true);
}
module.exports = { handle };
`}, nil, nil, 0)
	require.NoError(t, err)

	state := map[string]any{
		"action":      "off",
		"linkquality": 51,
		"device":      map[string]any{"friendlyName": "workshop-awtrix-switch"},
	}
	require.NoError(t, h.Handle(state))

	// Logged at Info through slog, tagged with the script, with console.log-like
	// rendering: strings verbatim, objects/arrays as JSON.
	want := `level=INFO msg="script: log" script=switch.js text="event for workshop-awtrix-switch action= off 51 ` +
		`{\"friendlyName\":\"workshop-awtrix-switch\"} [1,\"a\"] null undefined true"` + "\n"
	assert.Equal(t, want, buf.String())
}

func TestLogRespectsLevel(t *testing.T) {
	// ctx.log is Info, so it is dropped when LOG_LEVEL is warn or higher.
	buf := slogtest.Capture(t, slog.LevelWarn)
	h, err := script.Compile(newScript(`module.exports = { handle(state, ctx) { ctx.log("hidden"); } };`), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Handle(map[string]any{}))
	assert.Empty(t, buf.String())
}

func TestHandleTimesOutRunawayScript(t *testing.T) {
	// A handler that loops forever must be interrupted by the watchdog rather
	// than block its caller (in production, the MQTT delivery goroutine). Handle
	// returns the interrupt as an error, and must return promptly - well before
	// the test's own deadline.
	h, err := script.Compile(newScript(`
function handle(state, ctx) {
    while (true) {}
}
module.exports = { handle };
`), nil, nil, 20*time.Millisecond)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- h.Handle(map[string]any{"action": "any"}) }()

	select {
	case herr := <-done:
		require.ErrorContains(t, herr, "handler timed out after 20ms", "a timed-out handler must report the interrupt as an error")
	case <-time.After(5 * time.Second):
		t.Fatal("handle did not return: watchdog failed to interrupt the runaway script")
	}
}

func TestHandleRecoversAfterRepeatedTimeouts(t *testing.T) {
	// Each run that times out must leave the runtime clean, even when the
	// timer fires right as a run ends, so fast runs in between always pass.
	h, err := script.Compile(newScript(`
function handle(state, ctx) {
    if (state.spin) { while (true) {} }
}
module.exports = { handle };
`), nil, nil, 10*time.Millisecond)
	require.NoError(t, err)

	for range 20 {
		require.Error(t, h.Handle(map[string]any{"spin": true}))
		require.NoError(t, h.Handle(map[string]any{"spin": false}), "a timeout must not leak into the next run")
	}
}

func TestHandleWatchdogDoesNotInterruptFastScript(t *testing.T) {
	// A handler that finishes well within the timeout must run to completion:
	// the watchdog must not interrupt it, and the cleared interrupt must not
	// leak into a later call.
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
function handle(state, ctx) {
    ctx.z2m.publishProp("lamp", "state", "ON");
}
module.exports = { handle };
`), z2m, nil, time.Second)
	require.NoError(t, err)

	require.NoError(t, h.Handle(map[string]any{"action": "on"}))
	require.NoError(t, h.Handle(map[string]any{"action": "on"}), "a second call must not inherit a stale interrupt")
	assert.Len(t, z2m.publishPropCalls, 2)
}

func TestHandleKeepsModuleStateBetweenCalls(t *testing.T) {
	// Module-level variables live in the module's closure, so they persist
	// across calls without being globals.
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
let presses = 0;
function handle(state, ctx) {
    presses++;
    ctx.z2m.publishProp("counter", "presses", presses);
}
module.exports = { handle };
`), z2m, nil, 0)
	require.NoError(t, err)

	require.NoError(t, h.Handle(map[string]any{}))
	require.NoError(t, h.Handle(map[string]any{}))
	require.Len(t, z2m.publishPropCalls, 2)
	assert.Equal(t, int64(2), z2m.publishPropCalls[1].value)
}

func TestBrokenHandlerRunJobReturnsCompileError(t *testing.T) {
	// A broken handler (its script failed to compile) runs no scheduled job and
	// returns the recorded compile error. It never collected jobs, so a
	// hand-built ScheduledJob stands in for one the scheduler might still hold:
	// RunJob must refuse it and surface the compile error, not the callback.
	h, err := script.Compile(newScript(`function handle(state, ctx) {`), nil, nil, 0)
	require.NoError(t, err)
	require.Error(t, h.Error)
	assert.Empty(t, h.Jobs(), "a broken handler collects no jobs")

	err = h.RunJob(script.ScheduledJob{CronExpr: "0 17 * * *"})
	assert.ErrorIs(t, err, h.Error, "RunJob on a broken handler must return its compile error")
}

func TestMarshalJSONSurfacesError(t *testing.T) {
	// The API serializes a handler so the UI can see, for a broken handler, why
	// it failed. A broken handler's Error is a *httputil.Err, so it marshals to
	// the same {code,msg} shape the rest of the API uses for errors.
	h, err := script.Compile(newScript(`function handle(state, ctx) {`), nil, nil, 0)
	require.NoError(t, err)
	require.Error(t, h.Error)

	raw, err := json.Marshal(h)
	require.NoError(t, err)

	var got struct {
		Error struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.NotEmpty(t, got.Error.Msg, "compile error must be surfaced as a message")
}

func TestMarshalJSONOmitsErrorWhenHealthy(t *testing.T) {
	// A handler that compiled cleanly carries no error, so the "error" field is
	// omitted (omitempty) rather than serialized as an empty object.
	h, err := script.Compile(newScript(`module.exports = { handle(state, ctx) {} };`), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)

	raw, err := json.Marshal(h)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"error"`, "healthy handler must omit the error field")
}

// handlerShape is the serialized Handler as the UI reads it: hasHandle and
// jobCount let it tell a device that reacts to events apart from one that runs
// on a schedule, and error flags a broken script.
type handlerShape struct {
	HasHandle bool `json:"hasHandle"`
	JobCount  int  `json:"jobCount"`
	Error     *struct {
		Msg string `json:"msg"`
	} `json:"error"`
}

func marshalHandler(t *testing.T, h *script.Handler) handlerShape {
	t.Helper()
	raw, err := json.Marshal(h)
	require.NoError(t, err)
	var got handlerShape
	require.NoError(t, json.Unmarshal(raw, &got))
	return got
}

func TestMarshalJSONReportsHandleAndJobCount(t *testing.T) {
	// A script that exports a handle function and jobs reports both, so the UI
	// can show the handler and the schedule indicators together.
	h, err := script.Compile(newScript(`
function handle(state, ctx) {}
function noop(ctx) {}
module.exports = {
    handle,
    jobs: [
        { cron: "0 17 * * *", run: noop },
        { cron: "*/5 * * * *", run: noop },
    ],
};
`), nil, nil, 0)
	require.NoError(t, err)

	got := marshalHandler(t, h)
	assert.True(t, got.HasHandle, "a handle function must be reported")
	assert.Equal(t, 2, got.JobCount, "every job must be counted")
	assert.Nil(t, got.Error)
}

func TestMarshalJSONReportsJobsOnlyScript(t *testing.T) {
	// A jobs-only script has no handle function: hasHandle is false while
	// jobCount reflects the jobs, so the UI shows only a schedule indicator.
	h, err := script.Compile(newScript(`module.exports = { jobs: [{ cron: "0 17 * * *", run(ctx) {} }] };`), nil, nil, 0)
	require.NoError(t, err)

	got := marshalHandler(t, h)
	assert.False(t, got.HasHandle, "a jobs-only script exports no handle function")
	assert.Equal(t, 1, got.JobCount)
	assert.Nil(t, got.Error)
}

func TestMarshalJSONBrokenHandlerReportsNeither(t *testing.T) {
	// A broken handler runs nothing: both hasHandle and jobCount are zero and
	// only the error is reported, so the UI shows just the error indicator.
	h, err := script.Compile(newScript(`function handle(state, ctx) {`), nil, nil, 0)
	require.NoError(t, err)
	require.Error(t, h.Error)

	got := marshalHandler(t, h)
	assert.False(t, got.HasHandle)
	assert.Zero(t, got.JobCount)
	require.NotNil(t, got.Error)
	assert.NotEmpty(t, got.Error.Msg)
}

// panickyZ2M stands in for a host bug: its Publish panics.
type panickyZ2M struct{ fakeZ2M }

func (*panickyZ2M) Publish(string, any) error { panic("boom") }

func TestHandleRecoversGoPanic(t *testing.T) {
	// A Go panic in a native ctx call is a host bug: it must fail the run, not
	// crash the process, and the handler must keep working afterwards.
	slogtest.Capture(t, slog.LevelError)
	h, err := script.Compile(newScript(`
function handle(state, ctx) {
    if (state.publish) ctx.z2m.publish("lamp", { state: "ON" });
}
module.exports = { handle };
`), &panickyZ2M{}, nil, time.Second)
	require.NoError(t, err)

	require.ErrorContains(t, h.Handle(map[string]any{"publish": true}), "handler panicked: boom")
	assert.NoError(t, h.Handle(map[string]any{"publish": false}), "the handler must survive a recovered panic")
}

func TestRunJobRecoversGoPanic(t *testing.T) {
	slogtest.Capture(t, slog.LevelError)
	h, err := script.Compile(newScript(`
module.exports = { jobs: [{ cron: "0 17 * * *", run(ctx) { ctx.z2m.publish("lamp", {}); } }] };
`), &panickyZ2M{}, nil, 0)
	require.NoError(t, err)
	require.Len(t, h.Jobs(), 1)

	assert.ErrorContains(t, h.RunJob(h.Jobs()[0]), "scheduled job panicked: boom")
}

func TestRunJobWithoutCallableFails(t *testing.T) {
	// A hand-built ScheduledJob has no callback; running it must fail the
	// run rather than crash on the nil function.
	slogtest.Capture(t, slog.LevelError)
	h, err := script.Compile(newScript(`module.exports = { jobs: [{ cron: "0 17 * * *", run(ctx) {} }] };`), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)

	assert.ErrorContains(t, h.RunJob(script.ScheduledJob{CronExpr: "0 17 * * *"}), "scheduled job panicked")
}
