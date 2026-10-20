// Package context builds and owns the service's dependencies for the web
// server.
package context

import (
	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/db"
	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/svc/event"
	"github.com/homeland-live/homeland/internal/svc/scheduler"
	"github.com/homeland-live/homeland/internal/svc/scripts"
	"github.com/homeland-live/homeland/internal/z2m"
)

// Context owns every shared dependency and its lifecycle. API handlers read
// from its fields.
type Context struct {
	// DB is the event database.
	DB *db.Repo

	// Z2M is the Zigbee2MQTT client; it owns its MQTT connection.
	Z2M *z2m.Client

	// Scripts compiles device scripts once; Events and Scheduler both read
	// handlers from it.
	Scripts *scripts.Service

	// Events persists device updates and runs their handlers.
	Events *event.Service

	// Scheduler runs the jobs scripts export.
	Scheduler *scheduler.Service
}

// New builds and starts every service from cfg, which must be non-nil and
// complete (as config.Load returns it: defaults filled in, validated). Every
// subscriber is wired before its producer starts (scripts before the watcher,
// the event service before the broker connects), so no script change or
// device update is missed. When a step fails, everything built so far is shut
// down before the error is returned; on success the caller must call
// Shutdown.
func New(cfg *config.Config) (_ *Context, err error) {
	// Filled in as each dependency is built, so a failure can tear down
	// exactly what exists (Shutdown skips nil fields).
	ctx := &Context{}
	defer func() {
		if err != nil {
			ctx.Shutdown()
		}
	}()

	if ctx.DB, err = db.New(cfg.DB); err != nil {
		return nil, err
	}

	// Not connected yet; that happens last.
	if ctx.Z2M, err = z2m.NewClient(cfg.Z2MMQTT); err != nil {
		return nil, err
	}

	ctx.Scripts = scripts.New(cfg, ctx.Z2M)
	ctx.Events = event.New(ctx.Scripts, ctx.Z2M, ctx.DB)
	// Subscribes to scripts, so it must precede Scripts.Start.
	if ctx.Scheduler, err = scheduler.New(ctx.Scripts); err != nil {
		return nil, err
	}

	if err = ctx.Events.Start(); err != nil {
		return nil, err
	}
	if err = ctx.Scripts.Start(); err != nil {
		return nil, err
	}
	if err = ctx.Scheduler.Start(); err != nil {
		return nil, err
	}
	if err = ctx.Z2M.Connect(); err != nil {
		return nil, err
	}

	return ctx, nil
}

// HealthCheck composes the z2m client, database, script service and event
// service checks. The scheduler has no connection of its own to report.
func (ctx *Context) HealthCheck() healthcheck.HealthCheck {
	return healthcheck.Compose("homeland",
		ctx.Z2M.HealthCheck(),
		ctx.DB.HealthCheck(),
		ctx.Scripts.HealthCheck(),
		ctx.Events.HealthCheck(),
	)
}

// Shutdown stops the inputs that trigger script runs (scheduler, then device
// updates), waiting for runs in progress, then closes the connections and the
// database they use. Shut the HTTP server down first. Failures are logged, so
// every step runs.
func (ctx *Context) Shutdown() {
	if ctx.Scheduler != nil {
		ctx.Scheduler.Shutdown()
	}
	if ctx.Events != nil {
		ctx.Events.Shutdown()
	}
	if ctx.Z2M != nil {
		ctx.Z2M.Disconnect()
	}
	// Closes the file watcher and the external broker connection.
	if ctx.Scripts != nil {
		ctx.Scripts.Shutdown()
	}
	if ctx.DB != nil {
		ctx.DB.Close()
	}
}
