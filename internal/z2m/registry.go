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
	// devices, schemas, states and filters are keyed by IEEE address.
	devices map[string]*DeviceSpec
	schemas map[string]*StateSchema
	states  map[string]*DeviceState
	// filters holds the devices bridge/info lists with filtered_attributes of
	// their own; every other device gets defaultFilter (device_options).
	filters       map[string]AttrFilter
	defaultFilter AttrFilter

	bridgeOnline bool
}

func newRegistry() *registry {
	return &registry{
		addresses: make(map[string]string),
		devices:   make(map[string]*DeviceSpec),
		schemas:   make(map[string]*StateSchema),
		states:    make(map[string]*DeviceState),
		filters:   make(map[string]AttrFilter),
	}
}

// attrFilter returns the filter of the device with IEEE address addr, nil
// before bridge/info arrives. The caller holds mu.
func (r *registry) attrFilter(addr string) AttrFilter {
	if filter, ok := r.filters[addr]; ok {
		return filter
	}
	return r.defaultFilter
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
		newSchemas[d.IEEEAddress] = NewStateSchema(d.Definition, r.attrFilter(d.IEEEAddress))
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
// address) and reports whether subscribers should be notified. A devID the
// roster can't resolve yet keys the state as given (see replaceDevices).
//
//   - The first state for a device is stored but does not propagate: it is
//     the initial sync-up, not an event.
//   - A state that fails Validate against the device's schema is neither
//     stored nor propagated.
//   - A state that IsDuplicate of the previous one is neither stored nor
//     propagated.
//
// Any other state is stored and propagates.
func (r *registry) setState(devID string, nextState *DeviceState) (propagate bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	addr := r.addr(devID)
	prevState, exists := r.states[addr]
	if !exists {
		r.states[addr] = nextState
		return false
	}

	if err := nextState.Validate(r.schemas[addr]); err != nil {
		slog.Debug("z2m: ignore invalid event",
			"device", devID,
			"state", nextState,
			"err", err,
		)
		return false
	}

	if nextState.IsDuplicate(prevState) {
		slog.Debug("z2m: ignore duplicate event",
			"device", devID,
			"state", nextState,
		)
		return false
	}

	r.states[addr] = nextState
	return true
}

// setOptions swaps in the filters from the latest bridge/info's config and
// rebuilds every known device's schema (replacing it, since snapshots share
// it), so a removed filter takes effect too. z2m merges device_options into a
// device's own options shallowly, so a device's own list, even an empty one,
// replaces the default. Unsupported patterns are logged and skipped.
func (r *registry) setOptions(cfg BridgeConfig) {
	defaultFilter, err := NewAttrFilter(cfg.DeviceOptions.FilteredAttributes)
	if err != nil {
		slog.Warn("z2m: ignore unsupported device_options filter", "err", err)
	}
	filters := make(map[string]AttrFilter, len(cfg.Devices))
	for addr, opts := range cfg.Devices {
		if opts.FilteredAttributes == nil {
			continue
		}
		filter, err := NewAttrFilter(opts.FilteredAttributes)
		if err != nil {
			slog.Warn("z2m: ignore unsupported device filter", "device", addr, "err", err)
		}
		filters[addr] = filter
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.defaultFilter = defaultFilter
	r.filters = filters
	for addr, d := range r.devices {
		r.schemas[addr] = NewStateSchema(d.Definition, r.attrFilter(addr))
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
			Spec:   d,
			State:  r.states[addr],
			Schema: r.schemas[addr],
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

	addr := r.addr(devID)
	dev, ok := r.devices[addr]
	if !ok {
		return nil
	}
	return &Device{Spec: dev, State: r.states[addr], Schema: r.schemas[addr]}
}
