package config

import (
	"time"

	"github.com/avakarev/go-util/envutil"
)

// Scripts is the device-script configuration.
type Scripts struct {
	// Dir holds the "<device-id>.js" scripts, where the id is a friendly name
	// or an IEEE address.
	Dir string

	// EnableWatcher keeps the compiled handlers in sync with Dir at runtime.
	EnableWatcher bool

	// Timeout bounds each script run (top-level evaluation, handle() call and
	// scheduled job); <= 0 disables it.
	Timeout time.Duration
}

// loadScripts reads HL_SCRIPTS_DIR (default "scripts"), HL_SCRIPTS_WATCHER
// (default false) and HL_SCRIPTS_TIMEOUT (default 5s).
func loadScripts() (*Scripts, error) {
	enableWatcher, err := envutil.BoolDef("HL_SCRIPTS_WATCHER", false)
	if err != nil {
		return nil, err
	}
	timeout, err := envutil.DurDef("HL_SCRIPTS_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &Scripts{
		Dir:           envutil.StrDef("HL_SCRIPTS_DIR", "scripts"),
		EnableWatcher: enableWatcher,
		Timeout:       timeout,
	}, nil
}
