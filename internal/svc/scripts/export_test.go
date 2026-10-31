package scripts

import (
	"time"

	"github.com/homeland-live/homeland/internal/script"
)

// WithHandler caches h under devID without compiling from disk or notifying
// subscribers, and returns the service for chaining.
func (svc *Service) WithHandler(devID string, h *script.Handler) *Service {
	svc.cache.set(devID, h)
	return svc
}

// Debouncer is the watcher's per-file debounce, driven directly so tests
// control when timers fire relative to new events.
type Debouncer struct{ d *debouncer }

// Firing is a timer having fired for Name.
type Firing struct {
	Name string
	f    firing
}

// NewDebouncer returns a Debouncer whose fired timers stop waiting once done
// is closed.
func NewDebouncer(delay time.Duration, done <-chan struct{}) *Debouncer {
	return &Debouncer{d: newDebouncer(delay, done)}
}

// Touch (re)starts name's timer.
func (d *Debouncer) Touch(name string) { d.d.touch(name) }

// Next receives the next firing.
func (d *Debouncer) Next() Firing {
	f := <-d.d.due
	return Firing{Name: f.name, f: f}
}

// TryNext receives a firing that is already waiting, if any.
func (d *Debouncer) TryNext() (Firing, bool) {
	select {
	case f := <-d.d.due:
		return Firing{Name: f.name, f: f}, true
	default:
		return Firing{}, false
	}
}

// Take reports whether f is its name's latest timer.
func (d *Debouncer) Take(f Firing) bool { return d.d.take(f.f) }
