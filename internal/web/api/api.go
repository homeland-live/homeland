// Package api defines and handles http api
package api

import (
	"github.com/gofiber/fiber/v3"

	webcontext "github.com/homeland-live/homeland/internal/web/context"
)

// API implements api receiver
type API struct {
	ctx *webcontext.Context
}

// Routes adds api routes to the given fiber app
func (api *API) Routes(app *fiber.App) {
	router := app.Group("/api")
	// health
	router.Get("/v1/health", api.GetHealthHandlerV1)
	// status
	router.Get("/v1/status/buildinfo", api.GetStatusBuildInfoHandlerV1)
	// z2m
	router.Get("/v1/z2m/bridge/devices", api.ListZ2MBridgeDevicesHandlerV1)
	router.Get("/v1/z2m/bridge/info", api.GetZ2MBridgeInfoHandlerV1)
	router.Get("/v1/z2m/bridge/health", api.GetZ2MBridgeHealthHandlerV1)
	router.Get("/v1/z2m/devices/:id/events", api.ListDeviceEventsHandlerV1)
	router.Get("/v1/z2m/devices/:id/script", api.GetDeviceScriptHandlerV1)
	// scripts
	router.Get("/v1/scripts", api.ListScriptsHandlerV1)
}

// New returns new api value
func New(ctx *webcontext.Context) *API {
	return &API{
		ctx: ctx,
	}
}
