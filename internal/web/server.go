// Package web serves the JSON API under /api and the static UI under /ui.
package web

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/avakarev/go-util/httputil"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/homeland-live/homeland/internal/web/api"
	webcontext "github.com/homeland-live/homeland/internal/web/context"
)

// ShutdownTimeout bounds how long Shutdown waits for in-flight requests.
const ShutdownTimeout = 10 * time.Second

// Server represents web server
type Server struct {
	port int
	app  *fiber.App
	api  *api.API
}

// Serve starts an HTTP server
func (srv *Server) Serve() error {
	srv.app.Use(recover.New())
	srv.app.Use(logger.New(logger.Config{
		Next: func(c fiber.Ctx) bool {
			return c.Path() == "/api/v1/health"
		},
		LoggerFunc: LoggerFunc,
	}))
	srv.app.Use(helmet.New())

	srv.api.Routes(srv.app)

	srv.app.Use("/ui", static.New("ui/dist", static.Config{
		ModifyResponse: func(c fiber.Ctx) error {
			if p := c.Path(); p == "/ui" || p == "/ui/index.html" {
				c.Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
				c.Set("Pragma", "no-cache")
				c.Set("Expires", "0")
			}
			return nil
		},
	}))
	srv.app.Get("/", func(c fiber.Ctx) error {
		return c.Redirect().To("/ui")
	})

	slog.Info("web: starting server",
		"url", fmt.Sprintf("http://localhost:%d", srv.port),
		"handlers", srv.app.HandlersCount(),
	)
	return srv.app.Listen(fmt.Sprintf(":%d", srv.port), fiber.ListenConfig{DisableStartupMessage: true})
}

// Shutdown stops the app, waiting up to ShutdownTimeout for in-flight
// requests. It does not shut down the context the server was built with;
// the caller owns it and should shut it down afterwards, so no request
// reaches a closed dependency. Failures are logged.
func (srv *Server) Shutdown() {
	slog.Info("web: shutting down fiber app...")
	if err := srv.app.ShutdownWithTimeout(ShutdownTimeout); err != nil {
		slog.Error("web: failed to shut down fiber app", "err", err)
	}
}

// ErrorHandler renders errors as the standard httputil {code,msg} JSON, using
// a fiber.Error's status code when there is one.
func ErrorHandler(c fiber.Ctx, err error) error {
	httpErr := httputil.NewErrFrom(err)
	if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
		httpErr.Error.Code = fiberErr.Code
	}
	return c.Status(httpErr.Error.Code).JSON(httpErr)
}

// New returns a server listening on port.
func New(ctx *webcontext.Context, port int) *Server {
	return &Server{
		port: port,
		app:  fiber.New(fiber.Config{ErrorHandler: ErrorHandler}),
		api:  api.New(ctx),
	}
}
