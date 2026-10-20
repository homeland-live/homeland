package script

import "github.com/homeland-live/homeland/internal/z2m"

// Z2M backs ctx.z2m. Satisfied by *z2m.Client.
type Z2M interface {
	// Publish sends a JSON command object to a device's set topic.
	Publish(friendlyName string, payload any) error
	// PublishProp sets a single property via the set/<prop> topic.
	PublishProp(friendlyName, prop string, value any) error
	// DeviceState returns a device's latest state by friendly name or IEEE
	// address, or nil.
	DeviceState(id string) *z2m.DeviceState
}

// MQTT backs ctx.mqtt on the external broker. Satisfied by *mqtt.Client.
type MQTT interface {
	Publish(topic string, payload []byte) error
}
