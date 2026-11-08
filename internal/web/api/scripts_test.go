package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/web/webtest"
)

func TestListScriptsFlagsOrphans(t *testing.T) {
	// Only "lamp" is a real device; "lampp" is a typo'd orphan script.
	srv := webtest.NewServer(t, webtest.WithScripts("lamp.js", "lampp.js"))
	srv.HandleZ2M("bridge/devices", lampRoster)

	var body struct {
		Scripts []struct {
			DeviceID string `json:"deviceId"`
			Orphan   bool   `json:"orphan"`
			Script   struct {
				FileName string `json:"filename"`
			} `json:"script"`
			Handler struct {
				HasHandle bool `json:"hasHandle"`
			} `json:"handler"`
		} `json:"scripts"`
	}
	srv.GetJSON(t, "/api/v1/scripts", http.StatusOK, &body)
	require.Len(t, body.Scripts, 2)

	// List is sorted by device id: "lamp" before "lampp".
	assert.Equal(t, "lamp", body.Scripts[0].DeviceID)
	assert.False(t, body.Scripts[0].Orphan, "lamp names a real device")
	assert.Equal(t, "lamp.js", body.Scripts[0].Script.FileName)
	assert.True(t, body.Scripts[0].Handler.HasHandle)

	assert.Equal(t, "lampp", body.Scripts[1].DeviceID)
	assert.True(t, body.Scripts[1].Orphan, "lampp names no device")
}

func TestListScriptsReportsBrokenHandler(t *testing.T) {
	// A top-level throw breaks the script of a real device.
	srv := webtest.NewServer(t, webtest.WithScript("lamp.js", `throw new Error("boom");`))
	srv.HandleZ2M("bridge/devices", lampRoster)

	var body struct {
		Scripts []struct {
			DeviceID string `json:"deviceId"`
			Orphan   bool   `json:"orphan"`
			Handler  struct {
				Error *struct {
					Code int    `json:"code"`
					Msg  string `json:"msg"`
				} `json:"error"`
			} `json:"handler"`
		} `json:"scripts"`
	}
	srv.GetJSON(t, "/api/v1/scripts", http.StatusOK, &body)
	require.Len(t, body.Scripts, 1)
	assert.Equal(t, "lamp", body.Scripts[0].DeviceID)
	assert.False(t, body.Scripts[0].Orphan, "a broken script still names its device")
	require.NotNil(t, body.Scripts[0].Handler.Error)
	assert.Equal(t, http.StatusUnprocessableEntity, body.Scripts[0].Handler.Error.Code)
	assert.Contains(t, body.Scripts[0].Handler.Error.Msg, "boom")
}

func TestListScriptsNoKnownDevices(t *testing.T) {
	// No bridge/devices handled, so z2m knows no device and the script's id
	// resolves to nothing: it is reported as an orphan.
	srv := webtest.NewServer(t, webtest.WithScripts("lamp.js"))

	var body struct {
		Scripts []struct {
			DeviceID string `json:"deviceId"`
			Orphan   bool   `json:"orphan"`
		} `json:"scripts"`
	}
	srv.GetJSON(t, "/api/v1/scripts", http.StatusOK, &body)
	require.Len(t, body.Scripts, 1)
	assert.True(t, body.Scripts[0].Orphan)
}

func TestListScriptsEmpty(t *testing.T) {
	srv := webtest.NewServer(t)

	var body map[string]json.RawMessage
	srv.GetJSON(t, "/api/v1/scripts", http.StatusOK, &body)
	assert.JSONEq(t, `[]`, string(body["scripts"]))
}
