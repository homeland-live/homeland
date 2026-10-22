package z2m

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DeviceState is a device's state, parsed from its "<friendly_name>" topic.
// Attributes vary per device, so the raw JSON is kept. All methods are safe on
// a nil receiver.
type DeviceState struct {
	// ID and CreatedAt (UTC) are set by NewDeviceState when the state is
	// parsed.
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`

	// Raw is the state object as reported by the device.
	Raw map[string]json.RawMessage `json:"-"`
}

// Has reports whether the state carries prop.
func (state *DeviceState) Has(prop string) bool {
	if state == nil || state.Raw == nil {
		return false
	}
	_, has := state.Raw[prop]
	return has
}

// Keys returns the names of every attribute present in the state, sorted
// alphabetically.
func (state *DeviceState) Keys() []string {
	if state == nil || state.Raw == nil {
		return nil
	}
	keys := make([]string, 0, len(state.Raw))
	for k := range state.Raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Value returns the decoded Go value for an attribute (string, float64, bool,
// or nested map/slice for object/array attributes), and whether it exists.
func (state *DeviceState) Value(name string) (any, bool) {
	if state == nil || state.Raw == nil {
		return nil, false
	}
	raw, ok := state.Raw[name]
	if !ok {
		return nil, false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, false
	}
	return v, true
}

// Attributes returns every attribute decoded into Go values, keyed by name.
func (state *DeviceState) Attributes() map[string]any {
	if state == nil || state.Raw == nil {
		return nil
	}
	out := make(map[string]any, len(state.Raw))
	for k := range state.Raw {
		if v, ok := state.Value(k); ok {
			out[k] = v
		}
	}
	return out
}

// LogValue implements slog.LogValuer: a state logs as a group of its decoded
// attributes, sorted by name. slog resolves it only when a record is emitted,
// so logging a state at a disabled level decodes nothing. A nil or empty state
// is an empty group, which handlers omit.
func (state *DeviceState) LogValue() slog.Value {
	keys := state.Keys()
	attrs := make([]slog.Attr, 0, len(keys))
	for _, k := range keys {
		v, _ := state.Value(k)
		attrs = append(attrs, slog.Any(k, v))
	}
	return slog.GroupValue(attrs...)
}

// IsDuplicate reports whether state repeats the previous state other: the same
// attributes with byte-identical values, ignoring last_seen, linkquality and
// action. z2m republishes a device's full cached state on every keep-alive,
// which produces such repeats. A state that carries action is never a
// duplicate, so a repeated button press still counts. It is false when either
// state is nil.
func (state *DeviceState) IsDuplicate(other *DeviceState) bool {
	if state == nil || other == nil || state.Has(propAction) {
		return false
	}
	return state.isSubsetOf(other) && other.isSubsetOf(state)
}

// isSubsetOf reports whether every attribute of state that IsDuplicate
// compares is in other with the same bytes. It is false when either state is
// nil.
func (state *DeviceState) isSubsetOf(other *DeviceState) bool {
	if state == nil || other == nil {
		return false
	}
	for k, v := range state.Raw {
		if k == "last_seen" || k == "linkquality" || k == propAction {
			continue
		}
		if w, ok := other.Raw[k]; !ok || !bytes.Equal(v, w) {
			return false
		}
	}
	return true
}

// Validate returns nil when the state carries every primary prop of schema,
// and otherwise an error naming the missing ones. action is momentary (z2m
// sends it only with the press and never caches it), so it is required only
// from a device whose sole primary prop it is, such as a remote: for that
// device, only presses are events. A nil schema (a device without a
// definition) or one without primary props has nothing to require, so any
// state validates.
func (state *DeviceState) Validate(schema *StateSchema) error {
	if schema == nil || len(schema.PrimaryProps) == 0 {
		return nil
	}
	_, hasAction := schema.PrimaryProps[propAction]
	actionOnly := hasAction && len(schema.PrimaryProps) == 1

	missing := make([]string, 0, len(schema.PrimaryProps))
	for prop := range schema.PrimaryProps {
		if prop == propAction && !actionOnly {
			continue
		}
		if !state.Has(prop) {
			missing = append(missing, prop)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("missing primary props: %s", strings.Join(missing, ", "))
}

// NewDeviceState returns a state for raw with a fresh ID and the current UTC
// time.
func NewDeviceState(raw map[string]json.RawMessage) *DeviceState {
	return &DeviceState{
		ID:        uuid.NewString(),
		CreatedAt: time.Now().UTC(),
		Raw:       raw,
	}
}
