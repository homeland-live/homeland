package mqtt_test

import (
	"log/slog"
	"testing"
	"time"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/mqtt"
	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
)

const (
	connectFailed = "mqtt: connect failed, retrying"
	retryInterval = 20 * time.Millisecond
)

func TestConnectRetriesUntilBrokerIsUp(t *testing.T) {
	logs := slogtest.Capture(t, slog.LevelInfo)
	addr := mqtttest.FreeAddr(t)

	c, err := mqtt.New(config.MQTTConn{Broker: "tcp://" + addr, ClientID: "test"},
		mqtt.WithConnectRetryInterval(retryInterval))
	require.NoError(t, err)
	connected := onConnect(c)

	require.NoError(t, c.Connect(), "a broker that is down must not fail Connect")
	t.Cleanup(c.Disconnect)
	assert.False(t, c.IsConnected())
	require.Eventually(t, func() bool { return logs.Count(connectFailed) >= 2 },
		2*time.Second, 10*time.Millisecond, "each failed attempt is logged")

	// The broker comes up: the next attempt connects and OnConnect runs.
	mqtttest.StartBroker(t, addr)
	waitFor(t, connected, "client did not connect once the broker was up")
	assert.True(t, c.IsConnected())
	assert.Equal(t, 1, logs.Count("mqtt: connected"))
}

func TestDisconnectStopsConnecting(t *testing.T) {
	logs := slogtest.Capture(t, slog.LevelInfo)

	c, err := mqtt.New(config.MQTTConn{Broker: "tcp://" + mqtttest.FreeAddr(t), ClientID: "test"},
		mqtt.WithConnectRetryInterval(retryInterval))
	require.NoError(t, err)
	require.NoError(t, c.Connect())
	require.Eventually(t, func() bool { return logs.Count(connectFailed) >= 1 },
		2*time.Second, 10*time.Millisecond)

	c.Disconnect()
	// An attempt already in flight may still finish; after that, none start.
	time.Sleep(100 * time.Millisecond)
	after := logs.Count(connectFailed)
	assert.Never(t, func() bool { return logs.Count(connectFailed) > after },
		200*time.Millisecond, 10*time.Millisecond, "attempts continued after Disconnect")
}

func TestNewRejectsUnparsableBrokerURL(t *testing.T) {
	for _, broker := range []string{"://localhost", "tcp://local host:1883"} {
		_, err := mqtt.New(config.MQTTConn{Broker: broker})
		require.ErrorContains(t, err, "invalid broker URL", broker)
	}
	// The scheme is left to paho: an unknown one fails each connect attempt.
	for _, broker := range []string{"localhost:1883", "tcp://localhost:1883", "mqtts://localhost:8883", "ws://localhost:9001", "tpc://localhost:1883"} {
		_, err := mqtt.New(config.MQTTConn{Broker: broker})
		assert.NoError(t, err, broker)
	}
}
