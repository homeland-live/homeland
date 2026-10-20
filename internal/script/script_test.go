package script_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/homeland-live/homeland/internal/script"
)

func TestNewScriptDerivesDeviceID(t *testing.T) {
	// The device id is the file name without its ".js" extension, whether that
	// id is a friendly name or an IEEE address.
	friendly := script.NewScript("living-room-lamp.js", `// noop`)
	assert.Equal(t, "living-room-lamp", friendly.DeviceID)

	address := script.NewScript("0x00124b00247f9ca8.js", `// noop`)
	assert.Equal(t, "0x00124b00247f9ca8", address.DeviceID)
}
