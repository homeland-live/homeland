package z2m_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
	"github.com/homeland-live/homeland/internal/z2m"
)

// newPublishingClient returns a client over a fake MQTT client that records
// publishes.
func newPublishingClient(t *testing.T) (*z2m.Client, *mqtttest.Client) {
	t.Helper()
	m := &mqtttest.Client{Connected: true}
	c, err := z2m.NewClient(nil, z2m.WithMQTT(m))
	require.NoError(t, err)
	return c, m
}

func TestPublishSendsJSONToSetTopic(t *testing.T) {
	c, m := newPublishingClient(t)

	require.NoError(t, c.Publish("room/lamp", map[string]any{"state": "ON", "brightness": 200}))

	assert.Equal(t, []mqtttest.Message{
		{Topic: "room/lamp/set", Payload: `{"brightness":200,"state":"ON"}`},
	}, m.Published())
}

func TestPublishRejectsUnmarshalablePayload(t *testing.T) {
	c, m := newPublishingClient(t)

	require.ErrorContains(t, c.Publish("lamp", map[string]any{"bad": make(chan int)}), "z2m: marshal publish payload")
	assert.Empty(t, m.Published())
}

func TestPublishPropSendsScalarToPropTopic(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   any
		payload string
	}{
		{"string verbatim", "ON", `ON`},
		{"number as JSON", 42, `42`},
		{"float as JSON", 0.5, `0.5`},
		{"bool as JSON", true, `true`},
		{"object as JSON", map[string]any{"x": 0.3}, `{"x":0.3}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, m := newPublishingClient(t)

			require.NoError(t, c.PublishProp("lamp", "state", tc.value))

			assert.Equal(t, []mqtttest.Message{{Topic: "lamp/set/state", Payload: tc.payload}}, m.Published())
		})
	}
}

func TestPublishPropRejectsUnmarshalableValue(t *testing.T) {
	// Like Publish, it sends nothing rather than a %v rendering such as "NaN"
	// or a pointer address.
	for name, value := range map[string]any{"NaN": math.NaN(), "channel": make(chan int)} {
		t.Run(name, func(t *testing.T) {
			c, m := newPublishingClient(t)

			require.ErrorContains(t, c.PublishProp("lamp", "state", value), "z2m: marshal publish value")
			assert.Empty(t, m.Published())
		})
	}
}
