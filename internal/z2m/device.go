// Package z2m is a Zigbee2MQTT client: it tracks the bridge's device registry
// and device states over MQTT, publishes device commands, and fans device
// updates out to subscribers.
package z2m

// Device combines a device's spec, state schema and z2m options with its
// latest state (nil until the device reports). Options is nil until
// bridge/info arrives, and when neither the device nor device_options sets
// any; devices share it, so it must not be modified.
type Device struct {
	Spec    *DeviceSpec    `json:"spec"`
	Schema  *StateSchema   `json:"schema"`
	Options *DeviceOptions `json:"options"`
	State   *DeviceState   `json:"state"`
}
