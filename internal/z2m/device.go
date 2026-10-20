// Package z2m is a Zigbee2MQTT client: it tracks the bridge's device registry
// and device states over MQTT, publishes device commands, and fans device
// updates out to subscribers.
package z2m

// Device combines a device's spec and state schema with its latest state
// (nil until the device reports).
type Device struct {
	Spec   *DeviceSpec  `json:"spec"`
	Schema *StateSchema `json:"schema"`
	State  *DeviceState `json:"state"`
}
