// Package script runs user-defined JavaScript device scripts on the goja
// ECMAScript engine. A script is a CommonJS-style module with no host globals:
// it exports a handle(ctx, state, prevState) function, which receives the
// device's full latest state and the one before it on each update, a list of
// cron jobs, or both:
//
//	function handle(ctx, state, prevState) {
//	    if (state.action === "off") {
//	        ctx.z2m.publishProp("living-room-lamp", "state", "OFF");
//	    }
//	}
//
//	function turnOffLamp(ctx) {
//	    ctx.z2m.publishProp("living-room-lamp", "state", "OFF");
//	}
//
//	module.exports = {
//	    handle,
//	    jobs: [{ cron: "0 17 * * *", run: turnOffLamp }],
//	};
//
// ctx exposes ctx.z2m (publish, publishProp, getState), ctx.mqtt (publish to
// the external broker), ctx.sleep(ms) and ctx.log(...). See docs/script.d.ts
// for the full contract.
package script

import (
	"strings"
)

// FileExt is the file extension of device scripts.
const FileExt = ".js"

// Script is a device script loaded from the scripts directory.
type Script struct {
	FileName string `json:"filename"`
	Content  string `json:"content"`
	// DeviceID is FileName without ".js": a friendly name or an IEEE address.
	DeviceID string `json:"deviceId"`
}

// NewScript returns a script for the given file name and source.
func NewScript(fname string, content string) *Script {
	return &Script{
		FileName: fname,
		Content:  content,
		DeviceID: strings.TrimSuffix(fname, FileExt),
	}
}
