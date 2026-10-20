package api

import (
	"github.com/gofiber/fiber/v3"
)

// GetHealthHandlerV1 handles health request
func (api *API) GetHealthHandlerV1(c fiber.Ctx) error {
	hc := api.ctx.HealthCheck()
	if !hc.IsHealthy {
		return c.Status(fiber.StatusServiceUnavailable).JSON(hc)
	}
	return c.JSON(hc)
}
