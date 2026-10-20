package config

import "github.com/avakarev/go-util/envutil"

// DefaultHTTPPort is the HTTP port used when HL_HTTP_PORT is unset.
const DefaultHTTPPort = 8080

// Web is the web server (API and UI) configuration.
type Web struct {
	// HTTPPort is the TCP port the API and UI listen on, in 1-65535.
	HTTPPort int
}

// loadWeb reads HL_HTTP_PORT (default DefaultHTTPPort).
func loadWeb() (*Web, error) {
	port, err := envutil.PortDef("HL_HTTP_PORT", DefaultHTTPPort)
	if err != nil {
		return nil, err
	}
	return &Web{HTTPPort: port}, nil
}
