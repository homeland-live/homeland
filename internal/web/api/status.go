package api

import (
	"github.com/avakarev/go-util/buildmeta"
	"github.com/gofiber/fiber/v3"
)

// GetStatusBuildInfoHandlerV1 handles request to get build and runtime info
func (api *API) GetStatusBuildInfoHandlerV1(c fiber.Ctx) error {
	return c.JSON(buildmeta.New())
}
