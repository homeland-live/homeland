package mqtt_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/mqtt"
	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
)

func TestHealthCheckFollowsTheConnection(t *testing.T) {
	b := mqtttest.StartBroker(t, "127.0.0.1:0")
	c, err := mqtt.New(config.MQTTConn{Broker: b.URL(), ClientID: "test"})
	require.NoError(t, err)
	assert.False(t, c.HealthCheck("mqtt").IsHealthy, "not connected yet")

	connected := onConnect(c)
	require.NoError(t, c.Connect())
	t.Cleanup(c.Disconnect)
	waitFor(t, connected, "client did not connect")
	hc := c.HealthCheck("mqtt")
	assert.True(t, hc.IsHealthy)
	assert.Equal(t, "mqtt", hc.Name)

	// The broker goes away: paho starts reconnecting, which still counts as
	// connected for paho's IsConnected, but is an outage.
	b.Stop()
	assert.Eventually(t, func() bool { return !c.HealthCheck("mqtt").IsHealthy },
		2*time.Second, 10*time.Millisecond, "a reconnect in progress is an outage")
	assert.False(t, c.IsConnected())
}

func TestPublishFailsWithoutOpenConnection(t *testing.T) {
	c, err := mqtt.New(config.MQTTConn{Broker: "tcp://" + mqtttest.FreeAddr(t), BaseTopic: "zigbee2mqtt"})
	require.NoError(t, err)
	require.NoError(t, c.Connect())
	t.Cleanup(c.Disconnect)

	// paho would accept the message for later delivery and report success.
	err = c.Publish("lamp/set", []byte(`{"state":"ON"}`))
	require.ErrorContains(t, err, "connection is down")
}

func TestPublishPrefixesBaseTopic(t *testing.T) {
	b := mqtttest.StartBroker(t, "127.0.0.1:0")
	c := connectedClient(t, b, config.MQTTConn{BaseTopic: "zigbee2mqtt"})

	require.NoError(t, c.Publish("lamp/set", []byte(`{"state":"ON"}`)))
	flush(t, c)
	assert.Equal(t, []string{"zigbee2mqtt/lamp/set"}, b.Published())
}

func TestPublishReadOnlyDoesNotSend(t *testing.T) {
	b := mqtttest.StartBroker(t, "127.0.0.1:0")
	c := connectedClient(t, b, config.MQTTConn{ReadOnly: true})

	require.NoError(t, c.Publish("lamp/set", []byte(`{}`)))
	flush(t, c)
	assert.Empty(t, b.Published())
}

func TestSubscribeDeliversRelativeTopicAndRetainedFlag(t *testing.T) {
	b := mqtttest.StartBroker(t, "127.0.0.1:0")
	b.Retain("zigbee2mqtt/room/lamp", `{"state":"ON"}`)
	// "zigbee2mqtt/#" also matches the base topic itself.
	b.Retain("zigbee2mqtt", `online`)
	c := connectedClient(t, b, config.MQTTConn{BaseTopic: "zigbee2mqtt"})

	// message is a message as delivered to the subscriber.
	type message struct {
		topic    string
		payload  string
		retained bool
	}
	got := make(chan message, 8)
	require.NoError(t, c.Subscribe("#", func(topic string, payload []byte, retained bool) {
		got <- message{topic: topic, payload: string(payload), retained: retained}
	}))
	next := func() message {
		t.Helper()
		select {
		case m := <-got:
			return m
		case <-time.After(5 * time.Second):
			t.Fatal("no message delivered")
			return message{}
		}
	}

	// Stored copies sent for the subscription are flagged retained.
	assert.Equal(t, message{topic: "room/lamp", payload: `{"state":"ON"}`, retained: true}, next())
	assert.Equal(t, message{topic: "", payload: `online`, retained: true}, next())

	// A forwarded live publish is not.
	require.NoError(t, c.Publish("room/lamp", []byte(`{"state":"OFF"}`)))
	assert.Equal(t, message{topic: "room/lamp", payload: `{"state":"OFF"}`}, next())
}
