package script_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/script"
	"github.com/homeland-live/homeland/internal/z2m"
)

// newScript wraps raw JS source in a *script.Script for use in tests.
func newScript(src string) *script.Script {
	return &script.Script{FileName: "test.js", Content: src}
}

// writeScript creates a <name> file holding src under dir.
func writeScript(t *testing.T, dir, name, src string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600))
}

// fakeZ2M records z2m publish calls and serves canned device states. It
// satisfies Z2M.
type fakeZ2M struct {
	publishCalls     []publishCall
	publishPropCalls []publishPropCall
	states           map[string]map[string]any
}

type publishCall struct {
	name    string
	payload any
}

type publishPropCall struct {
	name  string
	prop  string
	value any
}

func (f *fakeZ2M) Publish(name string, payload any) error {
	f.publishCalls = append(f.publishCalls, publishCall{name, payload})
	return nil
}

func (f *fakeZ2M) PublishProp(name, prop string, value any) error {
	f.publishPropCalls = append(f.publishPropCalls, publishPropCall{name, prop, value})
	return nil
}

// DeviceState serves canned states keyed by friendly name or address, backing
// ctx.z2m.getState tests. Each canned prop map is wrapped into a
// *z2m.DeviceState whose Props() reproduces it, matching the real client's
// return type.
func (f *fakeZ2M) DeviceState(id string) *z2m.DeviceState {
	props, ok := f.states[id]
	if !ok {
		return nil
	}
	raw := make(map[string]json.RawMessage, len(props))
	for k, v := range props {
		b, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		raw[k] = b
	}
	return z2m.NewDeviceState(raw)
}

func (f *fakeZ2M) HealthCheck() healthcheck.HealthCheck { return healthcheck.New("z2m", nil) }

// fakeMQTT records raw publish calls. It satisfies MQTT.
type fakeMQTT struct {
	calls []rawCall
}

type rawCall struct {
	topic   string
	payload []byte
}

func (f *fakeMQTT) Publish(topic string, payload []byte) error {
	f.calls = append(f.calls, rawCall{topic, payload})
	return nil
}

func (f *fakeMQTT) HealthCheck(name string) healthcheck.HealthCheck {
	return healthcheck.New(name, nil)
}
