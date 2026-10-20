package scripts

import (
	"maps"
	"sync"

	"github.com/homeland-live/homeland/internal/script"
)

// handlerCache holds compiled handlers keyed by device id. Safe for concurrent
// use.
type handlerCache struct {
	mu       sync.Mutex
	handlers map[string]*script.Handler
}

// get returns the handler cached under devID.
func (c *handlerCache) get(devID string) (*script.Handler, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h, ok := c.handlers[devID]
	return h, ok
}

// set caches h under devID.
func (c *handlerCache) set(devID string, h *script.Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[devID] = h
}

// remove drops the handler cached under devID.
func (c *handlerCache) remove(devID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.handlers, devID)
}

// snapshot returns a copy of every cached handler keyed by device id, safe to
// read without holding the lock.
func (c *handlerCache) snapshot() map[string]*script.Handler {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]*script.Handler, len(c.handlers))
	maps.Copy(out, c.handlers)
	return out
}

// newHandlerCache returns an empty cache.
func newHandlerCache() *handlerCache {
	return &handlerCache{handlers: make(map[string]*script.Handler)}
}
