package config

import (
	"github.com/avakarev/go-util/envutil"
)

// MQTTConn holds the connection settings for a single MQTT broker.
type MQTTConn struct {
	// Broker is the broker URL, e.g. "tcp://localhost:1883".
	Broker string
	// ClientID must differ between connections to the same broker: the
	// broker disconnects a client when another connects with its id. Load
	// rejects the same id on the same broker (see ConflictsWith).
	ClientID string
	// Username and Password are optional credentials.
	Username string
	Password string
	// BaseTopic, if set, prefixes every topic. The Zigbee2MQTT connection
	// requires one and defaults to DefaultZ2MBaseTopic; the external one
	// defaults to none.
	BaseTopic string
	// ReadOnly makes publishes log instead of send; subscriptions still work.
	ReadOnly bool
}

// ConflictsWith reports whether c and other would connect to the same broker
// with the same non-empty client id, so each would keep disconnecting the
// other. Brokers match by exact URL. It is false when either is nil.
func (c *MQTTConn) ConflictsWith(other *MQTTConn) bool {
	if c == nil || other == nil {
		return false
	}
	return c.Broker == other.Broker && c.ClientID != "" && c.ClientID == other.ClientID
}

// DefaultZ2MBaseTopic is Zigbee2MQTT's own default mqtt.base_topic.
const DefaultZ2MBaseTopic = "zigbee2mqtt"

// loadMQTTConn reads "<prefix>_BROKER", "<prefix>_CLIENT_ID" (default
// "homeland-dev"), "<prefix>_USERNAME", "<prefix>_PASSWORD",
// "<prefix>_BASE_TOPIC" (default baseTopic) and "<prefix>_READONLY". It
// returns nil, nil when the broker is unset.
func loadMQTTConn(prefix, baseTopic string) (*MQTTConn, error) {
	broker := envutil.StrDef(prefix+"_BROKER", "")
	if broker == "" {
		return nil, nil
	}
	readOnly, err := envutil.BoolDef(prefix+"_READONLY", false)
	if err != nil {
		return nil, err
	}
	return &MQTTConn{
		Broker:    broker,
		ClientID:  envutil.StrDef(prefix+"_CLIENT_ID", "homeland-dev"),
		Username:  envutil.StrDef(prefix+"_USERNAME", ""),
		Password:  envutil.StrDef(prefix+"_PASSWORD", ""),
		BaseTopic: envutil.StrDef(prefix+"_BASE_TOPIC", baseTopic),
		ReadOnly:  readOnly,
	}, nil
}
