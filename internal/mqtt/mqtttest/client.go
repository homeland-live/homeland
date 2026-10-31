// Package mqtttest provides MQTT test doubles: Client, a broker-less fake
// client, and Broker, a minimal loopback broker for a real paho client. Import
// it from tests only.
package mqtttest

import (
	"errors"
	"slices"
	"sync"

	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/mqtt"
)

// BaseTopic is the base topic Client reports.
const BaseTopic = "zigbee2mqtt"

// Client is a fake MQTT client with no broker: it connects and subscribes
// without doing anything, records what it publishes, and its health follows
// Connected. No message is ever delivered; feed messages to the consumer
// directly instead. Safe for concurrent use.
type Client struct {
	Connected bool

	mu        sync.Mutex
	published []Message
}

// Message is a message published through Client, with its topic relative to
// BaseTopic, as passed to Publish.
type Message struct {
	Topic   string
	Payload string
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

// Publish records the message.
func (c *Client) Publish(topic string, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published = append(c.published, Message{Topic: topic, Payload: string(payload)})
	return nil
}

// Published returns every message published so far, in order.
func (c *Client) Published() []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.published)
}

// HealthCheck reports the connection as up when Connected is set.
func (c *Client) HealthCheck(name string) healthcheck.HealthCheck {
	if !c.Connected {
		return healthcheck.New(name, errors.New("mqtt: connection is down"))
	}
	return healthcheck.New(name, nil)
}
