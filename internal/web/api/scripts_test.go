package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListScriptsFlagsOrphans(t *testing.T) {
	// Only "lamp" is a real device; "lampp" is a typo'd orphan script.
	app, client, _ := newAppWithScripts(t, "lamp.js", "lampp.js")
	client.Handle("bridge/devices", lampRoster)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/scripts", nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

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
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Scripts, 2)

	// List is sorted by device id: "lamp" before "lampp".
	assert.Equal(t, "lamp", body.Scripts[0].DeviceID)
	assert.False(t, body.Scripts[0].Orphan, "lamp names a real device")
	assert.Equal(t, "lamp.js", body.Scripts[0].Script.FileName)
	assert.True(t, body.Scripts[0].Handler.HasHandle)

	assert.Equal(t, "lampp", body.Scripts[1].DeviceID)
	assert.True(t, body.Scripts[1].Orphan, "lampp names no device")
}

func TestListScriptsNoKnownDevices(t *testing.T) {
	// No bridge/devices handled, so z2m knows no device and the script's id
	// resolves to nothing: it is reported as an orphan.
	app, _, _ := newAppWithScripts(t, "lamp.js")

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/scripts", nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Scripts []struct {
			DeviceID string `json:"deviceId"`
			Orphan   bool   `json:"orphan"`
		} `json:"scripts"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Scripts, 1)
	assert.True(t, body.Scripts[0].Orphan)
}

func TestListScriptsEmpty(t *testing.T) {
	app, _, _ := newAppWithScripts(t)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/scripts", nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.JSONEq(t, `[]`, string(body["scripts"]))
}
