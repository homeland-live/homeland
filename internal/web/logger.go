package web

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
)

// LoggerFunc logs each request through slog: 5xx at error, 4xx at warn,
// everything else at info.
func LoggerFunc(c fiber.Ctx, data *logger.Data, _ *logger.Config) error {
	status := c.Response().StatusCode()

	level := slog.LevelInfo
	switch {
	case status >= fiber.StatusInternalServerError:
		level = slog.LevelError
	case status >= fiber.StatusBadRequest:
		level = slog.LevelWarn
	}

	args := []any{
		"status", status,
		"latency", data.Stop.Sub(data.Start).String(),
		"ip", c.IP(),
		"method", c.Method(),
		"path", c.Path(),
	}
	if data.ChainErr != nil {
		args = append(args, "err", data.ChainErr.Error())
	}

	slog.Default().Log(c, level, "http", args...)
	return nil
}
