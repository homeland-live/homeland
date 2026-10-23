package api

import (
	"fmt"
	"net/url"

	"github.com/gofiber/fiber/v3"

	"github.com/homeland-live/homeland/internal/db"
)

// deviceID returns the percent-decoded :id path parameter. fiber leaves path
// params encoded, so without decoding a friendly name with a space or a slash
// (sent as %20 or %2F) would never resolve. An undecodable id is a 400.
func deviceID(c fiber.Ctx) (string, error) {
	id, err := url.PathUnescape(c.Params("id"))
	if err != nil || id == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "invalid device id")
	}
	return id, nil
}

// ListZ2MBridgeDevicesHandlerV1 lists every device with its spec, schema,
// state and handler.
func (api *API) ListZ2MBridgeDevicesHandlerV1(c fiber.Ctx) error {
	devices := api.ctx.Z2M.Devices()
	resp := make([]any, 0, len(devices))
	for i := range devices {
		dev := &devices[i]
		resp = append(resp, fiber.Map{
			"spec":    dev.Spec,
			"schema":  dev.Schema,
			"state":   dev.State.Props(),
			"handler": api.ctx.Scripts.Handler(dev.Spec),
		})
	}
	return c.JSON(fiber.Map{"devices": resp})
}

// GetZ2MBridgeInfoHandlerV1 returns the latest bridge/info payload, or
// 503 until one has arrived.
func (api *API) GetZ2MBridgeInfoHandlerV1(c fiber.Ctx) error {
	if info := api.ctx.Z2M.Info(); info != nil {
		return c.JSON(fiber.Map{"info": info})
	}
	return fiber.NewError(fiber.StatusServiceUnavailable, "z2m bridge info not available yet")
}

// GetZ2MBridgeHealthHandlerV1 returns the latest bridge/health payload, or
// 503 until one has arrived (z2m publishes it rarely, or not at all).
func (api *API) GetZ2MBridgeHealthHandlerV1(c fiber.Ctx) error {
	if health := api.ctx.Z2M.Health(); health != nil {
		return c.JSON(fiber.Map{"health": health})
	}
	return fiber.NewError(fiber.StatusServiceUnavailable, "z2m bridge health not available yet")
}

// ListDeviceEventsHandlerV1 returns a page of a device's stored events along
// with its spec, schema, state and handler. The device id is a friendly name
// or an IEEE address.
func (api *API) ListDeviceEventsHandlerV1(c fiber.Ctx) error {
	id, err := deviceID(c)
	if err != nil {
		return err
	}

	dev := api.ctx.Z2M.Device(id)
	if dev == nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("no device found with given id %s", id))
	}

	var page db.Page
	if err := c.Bind().Query(&page); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	// Events are stored by IEEE address, so query by the resolved device's
	// address rather than the id from the path, which may be a friendly name.
	events, err := api.ctx.DB.QueryEvents(dev.Spec.IEEEAddress, &page)
	if err != nil {
		return err
	}

	var state fiber.Map
	if dev.State != nil {
		state = fiber.Map{
			"id":        dev.State.ID,
			"createdAt": dev.State.CreatedAt,
			"props":     dev.State.Props(),
		}
	}

	return c.JSON(fiber.Map{
		"spec":    dev.Spec,
		"schema":  dev.Schema,
		"state":   state,
		"handler": api.ctx.Scripts.Handler(dev.Spec),
		"events":  events,
		"page":    page,
	})
}

// GetDeviceScriptHandlerV1 returns the script and handler that run for a
// device (see scripts.Service.Handler); both are null when it has none. The
// device id is a friendly name or an IEEE address.
func (api *API) GetDeviceScriptHandlerV1(c fiber.Ctx) error {
	id, err := deviceID(c)
	if err != nil {
		return err
	}

	dev := api.ctx.Z2M.Device(id)
	if dev == nil {
		return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("no device found with given id %s", id))
	}

	// Script is nil-safe, so a device without a handler has a null script.
	handler := api.ctx.Scripts.Handler(dev.Spec)
	return c.JSON(fiber.Map{
		"script":  handler.Script(),
		"handler": handler,
	})
}
