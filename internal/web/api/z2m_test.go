package api_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/db"
	"github.com/homeland-live/homeland/internal/web/webtest"
)

// lampRoster is a bridge/devices payload with a single device, "lamp" at
// 0x01.
var lampRoster = []byte(`[{"ieee_address":"0x01","friendly_name":"lamp"}]`)

func TestListDeviceEventsDeviceWithoutState(t *testing.T) {
	srv := webtest.NewServer(t)
	// Known to the bridge, but has not reported any state yet.
	srv.HandleZ2M("bridge/devices", lampRoster)

	var body map[string]json.RawMessage
	srv.GetJSON(t, "/api/v1/z2m/devices/0x01/events", http.StatusOK, &body)
	assert.JSONEq(t, `null`, string(body["state"]), "a device without state must serialize state as null")
	assert.JSONEq(t, `[]`, string(body["events"]))
}

func TestListDeviceEventsDeviceWithState(t *testing.T) {
	srv := webtest.NewServer(t)
	srv.HandleZ2M("bridge/devices", lampRoster)
	srv.HandleZ2M("lamp", []byte(`{"state":"ON"}`))

	var body struct {
		State *struct {
			ID    string         `json:"id"`
			Props map[string]any `json:"props"`
		} `json:"state"`
	}
	srv.GetJSON(t, "/api/v1/z2m/devices/0x01/events", http.StatusOK, &body)
	require.NotNil(t, body.State)
	assert.NotEmpty(t, body.State.ID)
	assert.Equal(t, map[string]any{"state": "ON"}, body.State.Props)
}

// TestDeviceRoutesOmitPrevState covers a device whose state has a previous
// state: neither route serves it.
func TestDeviceRoutesOmitPrevState(t *testing.T) {
	srv := webtest.NewServer(t)
	srv.HandleZ2M("bridge/devices", lampRoster)
	srv.HandleZ2M("lamp", []byte(`{"state":"ON"}`))
	srv.HandleZ2M("lamp", []byte(`{"state":"OFF"}`))
	require.NotNil(t, srv.Ctx.Z2M.DeviceState("lamp").Prev)

	var events struct {
		State map[string]json.RawMessage `json:"state"`
	}
	srv.GetJSON(t, "/api/v1/z2m/devices/lamp/events", http.StatusOK, &events)
	assert.ElementsMatch(t, []string{"id", "createdAt", "props"}, slices.Collect(maps.Keys(events.State)))
	assert.JSONEq(t, `{"state":"OFF"}`, string(events.State["props"]))

	var devices struct {
		Devices []struct {
			State json.RawMessage `json:"state"`
		} `json:"devices"`
	}
	srv.GetJSON(t, "/api/v1/z2m/bridge/devices", http.StatusOK, &devices)
	require.Len(t, devices.Devices, 1)
	assert.JSONEq(t, `{"state":"OFF"}`, string(devices.Devices[0].State))
}

func TestListDeviceEventsByFriendlyNameOrAddress(t *testing.T) {
	srv := webtest.NewServer(t)
	srv.HandleZ2M("bridge/devices", lampRoster)
	// Events are stored by IEEE address.
	require.NoError(t, srv.Ctx.DB.CreateEvent(&db.Event{
		IEEEAddress: "0x01",
		Payload:     map[string]json.RawMessage{"state": json.RawMessage(`"ON"`)},
	}))

	// Looking the device up by either id must return the same events.
	for _, id := range []string{"0x01", "lamp"} {
		t.Run(id, func(t *testing.T) {
			var body struct {
				Events []db.Event `json:"events"`
				Page   db.Page    `json:"page"`
			}
			srv.GetJSON(t, "/api/v1/z2m/devices/"+id+"/events", http.StatusOK, &body)
			require.Len(t, body.Events, 1)
			assert.Equal(t, "0x01", body.Events[0].IEEEAddress)
			assert.Equal(t, 1, body.Page.Count)
		})
	}
}

func TestListDeviceEventsRejectsInvalidPage(t *testing.T) {
	srv := webtest.NewServer(t)
	srv.HandleZ2M("bridge/devices", lampRoster)

	// A huge pagenum would overflow the offset and quietly serve page 1.
	for _, query := range []string{"pagenum=0", "pagenum=1000001", "pagenum=922337203685477580", "pagesize=26"} {
		t.Run(query, func(t *testing.T) {
			srv.GetStatus(t, "/api/v1/z2m/devices/lamp/events?"+query, http.StatusBadRequest)
		})
	}
}

func TestListDeviceEventsUnknownDevice(t *testing.T) {
	srv := webtest.NewServer(t)
	srv.GetStatus(t, "/api/v1/z2m/devices/nope/events", http.StatusNotFound)
}

func TestGetDeviceScriptByFriendlyNameOrAddress(t *testing.T) {
	srv := webtest.NewServer(t)
	srv.HandleZ2M("bridge/devices", lampRoster)

	// The device has no script, so both fields are null for either id.
	for _, id := range []string{"0x01", "lamp"} {
		t.Run(id, func(t *testing.T) {
			var body map[string]json.RawMessage
			srv.GetJSON(t, "/api/v1/z2m/devices/"+id+"/script", http.StatusOK, &body)
			assert.JSONEq(t, `null`, string(body["script"]))
			assert.JSONEq(t, `null`, string(body["handler"]))
		})
	}
}

func TestGetDeviceScriptUnknownDevice(t *testing.T) {
	srv := webtest.NewServer(t)
	srv.GetStatus(t, "/api/v1/z2m/devices/nope/script", http.StatusNotFound)
}

func TestDeviceRoutesDecodeEncodedFriendlyName(t *testing.T) {
	srv := webtest.NewServer(t)
	// Friendly names may hold spaces and slashes; clients percent-encode them.
	srv.HandleZ2M("bridge/devices", []byte(
		`[{"ieee_address":"0x01","friendly_name":"Living Room"},{"ieee_address":"0x02","friendly_name":"kitchen/light"}]`))

	for _, id := range []string{"Living%20Room", "kitchen%2Flight"} {
		for _, route := range []string{"events", "script"} {
			t.Run(id+"/"+route, func(t *testing.T) {
				srv.GetStatus(t, "/api/v1/z2m/devices/"+id+"/"+route, http.StatusOK)
			})
		}
	}
}

func TestGetDeviceScriptReportsTwoScriptsConflict(t *testing.T) {
	srv := webtest.NewServer(t, webtest.WithScripts("0x01.js", "lamp.js"))
	srv.HandleZ2M("bridge/devices", lampRoster)

	var body struct {
		Script struct {
			FileName string `json:"filename"`
		} `json:"script"`
		Handler struct {
			HasHandle bool `json:"hasHandle"`
			Error     struct {
				Msg string `json:"msg"`
			} `json:"error"`
		} `json:"handler"`
	}
	srv.GetJSON(t, "/api/v1/z2m/devices/lamp/script", http.StatusOK, &body)
	assert.Contains(t, body.Handler.Error.Msg, "has two scripts, 0x01.js and lamp.js")
	assert.False(t, body.Handler.HasHandle)
	assert.Equal(t, "0x01.js", body.Script.FileName)
}

func TestGetDeviceScriptReportsBrokenScript(t *testing.T) {
	const src = `module.exports = {`
	srv := webtest.NewServer(t, webtest.WithScript("lamp.js", src))
	srv.HandleZ2M("bridge/devices", lampRoster)

	var body struct {
		Script struct {
			FileName string `json:"filename"`
			Content  string `json:"content"`
			DeviceID string `json:"deviceId"`
		} `json:"script"`
		Handler struct {
			HasHandle bool `json:"hasHandle"`
			JobCount  int  `json:"jobCount"`
			Error     *struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			} `json:"error"`
		} `json:"handler"`
	}
	srv.GetJSON(t, "/api/v1/z2m/devices/0x01/script", http.StatusOK, &body)
	assert.Equal(t, "lamp.js", body.Script.FileName)
	assert.Equal(t, src, body.Script.Content)
	assert.Equal(t, "lamp", body.Script.DeviceID)
	assert.False(t, body.Handler.HasHandle)
	assert.Zero(t, body.Handler.JobCount)
	require.NotNil(t, body.Handler.Error, "a script that doesn't parse is a broken handler")
	assert.Equal(t, http.StatusUnprocessableEntity, body.Handler.Error.Code)
	assert.NotEmpty(t, body.Handler.Error.Msg)
}

func TestGetDeviceScriptReportsJobsOnlyScript(t *testing.T) {
	srv := webtest.NewServer(t, webtest.WithScript("lamp.js",
		`module.exports = { jobs: [{ cron: "0 18 * * *", run(ctx) {} }, { cron: "30 0 * * *", run(ctx) {} }] };`))
	srv.HandleZ2M("bridge/devices", lampRoster)

	var body struct {
		Handler map[string]json.RawMessage `json:"handler"`
	}
	srv.GetJSON(t, "/api/v1/z2m/devices/lamp/script", http.StatusOK, &body)
	assert.JSONEq(t, `false`, string(body.Handler["hasHandle"]))
	assert.JSONEq(t, `2`, string(body.Handler["jobCount"]))
	assert.NotContains(t, body.Handler, "error", "a working handler serializes no error")
}

// TestDeviceRoutesReturnOptions covers the options both device routes return:
// the options z2m applies to the device, null until bridge/info arrives and
// while none are set.
func TestDeviceRoutesReturnOptions(t *testing.T) {
	srv := webtest.NewServer(t)
	srv.HandleZ2M("bridge/devices", lampRoster)

	options := func(t *testing.T) (list, events json.RawMessage) {
		t.Helper()
		var listBody struct {
			Devices []map[string]json.RawMessage `json:"devices"`
		}
		srv.GetJSON(t, "/api/v1/z2m/bridge/devices", http.StatusOK, &listBody)
		require.Len(t, listBody.Devices, 1)

		var eventsBody map[string]json.RawMessage
		srv.GetJSON(t, "/api/v1/z2m/devices/lamp/events", http.StatusOK, &eventsBody)
		return listBody.Devices[0]["options"], eventsBody["options"]
	}

	list, events := options(t)
	assert.JSONEq(t, `null`, string(list))
	assert.JSONEq(t, `null`, string(events))

	srv.HandleZ2M("bridge/info", []byte(`{"version":"2.6.0","config":{"devices":{"0x01":{"friendly_name":"lamp"}}}}`))

	list, events = options(t)
	assert.JSONEq(t, `null`, string(list), "no options set")
	assert.JSONEq(t, `null`, string(events), "no options set")

	srv.HandleZ2M("bridge/info", []byte(`{"version":"2.6.0","config":{
		"device_options":{"filtered_attributes":["battery"]},
		"devices":{"0x01":{"filtered_attributes":["linkquality"]}}
	}}`))

	list, events = options(t)
	assert.JSONEq(t, `{"filtered_attributes":["linkquality"]}`, string(list))
	assert.JSONEq(t, `{"filtered_attributes":["linkquality"]}`, string(events))
}
