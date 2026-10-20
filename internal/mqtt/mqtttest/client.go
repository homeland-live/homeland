// Package mqtttest provides MQTT test doubles: Client, a broker-less fake
// client, and Broker, a minimal loopback broker for a real paho client. Import
// it from tests only.
package mqtttest

import (
	"errors"

	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/mqtt"
)

// BaseTopic is the base topic Client reports.
const BaseTopic = "zigbee2mqtt"

// Client is a fake MQTT client with no broker: it connects, subscribes and
// publishes without doing anything, and its health follows Connected. No
// message is ever delivered; feed messages to the consumer directly instead.
type Client struct {
	Connected bool
}

// BaseTopic returns BaseTopic.
func (*Client) BaseTopic() string { return BaseTopic }

// OnConnect does nothing: Client never connects.
func (*Client) OnConnect(func()) {}

// Connect does nothing.
func (*Client) Connect() error { return nil }

// Disconnect does nothing.
func (*Client) Disconnect() {}

// Subscribe does nothing; no message is ever delivered.
func (*Client) Subscribe(string, mqtt.MessageHandler) error { return nil }

// Publish drops the message.
func (*Client) Publish(string, []byte) error { return nil }

// HealthCheck reports the connection as up when Connected is set.
func (c *Client) HealthCheck(name string) healthcheck.HealthCheck {
	if !c.Connected {
		return healthcheck.New(name, errors.New("mqtt: connection is down"))
	}
	return healthcheck.New(name, nil)
}
