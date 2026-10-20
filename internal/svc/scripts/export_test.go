package scripts

import "github.com/homeland-live/homeland/internal/script"

// WithHandler caches h under devID without compiling from disk or notifying
// subscribers, and returns the service for chaining.
func (svc *Service) WithHandler(devID string, h *script.Handler) *Service {
	svc.cache.set(devID, h)
	return svc
}
