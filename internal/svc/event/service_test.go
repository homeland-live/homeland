package event_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/script"
	"github.com/homeland-live/homeland/internal/svc/event"
	"github.com/homeland-live/homeland/internal/z2m"
)

// recordingZ2M records publishProp calls so a test can tell which handler ran.
// It satisfies script.Z2M and is safe for the device workers to call.
type recordingZ2M struct {
	mu     sync.Mutex
	props  []string
	values []any
}

func (r *recordingZ2M) Publish(_ string, _ any) error { return nil }
func (r *recordingZ2M) PublishProp(name, prop string, value any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.props = append(r.props, name+"."+prop)
	r.values = append(r.values, value)
	return nil
}
func (r *recordingZ2M) DeviceState(_ string) *z2m.DeviceState { return nil }

func (r *recordingZ2M) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.props...)
}

// recordedValues returns the values of every publishProp call, in order.
func (r *recordingZ2M) recordedValues() []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]any(nil), r.values...)
}

// lastValue returns the value of the most recent publishProp call.
func (r *recordingZ2M) lastValue() any {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.values) == 0 {
		return nil
	}
	return r.values[len(r.values)-1]
}

// markerHandler compiles a handler that publishes a single marker prop, so a
// test can observe that this specific handler (and not another) ran.
func markerHandler(t *testing.T, z2m script.Z2M, marker string) *script.Handler {
	t.Helper()
	src := `module.exports = { handle(ctx, state) { ctx.z2m.publishProp("` + marker + `", "ran", true); } };`
	h, err := script.Compile(&script.Script{FileName: marker + ".js", Content: src}, z2m, nil, 0)
	require.NoError(t, err)
	return h
}

// handlerMap is an event.Handlers fake: a device's handler is the one under
// its IEEE address, else the one under its friendly name. svc/scripts' own
// tests cover the real resolution rules.
type handlerMap map[string]*script.Handler

func (m handlerMap) Handler(spec *z2m.DeviceSpec) *script.Handler {
	if h, ok := m[spec.IEEEAddress]; ok {
		return h
	}
	return m[spec.FriendlyName]
}

// updates is an event.Z2M fake: it keeps the subscriber Start registers, so a
// test can deliver updates as the z2m client would.
type updates struct{ fn z2m.UpdateFunc }

func (u *updates) Subscribe(fn z2m.UpdateFunc) { u.fn = fn }

// serviceWith starts an event Service over handlers, storing no events, and
// returns it with the subscriber it registered, so a test can deliver
// updates to it.
func serviceWith(t *testing.T, handlers handlerMap) (svc *event.Service, deliver z2m.UpdateFunc) {
	t.Helper()
	return serviceWithStore(t, handlers, nil)
}

// serviceWithStore is serviceWith, storing events in store.
func serviceWithStore(t *testing.T, handlers handlerMap, store event.Store) (svc *event.Service, deliver z2m.UpdateFunc) {
	t.Helper()
	src := &updates{}
	svc = event.New(handlers, src, store)
	require.NoError(t, svc.Start())
	require.NotNil(t, src.fn, "Start must subscribe to device updates")
	return svc, src.fn
}

// withPrev sets prev as the state dev's state replaced, as the z2m registry
// does, and returns dev.
func withPrev(dev *z2m.Device, prev *z2m.DeviceState) *z2m.Device {
	dev.State.Prev = prev
	return dev
}

// stateWithN returns the state {"n": n}.
func stateWithN(n int) *z2m.DeviceState {
	return &z2m.DeviceState{Raw: map[string]json.RawMessage{"n": json.RawMessage(fmt.Sprint(n))}}
}

func device(ieee, friendly string) *z2m.Device {
	return &z2m.Device{
		Spec:  &z2m.DeviceSpec{IEEEAddress: ieee, FriendlyName: friendly},
		State: &z2m.DeviceState{Raw: map[string]json.RawMessage{}},
	}
}

// deviceWithN returns a device whose state is {"n": n}.
func deviceWithN(ieee, friendly string, n int) *z2m.Device {
	dev := device(ieee, friendly)
	dev.State.Raw["n"] = json.RawMessage(fmt.Sprint(n))
	return dev
}

// TestUpdateRunsDeviceHandler checks that an update runs the handler resolved
// for the device's spec, and runs nothing, logging no error, when it has none
// or a broken one (the API and UI report that).
func TestUpdateRunsDeviceHandler(t *testing.T) {
	const addr, name = "0x00124b00247f9ca8", "living-room-lamp"
	broken := script.NewBrokenHandler(script.NewScript(name+".js", ""), errors.New("device has two scripts"))
	cases := map[string]struct {
		handlers func(t *testing.T, z script.Z2M) handlerMap
		ran      []string
	}{
		"by IEEE address": {
			handlers: func(t *testing.T, z script.Z2M) handlerMap { return handlerMap{addr: markerHandler(t, z, addr)} },
			ran:      []string{addr + ".ran"},
		},
		"by friendly name": {
			handlers: func(t *testing.T, z script.Z2M) handlerMap { return handlerMap{name: markerHandler(t, z, name)} },
			ran:      []string{name + ".ran"},
		},
		"no handler": {
			handlers: func(*testing.T, script.Z2M) handlerMap { return handlerMap{} },
		},
		"broken handler": {
			handlers: func(*testing.T, script.Z2M) handlerMap { return handlerMap{name: broken} },
		},
	}
	for tcName, tc := range cases {
		t.Run(tcName, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				logs := slogtest.Capture(t, slog.LevelError)
				z2mRec := &recordingZ2M{}
				svc, deliver := serviceWith(t, tc.handlers(t, z2mRec))

				deliver(device(addr, name))
				synctest.Wait()
				// Shutdown waits for the workers, so their writes are visible.
				svc.Shutdown()

				assert.Equal(t, tc.ran, z2mRec.recorded())
				assert.Empty(t, logs.String())
			})
		})
	}
}

// TestUpdatePassesPreviousStateToHandler checks that the previous state an
// update carries reaches the handler as its prev argument, and that an update
// without one gets null.
func TestUpdatePassesPreviousStateToHandler(t *testing.T) {
	const addr, name = "0x01", "lamp"
	// The handler echoes the previous state's "state" prop, or "none".
	src := `module.exports = { handle(ctx, state, prevState) {
		ctx.z2m.publishProp("marker", "prev", prevState === null ? "none" : prevState.state);
	} };`

	cases := map[string]struct {
		prev *z2m.DeviceState
		want string
	}{
		"with previous state": {
			prev: &z2m.DeviceState{Raw: map[string]json.RawMessage{"state": json.RawMessage(`"ON"`)}},
			want: "ON",
		},
		"without previous state": {want: "none"},
	}
	for tcName, tc := range cases {
		t.Run(tcName, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				z2mRec := &recordingZ2M{}
				h, err := script.Compile(&script.Script{FileName: name + ".js", Content: src}, z2mRec, nil, 0)
				require.NoError(t, err)
				svc, deliver := serviceWith(t, handlerMap{addr: h})

				deliver(withPrev(device(addr, name), tc.prev))
				synctest.Wait()
				svc.Shutdown()

				require.Equal(t, []string{"marker.prev"}, z2mRec.recorded())
				assert.Equal(t, tc.want, z2mRec.lastValue())
			})
		})
	}
}

// The tests below run in a synctest bubble: synctest.Wait returns once every
// other goroutine (workers, Shutdown) is blocked, so they assert on settled
// state without sleeps or polling. A call that should return but blocks
// forever deadlocks the bubble, which fails the test.

// blockingZ2M records every publishProp, then blocks it until release is
// closed, so a test can hold a handler in flight. entered is closed on the
// first call.
type blockingZ2M struct {
	recordingZ2M
	mu      sync.Mutex
	once    sync.Once
	entered chan struct{}
	release chan struct{}
	calls   int
}

func newBlockingZ2M() *blockingZ2M {
	return &blockingZ2M{entered: make(chan struct{}), release: make(chan struct{})}
}

func (b *blockingZ2M) PublishProp(name, prop string, value any) error {
	_ = b.recordingZ2M.PublishProp(name, prop, value)
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return nil
}

func (b *blockingZ2M) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

// shutdownAsync starts Shutdown on its own goroutine and returns a channel
// closed when it returns.
func shutdownAsync(svc *event.Service) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		svc.Shutdown()
		close(done)
	}()
	return done
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestUpdateDoesNotWaitForTheHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		z2mFake := newBlockingZ2M()
		svc, deliver := serviceWith(t, handlerMap{
			"lamp": markerHandler(t, z2mFake, "lamp"),
		})

		// The subscriber runs on the MQTT delivery goroutine, so it must
		// return while the handler is still blocked.
		deliver(device("0x01", "lamp"))
		<-z2mFake.entered

		close(z2mFake.release)
		svc.Shutdown()
	})
}

// TestQueuedRunKeepsItsPrevState checks that each run carries the previous
// state of its own update, even when later updates arrive while it waits in
// the queue.
func TestQueuedRunKeepsItsPrevState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const addr, name = "0x01", "lamp"
		z2mFake := newBlockingZ2M()
		h, err := script.Compile(&script.Script{FileName: name + ".js", Content: `module.exports = {
			handle(ctx, state, prevState) { ctx.z2m.publishProp("marker", "prev", String(prevState.n)); }
		};`}, z2mFake, nil, 0)
		require.NoError(t, err)
		svc, deliver := serviceWith(t, handlerMap{addr: h})

		// The first run holds the worker, so the next two wait in the queue.
		deliver(withPrev(deviceWithN(addr, name, 2), stateWithN(1)))
		<-z2mFake.entered
		deliver(withPrev(deviceWithN(addr, name, 3), stateWithN(2)))
		deliver(withPrev(deviceWithN(addr, name, 4), stateWithN(3)))

		close(z2mFake.release)
		synctest.Wait()
		svc.Shutdown()

		assert.Equal(t, []any{"1", "2", "3"}, z2mFake.recordedValues(),
			"each run must see the previous state of its own update")
	})
}

func TestDevicesRunInParallel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		slow := newBlockingZ2M()
		fast := &recordingZ2M{}
		svc, deliver := serviceWith(t, handlerMap{
			"slow": markerHandler(t, slow, "slow"),
			"fast": markerHandler(t, fast, "fast"),
		})

		deliver(device("0x01", "slow"))
		<-slow.entered
		deliver(device("0x02", "fast"))
		synctest.Wait()

		assert.Equal(t, []string{"fast.ran"}, fast.recorded(),
			"a blocked device held up another device's handler")

		close(slow.release)
		svc.Shutdown()
	})
}

func TestDeviceRunsInArrivalOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		z2mRec := &recordingZ2M{}
		src := `module.exports = { handle(ctx, state) { ctx.z2m.publishProp("lamp", String(state.n), true); } };`
		h, err := script.Compile(&script.Script{FileName: "lamp.js", Content: src}, z2mRec, nil, 0)
		require.NoError(t, err)
		svc, deliver := serviceWith(t, handlerMap{"lamp": h})

		want := make([]string, 0, 20)
		for n := range 20 {
			deliver(deviceWithN("0x01", "lamp", n))
			want = append(want, fmt.Sprintf("lamp.%d", n))
		}
		synctest.Wait()

		assert.Equal(t, want, z2mRec.recorded())
		svc.Shutdown()
	})
}

func TestFullDeviceQueueDropsNewUpdates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		z2mFake := newBlockingZ2M()
		svc, deliver := serviceWith(t, handlerMap{
			"lamp": markerHandler(t, z2mFake, "lamp"),
		})
		dev := device("0x01", "lamp")

		deliver(dev)
		<-z2mFake.entered
		// One run is in progress; fill the queue, then overflow it. None of
		// these may block.
		for range event.DeviceQueueSize + 5 {
			deliver(dev)
		}

		close(z2mFake.release)
		synctest.Wait()
		assert.Equal(t, 1+event.DeviceQueueSize, z2mFake.callCount(),
			"updates beyond the queue size must be dropped")
		svc.Shutdown()
	})
}

func TestShutdownWaitsForInFlightRunAndDropsLaterUpdates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		z2mFake := newBlockingZ2M()
		svc, deliver := serviceWith(t, handlerMap{
			"lamp": markerHandler(t, z2mFake, "lamp"),
		})
		dev := device("0x01", "lamp")

		deliver(dev)
		<-z2mFake.entered

		done := shutdownAsync(svc)
		synctest.Wait()
		assert.False(t, isClosed(done), "Shutdown returned while a handler was in flight")

		close(z2mFake.release)
		<-done

		// An update delivered after Shutdown is dropped: the handler does not
		// run.
		deliver(dev)
		synctest.Wait()
		assert.Equal(t, 1, z2mFake.callCount())
	})
}

func TestShutdownDropsQueuedRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		z2mFake := newBlockingZ2M()
		svc, deliver := serviceWith(t, handlerMap{
			"lamp": markerHandler(t, z2mFake, "lamp"),
		})
		dev := device("0x01", "lamp")

		deliver(dev)
		<-z2mFake.entered
		for range 3 {
			deliver(dev)
		}

		done := shutdownAsync(svc)
		// Returns once Shutdown is blocked waiting for the run in progress,
		// i.e. after it has signaled the worker to stop.
		synctest.Wait()

		close(z2mFake.release)
		<-done
		assert.Equal(t, 1, z2mFake.callCount(), "queued runs must not start after Shutdown")
	})
}
