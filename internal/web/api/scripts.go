package api

import (
	"github.com/gofiber/fiber/v3"
)

// ListScriptsHandlerV1 lists every loaded script with its handler, flagging
// orphans. A script is orphan when its device id (file name without ".js")
// names no device known to z2m, i.e. a typo or a stale friendly name.
func (api *API) ListScriptsHandlerV1(c fiber.Ctx) error {
	handlers := api.ctx.Scripts.ListHandlers()
	resp := make([]any, 0, len(handlers))
	for _, h := range handlers {
		devID := h.Script().DeviceID
		resp = append(resp, fiber.Map{
			"deviceId": devID,
			"script":   h.Script(),
			"handler":  h,
			"orphan":   api.ctx.Z2M.Device(devID) == nil,
		})
	}
	return c.JSON(fiber.Map{"scripts": resp})
}
