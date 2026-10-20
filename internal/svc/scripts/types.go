package scripts

import (
	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/script"
	"github.com/homeland-live/homeland/internal/z2m"
)

// Z2M is the z2m client API compiled handlers use (ctx.z2m), plus the device
// lookup InConflict uses. Satisfied by *z2m.Client.
type Z2M interface {
	script.Z2M
	// Device returns a device by friendly name or IEEE address, or nil while
	// the device list hasn't arrived or the device is unknown.
	Device(id string) *z2m.Device
}

// MQTT is the external broker connection backing ctx.mqtt. Satisfied by
// *mqtt.Client.
type MQTT interface {
	script.MQTT
	// Connect starts connecting without waiting for the connection.
	Connect() error
	// Disconnect closes the connection, or stops connecting.
	Disconnect()
	// HealthCheck reports the connection health as a check with the given name.
	HealthCheck(name string) healthcheck.HealthCheck
}
