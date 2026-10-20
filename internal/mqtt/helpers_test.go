package mqtt_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/mqtt"
	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
)

// flush returns once the broker has read everything c sent before the call:
// a SUBSCRIBE travels on the same connection after any earlier PUBLISH, and
// Subscribe waits for its SUBACK.
func flush(t *testing.T, c *mqtt.Client) {
	t.Helper()
	require.NoError(t, c.Subscribe("flush", func(string, []byte, bool) {}))
}

// connectedClient builds a client for cfg (Broker is set to b) and connects
// it, returning once connected.
func connectedClient(t *testing.T, b *mqtttest.Broker, cfg config.MQTTConn) *mqtt.Client {
	t.Helper()
	cfg.Broker = b.URL()
	c, err := mqtt.New(cfg)
	require.NoError(t, err)
	connected := onConnect(c)
	require.NoError(t, c.Connect())
	t.Cleanup(c.Disconnect)
	waitFor(t, connected, "client did not connect")
	return c
}

// onConnect returns a channel closed on c's first connect. Call it before
// Connect.
func onConnect(c *mqtt.Client) <-chan struct{} {
	ch := make(chan struct{})
	var once sync.Once
	c.OnConnect(func() { once.Do(func() { close(ch) }) })
	return ch
}

func waitFor(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal(msg)
	}
}
