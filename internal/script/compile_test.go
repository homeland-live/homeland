package script_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/script"
)

func TestScriptWithoutExportsYieldsBrokenHandler(t *testing.T) {
	// A script that exports neither a handle function nor a job is a
	// script-author error, not a host error: Compile returns a broken handler
	// that records the problem on Error and refuses to run.
	h, err := script.Compile(newScript(`function handle(ctx, state) {}`), nil, nil, 0)
	require.NoError(t, err, "missing exports must not be a Compile error")
	require.NotNil(t, h)
	require.ErrorContains(t, h.Error, "must export a handle function, at least one job, or both")
	assert.Error(t, h.Handle(map[string]any{"action": "any"}, nil), "broken handler must refuse to run")
}

func TestSyntaxErrorYieldsBrokenHandler(t *testing.T) {
	// A JS syntax error is likewise recorded on the handler rather than
	// returned, so loading the rest of the scripts is not aborted.
	h, err := script.Compile(newScript(`function handle(ctx, state) {`), nil, nil, 0)
	require.NoError(t, err, "syntax error must not be a Compile error")
	require.NotNil(t, h)
	require.Error(t, h.Error, "broken handler must record the compile error")
	assert.Error(t, h.Handle(map[string]any{"action": "any"}, nil), "broken handler must refuse to run")
}

func TestErrorsReportScriptLineNumbers(t *testing.T) {
	// The module wrapper adds no lines, so positions match the script file.
	h, err := script.Compile(newScript("module.exports = {\n  handle(ctx, state) {\n    missing();\n  },\n};"), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)
	require.ErrorContains(t, h.Handle(map[string]any{}, nil), "test.js:3:")

	h, err = script.Compile(newScript("module.exports = {};\n\nthrow new Error('boom');"), nil, nil, 0)
	require.NoError(t, err)
	assert.ErrorContains(t, h.Error, "test.js:3:")
}

func TestStrayClosingBracketCannotEscapeModule(t *testing.T) {
	// A stray "})" must not close the module wrapper and run the rest of the
	// script at global scope.
	h, err := script.Compile(newScript(`}); module.exports = { handle() {} }; (function () {`), nil, nil, 0)
	require.NoError(t, err)
	assert.ErrorContains(t, h.Error, "unbalanced brackets")
}

func TestSourceMapIsNotLoaded(t *testing.T) {
	// A sourceMappingURL comment must not make the host read a file outside
	// the scripts dir (goja resolves it against the process's working
	// directory), so error positions stay those of the script file.
	t.Chdir(t.TempDir())
	// Maps lines 1-4 to elsewhere.js.
	sm := `{"version":3,"sources":["elsewhere.js"],"names":[],"mappings":"AAAA;AACA;AACA;AACA"}`
	require.NoError(t, os.WriteFile("elsewhere.js.map", []byte(sm), 0o600))

	h, err := script.Compile(newScript("module.exports = {};\nthrow new Error('boom');\n//# sourceMappingURL=elsewhere.js.map"), nil, nil, 0)
	require.NoError(t, err)
	require.ErrorContains(t, h.Error, "test.js:2:")
	assert.NotContains(t, h.Error.Error(), "elsewhere.js")
}

func TestCompileTimesOutRunawayTopLevel(t *testing.T) {
	// A script that loops at module scope must not hang Compile (startup or a
	// watcher reload). The watchdog interrupts the top-level evaluation, which
	// surfaces as a broken handler rather than a Compile error.
	done := make(chan struct {
		h   *script.Handler
		err error
	}, 1)
	go func() {
		h, err := script.Compile(newScript(`while (true) {}`), nil, nil, 20*time.Millisecond)
		done <- struct {
			h   *script.Handler
			err error
		}{h, err}
	}()

	select {
	case res := <-done:
		require.NoError(t, res.err, "an interrupted top-level evaluation must not be a host-side Compile error")
		require.NotNil(t, res.h)
		require.ErrorContains(t, res.h.Error, "top-level evaluation timed out after 20ms",
			"a timed-out top-level evaluation must be recorded as a broken handler")
	case <-time.After(5 * time.Second):
		t.Fatal("Compile did not return: watchdog failed to interrupt the top-level loop")
	}
}
