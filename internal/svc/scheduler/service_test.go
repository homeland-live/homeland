package scheduler_test

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/script"
	"github.com/homeland-live/homeland/internal/svc/scheduler"
	"github.com/homeland-live/homeland/internal/svc/scripts"
	"github.com/homeland-live/homeland/internal/z2m"
)

// recordingZ2M records publishProp calls so a test can observe that a
// scheduled callback ran. Scheduled jobs fire on gocron goroutines, so its
// recording is mutex-guarded and read via calls(). It satisfies script.Z2M.
type recordingZ2M struct {
	mu    sync.Mutex
	props []string
}

func (r *recordingZ2M) Publish(_ string, _ any) error { return nil }
func (r *recordingZ2M) PublishProp(name, prop string, _ any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.props = append(r.props, name+"."+prop)
	return nil
}
func (r *recordingZ2M) DeviceState(_ string) *z2m.DeviceState { return nil }

// calls returns a copy of the recorded publishProp calls, safe to read while
// scheduled jobs may still be firing.
func (r *recordingZ2M) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.props...)
}

// scheduledHandler compiles a handler whose single job publishes a marker
// prop, so a test can observe it ran.
func scheduledHandler(t *testing.T, z2m script.Z2M, cron, marker string) *script.Handler {
	t.Helper()
	src := `function mark(ctx) { ctx.z2m.publishProp("` + marker + `", "ran", true); }
module.exports = { handle(ctx, state) {}, jobs: [{ cron: "` + cron + `", run: mark }] };`
	h, err := script.Compile(&script.Script{FileName: marker + ".js", Content: src}, z2m, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)
	return h
}

// fakeScripts is a scheduler.Scripts fake: it records subscribers and reports
// the device ids in conflict as in conflict. svc/scripts' own tests cover the
// real conflict rules.
type fakeScripts struct {
	conflict map[string]bool
	subs     []scripts.ChangeFunc
}

func (f *fakeScripts) Subscribe(fn scripts.ChangeFunc) { f.subs = append(f.subs, fn) }
func (f *fakeScripts) InConflict(devID string) bool    { return f.conflict[devID] }
func inConflict(devIDs ...string) *fakeScripts {
	f := &fakeScripts{conflict: map[string]bool{}}
	for _, id := range devIDs {
		f.conflict[id] = true
	}
	return f
}

// newService builds and starts a scheduler service over scripts, registers
// t.Cleanup for shutdown, and returns it. Jobs are registered by driving Sync
// directly, standing in for svc/scripts notifying its subscribers.
func newService(t *testing.T, scripts scheduler.Scripts) *scheduler.Service {
	t.Helper()
	svc, err := scheduler.New(scripts)
	require.NoError(t, err)
	require.NoError(t, svc.Start())
	t.Cleanup(svc.Shutdown)
	return svc
}

func TestSyncRegistersJobs(t *testing.T) {
	z2m := &recordingZ2M{}
	svc := newService(t, &fakeScripts{})

	// svc/scripts would deliver this handler to its subscribers; drive Sync directly.
	svc.Sync("living-room-lamp", scheduledHandler(t, z2m, "0 17 * * *", "lamp"))
	assert.Equal(t, 1, svc.JobCount())

	// Trigger the scheduled run and confirm the callback executed via z2m.
	require.NoError(t, svc.RunNow("living-room-lamp"))
	assert.Eventually(t, func() bool {
		c := z2m.calls()
		return len(c) == 1 && c[0] == "lamp.ran"
	}, time.Second, 10*time.Millisecond)
}

func TestSyncBrokenHandlerRegistersNothing(t *testing.T) {
	// An invalid cron expression makes the whole script broken at compile, so
	// the scheduler gets a handler with no jobs, and other devices are
	// unaffected.
	z2m := &recordingZ2M{}
	svc := newService(t, &fakeScripts{})

	broken, err := script.Compile(&script.Script{FileName: "bad.js",
		Content: `module.exports = { jobs: [{ cron: "not a cron", run(ctx) {} }] };`}, z2m, nil, 0)
	require.NoError(t, err)
	require.Error(t, broken.Error)

	svc.Sync("good", scheduledHandler(t, z2m, "0 17 * * *", "good"))
	svc.Sync("bad", broken)

	assert.Equal(t, 1, svc.JobCount(), "only the valid script's job is registered")
}

func TestSyncBrokenReloadDropsDeviceJobs(t *testing.T) {
	// A reload that breaks a script removes its previously registered jobs.
	z2m := &recordingZ2M{}
	svc := newService(t, &fakeScripts{})
	svc.Sync("lamp", scheduledHandler(t, z2m, "0 17 * * *", "lamp"))
	require.Equal(t, 1, svc.JobCount())

	broken, err := script.Compile(&script.Script{FileName: "lamp.js", Content: `module.exports = {`}, z2m, nil, 0)
	require.NoError(t, err)
	svc.Sync("lamp", broken)
	assert.Equal(t, 0, svc.JobCount())
}

func TestSyncReplacesDeviceJobs(t *testing.T) {
	z2m := &recordingZ2M{}
	svc := newService(t, &fakeScripts{})

	svc.Sync("lamp", scheduledHandler(t, z2m, "0 17 * * *", "lamp-v1"))
	require.Equal(t, 1, svc.JobCount())

	// A reload for the same device replaces its jobs: still one job, and it is
	// the new handler's.
	svc.Sync("lamp", scheduledHandler(t, z2m, "*/5 * * * *", "lamp-v2"))
	assert.Equal(t, 1, svc.JobCount())

	require.NoError(t, svc.RunNow("lamp"))
	assert.Eventually(t, func() bool {
		c := z2m.calls()
		return len(c) == 1 && c[0] == "lamp-v2.ran"
	}, time.Second, 10*time.Millisecond)
}

func TestSyncRemovesDeviceJobs(t *testing.T) {
	svc := newService(t, &fakeScripts{})

	svc.Sync("lamp", scheduledHandler(t, &recordingZ2M{}, "0 17 * * *", "lamp"))
	require.Equal(t, 1, svc.JobCount())

	// A removal (nil handler) drops the device's jobs.
	svc.Sync("lamp", nil)
	assert.Equal(t, 0, svc.JobCount())
	assert.Error(t, svc.RunNow("lamp"), "a device with no jobs has nothing to run")
}

func TestSyncHandlerWithoutScheduleRegistersNothing(t *testing.T) {
	src := `module.exports = { handle(ctx, state) {} };`
	h, err := script.Compile(&script.Script{FileName: "quiet.js", Content: src}, nil, nil, 0)
	require.NoError(t, err)

	svc := newService(t, &fakeScripts{})
	svc.Sync("quiet", h)

	assert.Equal(t, 0, svc.JobCount())
}

func TestNewRequiresScriptService(t *testing.T) {
	_, err := scheduler.New(nil)
	assert.Error(t, err, "a missing script service is an error")
}

func TestSyncAfterShutdownIsIgnored(t *testing.T) {
	z2m := &recordingZ2M{}
	svc := newService(t, &fakeScripts{})
	svc.Shutdown()

	// The script watcher may still deliver a change during teardown.
	svc.Sync("lamp", scheduledHandler(t, z2m, "0 17 * * *", "lamp"))
	assert.Equal(t, 0, svc.JobCount())
}

func TestJobsOfDeviceInConflictDoNotRun(t *testing.T) {
	// One device, scripted under both ids: neither script's jobs may run, while
	// another device's still do.
	z2m := &recordingZ2M{}
	svc := newService(t, inConflict("lamp", "0x01"))
	svc.Sync("lamp", scheduledHandler(t, z2m, "0 17 * * *", "by-name"))
	svc.Sync("0x01", scheduledHandler(t, z2m, "0 17 * * *", "by-addr"))
	svc.Sync("door", scheduledHandler(t, z2m, "0 17 * * *", "door"))
	require.Equal(t, 3, svc.JobCount())

	for _, id := range []string{"lamp", "0x01", "door"} {
		require.NoError(t, svc.RunNow(id))
	}
	assert.Eventually(t, func() bool { return len(z2m.calls()) == 1 }, time.Second, 10*time.Millisecond)
	assert.Never(t, func() bool { return len(z2m.calls()) > 1 },
		200*time.Millisecond, 10*time.Millisecond, "a conflicting device's jobs must not run")
	assert.Equal(t, []string{"door.ran"}, z2m.calls())
}

func TestNewSubscribesSync(t *testing.T) {
	// The scheduler follows svc/scripts' handler changes, starting with the
	// initial set, so it must subscribe before svc/scripts starts.
	scripts := &fakeScripts{}
	svc := newService(t, scripts)
	require.Len(t, scripts.subs, 1)

	scripts.subs[0]("lamp", scheduledHandler(t, &recordingZ2M{}, "0 17 * * *", "lamp"))
	assert.Equal(t, 1, svc.JobCount())
}

func TestConcurrentSyncsKeepOneJobSet(t *testing.T) {
	// Each Sync replaces the device's jobs in one step, so concurrent reloads
	// of one script leave exactly its jobs registered, never several sets.
	z2m := &recordingZ2M{}
	svc := newService(t, &fakeScripts{})
	h := scheduledHandler(t, z2m, "0 17 * * *", "lamp")

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { svc.Sync("lamp", h) })
	}
	wg.Wait()
	assert.Equal(t, 1, svc.JobCount())
}

func TestSyncDuringShutdownRegistersNothingLate(t *testing.T) {
	// A Sync racing Shutdown either finishes first or is a no-op: it never
	// registers with a scheduler that is shutting down.
	logs := slogtest.Capture(t, slog.LevelError)
	z2m := &recordingZ2M{}
	svc := newService(t, &fakeScripts{})
	h := scheduledHandler(t, z2m, "0 17 * * *", "lamp")

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { svc.Sync("lamp", h) })
	}
	wg.Go(svc.Shutdown)
	wg.Wait()
	assert.NotContains(t, logs.String(), "register job failed")
	assert.NotContains(t, logs.String(), "remove job")
}

// blockingZ2M blocks every publishProp until release is closed and counts the
// calls, so a test can hold a job run in progress. entered is closed on the
// first call.
type blockingZ2M struct {
	recordingZ2M
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (b *blockingZ2M) PublishProp(name, prop string, v any) error {
	_ = b.recordingZ2M.PublishProp(name, prop, v)
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return nil
}

const skipped = "svc/scheduler: skip job run, previous run still in progress"

func TestOverlappingJobRunsAreSkipped(t *testing.T) {
	logs := slogtest.Capture(t, slog.LevelInfo)
	z2m := &blockingZ2M{entered: make(chan struct{}), release: make(chan struct{})}
	svc := newService(t, &fakeScripts{})
	svc.Sync("lamp", scheduledHandler(t, z2m, "0 17 * * *", "lamp"))

	require.NoError(t, svc.RunNow("lamp"))
	select {
	case <-z2m.entered:
	case <-time.After(time.Second):
		t.Fatal("the first run did not start")
	}

	// The first run is still in progress, so these are skipped rather than
	// queued behind it. RunNow only hands the run to gocron, so wait for the
	// skips to be logged before letting the first run finish.
	for range 3 {
		require.NoError(t, svc.RunNow("lamp"))
	}
	require.Eventually(t, func() bool { return logs.Count(skipped) == 3 },
		time.Second, 5*time.Millisecond, "each overlapping run is skipped and logged")
	assert.Contains(t, logs.String(), "device=lamp")
	assert.Contains(t, logs.String(), `cron="0 17 * * *"`)
	close(z2m.release)

	assert.Never(t, func() bool { return len(z2m.calls()) > 1 },
		100*time.Millisecond, 10*time.Millisecond, "a skipped run ran anyway")
}
