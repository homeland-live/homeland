package event_test

import (
	"log/slog"
	"testing"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/mqtt/mqtttest"
	"github.com/homeland-live/homeland/internal/svc/event"
	"github.com/homeland-live/homeland/internal/z2m"
)

// deliverUpdate wires the event service to a z2m client and delivers one
// update that propagates (the first state is only the initial sync-up).
func deliverUpdate(t *testing.T) {
	t.Helper()
	client, err := z2m.NewClient(nil, z2m.WithMQTT(&mqtttest.Client{Connected: true}))
	require.NoError(t, err)
	require.NoError(t, event.New(nil, client, nil).Start())

	client.Handle("bridge/devices", []byte(`[{"ieee_address":"0x01","friendly_name":"lamp","definition":{"exposes":[
		{"type":"binary","name":"state","property":"state","access":7}
	]}}]`))
	client.Handle("lamp", []byte(`{"state":"OFF"}`))
	client.Handle("lamp", []byte(`{"state":"ON"}`))
}

func TestDeviceUpdateNotLoggedAtInfo(t *testing.T) {
	logs := slogtest.Capture(t, slog.LevelInfo)
	deliverUpdate(t)
	assert.NotContains(t, logs.String(), "device update", "device updates must not be logged at Info")
}

func TestDeviceUpdateLoggedAtDebugWithSummary(t *testing.T) {
	logs := slogtest.Capture(t, slog.LevelDebug)
	deliverUpdate(t)
	assert.Contains(t, logs.String(), "svc/event: device update")
	assert.Contains(t, logs.String(), "state.state=ON", "the debug line must carry the state as a group")
}
