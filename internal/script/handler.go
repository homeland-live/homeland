package script

import (
	"fmt"
	"sync"
	"time"

	"github.com/avakarev/go-util/httputil"
	"github.com/dop251/goja"
)

// Handler is a compiled script bound to its own goja runtime. Calls into the
// runtime (Handle, RunJob) are serialized with a mutex, since goja is
// single-threaded, so a Handler is safe for concurrent use.
//
// A broken handler (Error set) is one whose script failed to compile: it runs
// nothing and returns Error from Handle and RunJob. A nil *Handler is a no-op.
type Handler struct {
	mu     sync.Mutex
	script *Script
	handle goja.Callable

	// ctx is nil for a broken handler.
	ctx *Context

	// timeout bounds each Handle and RunJob call; <= 0 disables it.
	timeout time.Duration

	// HasHandle and JobCount let the UI tell event handlers, jobs-only
	// scripts and both apart. Both are zero for a broken handler.
	HasHandle bool `json:"hasHandle"`
	JobCount  int  `json:"jobCount"`

	// Error is why the script failed to compile, or nil. It is a
	// *httputil.Err, so it marshals to the API's standard {code,msg} shape.
	Error error `json:"error,omitempty"`
}

// Script returns the handler's source script (set for broken handlers too), or
// nil for a nil handler.
func (h *Handler) Script() *Script {
	if h == nil {
		return nil
	}
	return h.script
}

// Handle runs handle(state, ctx) with state converted to a JS value. It
// returns Error for a broken handler and is a no-op for a nil handler or a
// script without a handle function. A call that outruns the timeout is
// interrupted and returns an error, and so does a Go panic raised during the
// call.
func (h *Handler) Handle(state any) error {
	if h == nil {
		return nil
	}
	if h.Error != nil {
		return h.Error
	}
	if h.handle == nil {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	return h.ctx.withTimeout(h.timeout, "handler", func() (err error) {
		defer h.ctx.recoverPanic("handler", &err)
		if _, err = h.handle(goja.Undefined(), h.ctx.runtime.ToValue(state), h.ctx.ctxArg); err != nil {
			return fmt.Errorf("script: handle: %w", err)
		}
		return nil
	})
}

// Jobs returns the jobs the script exports in module.exports.jobs, in order,
// or nil for a broken handler. They are read once, by Compile, so the result
// never changes and is safe to read concurrently.
func (h *Handler) Jobs() []ScheduledJob {
	if h == nil || h.ctx == nil {
		return nil
	}
	return h.ctx.jobs
}

// RunJob runs a scheduled callback as fn(ctx), with the same serialization and
// timeout as Handle, and returns a Go panic during the call as an error. It
// returns Error for a broken handler and is a no-op for a nil handler.
func (h *Handler) RunJob(job ScheduledJob) error {
	if h == nil {
		return nil
	}
	if h.Error != nil {
		return h.Error
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	return h.ctx.withTimeout(h.timeout, "scheduled job", func() (err error) {
		defer h.ctx.recoverPanic("scheduled job", &err)
		if _, err = job.fn(goja.Undefined(), h.ctx.ctxArg); err != nil {
			return fmt.Errorf("script: run scheduled job: %w", err)
		}
		return nil
	})
}

// NewBrokenHandler returns a broken handler for s: it runs nothing, has no
// jobs, and reports err as its Error (a *httputil.Err, so it marshals to the
// API's {code,msg} shape).
func NewBrokenHandler(s *Script, err error) *Handler {
	return &Handler{script: s, Error: httputil.Errf("%v", err)}
}
