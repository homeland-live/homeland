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

func TestHandlePromiseOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		err  string // "" means the run succeeds
	}{
		{
			name: "async throw before await",
			src:  `async function handle() { throw new Error("early"); }`,
			err:  "script: handle: Error: early at handle (test.js:1:",
		},
		{
			name: "async throw after await",
			src: `async function handle() {
    await null;
    throw new Error("late");
}`,
			err: "script: handle: Error: late at handle (test.js:3:",
		},
		{
			name: "returned rejection of a non-error",
			src:  `function handle() { return Promise.reject("nope"); }`,
			err:  "script: handle: nope",
		},
		{
			name: "fire-and-forget rejection",
			src: `async function lost() { throw new Error("lost"); }
function handle() { lost(); }`,
			err: "script: handle: unhandled promise rejection: Error: lost at lost (test.js:1:",
		},
		{
			name: "returned promise that never settles",
			src:  `function handle() { return new Promise(() => {}); }`,
			err:  "script: handle: returned a promise that never settled",
		},
		{
			name: "rejection handled later in the run",
			src: `async function handle() {
    const p = Promise.reject(new Error("caught"));
    await null;
    await p.catch(() => {});
}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := script.Compile(newScript(tc.src+"\nmodule.exports = { handle };"), nil, nil, time.Second)
			require.NoError(t, err)
			require.NoError(t, h.Error)

			err = h.Handle(map[string]any{})
			if tc.err == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tc.err)
		})
	}
}

func TestHandleAsyncSuccessRunsToCompletion(t *testing.T) {
	// Promise jobs run before Handle returns, so work after an await is done
	// by then.
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
async function handle(ctx, state) {
    await null;
    ctx.z2m.publish("lamp", { state: "ON" });
}
module.exports = { handle };
`), z2m, nil, time.Second)
	require.NoError(t, err)

	require.NoError(t, h.Handle(map[string]any{}))
	assert.Len(t, z2m.publishCalls, 1)
}

func TestHandleRejectionDoesNotCarryOver(t *testing.T) {
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    if (state.reject) Promise.reject(new Error("once"));
}
module.exports = { handle };
`), nil, nil, time.Second)
	require.NoError(t, err)

	require.ErrorContains(t, h.Handle(map[string]any{"reject": true}), "unhandled promise rejection: Error: once")
	assert.NoError(t, h.Handle(map[string]any{"reject": false}), "a rejection must fail only its own run")
}

func TestHandleRejectionDoesNotCarryOverAfterThrow(t *testing.T) {
	// A run that throws is reported by its throw; the rejection it left must
	// not surface in the next run either.
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    if (state.fail) {
        Promise.reject(new Error("dangling"));
        throw new Error("thrown");
    }
}
module.exports = { handle };
`), nil, nil, time.Second)
	require.NoError(t, err)

	err = h.Handle(map[string]any{"fail": true})
	require.ErrorContains(t, err, "Error: thrown")
	assert.NotContains(t, err.Error(), "dangling")
	assert.NoError(t, h.Handle(map[string]any{"fail": false}))
}

func TestHandleTimesOutRunawayAsyncScript(t *testing.T) {
	// The code after an await runs inside the call, so the timeout bounds it.
	h, err := script.Compile(newScript(`
async function handle() {
    await null;
    while (true) {}
}
module.exports = { handle };
`), nil, nil, 20*time.Millisecond)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- h.Handle(map[string]any{}) }()

	select {
	case err := <-done:
		require.ErrorContains(t, err, "handler timed out after 20ms")
	case <-time.After(5 * time.Second):
		t.Fatal("handle did not return: the timeout did not interrupt the promise job")
	}
}

func TestRunJobAsyncThrowFails(t *testing.T) {
	h, err := script.Compile(newScript(`
async function run() {
    await null;
    throw new Error("job");
}
module.exports = { jobs: [{ cron: "0 17 * * *", run }] };
`), nil, nil, time.Second)
	require.NoError(t, err)
	require.Len(t, h.Jobs(), 1)

	assert.ErrorContains(t, h.RunJob(h.Jobs()[0]), "script: run scheduled job: Error: job at run (test.js:4:")
}

func TestHandleReturnValueIsOnlyCheckedForAPromise(t *testing.T) {
	// A returned object is not exported to Go to look for a promise, so its
	// getters never run.
	h, err := script.Compile(newScript(`
module.exports = { handle() { return { get x() { throw new Error("read"); } }; } };
`), nil, nil, time.Second)
	require.NoError(t, err)

	assert.NoError(t, h.Handle(map[string]any{}))
}

func TestCompileTopLevelRejectionBreaksScript(t *testing.T) {
	h, err := script.Compile(newScript(`
Promise.reject(new Error("init"));
module.exports = { handle() {} };
`), nil, nil, time.Second)
	require.NoError(t, err)

	require.Error(t, h.Error)
	assert.Contains(t, h.Error.Error(), "script: compile: unhandled promise rejection: Error: init at test.js:2:")
	assert.False(t, h.HasHandle)
}

func TestHandleTimesOutRenderingRejection(t *testing.T) {
	// Rendering a rejection reason runs its toString; a loop there is the
	// script's timeout, not a recovered host panic.
	buf := slogtest.Capture(t, slog.LevelError)
	h, err := script.Compile(newScript(`
function handle() {
    return Promise.reject({ toString() { while (true) {} } });
}
module.exports = { handle };
`), nil, nil, 20*time.Millisecond)
	require.NoError(t, err)

	err = h.Handle(map[string]any{})
	require.ErrorContains(t, err, "script: handle: handler timed out after 20ms")
	assert.NotContains(t, err.Error(), "panicked")
	assert.NotContains(t, buf.String(), "recovered panic")
}
