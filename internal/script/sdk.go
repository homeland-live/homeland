package script

import (
	"encoding/json"
	"errors"
	"fmt"
)

// z2mSDK backs ctx.z2m. Its exported methods are exposed to scripts with
// lower-cased names (Publish becomes ctx.z2m.publish). With a nil client every
// method returns an error, which goja throws in the script.
type z2mSDK struct {
	client Z2M
}

// Publish backs ctx.z2m.publish: it sends a JSON command to a device's set
// topic.
func (s *z2mSDK) Publish(friendlyName string, payload any) error {
	if s.client == nil {
		return errors.New("ctx.z2m is not available")
	}
	return s.client.Publish(friendlyName, payload)
}

// PublishProp backs ctx.z2m.publishProp: it sets one property via the
// set/<prop> topic.
func (s *z2mSDK) PublishProp(friendlyName, prop string, value any) error {
	if s.client == nil {
		return errors.New("ctx.z2m is not available")
	}
	return s.client.PublishProp(friendlyName, prop, value)
}

// GetState backs ctx.z2m.getState: it returns a device's latest state (by
// friendly name or IEEE address) as a plain object, or null when the device is
// unknown or has not reported yet.
func (s *z2mSDK) GetState(devID string) (any, error) {
	if s.client == nil {
		return nil, errors.New("ctx.z2m is not available")
	}
	state := s.client.DeviceState(devID)
	if state == nil {
		return nil, nil
	}
	return state.Props(), nil
}

// mqttSDK backs ctx.mqtt. With a nil client (no external broker configured)
// Publish throws rather than falling back to the z2m connection.
type mqttSDK struct {
	client MQTT
}

// Publish backs ctx.mqtt.publish: it publishes payload as JSON to an arbitrary
// topic on the external broker.
func (s *mqttSDK) Publish(topic string, payload any) error {
	if s.client == nil {
		return errors.New(
			"ctx.mqtt.publish: external MQTT broker is not configured; " +
				"set HL_EXT_MQTT_BROKER (and related HL_EXT_MQTT_* vars) to enable it")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ctx.mqtt.publish: marshal payload: %w", err)
	}
	return s.client.Publish(topic, body)
}
