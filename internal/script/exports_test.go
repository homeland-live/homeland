package script_test

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/script"
)

func TestExportsHandleAndJobs(t *testing.T) {
	// The documented shape: named functions, exported by reference.
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
function handle(ctx, state) {}

function turnOffLamp(ctx) {
    ctx.z2m.publishProp("living-room-lamp", "state", "OFF");
}

module.exports = {
    handle,
    jobs: [
        { cron: "0 17 * * *", run: turnOffLamp },
        { cron: "@every 10m", run: turnOffLamp },
    ],
};
`), z2m, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)
	assert.True(t, h.HasHandle)

	jobs := h.Jobs()
	require.Len(t, jobs, 2)
	assert.Equal(t, "0 17 * * *", jobs[0].CronExpr)
	assert.Equal(t, "@every 10m", jobs[1].CronExpr)

	require.NoError(t, h.RunJob(jobs[0]))
	require.Len(t, z2m.publishPropCalls, 1)
	assert.Equal(t, publishPropCall{"living-room-lamp", "state", "OFF"}, z2m.publishPropCalls[0])
}

func TestExportsViaExportsObject(t *testing.T) {
	// Setting properties on the exports object works like CommonJS.
	h, err := script.Compile(newScript(`exports.handle = function (ctx, state) {};`), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)
	assert.True(t, h.HasHandle)
}

func TestReassigningExportsVariableExportsNothing(t *testing.T) {
	// The classic CommonJS mistake: rebinding the local exports variable
	// doesn't change module.exports, so nothing is exported.
	h, err := script.Compile(newScript(`exports = { handle(ctx, state) {} };`), nil, nil, 0)
	require.NoError(t, err)
	assert.ErrorContains(t, h.Error, "must export a handle function, at least one job, or both")
}

func TestExportsAreReadOnce(t *testing.T) {
	// Changing module.exports after load has no effect: the host read it once.
	z2m := &fakeZ2M{}
	h, err := script.Compile(newScript(`
function handle(ctx, state) {
    module.exports.handle = function () { ctx.z2m.publishProp("replaced", "ran", true); };
    module.exports.jobs = [{ cron: "* * * * *", run() {} }];
    ctx.z2m.publishProp("original", "ran", true);
}
module.exports = { handle };
`), z2m, nil, 0)
	require.NoError(t, err)

	require.NoError(t, h.Handle(map[string]any{}, nil))
	require.NoError(t, h.Handle(map[string]any{}, nil))
	require.Len(t, z2m.publishPropCalls, 2)
	assert.Equal(t, "original", z2m.publishPropCalls[1].name)
	assert.Empty(t, h.Jobs())
}

func TestModuleDefinesNoGlobals(t *testing.T) {
	// Declarations stay inside the module, and strict mode makes an undeclared
	// assignment throw instead of creating a global.
	h, err := script.Compile(newScript(`
var counter = 0;
module.exports = { handle(ctx, state) { leaked = 1; } };
`), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)
	assert.ErrorContains(t, h.Handle(map[string]any{}, nil), "leaked is not defined")
}

func TestNoHostGlobals(t *testing.T) {
	// Only ctx reaches the host: there is no global schedule or console, so
	// touching one at load makes the script broken.
	for _, name := range []string{"schedule", "console"} {
		t.Run(name, func(t *testing.T) {
			h, err := script.Compile(newScript(name+`; module.exports = { handle() {} };`), nil, nil, 0)
			require.NoError(t, err)
			assert.ErrorContains(t, h.Error, name+" is not defined")
		})
	}
}

func TestInvalidExportsYieldBrokenHandler(t *testing.T) {
	cases := map[string]struct {
		src  string
		want string
	}{
		"exports not an object": {
			`module.exports = function handle() {};`,
			"module.exports must be an object",
		},
		"unknown key": {
			`module.exports = { handle() {}, job: [] };`,
			"module.exports has unknown key(s) job; allowed: handle, jobs",
		},
		"handle not a function": {
			`module.exports = { handle: "nope" };`,
			"module.exports.handle must be a function",
		},
		"jobs not an array": {
			`module.exports = { jobs: { cron: "0 17 * * *", run() {} } };`,
			"module.exports.jobs must be an array",
		},
		"job not an object": {
			`module.exports = { jobs: ["0 17 * * *"] };`,
			"module.exports.jobs[0] must be a { cron, run } object",
		},
		"sparse jobs array with maximal length": {
			// Rejected by length, without allocating for it.
			`module.exports = { jobs: new Array(4294967295) };`,
			"module.exports.jobs has 4294967295 jobs; at most 20 are allowed",
		},
		"too many jobs": {
			`module.exports = { jobs: Array.from({ length: 21 }, () => ({ cron: "0 17 * * *", run() {} })) };`,
			"module.exports.jobs has 21 jobs; at most 20 are allowed",
		},
		"job with unknown key": {
			`module.exports = { jobs: [{ cron: "0 17 * * *", run() {}, name: "x" }] };`,
			"module.exports.jobs[0] has unknown key(s) name",
		},
		"job without cron": {
			`module.exports = { jobs: [{ run() {} }] };`,
			"module.exports.jobs[0].cron must be a cron expression string",
		},
		"job with invalid cron": {
			`module.exports = { jobs: [{ cron: "0 17 * * *", run() {} }, { cron: "not a cron", run() {} }] };`,
			`module.exports.jobs[1].cron "not a cron" is invalid`,
		},
		"job run not a function": {
			`module.exports = { jobs: [{ cron: "0 17 * * *", run: "turnOff" }] };`,
			"module.exports.jobs[0].run must be a function",
		},
		"empty exports": {
			`module.exports = { jobs: [] };`,
			"must export a handle function, at least one job, or both",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h, err := script.Compile(newScript(tc.src), nil, nil, 0)
			require.NoError(t, err, "invalid exports must not be a Compile error")
			require.ErrorContains(t, h.Error, tc.want)
			assert.Zero(t, h.JobCount)
		})
	}
}

func TestMaxJobsAreAccepted(t *testing.T) {
	src := fmt.Sprintf(`module.exports = { jobs: Array.from({ length: %d }, () => ({ cron: "0 17 * * *", run() {} })) };`,
		script.MaxJobs)
	h, err := script.Compile(newScript(src), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)
	assert.Len(t, h.Jobs(), script.MaxJobs)
}

func TestCronAcceptsTimezonePrefix(t *testing.T) {
	h, err := script.Compile(newScript(`module.exports = { jobs: [{ cron: "CRON_TZ=Europe/Berlin 0 7 * * *", run() {} }] };`),
		nil, nil, 0)
	require.NoError(t, err)
	assert.NoError(t, h.Error)
}

func TestJobErrorIsReturned(t *testing.T) {
	// A throw inside a job surfaces as an error from RunJob, so the scheduler
	// can log it.
	h, err := script.Compile(newScript(`module.exports = { jobs: [{ cron: "0 17 * * *", run() { throw new Error("boom"); } }] };`),
		nil, nil, 0)
	require.NoError(t, err)
	require.Len(t, h.Jobs(), 1)
	assert.ErrorContains(t, h.RunJob(h.Jobs()[0]), "boom")
}

func TestJobsOnlyScriptIgnoresEvents(t *testing.T) {
	h, err := script.Compile(newScript(`module.exports = { jobs: [{ cron: "0 17 * * *", run() {} }] };`), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)
	assert.False(t, h.HasHandle)
	assert.NoError(t, h.Handle(map[string]any{"action": "any"}, nil), "a jobs-only handler must no-op on events")
}

func TestThrowingExportGetterYieldsBrokenHandler(t *testing.T) {
	// Reading exports can run script code; a throw there is an ordinary load
	// error, not a crash or a recovered host panic.
	buf := slogtest.Capture(t, slog.LevelError)
	h, err := script.Compile(newScript(`module.exports = { get handle() { throw new Error("nope"); } };`), nil, nil, 0)
	require.NoError(t, err)
	require.ErrorContains(t, h.Error, "nope")
	assert.NotContains(t, buf.String(), "recovered panic")
}

func TestLoopingExportGetterTimesOut(t *testing.T) {
	// A timeout while reading exports is the script's mistake, reported as a
	// timeout like one in the top-level code, not as a recovered host panic.
	buf := slogtest.Capture(t, slog.LevelError)
	h, err := script.Compile(newScript(`module.exports = { get handle() { while (true) {} } };`), nil, nil, 20*time.Millisecond)
	require.NoError(t, err)
	require.Error(t, h.Error)
	assert.Contains(t, h.Error.Error(), "script: compile: top-level evaluation timed out after 20ms")
	assert.NotContains(t, h.Error.Error(), "panicked")
	assert.NotContains(t, buf.String(), "recovered panic")
}
