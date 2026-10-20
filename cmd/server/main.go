// Command server runs Homeland: it connects to the Zigbee2MQTT broker, runs
// the device scripts on device events and schedules, and serves the HTTP API
// and UI until interrupted.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/avakarev/go-util/slogutil"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/web"
	webcontext "github.com/homeland-live/homeland/internal/web/context"
)

func main() {
	os.Exit(run())
}

// run serves until a shutdown signal arrives or the server fails, then tears
// everything down on this goroutine, so the process only exits once every
// service has drained and closed. It returns the process exit code.
func run() int {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("main: failed to load config", "err", err)
		return 1
	}

	webctx, err := webcontext.New(cfg)
	if err != nil {
		slog.Error("main: failed to create web context", "err", err)
		return 1
	}

	server := web.New(webctx, cfg.Web.HTTPPort)

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve() }()

	code := 0
	select {
	case <-sigCtx.Done():
		slog.Info("main: received shutdown signal")
	case err := <-serveErr:
		slog.Error("main: server stopped unexpectedly", "err", err)
		code = 1
	}
	// HTTP first, so no request reaches a closed dependency.
	server.Shutdown()
	webctx.Shutdown()
	return code
}

func init() {
	slogutil.MustConfigure(slogutil.WithJSON(false))
}
