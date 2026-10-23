package z2m_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/z2m"
)

// TestDeviceStateValidate verifies DeviceState.Validate requires every primary
// prop to be present in Raw (action only when it is the sole one): all present
// validates (nil error), any missing errors and names the missing props, and a
// nil schema or one without primary props always validates regardless of
// state.
func TestDeviceStateValidate(t *testing.T) {
	mk := func(keys ...string) *z2m.DeviceState {
		raw := map[string]json.RawMessage{}
		for _, k := range keys {
			raw[k] = json.RawMessage(`1`)
		}
		return &z2m.DeviceState{Raw: raw}
	}
	schema := func(primary ...string) *z2m.StateSchema {
		props := make(map[string]string, len(primary))
		for _, p := range primary {
			props[p] = ""
		}
		return &z2m.StateSchema{PrimaryProps: props}
	}

	t.Run("all primary props present validates", func(t *testing.T) {
		assert.NoError(t, mk("state", "brightness").Validate(schema("state", "brightness")))
	})

	t.Run("secondary-only extras do not affect validity", func(t *testing.T) {
		assert.NoError(t, mk("state", "linkquality", "battery").Validate(schema("state")))
	})

	t.Run("a missing primary prop errors and names it", func(t *testing.T) {
		err := mk("state").Validate(schema("state", "brightness"))
		assert.EqualError(t, err, "missing primary props: brightness")
	})

	t.Run("multiple missing primary props are named sorted", func(t *testing.T) {
		err := mk().Validate(schema("state", "brightness"))
		assert.EqualError(t, err, "missing primary props: brightness, state")
	})

	t.Run("no primary props always validates", func(t *testing.T) {
		assert.NoError(t, mk("anything").Validate(schema()))
		assert.NoError(t, mk().Validate(schema()))
	})

	t.Run("nil schema validates", func(t *testing.T) {
		// A device without a definition has no primary props to require.
		assert.NoError(t, mk("state").Validate(nil))
		assert.NoError(t, mk().Validate(nil))
	})

	t.Run("action is not required next to other primary props", func(t *testing.T) {
		// A switch with a relay: a toggle reports state_left without action.
		require.NoError(t, mk("state_left").Validate(schema("state_left", "action")))
		assert.EqualError(t, mk("action").Validate(schema("state_left", "action")),
			"missing primary props: state_left")
	})

	t.Run("action is required when it is the only primary prop", func(t *testing.T) {
		// A remote: only presses are events.
		require.NoError(t, mk("action", "battery").Validate(schema("action")))
		assert.EqualError(t, mk("battery").Validate(schema("action")), "missing primary props: action")
	})

	t.Run("nil/empty state with required primary props errors", func(t *testing.T) {
		var nilState *z2m.DeviceState
		require.Error(t, nilState.Validate(schema("state")))
		require.Error(t, (&z2m.DeviceState{}).Validate(schema("state")))
	})

	t.Run("nil state with no primary props validates", func(t *testing.T) {
		var nilState *z2m.DeviceState
		require.NoError(t, nilState.Validate(schema()))
		require.NoError(t, nilState.Validate(nil))
	})
}

// TestDeviceStateIsDuplicate verifies IsDuplicate ignores last_seen,
// linkquality and action, never treats a press as a duplicate, and compares
// other values byte-for-byte, so equal values serialized differently are
// distinct.
func TestDeviceStateIsDuplicate(t *testing.T) {
	mk := func(raw map[string]string) *z2m.DeviceState {
		m := make(map[string]json.RawMessage, len(raw))
		for k, v := range raw {
			m[k] = json.RawMessage(v)
		}
		return &z2m.DeviceState{Raw: m}
	}

	t.Run("identical states are duplicates", func(t *testing.T) {
		a := mk(map[string]string{"state": `"ON"`, "battery": `97`})
		b := mk(map[string]string{"state": `"ON"`, "battery": `97`})
		assert.True(t, a.IsDuplicate(b))
	})

	t.Run("only last_seen differs is still a duplicate", func(t *testing.T) {
		a := mk(map[string]string{"state": `"ON"`, "last_seen": `"2026-10-06T10:00:00Z"`})
		b := mk(map[string]string{"state": `"ON"`, "last_seen": `"2026-10-06T10:05:00Z"`})
		assert.True(t, a.IsDuplicate(b))
	})

	t.Run("a differing value is not a duplicate", func(t *testing.T) {
		a := mk(map[string]string{"state": `"ON"`})
		b := mk(map[string]string{"state": `"OFF"`})
		assert.False(t, a.IsDuplicate(b))
	})

	t.Run("different number of keys is not a duplicate", func(t *testing.T) {
		a := mk(map[string]string{"state": `"ON"`})
		b := mk(map[string]string{"state": `"ON"`, "battery": `97`})
		assert.False(t, a.IsDuplicate(b))
	})

	t.Run("same key count but different keys is not a duplicate, either way", func(t *testing.T) {
		a := mk(map[string]string{"state": `"ON"`, "battery": `97`})
		b := mk(map[string]string{"state": `"ON"`, "linkquality": `97`})
		assert.False(t, a.IsDuplicate(b))
		assert.False(t, b.IsDuplicate(a))
	})

	t.Run("only linkquality differs or appears is still a duplicate", func(t *testing.T) {
		a := mk(map[string]string{"state": `"ON"`, "linkquality": `50`})
		b := mk(map[string]string{"state": `"ON"`, "linkquality": `90`})
		c := mk(map[string]string{"state": `"ON"`})
		assert.True(t, a.IsDuplicate(b))
		assert.True(t, a.IsDuplicate(c))
		assert.True(t, c.IsDuplicate(a))
	})

	t.Run("a state with action is never a duplicate", func(t *testing.T) {
		press := mk(map[string]string{"battery": `97`, "action": `"single"`})
		assert.False(t, press.IsDuplicate(mk(map[string]string{"battery": `97`, "action": `"single"`})))
		assert.False(t, press.IsDuplicate(mk(map[string]string{"battery": `97`})))
	})

	t.Run("the previous state's action is not compared", func(t *testing.T) {
		// z2m never caches action, so the state after a press lacks it.
		afterPress := mk(map[string]string{"battery": `97`})
		assert.True(t, afterPress.IsDuplicate(mk(map[string]string{"battery": `97`, "action": `"single"`})))
	})

	t.Run("empty states are duplicates", func(t *testing.T) {
		assert.True(t, mk(nil).IsDuplicate(mk(nil)))
	})

	t.Run("a nil state is not a duplicate", func(t *testing.T) {
		var none *z2m.DeviceState
		assert.False(t, mk(map[string]string{"state": `"ON"`}).IsDuplicate(nil))
		assert.False(t, none.IsDuplicate(mk(nil)))
	})

	// The following cases document that comparison is byte-for-byte on the raw
	// JSON, not a semantic JSON compare. Equal values serialized differently
	// are NOT treated as duplicates.
	t.Run("numeric formatting differences are not duplicates", func(t *testing.T) {
		a := mk(map[string]string{"battery": `97`})
		b := mk(map[string]string{"battery": `97.0`})
		assert.False(t, a.IsDuplicate(b))
	})

	t.Run("whitespace differences in nested objects are not duplicates", func(t *testing.T) {
		a := mk(map[string]string{"color": `{"x":0.5,"y":0.4}`})
		b := mk(map[string]string{"color": `{"x": 0.5, "y": 0.4}`})
		assert.False(t, a.IsDuplicate(b))
	})

	t.Run("object key order differences are not duplicates", func(t *testing.T) {
		a := mk(map[string]string{"color": `{"x":0.5,"y":0.4}`})
		b := mk(map[string]string{"color": `{"y":0.4,"x":0.5}`})
		assert.False(t, a.IsDuplicate(b))
	})
}

// TestNewDeviceState verifies NewDeviceState stamps each state with a fresh
// unique ID and a UTC CreatedAt, and keeps the raw props it is given.
func TestNewDeviceState(t *testing.T) {
	raw := map[string]json.RawMessage{"state": json.RawMessage(`"ON"`)}
	before := time.Now().UTC()
	s := z2m.NewDeviceState(raw)
	after := time.Now().UTC()

	require.NotNil(t, s)
	assert.Equal(t, raw, s.Raw)

	t.Run("stamps a non-empty ID", func(t *testing.T) {
		assert.NotEmpty(t, s.ID)
	})

	t.Run("stamps a UTC CreatedAt within the call window", func(t *testing.T) {
		assert.Equal(t, time.UTC, s.CreatedAt.Location())
		assert.False(t, s.CreatedAt.Before(before), "CreatedAt before the call")
		assert.False(t, s.CreatedAt.After(after), "CreatedAt after the call")
	})

	t.Run("mints a distinct ID per state", func(t *testing.T) {
		other := z2m.NewDeviceState(raw)
		assert.NotEqual(t, s.ID, other.ID)
	})
}

// TestDeviceStateLogValue verifies a state logs as a group of its decoded
// props, sorted by name, and that a nil state logs as nothing.
func TestDeviceStateLogValue(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))

	state := z2m.NewDeviceState(map[string]json.RawMessage{
		"state":   json.RawMessage(`"ON"`),
		"battery": json.RawMessage(`97`),
	})
	logger.Info("update", "state", state)
	assert.Equal(t, "level=INFO msg=update state.battery=97 state.state=ON\n", buf.String())

	buf.Reset()
	var none *z2m.DeviceState
	logger.Info("update", "state", none)
	assert.Equal(t, "level=INFO msg=update\n", buf.String(), "a nil state must log as an omitted empty group")
}
