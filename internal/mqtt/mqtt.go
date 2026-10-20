// Package mqtt is a thin, application-agnostic MQTT client on top of paho.
package mqtt

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/healthcheck"
)

// MessageHandler receives a message delivered to a subscribed topic, with the
// topic relative to the connection's BaseTopic ("" for BaseTopic itself).
// retained is true when the broker sent its stored copy because of a
// (re)subscription, rather than forwarding a live publish; with a clean
// session that happens on every reconnect.
type MessageHandler func(topic string, payload []byte, retained bool)

const (
	// connectTimeout bounds each connection attempt (dial and handshake).
	connectTimeout = 10 * time.Second
	// defaultConnectRetryInterval is the wait between attempts until the
	// first connection succeeds.
	defaultConnectRetryInterval = 10 * time.Second
	// maxReconnectInterval caps paho's exponential backoff (from 1s) between
	// reconnect attempts after a connection is lost, so a client notices a
	// returning broker within a minute (paho's default cap is 10m).
	maxReconnectInterval = time.Minute
)

// Option configures a Client at construction time.
type Option func(*options)

type options struct {
	connectRetryInterval time.Duration
}

// WithConnectRetryInterval sets the wait between attempts until the first
// connection succeeds (default 10s).
func WithConnectRetryInterval(d time.Duration) Option {
	return func(o *options) {
		o.connectRetryInterval = d
	}
}

// Client is a generic MQTT client. It keeps retrying at a fixed interval until
// the first connection succeeds, reconnects with capped exponential backoff
// after that, never gives up before Disconnect, and uses a clean session.
type Client struct {
	cfg    config.MQTTConn
	paho   paho.Client
	onConn []func()
}

// New builds a client; call Connect to start it. It fails only on a missing
// or unparsable broker URL.
func New(cfg config.MQTTConn, opts ...Option) (*Client, error) {
	if cfg.Broker == "" {
		return nil, fmt.Errorf("mqtt: broker is required")
	}
	o := options{connectRetryInterval: defaultConnectRetryInterval}
	for _, opt := range opts {
		opt(&o)
	}

	c := &Client{cfg: cfg}

	po := paho.NewClientOptions().
		AddBroker(cfg.Broker).
		SetConnectTimeout(connectTimeout).
		SetConnectRetry(true).
		SetConnectRetryInterval(o.connectRetryInterval).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(maxReconnectInterval).
		SetCleanSession(true).
		SetOnConnectHandler(func(_ paho.Client) {
			slog.Info("mqtt: connected", "broker", cfg.Broker, "clientId", cfg.ClientID)
			for _, fn := range c.onConn {
				fn()
			}
		}).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			slog.Error("mqtt: connection lost", "err", err)
		}).
		SetConnectionNotificationHandler(func(_ paho.Client, n paho.ConnectionNotification) {
			// paho logs failed attempts only to its own (silent) logger.
			if failed, ok := n.(paho.ConnectionNotificationFailed); ok {
				slog.Warn("mqtt: connect failed, retrying", "broker", cfg.Broker, "err", failed.Reason)
			}
		})
	// AddBroker silently drops a URL it can't parse, which would leave paho
	// with no broker to retry against.
	if len(po.Servers) == 0 {
		return nil, fmt.Errorf("mqtt: invalid broker URL %q", cfg.Broker)
	}
	if cfg.ClientID != "" {
		po.SetClientID(cfg.ClientID)
	}
	if cfg.Username != "" {
		po.SetUsername(cfg.Username)
		po.SetPassword(cfg.Password)
	}

	c.paho = paho.NewClient(po)
	return c, nil
}

// BaseTopic returns the topic every other topic is relative to, or "".
func (c *Client) BaseTopic() string {
	return c.cfg.BaseTopic
}

// OnConnect registers fn to run on every connect and reconnect, typically to
// (re)subscribe. Call it before Connect.
func (c *Client) OnConnect(fn func()) {
	c.onConn = append(c.onConn, fn)
}

// Connect starts connecting in the background and returns without waiting,
// so an unreachable broker doesn't stop the caller from starting. Attempts
// repeat every connectRetryInterval until one succeeds, and each failure is
// logged; OnConnect callbacks run once connected, and HealthCheck reports the
// connection as down until then. It returns an error only when the client
// can't start connecting at all (for example, it is already connected).
func (c *Client) Connect() error {
	token := c.paho.Connect()
	// With ConnectRetry the token completes only once connected, unless paho
	// refused to start, which it reports synchronously.
	select {
	case <-token.Done():
		if err := token.Error(); err != nil {
			return fmt.Errorf("mqtt: connect failed: %w", err)
		}
	default:
	}
	return nil
}

// Disconnect closes the connection, or stops connecting, waiting up to 250ms
// for pending work.
func (c *Client) Disconnect() {
	c.paho.Disconnect(250)
}

// IsConnected reports whether the client has an open connection to the
// broker. It is false while reconnecting (unlike paho's IsConnected, which
// counts a reconnect in progress as connected).
func (c *Client) IsConnected() bool {
	return c.paho.IsConnectionOpen()
}

// HealthCheck reports the connection as a check named name (e.g. "mqtt",
// "ext-mqtt"); it is healthy only while the connection is open, so an outage
// shows even while the client reconnects.
func (c *Client) HealthCheck(name string) healthcheck.HealthCheck {
	if !c.IsConnected() {
		return healthcheck.New(name, errors.New("mqtt: connection is down"))
	}
	return healthcheck.New(name, nil)
}

// prefix prepends BaseTopic to a relative topic.
func (c *Client) prefix(topic string) string {
	if c.cfg.BaseTopic == "" {
		return topic
	}
	return c.cfg.BaseTopic + "/" + topic
}

// strip removes BaseTopic from a delivered topic. BaseTopic itself, which a
// "<BaseTopic>/#" subscription also matches, becomes "".
func (c *Client) strip(topic string) string {
	if c.cfg.BaseTopic == "" {
		return topic
	}
	if topic == c.cfg.BaseTopic {
		return ""
	}
	return strings.TrimPrefix(topic, c.cfg.BaseTopic+"/")
}

// Subscribe subscribes to a BaseTopic-relative topic at QoS 0.
func (c *Client) Subscribe(topic string, handler MessageHandler) error {
	full := c.prefix(topic)
	token := c.paho.Subscribe(full, 0, func(_ paho.Client, m paho.Message) {
		handler(c.strip(m.Topic()), m.Payload(), m.Retained())
	})
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt: subscribe to %q: %w", full, err)
	}
	slog.Info("mqtt: subscribed", "topic", full)
	return nil
}

// Publish publishes to a BaseTopic-relative topic at QoS 0, not retained. A
// ReadOnly connection logs the publish instead of sending it. Without an open
// connection it returns an error: paho would otherwise drop a QoS 0 message
// during a reconnect and still report success.
func (c *Client) Publish(topic string, payload []byte) error {
	full := c.prefix(topic)
	if c.cfg.ReadOnly {
		slog.Info("mqtt: read-only, skipping publish", "topic", full, "payload", string(payload))
		return nil
	}
	if !c.IsConnected() {
		return fmt.Errorf("mqtt: publish to %q: connection is down", full)
	}
	token := c.paho.Publish(full, 0, false, payload)
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt: publish to %q: %w", full, err)
	}
	slog.Info("mqtt: published", "topic", full, "payload", string(payload))
	return nil
}
