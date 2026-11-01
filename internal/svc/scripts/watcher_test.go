package scripts_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/svc/scripts"
	"github.com/homeland-live/homeland/internal/z2m"
)

// startWatching starts a service with the watcher on over a fresh scripts dir
// and returns the dir, the service and a recorder of its handler changes.
func startWatching(t *testing.T) (string, *scripts.Service, *changeRecorder) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{Scripts: &config.Scripts{Dir: dir, EnableWatcher: true}}
	svc := scripts.New(cfg, stubZ2M{})
	rec := &changeRecorder{}
	svc.Subscribe(rec.record)
	require.NoError(t, svc.Start())
	t.Cleanup(svc.Shutdown)
	return dir, svc, rec
}

// named returns a device whose friendly name is name, so a script file
// <name>.js resolves for it.
func named(name string) *z2m.DeviceSpec {
	return &z2m.DeviceSpec{IEEEAddress: "0xffff", FriendlyName: name}
}

// content returns the source of the handler that runs for the device named
// name, or "".
func content(svc *scripts.Service, name string) string {
	if h := svc.Handler(named(name)); h != nil {
		return h.Script().Content
	}
	return ""
}

func TestWatcherDebouncesPartialWrites(t *testing.T) {
	dir, svc, rec := startWatching(t)
	path := filepath.Join(dir, "lamp.js")
	const src = `module.exports = { handle(ctx, state) {} };`

	// An editor that truncates, then writes in pieces: each step on its own
	// is a broken (or empty) script.
	start := time.Now()
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	require.NoError(t, os.WriteFile(path, []byte(`function handle(ctx, state) {`), 0o600))
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))
	// No sync can start before start+WatchDebounce, so the guarantee only
	// holds for a save that finishes within one debounce window. A writer
	// stalled longer (a loaded CI runner) looks like a finished save, and
	// compiling what is on disk is then correct.
	if elapsed := time.Since(start); elapsed >= scripts.WatchDebounce {
		t.Skipf("writes took %s, not within the %s debounce", elapsed, scripts.WatchDebounce)
	}

	require.Eventually(t, func() bool { return content(svc, "lamp") == src },
		2*time.Second, 10*time.Millisecond)
	assert.NotContains(t, rec.snapshot(), "!lamp", "a half-written file must never be compiled")
}

func TestWatcherKeepsHandlerOnAtomicSave(t *testing.T) {
	dir, svc, rec := startWatching(t)
	path := filepath.Join(dir, "lamp.js")
	require.NoError(t, os.WriteFile(path, []byte(`module.exports = { handle(ctx, state) { var v = 1; } };`), 0o600))
	require.Eventually(t, func() bool { return svc.Handler(named("lamp")) != nil },
		2*time.Second, 10*time.Millisecond)

	// Write a temp file, then rename it over the script, as many editors do.
	const updated = `module.exports = { handle(ctx, state) { var v = 2; } };`
	tmp := filepath.Join(dir, "lamp.js.tmp")
	require.NoError(t, os.WriteFile(tmp, []byte(updated), 0o600))
	require.NoError(t, os.Rename(tmp, path))

	require.Eventually(t, func() bool { return content(svc, "lamp") == updated },
		2*time.Second, 10*time.Millisecond)
	assert.NotContains(t, rec.snapshot(), "-lamp", "replacing a script must not drop its handler")
}

func TestWatcherDropsRemovedScript(t *testing.T) {
	dir, svc, rec := startWatching(t)
	path := filepath.Join(dir, "lamp.js")
	require.NoError(t, os.WriteFile(path, []byte(`module.exports = { handle(ctx, state) {} };`), 0o600))
	require.Eventually(t, func() bool { return svc.Handler(named("lamp")) != nil },
		2*time.Second, 10*time.Millisecond)

	require.NoError(t, os.Remove(path))
	require.Eventually(t, func() bool { return svc.Handler(named("lamp")) == nil },
		2*time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, countOf(rec.snapshot(), "-lamp"), "the removal is notified once")
}

func TestWatcherIgnoresHiddenAndOtherFiles(t *testing.T) {
	dir, svc, rec := startWatching(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".#lamp.js"), []byte(`lock`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(`hi`), 0o600))
	// A real script afterwards proves the watcher was running all along.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lamp.js"), []byte(`module.exports = { handle(ctx, state) {} };`), 0o600))

	require.Eventually(t, func() bool { return svc.Handler(named("lamp")) != nil },
		2*time.Second, 10*time.Millisecond)
	assert.Nil(t, svc.Handler(named(".#lamp")))
	assert.Equal(t, []string{"+lamp"}, rec.snapshot())
}

func countOf(changes []string, change string) int {
	n := 0
	for _, c := range changes {
		if c == change {
			n++
		}
	}
	return n
}
