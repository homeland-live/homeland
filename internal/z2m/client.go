package z2m

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/mqtt"
)

// UpdateFunc is a device-update subscriber, called with the device whenever it
// reports a new state.
type UpdateFunc func(*Device)

// Client is a Zigbee2MQTT client. It tracks the device registry and device
// states from the bridge topics and fans device updates out to subscribers.
type Client struct {
	mqtt     MQTTClient
	registry *registry

	mu     sync.RWMutex
	info   *BridgeInfo
	health *BridgeHealth

	subMu sync.RWMutex
	subs  []UpdateFunc
}

// MQTTClient is the subset of *mqtt.Client the z2m client uses; tests
// substitute a fake.
type MQTTClient interface {
	BaseTopic() string
	OnConnect(fn func())
	Connect() error
	Disconnect()
	Subscribe(topic string, handler mqtt.MessageHandler) error
	Publish(topic string, payload []byte) error
	HealthCheck(name string) healthcheck.HealthCheck
}

// Option customizes how NewClient builds a Client.
type Option func(*options)

type options struct {
	mqtt MQTTClient
}

// WithMQTT injects the MQTT connection instead of building one from config
// (used by tests).
func WithMQTT(m MQTTClient) Option {
	return func(o *options) {
		o.mqtt = m
	}
}

// NewClient returns a client that owns its MQTT connection, built from cfg
// unless WithMQTT is given. The connection must have a non-empty BaseTopic.
// The client is not connected: register subscribers, then call Connect.
func NewClient(cfg *config.MQTTConn, opts ...Option) (*Client, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	c := &Client{
		mqtt:     o.mqtt,
		registry: newRegistry(),
	}

	if c.mqtt == nil {
		if cfg == nil {
			return nil, errors.New("z2m: mqtt config is required, but missing")
		}
		conn, err := mqtt.New(*cfg)
		if err != nil {
			return nil, err
		}
		c.mqtt = conn
	}

	if c.mqtt.BaseTopic() == "" {
		return nil, fmt.Errorf("z2m: mqtt client requires a non-empty BaseTopic")
	}

	// Resubscribe on every (re)connect.
	c.mqtt.OnConnect(c.subscribe)
	return c, nil
}

// Connect starts connecting to the broker without waiting for it (see
// mqtt.Client.Connect); the device topics are subscribed once connected. Call
// it after every subscriber is registered, so no update is missed.
func (c *Client) Connect() error {
	return c.mqtt.Connect()
}

// Disconnect closes the MQTT connection, which stops device-update delivery.
func (c *Client) Disconnect() {
	c.mqtt.Disconnect()
}

// Subscribe registers fn to be called on every device state update.
// Subscribers run synchronously, in registration order, on the MQTT delivery
// goroutine, so a slow one delays all others and the next message: keep them
// fast or hand work off. Safe for concurrent use.
func (c *Client) Subscribe(fn UpdateFunc) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	c.subs = append(c.subs, fn)
}

// notify calls every subscriber with dev.
func (c *Client) notify(dev *Device) {
	c.subMu.RLock()
	defer c.subMu.RUnlock()
	for _, fn := range c.subs {
		fn(dev)
	}
}

// subscribe subscribes to every topic under the base topic. A single "#" is
// used, rather than "#" plus bridge topics, because overlapping subscriptions
// deliver a message once per match; "#" rather than "+" also covers friendly
// names that contain slashes.
func (c *Client) subscribe() {
	if err := c.mqtt.Subscribe("#", c.handle); err != nil {
		slog.Error("z2m: subscribe failed", "topic", "#", "err", err)
	}
}

// Handle processes a live message on a base-topic-relative topic, as the
// subscription does for one the broker forwards (see handle).
func (c *Client) Handle(topic string, payload []byte) {
	c.handle(topic, payload, false)
}

// handle is the "#" subscription's mqtt.MessageHandler. It consumes
// bridge/devices, bridge/state, bridge/info and bridge/health, records device
// state topics (see isStateTopic) and ignores everything else. retained marks
// the broker's stored copy of a topic, sent on every (re)subscribe.
func (c *Client) handle(topic string, payload []byte, retained bool) {
	switch topic {
	case "bridge/devices":
		c.handleDevices(payload)
	case "bridge/state":
		c.handleBridgeState(payload)
	case "bridge/info":
		c.handleBridgeInfo(payload)
	case "bridge/health":
		c.handleBridgeHealth(payload)
	default:
		if isStateTopic(topic) {
			c.handleDeviceState(topic, payload, retained)
		}
	}
}

// isStateTopic reports whether a base-topic-relative topic is a device's
// "<friendly_name>" state topic (the name may contain slashes). The "#"
// subscription also delivers the base topic itself (""), bridge topics,
// command topics (set, set/<prop>, get, get/<prop>) and availability, which
// would otherwise overwrite state with a lone "state" attribute.
func isStateTopic(topic string) bool {
	switch {
	case topic == "", topic == "bridge", strings.HasPrefix(topic, "bridge/"):
		return false
	case topic == "set", strings.HasSuffix(topic, "/set"), strings.Contains(topic, "/set/"):
		return false
	case topic == "get", strings.HasSuffix(topic, "/get"), strings.Contains(topic, "/get/"):
		return false
	case topic == "availability", strings.HasSuffix(topic, "/availability"):
		return false
	}
	return true
}

// handleDevices replaces the registry with the bridge/devices roster. Device
// state is not requested here, so it relies on z2m retaining device state.
func (c *Client) handleDevices(payload []byte) {
	var devices []DeviceSpec
	if err := json.Unmarshal(payload, &devices); err != nil {
		slog.Error("z2m: failed to parse bridge/devices", "err", err)
		return
	}

	c.registry.replaceDevices(devices)
	slog.Info("z2m: loaded devices", "count", len(devices))
}

func (c *Client) handleBridgeState(payload []byte) {
	var bs BridgeState
	if err := json.Unmarshal(payload, &bs); err != nil {
		// Older z2m versions publish a plain "online"/"offline" string.
		bs.State = strings.Trim(string(payload), `"`)
	}
	online := bs.State == "online"
	c.registry.setBridgeOnline(online)
	slog.Info("z2m: bridge state", "state", bs.State)
}

func (c *Client) handleBridgeInfo(payload []byte) {
	var info BridgeInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		slog.Error("z2m: failed to parse bridge/info", "err", err)
		return
	}
	c.mu.Lock()
	c.info = &info
	c.mu.Unlock()
	slog.Info("z2m: bridge info",
		"version", info.Version,
		"coordinator", info.Coordinator.Type,
		"permitJoin", info.PermitJoin,
		"restartRequired", info.RestartRequired,
	)
}

func (c *Client) handleBridgeHealth(payload []byte) {
	var health BridgeHealth
	if err := json.Unmarshal(payload, &health); err != nil {
		slog.Error("z2m: failed to parse bridge/health", "err", err)
		return
	}
	c.mu.Lock()
	c.health = &health
	c.mu.Unlock()
	slog.Info("z2m: bridge health",
		"mqttConnected", health.MQTT.Connected,
		"processUptimeSec", health.Process.UptimeSec,
		"osMemoryPercent", health.OS.MemoryPercent,
		"devices", len(health.Devices),
	)
}

// handleDeviceState records the state message of device id (a friendly name
// or IEEE address) and notifies subscribers when the registry says the update
// should propagate. An empty payload is z2m clearing a retained topic (on a
// device rename or removal) and is ignored.
func (c *Client) handleDeviceState(id string, payload []byte, retained bool) {
	if len(payload) == 0 {
		return
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		slog.Error("z2m: failed to parse device state", "device", id, "err", err)
		return
	}

	// Drop the device info (mqtt.include_device_information) and OTA update
	// info z2m may embed.
	delete(raw, "device")
	delete(raw, "update")
	// A retained copy is a replay of the topic's last message, so its action
	// is a press that already happened, not a new one.
	if retained {
		delete(raw, propAction)
	}

	propagate := c.registry.setState(id, NewDeviceState(raw))
	if !propagate {
		return
	}

	if dev := c.registry.device(id); dev != nil {
		c.notify(dev)
	}
}

// Publish sends payload, marshaled to a JSON object, to "<friendlyName>/set",
// e.g. map[string]any{"state": "ON", "brightness": 200}.
func (c *Client) Publish(friendlyName string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("z2m: marshal publish payload: %w", err)
	}
	topic := friendlyName + "/set"
	return c.mqtt.Publish(topic, body)
}

// PublishProp sends a raw value to "<friendlyName>/set/<prop>", so
// PublishProp("bulb", "state", "ON") is equivalent to
// Publish("bulb", {"state": "ON"}).
func (c *Client) PublishProp(friendlyName, prop string, value any) error {
	topic := friendlyName + "/set/" + prop
	return c.mqtt.Publish(topic, []byte(scalarPayload(value)))
}

// scalarPayload renders a string verbatim and any other value as JSON.
func scalarPayload(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(b)
}

// Devices returns a snapshot of all known devices, sorted by friendly name.
func (c *Client) Devices() []Device {
	return c.registry.snapshot()
}

// Device returns a device by friendly name or IEEE address, or nil.
func (c *Client) Device(id string) *Device {
	return c.registry.device(id)
}

// DeviceState returns a device's latest state by friendly name or IEEE
// address, or nil when the device is unknown or has not reported yet.
func (c *Client) DeviceState(devID string) *DeviceState {
	if dev := c.registry.device(devID); dev != nil {
		return dev.State
	}
	return nil
}

// HealthCheck composes the MQTT connection and bridge checks.
func (c *Client) HealthCheck() healthcheck.HealthCheck {
	return healthcheck.Compose("z2m",
		c.mqtt.HealthCheck("mqtt"),
		c.bridgeHealthCheck(),
	)
}

// bridgeHealthCheck is unhealthy when the bridge last announced itself offline
// or its latest bridge/health reports its MQTT link down. A missing
// bridge/health is not a failure, since z2m publishes it rarely or not at all.
func (c *Client) bridgeHealthCheck() healthcheck.HealthCheck {
	if !c.registry.isBridgeOnline() {
		return healthcheck.New("bridge", fmt.Errorf("z2m: bridge is offline"))
	}
	if h := c.Health(); h != nil && !h.MQTT.Connected {
		return healthcheck.New("bridge", fmt.Errorf("z2m: bridge reports its mqtt link is down"))
	}
	return healthcheck.New("bridge", nil)
}

// Info returns the latest bridge/info payload, or nil if none arrived yet.
func (c *Client) Info() *BridgeInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.info
}

// Health returns the latest bridge/health payload, or nil if none arrived yet.
func (c *Client) Health() *BridgeHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}
