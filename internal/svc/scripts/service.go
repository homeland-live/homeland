// Package scripts compiles device scripts into a handler cache keyed by device
// id and keeps it in sync with the scripts directory.
package scripts

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/mqtt"
	"github.com/homeland-live/homeland/internal/script"
	"github.com/homeland-live/homeland/internal/z2m"
)

// Service caches compiled handlers keyed by device id: a script's file name
// without ".js", which is a friendly name or an IEEE address. Both ids may be
// cached for one device, since only z2m knows they belong together; Handler
// and InConflict treat that as a conflict, so neither runs. Safe for
// concurrent use.
type Service struct {
	cfg  *config.Config
	z2m  Z2M
	mqtt MQTT

	cache *handlerCache

	watcher *watcher

	subMu sync.RWMutex
	subs  []ChangeFunc
}

// ChangeFunc is called with a device id and its new handler when the handler
// enters the cache, or with a nil handler when it leaves.
type ChangeFunc func(devID string, h *script.Handler)

// Start starts connecting the optional external broker (without waiting for
// it; see mqtt.Client.Connect), compiles every script in the scripts
// directory, notifies subscribers of each handler, and then (when enabled)
// starts watching the directory. A missing or empty directory is not an
// error. Subscribers registered before Start see the initial set before any
// watcher change.
func (svc *Service) Start() (err error) {
	if svc.cfg == nil || svc.cfg.Scripts == nil {
		return errors.New("svc/scripts: scripts configuration is required, but missing")
	}
	if svc.z2m == nil {
		return errors.New("svc/scripts: z2m client is required, but missing")
	}

	// Handlers capture svc.mqtt at compile time, so it must exist before
	// populate. Without an external broker it stays nil and ctx.mqtt.publish
	// throws.
	if svc.cfg.ExtMQTT != nil && svc.mqtt == nil {
		extMQTT, err := mqtt.New(*svc.cfg.ExtMQTT)
		if err != nil {
			return fmt.Errorf("svc/scripts: create ext-mqtt client: %w", err)
		}
		svc.mqtt = extMQTT
	}

	if svc.mqtt != nil {
		broker := "(injected)"
		if svc.cfg.ExtMQTT != nil {
			broker = svc.cfg.ExtMQTT.Broker
		}
		slog.Info("svc/scripts: connecting to ext broker", "broker", broker)
		if err := svc.mqtt.Connect(); err != nil {
			return fmt.Errorf("svc/scripts: connect to ext broker: %w", err)
		}
		// Don't leave the broker connected when a later step fails.
		defer func() {
			if err != nil {
				svc.mqtt.Disconnect()
			}
		}()
	}

	// No scripts yet is fine; with the watcher enabled, later ones are picked
	// up as they appear.
	scripts, err := script.LoadAll(svc.cfg.Scripts.Dir)
	switch {
	case errors.Is(err, script.ErrNotExist):
		slog.Warn("svc/scripts: no scripts loaded", "err", err)
	case err != nil:
		return fmt.Errorf("svc/scripts: load scripts: %w", err)
	}

	if err := svc.populate(scripts); err != nil {
		return err
	}

	if svc.cfg.Scripts.EnableWatcher {
		w, err := newWatcher(svc)
		if err != nil {
			return fmt.Errorf("svc/scripts: start watcher: %w", err)
		}
		svc.watcher = w
	}

	return nil
}

// populate compiles and caches every script, notifying subscribers of each.
func (svc *Service) populate(scripts []*script.Script) error {
	for _, s := range scripts {
		if err := svc.reload(s); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown stops the watcher, so no reload runs against a closed broker, then
// disconnects the external broker, if either was started. Call it once.
func (svc *Service) Shutdown() {
	if svc.watcher != nil {
		svc.watcher.close()
		svc.watcher = nil
	}
	if svc.mqtt != nil {
		svc.mqtt.Disconnect()
	}
}

// store compiles s and caches the handler under s.DeviceID. A script-author
// mistake yields a cached broken handler; the returned error is reserved for
// host-side failures.
func (svc *Service) store(s *script.Script) error {
	h, err := script.Compile(s, svc.z2m, svc.mqtt, svc.cfg.Scripts.Timeout)
	if err != nil {
		return fmt.Errorf("svc/scripts: compile %q: %w", s.FileName, err)
	}
	svc.cache.set(s.DeviceID, h)
	return nil
}

// reload compiles and caches s, then notifies subscribers with the new
// handler (broken or not).
func (svc *Service) reload(s *script.Script) error {
	if err := svc.store(s); err != nil {
		return err
	}
	h, _ := svc.cache.get(s.DeviceID)
	svc.notify(s.DeviceID, h)
	return nil
}

// drop removes the handler cached under devID and notifies subscribers with a
// nil handler. It reports whether a handler was cached; when none was, it does
// nothing.
func (svc *Service) drop(devID string) bool {
	if _, ok := svc.cache.get(devID); !ok {
		return false
	}
	svc.cache.remove(devID)
	svc.notify(devID, nil)
	return true
}

// Subscribe registers fn to be called on every handler change, including the
// initial set compiled by Start, so register before Start. Subscribers run
// synchronously, in registration order, on the goroutine that made the change
// (Start's caller, then the watcher), so keep them fast. Safe for concurrent
// use.
func (svc *Service) Subscribe(fn ChangeFunc) {
	svc.subMu.Lock()
	defer svc.subMu.Unlock()
	svc.subs = append(svc.subs, fn)
}

// notify calls every subscriber with the change.
func (svc *Service) notify(devID string, h *script.Handler) {
	svc.subMu.RLock()
	defer svc.subMu.RUnlock()
	for _, fn := range svc.subs {
		fn(devID, h)
	}
}

// handler returns the handler cached under devID (a friendly name or an IEEE
// address), or nil. It does not check for a script under the device's other
// id; callers outside the cache go through Handler.
func (svc *Service) handler(devID string) *script.Handler {
	handler, ok := svc.cache.get(devID)
	if !ok || handler == nil {
		return nil
	}
	return handler
}

// Handler returns the handler that runs for a device: the one cached under
// its IEEE address or under its friendly name, or nil when it has none. A
// device may have only one script. When both ids have a handler (broken or
// not), it returns a broken handler naming both files, so neither runs. Its
// Script is the IEEE-address one. A friendly name equal to the IEEE address
// (Zigbee2MQTT's default) is one script, not a conflict.
func (svc *Service) Handler(spec *z2m.DeviceSpec) *script.Handler {
	if spec == nil {
		return nil
	}
	byAddr := svc.handler(spec.IEEEAddress)
	if spec.FriendlyName == spec.IEEEAddress {
		return byAddr
	}
	byName := svc.handler(spec.FriendlyName)
	switch {
	case byAddr == nil:
		return byName
	case byName == nil:
		return byAddr
	}
	return script.NewBrokenHandler(byAddr.Script(), fmt.Errorf(
		"svc/scripts: device %q has two scripts, %s and %s; keep only one",
		spec.FriendlyName, byAddr.Script().FileName, byName.Script().FileName))
}

// InConflict reports whether the device that devID (a script's device id)
// names also has a script under its other id, so neither script may run. It
// is false while the device is unknown to z2m, since only z2m knows which
// friendly name belongs to which IEEE address.
func (svc *Service) InConflict(devID string) bool {
	if svc.z2m == nil {
		return false
	}
	dev := svc.z2m.Device(devID)
	if dev == nil || dev.Spec == nil || dev.Spec.FriendlyName == dev.Spec.IEEEAddress {
		return false
	}
	return svc.handler(dev.Spec.IEEEAddress) != nil && svc.handler(dev.Spec.FriendlyName) != nil
}

// ListHandlers returns every loaded script's handler (broken or not), sorted
// by device id (a friendly name or an IEEE address, i.e. the file name without
// ".js"; read from each handler's Script). Unlike Handler, it enumerates from
// the compiled cache rather than the device roster, so it includes orphan
// handlers whose id names no known device.
func (svc *Service) ListHandlers() []*script.Handler {
	snapshot := svc.cache.snapshot()
	out := make([]*script.Handler, 0, len(snapshot))
	for _, h := range snapshot {
		out = append(out, h)
	}
	slices.SortFunc(out, func(a, b *script.Handler) int {
		return strings.Compare(a.Script().DeviceID, b.Script().DeviceID)
	})
	return out
}

// HealthCheck reports the external broker connection. When no external broker
// is configured the leaf is healthy, so it shows as intentionally absent.
func (svc *Service) HealthCheck() healthcheck.HealthCheck {
	extCheck := healthcheck.New("ext-mqtt", nil)
	if svc.mqtt != nil {
		extCheck = svc.mqtt.HealthCheck("ext-mqtt")
	}

	return healthcheck.Compose("svc/scripts", extCheck)
}

// Option configures a Service at construction time.
type Option func(*Service)

// WithMQTT injects the external MQTT client backing ctx.mqtt instead of the one
// Start would build from config.
func WithMQTT(mqtt MQTT) Option {
	return func(svc *Service) {
		svc.mqtt = mqtt
	}
}

// New returns a service that is not started yet.
func New(cfg *config.Config, z2m Z2M, opts ...Option) *Service {
	svc := &Service{
		cfg:   cfg,
		z2m:   z2m,
		cache: newHandlerCache(),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}
