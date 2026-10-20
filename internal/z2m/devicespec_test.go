package z2m_test

import (
	"encoding/json"
	"testing"

	"github.com/avakarev/go-util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/z2m"
)

// TestDeviceExposesParsing verifies a device definition's exposes, including
// nested composite features, are parsed and retained.
func TestDeviceExposesParsing(t *testing.T) {
	payload := []byte(`{
		"friendly_name": "switch-1",
		"definition": {
			"model": "TS0001",
			"vendor": "Tuya",
			"exposes": [
				{
					"type": "switch",
					"features": [
						{
							"access": 7,
							"description": "On/off state of the switch",
							"label": "State",
							"name": "state",
							"property": "state",
							"type": "binary",
							"value_off": "OFF",
							"value_on": "ON",
							"value_toggle": "TOGGLE"
						}
					]
				},
				{
					"access": 1,
					"category": "diagnostic",
					"description": "Link quality (signal strength)",
					"label": "Linkquality",
					"name": "linkquality",
					"property": "linkquality",
					"type": "numeric",
					"unit": "lqi",
					"value_max": 255,
					"value_min": 0
				}
			]
		}
	}`)

	var d z2m.DeviceSpec
	require.NoError(t, json.Unmarshal(payload, &d))
	require.NotNil(t, d.Definition)
	require.Len(t, d.Definition.Exposes, 2)

	sw := d.Definition.Exposes[0]
	assert.Equal(t, "switch", sw.Type)
	require.Len(t, sw.Features, 1)
	state := sw.Features[0]
	assert.Equal(t, "state", state.Name)
	assert.Equal(t, 7, state.Access)
	assert.Equal(t, "On/off state of the switch", state.Description)
	assert.Equal(t, "ON", state.ValueOn)
	assert.Equal(t, "OFF", state.ValueOff)
	assert.Equal(t, "TOGGLE", state.ValueToggle)

	lqi := d.Definition.Exposes[1]
	assert.Equal(t, "diagnostic", lqi.Category)
	assert.Equal(t, "lqi", lqi.Unit)
	assert.Equal(t, "Link quality (signal strength)", lqi.Description)
	require.NotNil(t, lqi.ValueMin)
	require.NotNil(t, lqi.ValueMax)
	assert.InDelta(t, 0.0, *lqi.ValueMin, 0)
	assert.InDelta(t, 255.0, *lqi.ValueMax, 0)
}

// walkExposes visits an expose and all of its nested features / list element
// schema, invoking fn on each.
func walkExposes(exposes []z2m.Expose, fn func(z2m.Expose)) {
	for _, e := range exposes {
		fn(e)
		walkExposes(e.Features, fn)
		if e.ItemType != nil {
			walkExposes([]z2m.Expose{*e.ItemType}, fn)
		}
	}
}

// loadFixtureDevices parses the real bridge/devices payload captured from a
// live Zigbee2MQTT instance (test/fixtures/devices.json).
func loadFixtureDevices(t *testing.T) []z2m.DeviceSpec {
	t.Helper()
	var devices []z2m.DeviceSpec
	require.NoError(t, json.Unmarshal(testutil.FixtureBytes(t, "devices.json"), &devices), "unmarshal fixture")
	require.NotEmpty(t, devices, "fixture has no devices")
	return devices
}

func lookupFixtureDevice(t *testing.T, id string) *z2m.DeviceSpec {
	devices := loadFixtureDevices(t)
	for i := range devices {
		if devices[i].FriendlyName == id || devices[i].IEEEAddress == id {
			return &devices[i]
		}
	}
	t.Fatalf("there is no device with friendly-name/ieee-addr %s", id)
	return nil
}

// TestFixtureExposesCovered verifies that every expose type present in the
// real device fixture is one the Expose model understands, and that the
// type-specific fields those exposes carry (numeric bounds/step/presets, enum
// values, binary on/off, composite features, list item schema) are actually
// captured after unmarshalling rather than silently dropped.
func TestFixtureExposesCovered(t *testing.T) {
	devices := loadFixtureDevices(t)

	// The expose types we model. Any type in the fixture outside this set is
	// a coverage gap that should fail the test.
	known := map[string]bool{
		"binary":    true,
		"numeric":   true,
		"enum":      true,
		"composite": true,
		"list":      true,
		"switch":    true, // composite grouping
		"light":     true, // composite grouping
	}

	seenTypes := map[string]bool{}
	var checkedStep, checkedPresets, checkedValues, checkedBinary bool

	for _, d := range devices {
		if d.Definition == nil {
			continue
		}
		walkExposes(d.Definition.Exposes, func(e z2m.Expose) {
			assert.NotEmptyf(t, e.Type, "device %q has an expose with empty type", d.FriendlyName)
			if e.Type == "" {
				return
			}
			seenTypes[e.Type] = true
			assert.Truef(t, known[e.Type], "device %q exposes unmodeled type %q", d.FriendlyName, e.Type)

			switch e.Type {
			case "numeric":
				// value_step / presets appear on some numeric exposes; make
				// sure that when they do, we captured them.
				if e.ValueStep != nil {
					checkedStep = true
				}
				if len(e.Presets) > 0 {
					checkedPresets = true
					for _, p := range e.Presets {
						assert.NotEmptyf(t, p.Name, "device %q preset not fully parsed: %+v", d.FriendlyName, p)
						assert.NotNilf(t, p.Value, "device %q preset not fully parsed: %+v", d.FriendlyName, p)
					}
				}
			case "enum":
				if len(e.Values) > 0 {
					checkedValues = true
				}
			case "binary":
				// value_on/value_off are either strings or bools; both must
				// survive into the any fields.
				if e.ValueOn != nil && e.ValueOff != nil {
					checkedBinary = true
				}
			}
		})
	}

	// The fixture is a real capture; assert it actually exercised each field
	// so this test fails loudly if the fixture is replaced with thinner data.
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"value_step", checkedStep},
		{"presets", checkedPresets},
		{"enum values", checkedValues},
		{"binary value_on/off", checkedBinary},
	} {
		assert.Truef(t, tc.ok, "fixture never exercised %s; field coverage unverified", tc.name)
	}

	// These are the expose types the fixture's device exposes actually use.
	// "list" is only used by device options (settable config), not by exposes
	// (published state), so it is covered separately in TestExposeListItemType.
	for _, want := range []string{"binary", "numeric", "enum", "composite", "light", "switch"} {
		assert.Truef(t, seenTypes[want], "fixture missing expected expose type %q", want)
	}
}

// TestExposeListItemType verifies a "list" expose's element schema is parsed
// into ItemType. z2m uses this shape for list-valued options such as an
// occupancy "no_occupancy_since" timer list.
func TestExposeListItemType(t *testing.T) {
	payload := []byte(`{
		"type": "list",
		"name": "no_occupancy_since",
		"property": "no_occupancy_since",
		"item_type": {
			"type": "numeric",
			"name": "time",
			"label": "Time",
			"access": 3
		}
	}`)

	var e z2m.Expose
	require.NoError(t, json.Unmarshal(payload, &e))
	assert.Equal(t, "list", e.Type)
	require.NotNil(t, e.ItemType)
	assert.Equal(t, "numeric", e.ItemType.Type)
	assert.Equal(t, "time", e.ItemType.Name)
	assert.Equal(t, 3, e.ItemType.Access)
}
