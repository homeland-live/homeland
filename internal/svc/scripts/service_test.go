package scripts_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/script"
	"github.com/homeland-live/homeland/internal/svc/scripts"
	"github.com/homeland-live/homeland/internal/z2m"
)

// Fakes and helpers.

// stubZ2M publishes nothing and knows no devices. It satisfies Z2M.
type stubZ2M struct{}

func (stubZ2M) Publish(_ string, _ any) error         { return nil }
func (stubZ2M) PublishProp(_, _ string, _ any) error  { return nil }
func (stubZ2M) DeviceState(_ string) *z2m.DeviceState { return nil }
func (stubZ2M) Device(_ string) *z2m.Device           { return nil }

// rosterZ2M knows devices by friendly name and IEEE address, like the real
// client once bridge/devices has arrived.
type rosterZ2M struct {
	stubZ2M
	devices map[string]*z2m.Device
}

func (r rosterZ2M) Device(id string) *z2m.Device { return r.devices[id] }

func newRoster(specs ...*z2m.DeviceSpec) rosterZ2M {
	r := rosterZ2M{devices: map[string]*z2m.Device{}}
	for _, s := range specs {
		dev := &z2m.Device{Spec: s}
		r.devices[s.IEEEAddress] = dev
		r.devices[s.FriendlyName] = dev
	}
	return r
}

// stubMQTT returns a fixed health result. It satisfies MQTT.
type stubMQTT struct {
	healthy bool
}

func (s stubMQTT) Connect() error                   { return nil }
func (s stubMQTT) Disconnect()                      {}
func (s stubMQTT) Publish(_ string, _ []byte) error { return nil }

func (s stubMQTT) HealthCheck(name string) healthcheck.HealthCheck {
	if s.healthy {
		return healthcheck.New(name, nil)
	}
	return healthcheck.New(name, fmt.Errorf("mqtt: connection is down"))
}

// countingMQTT counts connects and disconnects. It satisfies MQTT.
type countingMQTT struct {
	stubMQTT
	connects, disconnects int
}

func (c *countingMQTT) Connect() error { c.connects++; return nil }
func (c *countingMQTT) Disconnect()    { c.disconnects++ }

// changeRecorder is a handler-change subscriber that records every change it
// receives. Changes may arrive on the watcher goroutine, so it is mutex-guarded.
type changeRecorder struct {
	mu      sync.Mutex
	changes []string
}

// record logs "+id" for a working handler, "!id" for a broken one and "-id"
// for a removal.
func (r *changeRecorder) record(devID string, h *script.Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case h == nil:
		r.changes = append(r.changes, "-"+devID)
	case h.Error != nil:
		r.changes = append(r.changes, "!"+devID)
	default:
		r.changes = append(r.changes, "+"+devID)
	}
}

func (r *changeRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.changes...)
}

// waitFor waits until r has recorded change. The service updates its cache
// before notifying, so once change is recorded the cache reflects it too;
// waiting on the cache instead could check r before it is notified.
func (r *changeRecorder) waitFor(t *testing.T, change string) {
	t.Helper()
	require.Eventually(t, func() bool { return slices.Contains(r.snapshot(), change) },
		2*time.Second, 10*time.Millisecond, "no %q change recorded", change)
}

// lamp is a device whose scripts would be 0x01.js and lamp.js.
var lamp = &z2m.DeviceSpec{IEEEAddress: "0x01", FriendlyName: "lamp"}

// compiled returns a working handler compiled from fileName.
func compiled(t *testing.T, fileName string) *script.Handler {
	t.Helper()
	h, err := script.Compile(script.NewScript(fileName, `module.exports = { handle(ctx, state) {} };`), nil, nil, 0)
	require.NoError(t, err)
	require.NoError(t, h.Error)
	return h
}

// broken returns a handler whose fileName fails to compile.
func broken(t *testing.T, fileName string) *script.Handler {
	t.Helper()
	h, err := script.Compile(script.NewScript(fileName, `module.exports = {`), nil, nil, 0)
	require.NoError(t, err)
	require.Error(t, h.Error)
	return h
}

// withHandlers returns a service, not started, whose cache holds handlers
// keyed by device id.
func withHandlers(z scripts.Z2M, handlers map[string]*script.Handler) *scripts.Service {
	svc := scripts.New(&config.Config{}, z)
	for id, h := range handlers {
		svc.WithHandler(id, h)
	}
	return svc
}

// writeScripts writes a working script for each file name into dir.
func writeScripts(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		src := []byte(`module.exports = { handle(ctx, state) {} };`)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), src, 0o600))
	}
}

func childByName(hc healthcheck.HealthCheck, name string) (healthcheck.HealthCheck, bool) {
	for _, c := range hc.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return healthcheck.HealthCheck{}, false
}

// Start and Shutdown.

func TestStartRequiresScriptsConfig(t *testing.T) {
	svc := scripts.New(&config.Config{}, stubZ2M{})
	assert.Error(t, svc.Start())
}

func TestStartLoadsScriptsByDeviceID(t *testing.T) {
	// A script's device id is its file name without ".js": a friendly name or
	// an IEEE address. Each one resolves for the device it names.
	dir := t.TempDir()
	writeScripts(t, dir, "living-room-lamp.js", "0x00124b00247f9ca8.js")

	cfg := &config.Config{Scripts: &config.Scripts{Dir: dir}}
	svc := scripts.New(cfg, stubZ2M{})
	require.NoError(t, svc.Start())
	t.Cleanup(svc.Shutdown)

	byName := svc.Handler(&z2m.DeviceSpec{IEEEAddress: "0x01", FriendlyName: "living-room-lamp"})
	require.NotNil(t, byName, "a friendly-name script resolves for its device")
	assert.Equal(t, "living-room-lamp.js", byName.Script().FileName)

	byAddr := svc.Handler(&z2m.DeviceSpec{IEEEAddress: "0x00124b00247f9ca8", FriendlyName: "hallway"})
	require.NotNil(t, byAddr, "an IEEE-address script resolves for its device")
	assert.Equal(t, "0x00124b00247f9ca8.js", byAddr.Script().FileName)

	assert.Nil(t, svc.Handler(&z2m.DeviceSpec{IEEEAddress: "0x02", FriendlyName: "living-room-lamp.js"}),
		"a device id never carries the .js extension")
}

func TestStartConnectsInjectedExtBroker(t *testing.T) {
	// An injected broker needs no HL_EXT_MQTT_* config. Start connects it and
	// Shutdown disconnects it.
	ext := &countingMQTT{}
	cfg := &config.Config{Scripts: &config.Scripts{Dir: t.TempDir()}}
	svc := scripts.New(cfg, stubZ2M{}, scripts.WithMQTT(ext))

	require.NoError(t, svc.Start())
	assert.Equal(t, 1, ext.connects)
	svc.Shutdown()
	assert.Equal(t, 1, ext.disconnects)
}

func TestStartDisconnectsExtBrokerWhenLaterStepFails(t *testing.T) {
	// A scripts "dir" that is a file can't be loaded, which fails Start after
	// the external broker connected; it must not stay connected.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	ext := &countingMQTT{}
	cfg := &config.Config{Scripts: &config.Scripts{Dir: file}}
	svc := scripts.New(cfg, stubZ2M{}, scripts.WithMQTT(ext))

	require.Error(t, svc.Start())
	assert.Equal(t, 1, ext.connects)
	assert.Equal(t, 1, ext.disconnects)
}

// Subscribe.

func TestSubscribeFansOutInitialSetToEverySubscriber(t *testing.T) {
	dir := t.TempDir()
	writeScripts(t, dir, "lamp.js", "switch.js")

	cfg := &config.Config{Scripts: &config.Scripts{Dir: dir}}
	svc := scripts.New(cfg, stubZ2M{})

	// Registration order is delivery order: record into one shared log too.
	var order []string
	first, second := &changeRecorder{}, &changeRecorder{}
	svc.Subscribe(func(devID string, h *script.Handler) {
		first.record(devID, h)
		order = append(order, "first:"+devID)
	})
	svc.Subscribe(func(devID string, h *script.Handler) {
		second.record(devID, h)
		order = append(order, "second:"+devID)
	})

	require.NoError(t, svc.Start())
	t.Cleanup(svc.Shutdown)

	// A second Subscribe must not replace the first: both see the full set.
	want := []string{"+lamp", "+switch"}
	assert.Equal(t, want, first.snapshot())
	assert.Equal(t, want, second.snapshot())
	assert.Equal(t, []string{"first:lamp", "second:lamp", "first:switch", "second:switch"}, order)
}

func TestSubscribeFansOutWatcherChangesToEverySubscriber(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Scripts: &config.Scripts{Dir: dir, EnableWatcher: true}}
	svc := scripts.New(cfg, stubZ2M{})

	first, second := &changeRecorder{}, &changeRecorder{}
	svc.Subscribe(first.record)
	svc.Subscribe(second.record)

	require.NoError(t, svc.Start())
	t.Cleanup(svc.Shutdown)

	writeScripts(t, dir, "lamp.js")
	for _, r := range []*changeRecorder{first, second} {
		assert.Eventually(t, func() bool {
			return slices.Contains(r.snapshot(), "+lamp")
		}, 2*time.Second, 10*time.Millisecond, "every subscriber must see the added script")
	}

	require.NoError(t, os.Remove(filepath.Join(dir, "lamp.js")))
	for _, r := range []*changeRecorder{first, second} {
		assert.Eventually(t, func() bool {
			return slices.Contains(r.snapshot(), "-lamp")
		}, 2*time.Second, 10*time.Millisecond, "every subscriber must see the removed script")
	}
}

// Handler.

func TestHandlerByEitherID(t *testing.T) {
	for id, fileName := range map[string]string{"0x01": "0x01.js", "lamp": "lamp.js"} {
		t.Run(fileName, func(t *testing.T) {
			h := compiled(t, fileName)
			assert.Same(t, h, withHandlers(nil, map[string]*script.Handler{id: h}).Handler(lamp))
		})
	}
}

func TestHandlerReturnsBrokenScript(t *testing.T) {
	// A script that failed to compile still belongs to its device: it is
	// returned as is, so the API shows the error and running it reports it.
	h := broken(t, "lamp.js")
	got := withHandlers(nil, map[string]*script.Handler{"lamp": h}).Handler(lamp)
	require.Same(t, h, got)
	assert.Error(t, got.Handle(map[string]any{}))
}

func TestHandlerWithoutScriptIsNil(t *testing.T) {
	svc := withHandlers(nil, map[string]*script.Handler{"other": compiled(t, "other.js")})
	h := svc.Handler(lamp)
	assert.Nil(t, h)
	require.NoError(t, h.Handle(map[string]any{}), "a nil handler is a no-op")
	assert.Nil(t, svc.Handler(nil))
}

func TestHandlerWithScriptsUnderBothIDsIsConflict(t *testing.T) {
	// Whether either script compiled doesn't matter: two scripts for one
	// device is a conflict, reported as a broken handler naming both files.
	cases := map[string]map[string]*script.Handler{
		"both compile":   {"0x01": compiled(t, "0x01.js"), "lamp": compiled(t, "lamp.js")},
		"one broken":     {"0x01": broken(t, "0x01.js"), "lamp": compiled(t, "lamp.js")},
		"both broken":    {"0x01": broken(t, "0x01.js"), "lamp": broken(t, "lamp.js")},
		"name only fine": {"0x01": compiled(t, "0x01.js"), "lamp": broken(t, "lamp.js")},
	}
	for name, handlers := range cases {
		t.Run(name, func(t *testing.T) {
			h := withHandlers(nil, handlers).Handler(lamp)
			require.NotNil(t, h)
			require.ErrorContains(t, h.Error, `device "lamp" has two scripts, 0x01.js and lamp.js; keep only one`)
			assert.False(t, h.HasHandle)
			assert.Empty(t, h.Jobs())
			assert.Equal(t, "0x01.js", h.Script().FileName)
			require.Error(t, h.Handle(map[string]any{}), "neither script may run")
		})
	}
}

func TestHandlerFriendlyNameEqualToAddressIsOneScript(t *testing.T) {
	// Zigbee2MQTT's default friendly name is the IEEE address: one file.
	h := compiled(t, "0x02.js")
	svc := withHandlers(nil, map[string]*script.Handler{"0x02": h})
	assert.Same(t, h, svc.Handler(&z2m.DeviceSpec{IEEEAddress: "0x02", FriendlyName: "0x02"}))
	assert.False(t, svc.InConflict("0x02"))
}

// InConflict.

func TestInConflict(t *testing.T) {
	both := map[string]*script.Handler{"0x01": compiled(t, "0x01.js"), "lamp": compiled(t, "lamp.js")}

	svc := withHandlers(newRoster(lamp), both)
	assert.True(t, svc.InConflict("lamp"))
	assert.True(t, svc.InConflict("0x01"))

	single := withHandlers(newRoster(lamp), map[string]*script.Handler{"lamp": compiled(t, "lamp.js")})
	assert.False(t, single.InConflict("lamp"))

	// Before the device list arrives, the ids can't be linked.
	assert.False(t, withHandlers(newRoster(), both).InConflict("lamp"))
	assert.False(t, withHandlers(nil, both).InConflict("lamp"))
}

// HealthCheck.

func TestHealthCheckWithoutExtBrokerIsHealthy(t *testing.T) {
	hc := scripts.New(&config.Config{}, nil).HealthCheck()
	assert.True(t, hc.IsHealthy)
	// An unconfigured external broker still appears, as a healthy child.
	ext, ok := childByName(hc, "ext-mqtt")
	require.Truef(t, ok, "expected ext-mqtt child, got %+v", hc.Checks)
	assert.True(t, ext.IsHealthy)
}

func TestHealthCheckReportsExtBrokerDown(t *testing.T) {
	hc := scripts.New(&config.Config{}, nil, scripts.WithMQTT(stubMQTT{healthy: false})).HealthCheck()
	assert.False(t, hc.IsHealthy)
	ext, ok := childByName(hc, "ext-mqtt")
	require.Truef(t, ok, "expected ext-mqtt child, got %+v", hc.Checks)
	assert.False(t, ext.IsHealthy)
}
