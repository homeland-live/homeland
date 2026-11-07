package z2m

import (
	"cmp"
	"log/slog"
	"slices"
	"sync"
)

// registry merges device specs from bridge/devices with live device state.
// Safe for concurrent use.
type registry struct {
	mu sync.RWMutex

	// addresses maps friendly name to IEEE address.
	addresses map[string]string
	// devices, schemas, states and options are keyed by IEEE address.
	devices map[string]*DeviceSpec
	schemas map[string]*StateSchema
	states  map[string]*DeviceState
	// options holds the merged options (CompileDeviceOptions) of every
	// device bridge/info's config.devices lists; one without
	// filtered_attributes of its own holds defaultOptions. Every other device
	// gets defaultOptions (device_options).
	options        map[string]*DeviceOptions
	defaultOptions *DeviceOptions

	bridgeOnline bool
}

func newRegistry() *registry {
	return &registry{
		addresses: make(map[string]string),
		devices:   make(map[string]*DeviceSpec),
		schemas:   make(map[string]*StateSchema),
		states:    make(map[string]*DeviceState),
		options:   make(map[string]*DeviceOptions),
	}
}

// deviceOptions returns the options of the device with IEEE address addr,
// nil before bridge/info arrives or when neither the device nor
// device_options sets any. The caller holds mu.
func (r *registry) deviceOptions(addr string) *DeviceOptions {
	if opts, ok := r.options[addr]; ok {
		return opts
	}
	return r.defaultOptions
}

// addr maps a friendly name to its IEEE address; any other id is returned
// unchanged. The caller holds mu, so the result can't go stale before use.
func (r *registry) addr(id string) string {
	if found, ok := r.addresses[id]; ok {
		return found
	}
	return id
}

// replaceDevices swaps in the full roster (bridge/devices is a complete
// list), keeping state for devices that remain and dropping the rest. A state
// stored under a friendly name, because it arrived before the roster could
// resolve it, moves to the device's IEEE address; when the address already
// has a state, the newer of the two is kept. Entries without an IEEE address
// or friendly name are skipped. Each schema applies the device's filter.
func (r *registry) replaceDevices(devices []DeviceSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()

	newAddresses := make(map[string]string, len(devices))
	newDevices := make(map[string]*DeviceSpec, len(devices))
	newSchemas := make(map[string]*StateSchema, len(devices))
	for _, d := range devices {
		if d.IEEEAddress == "" || d.FriendlyName == "" {
			continue
		}
		newAddresses[d.FriendlyName] = d.IEEEAddress
		newDevices[d.IEEEAddress] = &d
		newSchemas[d.IEEEAddress] = NewStateSchema(d.Definition, r.deviceOptions(d.IEEEAddress))
	}

	r.addresses = newAddresses
	r.devices = newDevices
	r.schemas = newSchemas

	for id, state := range r.states {
		if _, ok := newDevices[id]; ok {
			continue
		}
		delete(r.states, id)
		addr, ok := newAddresses[id]
		if !ok {
			continue
		}
		if prev, ok := r.states[addr]; !ok || state.CreatedAt.After(prev.CreatedAt) {
			r.states[addr] = state
		}
	}
}

// setState records a device's new state (devID is a friendly name or IEEE
// address). When subscribers should be notified, it returns the device with
// its new state; otherwise it returns nil. A devID the roster can't resolve
// yet keys the state as given (see replaceDevices). A stored state's Prev is
// a copy of the state it replaced, taken under the same lock, so the two
// always form a consecutive pair. The replaced state itself is left
// untouched, since earlier snapshots may still be reading it.
//
//   - The first state for a device is stored, without a Prev, but does not
//     propagate: it is the initial sync-up, not an event.
//   - A state that fails Validate against the device's schema is neither
//     stored nor propagated.
//   - A state that IsDuplicate of the previous one is neither stored nor
//     propagated.
//
// Any other state is stored, and propagates once the roster knows the
// device.
func (r *registry) setState(devID string, nextState *DeviceState) *Device {
	r.mu.Lock()
	defer r.mu.Unlock()

	addr := r.addr(devID)
	prevState, exists := r.states[addr]
	if !exists {
		r.states[addr] = nextState
		return nil
	}

	if err := nextState.Validate(r.schemas[addr]); err != nil {
		slog.Debug("z2m: ignore invalid event",
			"device", devID,
			"state", nextState,
			"err", err,
		)
		return nil
	}

	if nextState.IsDuplicate(prevState) {
		slog.Debug("z2m: ignore duplicate event",
			"device", devID,
			"state", nextState,
		)
		return nil
	}

	nextState.Prev = prevState.withoutPrev()
	r.states[addr] = nextState
	return r.deviceLocked(addr)
}

// setOptions swaps in the device options from the latest bridge/info's
// config (see CompileDeviceOptions) and rebuilds every known device's schema
// (replacing it, since snapshots share it), so a removed filter takes effect
// too. Options are replaced, never modified, so snapshots may share them.
// Unsupported patterns are logged once here and skipped.
func (r *registry) setOptions(cfg BridgeConfig) {
	defaults, err := CompileDeviceOptions(cfg.DeviceOptions, nil)
	if err != nil {
		slog.Warn("z2m: ignore unsupported device_options filter", "err", err)
	}
	options := make(map[string]*DeviceOptions, len(cfg.Devices))
	for addr, own := range cfg.Devices {
		opts, err := CompileDeviceOptions(own, defaults)
		if err != nil {
			slog.Warn("z2m: ignore unsupported device filter", "device", addr, "err", err)
		}
		options[addr] = opts
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.defaultOptions = defaults
	r.options = options
	for addr, d := range r.devices {
		r.schemas[addr] = NewStateSchema(d.Definition, r.deviceOptions(addr))
	}
}

func (r *registry) setBridgeOnline(online bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bridgeOnline = online
}

// isBridgeOnline reports whether the bridge last announced itself online.
func (r *registry) isBridgeOnline() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bridgeOnline
}

// snapshot returns every known device with its latest state, sorted by
// friendly name (then IEEE address).
func (r *registry) snapshot() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Device, 0, len(r.devices))
	for addr, d := range r.devices {
		out = append(out, Device{
			Spec:    d,
			State:   r.states[addr],
			Schema:  r.schemas[addr],
			Options: r.deviceOptions(addr),
		})
	}
	slices.SortFunc(out, func(a, b Device) int {
		return cmp.Or(
			cmp.Compare(a.Spec.FriendlyName, b.Spec.FriendlyName),
			cmp.Compare(a.Spec.IEEEAddress, b.Spec.IEEEAddress),
		)
	})
	return out
}

// device returns a device by friendly name or IEEE address, or nil.
func (r *registry) device(devID string) *Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.deviceLocked(r.addr(devID))
}

// deviceLocked returns the device with IEEE address addr, or nil when the
// roster doesn't list it. The caller holds mu.
func (r *registry) deviceLocked(addr string) *Device {
	dev, ok := r.devices[addr]
	if !ok {
		return nil
	}
	return &Device{
		Spec:    dev,
		State:   r.states[addr],
		Schema:  r.schemas[addr],
		Options: r.deviceOptions(addr),
	}
}
