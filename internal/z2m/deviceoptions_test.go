package z2m_test

import (
	"log/slog"
	"regexp/syntax"
	"testing"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/z2m"
)

// presenceRoster lists a presence sensor, "hall" (0x05), whose primary props
// are presence and illuminance.
var presenceRoster = []byte(`[{"ieee_address":"0x05","friendly_name":"hall","definition":{"exposes":[
	{"type":"binary","name":"presence","property":"presence","access":1},
	{"type":"numeric","name":"illuminance","property":"illuminance","access":1},
	{"type":"numeric","name":"battery","property":"battery","access":1,"category":"diagnostic"}
]}}]`)

// bridgeInfo returns a bridge/info payload with the given config.
func bridgeInfo(config string) []byte {
	return []byte(`{"version":"2.6.0","config":` + config + `}`)
}

// TestFilteredAttributeIsNotRequired covers a device whose z2m options filter
// a primary prop out of every publish: the prop leaves the schema, so states
// without it are events. bridge/info and bridge/devices are both retained and
// arrive in either order.
func TestFilteredAttributeIsNotRequired(t *testing.T) {
	type message struct {
		topic   string
		payload []byte
	}
	info := message{"bridge/info", bridgeInfo(`{"devices":{"0x05":{"friendly_name":"hall","filtered_attributes":["illuminance"]}}}`)}
	devices := message{"bridge/devices", presenceRoster}

	for name, msgs := range map[string][]message{
		"info first":    {info, devices},
		"devices first": {devices, info},
	} {
		t.Run(name, func(t *testing.T) {
			c, calls := newCountingClient(t)
			for _, m := range msgs {
				c.Handle(m.topic, m.payload)
			}

			schema := c.Device("hall").Schema
			require.NotNil(t, schema)
			assert.Equal(t, map[string]string{"presence": "binary"}, schema.PrimaryProps)

			c.Handle("hall", []byte(`{"presence":false,"battery":90}`))
			c.Handle("hall", []byte(`{"presence":true,"battery":90}`))
			assert.Equal(t, 1, *calls)
		})
	}
}

// TestDeviceOptionsChangeRebuildsSchema covers z2m republishing bridge/info
// after its options change: the schema follows the latest one, including a
// filter being removed.
func TestDeviceOptionsChangeRebuildsSchema(t *testing.T) {
	c, calls := newCountingClient(t)
	c.Handle("bridge/devices", presenceRoster)
	c.Handle("hall", []byte(`{"presence":false}`))

	c.Handle("hall", []byte(`{"presence":true}`))
	require.Zero(t, *calls, "illuminance is required before it is filtered")

	c.Handle("bridge/info", bridgeInfo(`{"devices":{"0x05":{"filtered_attributes":["illuminance"]}}}`))
	c.Handle("hall", []byte(`{"presence":true}`))
	assert.Equal(t, 1, *calls)

	c.Handle("bridge/info", bridgeInfo(`{"devices":{"0x05":{}}}`))
	assert.Contains(t, c.Device("hall").Schema.PrimaryProps, "illuminance", "the filter is gone")
}

// TestDeviceOptionsMergeLikeZ2M covers device_options: its list applies to
// every device without its own, including one bridge/info doesn't list, and a
// device's own list, even an empty one, replaces it.
func TestDeviceOptionsMergeLikeZ2M(t *testing.T) {
	c, _ := newCountingClient(t)
	exposes := `{"exposes":[
		{"type":"numeric","name":"illuminance","property":"illuminance","access":1},
		{"type":"binary","name":"presence","property":"presence","access":1}
	]}`
	c.Handle("bridge/devices", []byte(`[
		{"ieee_address":"0x01","friendly_name":"a","definition":`+exposes+`},
		{"ieee_address":"0x02","friendly_name":"b","definition":`+exposes+`},
		{"ieee_address":"0x03","friendly_name":"c","definition":`+exposes+`},
		{"ieee_address":"0x04","friendly_name":"d","definition":`+exposes+`}
	]`))

	c.Handle("bridge/info", bridgeInfo(`{
		"device_options":{"filtered_attributes":["illuminance"]},
		"devices":{
			"0x01":{"friendly_name":"a"},
			"0x02":{"filtered_attributes":["presence"]},
			"0x03":{"filtered_attributes":[]}
		}
	}`))

	assert.Equal(t, map[string]string{"presence": "binary"}, c.Device("a").Schema.PrimaryProps, "the default applies")
	assert.Equal(t, map[string]string{"illuminance": "numeric"}, c.Device("b").Schema.PrimaryProps, "its own list replaces the default")
	assert.Len(t, c.Device("c").Schema.PrimaryProps, 2, "an empty list replaces the default")
	assert.Equal(t, map[string]string{"presence": "binary"}, c.Device("d").Schema.PrimaryProps, "an unlisted device gets the default")
}

// TestUnsupportedFilterPatternIsLogged covers a JS-only pattern Go can't
// compile: it is logged once per bridge/info, not on every schema rebuild,
// and filters nothing while the device's other patterns still apply.
func TestUnsupportedFilterPatternIsLogged(t *testing.T) {
	logs := slogtest.Capture(t, slog.LevelWarn)
	c, _ := newCountingClient(t)
	c.Handle("bridge/devices", presenceRoster)

	c.Handle("bridge/info", bridgeInfo(`{"devices":{"0x05":{"filtered_attributes":["(?!presence).*","battery"]}}}`))
	c.Handle("bridge/devices", presenceRoster)

	schema := c.Device("hall").Schema
	assert.Len(t, schema.PrimaryProps, 2)
	assert.Empty(t, schema.SecondaryProps)
	assert.Equal(t, 1, logs.Count("z2m: ignore unsupported device filter"))
	assert.Contains(t, logs.String(), "device=0x05")
}

// TestNewAttrFilterReportsEveryUnsupportedPattern covers NewAttrFilter's
// error: it names each pattern Go's regexp can't compile, not only the first,
// and the filter keeps the patterns that compiled.
func TestNewAttrFilterReportsEveryUnsupportedPattern(t *testing.T) {
	filter, err := z2m.NewAttrFilter([]string{"(?!a)", "battery", "(?<=b)"})

	require.Error(t, err)
	var syntaxErr *syntax.Error
	require.ErrorAs(t, err, &syntaxErr, "the regexp errors stay reachable")
	assert.Contains(t, err.Error(), `"(?!a)"`)
	assert.Contains(t, err.Error(), `"(?<=b)"`)
	assert.NotContains(t, err.Error(), `"battery"`)
	assert.True(t, filter.Match("battery"))
}

// TestBridgeInfoKeepsConfig covers Info: it holds the device options as z2m
// published them.
func TestBridgeInfoKeepsConfig(t *testing.T) {
	c, _ := newCountingClient(t)

	c.Handle("bridge/info", bridgeInfo(`{
		"device_options":{"filtered_attributes":["illuminance"]},
		"devices":{"0x01":{"friendly_name":"a"},"0x02":{"filtered_attributes":["presence"]}}
	}`))

	info := c.Info()
	require.NotNil(t, info)
	assert.Equal(t, z2m.BridgeConfig{
		DeviceOptions: z2m.DeviceOptions{FilteredAttributes: []string{"illuminance"}},
		Devices: map[string]z2m.DeviceOptions{
			"0x01": {},
			"0x02": {FilteredAttributes: []string{"presence"}},
		},
	}, info.Config)
}

// TestBridgeInfoWithoutConfig covers a bridge/info without config: nothing is
// filtered and the info is still recorded.
func TestBridgeInfoWithoutConfig(t *testing.T) {
	c, _ := newCountingClient(t)
	c.Handle("bridge/devices", presenceRoster)

	c.Handle("bridge/info", []byte(`{"version":"2.6.0"}`))

	require.NotNil(t, c.Info())
	assert.Equal(t, "2.6.0", c.Info().Version)
	assert.Len(t, c.Device("hall").Schema.PrimaryProps, 2)
}

// TestDeviceSchemaIsNotMutatedByOptions covers a device read before
// bridge/info: new options swap its schema instead of editing the one a
// caller already holds.
func TestDeviceSchemaIsNotMutatedByOptions(t *testing.T) {
	c, _ := newCountingClient(t)
	c.Handle("bridge/devices", presenceRoster)
	before := c.Device("hall").Schema

	c.Handle("bridge/info", bridgeInfo(`{"devices":{"0x05":{"filtered_attributes":["illuminance"]}}}`))

	assert.Len(t, before.PrimaryProps, 2)
	assert.Equal(t, map[string]string{"presence": "binary"}, c.Device("hall").Schema.PrimaryProps)
}
