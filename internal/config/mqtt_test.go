package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
)

// setMQTTEnv sets every MQTT variable the test does not care about to empty,
// so values from the developer's environment (e.g. a loaded .env) don't leak in.
func setMQTTEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, prefix := range []string{"HL_Z2M_MQTT", "HL_EXT_MQTT"} {
		for _, key := range []string{"_BROKER", "_CLIENT_ID", "_USERNAME", "_PASSWORD", "_BASE_TOPIC", "_READONLY"} {
			t.Setenv(prefix+key, "")
		}
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestLoadZ2MBaseTopicDefaultsToZigbee2MQTT(t *testing.T) {
	setMQTTEnv(t, map[string]string{
		"HL_Z2M_MQTT_BROKER": "tcp://localhost:1883",
		"HL_EXT_MQTT_BROKER": "tcp://localhost:1884",
	})

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg.Z2MMQTT)
	require.NotNil(t, cfg.ExtMQTT)
	assert.Equal(t, config.DefaultZ2MBaseTopic, cfg.Z2MMQTT.BaseTopic)
	assert.Empty(t, cfg.ExtMQTT.BaseTopic, "the external broker has no default base topic")
}

func TestLoadZ2MBaseTopicOverride(t *testing.T) {
	setMQTTEnv(t, map[string]string{
		"HL_Z2M_MQTT_BROKER":     "tcp://localhost:1883",
		"HL_Z2M_MQTT_BASE_TOPIC": "z2m-garage",
	})

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg.Z2MMQTT)
	assert.Equal(t, "z2m-garage", cfg.Z2MMQTT.BaseTopic)
	assert.Nil(t, cfg.ExtMQTT, "an unset external broker disables ctx.mqtt")
}

func TestLoadRejectsInvalidReadOnly(t *testing.T) {
	setMQTTEnv(t, map[string]string{
		"HL_Z2M_MQTT_BROKER":   "tcp://localhost:1883",
		"HL_Z2M_MQTT_READONLY": "maybe",
	})

	_, err := config.Load()
	assert.Error(t, err)
}

func TestLoadRejectsSameClientIDOnSameBroker(t *testing.T) {
	cases := map[string]map[string]string{
		"both ids defaulted": {
			"HL_Z2M_MQTT_BROKER": "tcp://localhost:1883",
			"HL_EXT_MQTT_BROKER": "tcp://localhost:1883",
		},
		"same id set explicitly": {
			"HL_Z2M_MQTT_BROKER":    "tcp://mosquitto:1883",
			"HL_Z2M_MQTT_CLIENT_ID": "homeland",
			"HL_EXT_MQTT_BROKER":    "tcp://mosquitto:1883",
			"HL_EXT_MQTT_CLIENT_ID": "homeland",
		},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			setMQTTEnv(t, env)
			_, err := config.Load()
			require.ErrorContains(t, err, "HL_Z2M_MQTT_CLIENT_ID and HL_EXT_MQTT_CLIENT_ID are both")
		})
	}
}

func TestLoadAcceptsSameClientIDOnDifferentBrokers(t *testing.T) {
	cases := map[string]map[string]string{
		"different ports": {
			"HL_Z2M_MQTT_BROKER": "tcp://localhost:1883",
			"HL_EXT_MQTT_BROKER": "tcp://localhost:1884",
		},
		"different hosts": {
			"HL_Z2M_MQTT_BROKER": "tcp://z2m-broker:1883",
			"HL_EXT_MQTT_BROKER": "tcp://other-broker:1883",
		},
		"different ids on one broker": {
			"HL_Z2M_MQTT_BROKER":    "tcp://localhost:1883",
			"HL_EXT_MQTT_BROKER":    "tcp://localhost:1883",
			"HL_EXT_MQTT_CLIENT_ID": "homeland-ext",
		},
		"no external broker": {
			"HL_Z2M_MQTT_BROKER": "tcp://localhost:1883",
		},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			setMQTTEnv(t, env)
			_, err := config.Load()
			assert.NoError(t, err)
		})
	}
}
