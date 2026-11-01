// Command server runs Homeland: it connects to the Zigbee2MQTT broker, runs
// the device scripts on device events and schedules, and serves the HTTP API
// and UI until interrupted.
package main

import (
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
// service has drained and closed. A signal during startup lets startup finish
// and then shuts down without serving. It returns the process exit code.
func run() int {
	shutdown := notifyShutdown()

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

	code := 0
	select {
	case <-shutdown:
		// The signal arrived during startup, so there is nothing to serve.
	default:
		code = serve(web.New(webctx, cfg.Web.HTTPPort), shutdown)
	}
	webctx.Shutdown()
	return code
}

// notifyShutdown returns a channel that is closed on the first SIGINT or
// SIGTERM. That signal also restores the default handling, so a second one
// exits at once, even while startup or shutdown is still running.
func notifyShutdown() <-chan struct{} {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	shutdown := make(chan struct{})
	go func() {
		sig := <-sigs
		signal.Stop(sigs)
		slog.Info("main: received shutdown signal; send it again to exit immediately", "signal", sig)
		close(shutdown)
	}()
	return shutdown
}

// serve runs server until shutdown is closed or the server fails, then shuts
// it down, and returns the exit code.
func serve(server *web.Server, shutdown <-chan struct{}) int {
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve() }()

	code := 0
	select {
	case <-shutdown:
	case err := <-serveErr:
		slog.Error("main: server stopped unexpectedly", "err", err)
		code = 1
	}
	// HTTP first, so no request reaches a closed dependency.
	server.Shutdown()
	return code
}

func init() {
	slogutil.MustConfigure(slogutil.WithJSON(false))
}
