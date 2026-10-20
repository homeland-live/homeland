// Package config loads the service configuration from environment variables.
package config

import "fmt"

// Config holds all runtime configuration for the services
type Config struct {
	// Web is the web server (API and UI) configuration.
	Web *Web

	// DB is the event database configuration.
	DB *DB

	// Z2MMQTT is the Zigbee2MQTT broker connection.
	Z2MMQTT *MQTTConn

	// ExtMQTT is the optional external broker backing ctx.mqtt; nil when
	// HL_EXT_MQTT_BROKER is unset.
	ExtMQTT *MQTTConn

	// Scripts is the device-script configuration.
	Scripts *Scripts
}

// Load reads configuration from the process environment
func Load() (*Config, error) {
	web, err := loadWeb()
	if err != nil {
		return nil, err
	}

	db, err := loadDB()
	if err != nil {
		return nil, err
	}

	z2mMQTT, err := loadMQTTConn("HL_Z2M_MQTT", DefaultZ2MBaseTopic)
	if err != nil {
		return nil, err
	}

	extMQTT, err := loadMQTTConn("HL_EXT_MQTT", "")
	if err != nil {
		return nil, err
	}

	if z2mMQTT.ConflictsWith(extMQTT) {
		return nil, fmt.Errorf("config: HL_Z2M_MQTT_CLIENT_ID and HL_EXT_MQTT_CLIENT_ID are both %q on broker %q; "+
			"set them to different values", z2mMQTT.ClientID, z2mMQTT.Broker)
	}

	scripts, err := loadScripts()
	if err != nil {
		return nil, err
	}

	return &Config{
		Web:     web,
		DB:      db,
		Z2MMQTT: z2mMQTT,
		ExtMQTT: extMQTT,
		Scripts: scripts,
	}, nil
}
