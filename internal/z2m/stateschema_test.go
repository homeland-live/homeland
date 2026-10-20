package z2m_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
	"github.com/homeland-live/homeland/internal/z2m"
)

// A device's StateSchema is derived from its definition when the bridge/devices
// roster arrives and exposed on Device.Schema. buildSchema unmarshals a roster
// of one device, feeds it through the client, and returns the device's computed
// schema.
func buildSchema(t *testing.T, exposesJSON string) *z2m.StateSchema {
	t.Helper()
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	roster := `[{"ieee_address":"0x01","friendly_name":"dev","definition":{"exposes":` + exposesJSON + `}}]`
	c.Handle("bridge/devices", []byte(roster))

	d := c.Device("0x01")
	require.NotNil(t, d, "device should be known after roster")
	return d.Schema
}

// TestStateSchemaPartitionsByCategory verifies published properties with no
// expose category go to PrimaryProps and those with any non-empty category go
// to SecondaryProps.
func TestStateSchemaPartitionsByCategory(t *testing.T) {
	schema := buildSchema(t, `[
		{"type":"binary","name":"state","property":"state","access":7},
		{"type":"numeric","name":"brightness","property":"brightness","access":7},
		{"type":"numeric","name":"linkquality","property":"linkquality","access":1,"category":"diagnostic"},
		{"type":"numeric","name":"battery","property":"battery","access":1,"category":"diagnostic"},
		{"type":"enum","name":"power_on_behavior","property":"power_on_behavior","access":7,"category":"config"}
	]`)

	require.NotNil(t, schema)
	assert.Equal(t, map[string]string{"state": "binary", "brightness": "numeric"}, schema.PrimaryProps)
	assert.Equal(t, map[string]string{
		"linkquality":       "numeric",
		"battery":           "numeric",
		"power_on_behavior": "enum",
	}, schema.SecondaryProps)
}

// TestStateSchemaOnlyPublished verifies properties that are not published
// in state (access bit 1 unset, e.g. set-only or get-only) are excluded from
// both buckets.
func TestStateSchemaOnlyPublished(t *testing.T) {
	schema := buildSchema(t, `[
		{"type":"binary","name":"state","property":"state","access":7},
		{"type":"enum","name":"effect","property":"effect","access":2},
		{"type":"numeric","name":"identify","property":"identify","access":4}
	]`)

	require.NotNil(t, schema)
	assert.Equal(t, map[string]string{"state": "binary"}, schema.PrimaryProps)
	// The unpublished exposes (effect, identify) are excluded entirely, so
	// nothing lands in the secondary bucket.
	assert.Empty(t, schema.SecondaryProps)
}

// TestStateSchemaWalksGroupingFeatures verifies the features of a grouping
// expose with no property of its own (light, switch) surface as top-level
// state keys, and that a property is recorded once, under the first expose
// that declares it (dedup across overlapping groupings).
func TestStateSchemaWalksGroupingFeatures(t *testing.T) {
	schema := buildSchema(t, `[
		{"type":"light","features":[
			{"type":"binary","name":"state","property":"state","access":7},
			{"type":"numeric","name":"brightness","property":"brightness","access":7}
		]},
		{"type":"switch","features":[
			{"type":"binary","name":"state","property":"state","access":7}
		]},
		{"type":"numeric","name":"device_temperature","property":"device_temperature","access":1,"category":"diagnostic"}
	]`)

	require.NotNil(t, schema)
	assert.Equal(t, map[string]string{"state": "binary", "brightness": "numeric"}, schema.PrimaryProps)
	assert.Equal(t, map[string]string{"device_temperature": "numeric"}, schema.SecondaryProps)
}

// TestNewStateSchemaNestedCompositeNotFlattened verifies a composite expose
// that carries its own property (e.g. level_config, color) is recorded as a
// single top-level state key, and its nested features (current_level_startup,
// x, y) are NOT flattened into top-level props: z2m publishes them nested
// under the composite's property, so they never appear as top-level keys.
func TestStateSchemaNestedCompositeNotFlattened(t *testing.T) {
	schema := buildSchema(t, `[
		{"type":"light","features":[
			{"type":"binary","name":"state","property":"state","access":7},
			{"type":"composite","name":"color_xy","property":"color","access":7,"features":[
				{"type":"numeric","name":"x","property":"x","access":7},
				{"type":"numeric","name":"y","property":"y","access":7}
			]},
			{"type":"composite","name":"level_config","property":"level_config","access":7,"category":"config","features":[
				{"type":"numeric","name":"current_level_startup","property":"current_level_startup","access":7}
			]}
		]}
	]`)

	require.NotNil(t, schema)
	// state and color are top-level published keys (color is a nested object);
	// x, y, and current_level_startup live inside their parents, not at top
	// level, so they are absent.
	assert.Equal(t, map[string]string{"state": "binary", "color": "composite"}, schema.PrimaryProps)
	// level_config is config-category, so it is secondary (one key, not its
	// nested current_level_startup).
	assert.Equal(t, map[string]string{"level_config": "composite"}, schema.SecondaryProps)
}

// TestStateSchemaActionAlwaysPrimary verifies the "action" property is
// primary even when z2m tags it with a category.
func TestStateSchemaActionAlwaysPrimary(t *testing.T) {
	schema := buildSchema(t, `[
		{"type":"enum","name":"action","property":"action","access":1,"category":"diagnostic"},
		{"type":"numeric","name":"battery","property":"battery","access":1,"category":"diagnostic"}
	]`)

	require.NotNil(t, schema)
	assert.Equal(t, map[string]string{"action": "enum"}, schema.PrimaryProps)
	assert.Equal(t, map[string]string{"battery": "numeric"}, schema.SecondaryProps)
}

// TestStateSchemaLastSeenNotInjected verifies last_seen is not added to the
// schema: it is housekeeping z2m injects outside the device's exposes, so a
// device that does not expose it has no last_seen prop. A device that does
// expose it is treated like any other published prop.
func TestStateSchemaLastSeenNotInjected(t *testing.T) {
	// Not exposed: not present in either bucket.
	schema := buildSchema(t, `[
		{"type":"binary","name":"state","property":"state","access":7}
	]`)
	require.NotNil(t, schema)
	assert.Empty(t, schema.SecondaryProps)

	// Exposed explicitly: captured like any other published prop, keeping the
	// expose type from its declaration.
	schema = buildSchema(t, `[
		{"type":"binary","name":"state","property":"state","access":7},
		{"type":"numeric","name":"last_seen","property":"last_seen","access":1,"category":"diagnostic"}
	]`)
	require.NotNil(t, schema)
	assert.Equal(t, map[string]string{"last_seen": "numeric"}, schema.SecondaryProps)
}

// TestStateSchemaNilDefinition verifies a device without a definition
// yields a nil schema rather than panicking.
func TestStateSchemaNilDefinition(t *testing.T) {
	c, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	c.Handle("bridge/devices", []byte(`[{"ieee_address":"0x01","friendly_name":"dev"}]`))

	d := c.Device("0x01")
	require.NotNil(t, d)
	assert.Nil(t, d.Schema)
}

// TestStateSchemaJSONTags verifies the schema serializes under the camelCase
// keys the UI consumes.
func TestStateSchemaJSONTags(t *testing.T) {
	b, err := json.Marshal(z2m.StateSchema{
		PrimaryProps:   map[string]string{"state": "binary"},
		SecondaryProps: map[string]string{"battery": "numeric"},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"primaryProps":{"state":"binary"},"secondaryProps":{"battery":"numeric"}}`, string(b))
}

func TestNewStateSchema(t *testing.T) {
	assert.Equal(t,
		&z2m.StateSchema{
			PrimaryProps: map[string]string{
				"illuminance": "numeric",
				"occupancy":   "binary",
			},
			SecondaryProps: map[string]string{
				"battery":            "numeric",
				"device_temperature": "numeric",
				"linkquality":        "numeric",
				"power_outage_count": "numeric",
				"voltage":            "numeric",
			},
		},
		z2m.NewStateSchema(lookupFixtureDevice(t, "motion-sensor").Definition),
	)
	assert.Equal(t,
		&z2m.StateSchema{
			PrimaryProps: map[string]string{
				"brightness":   "numeric",
				"level_config": "composite",
				"state":        "binary",
			},
			SecondaryProps: map[string]string{
				"linkquality":       "numeric",
				"power_on_behavior": "enum",
			},
		},
		z2m.NewStateSchema(lookupFixtureDevice(t, "mirror-light").Definition),
	)
}
