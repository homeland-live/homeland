package script

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

	// Error is why the script failed to compile, or nil. It keeps the error
	// it wraps, for errors.Is and errors.As, and marshals to the API's
	// standard {code,msg} shape (see NewBrokenHandler).
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

// Handle runs handle(ctx, state) with state converted to a JS value. It
// returns Error for a broken handler and is a no-op for a nil handler or a
// script without a handle function. A call that outruns the timeout is
// interrupted and returns an error, and so does a Go panic raised during the
// call. An async handle fails like a throwing one: the call returns an error
// when its promise rejects or never settles, and when any promise the call
// rejected is left without a handler.
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

	err := h.ctx.run(h.timeout, "handler", func() (goja.Value, error) {
		return h.handle(goja.Undefined(), h.ctx.ctxArg, h.ctx.runtime.ToValue(state))
	})
	if err != nil {
		return fmt.Errorf("script: handle: %w", err)
	}
	return nil
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

// RunJob runs a scheduled callback as fn(ctx), with the same serialization,
// timeout and promise handling as Handle, and returns a Go panic during the
// call as an error. It returns Error for a broken handler and is a no-op for a
// nil handler. A job not from h's Jobs (another handler's, or hand-built) is
// an error and never runs.
func (h *Handler) RunJob(job ScheduledJob) error {
	if h == nil {
		return nil
	}
	if h.Error != nil {
		return h.Error
	}
	if job.owner != h {
		return errors.New("script: run scheduled job: not a job of this handler")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	err := h.ctx.run(h.timeout, "scheduled job", func() (goja.Value, error) {
		return job.fn(goja.Undefined(), h.ctx.ctxArg)
	})
	if err != nil {
		return fmt.Errorf("script: run scheduled job: %w", err)
	}
	return nil
}

// NewBrokenHandler returns a broken handler for s: it runs nothing, has no
// jobs, and reports err as its Error. Error wraps err and marshals to the
// API's {code,msg} shape, with code 422: the script can't be processed.
func NewBrokenHandler(s *Script, err error) *Handler {
	return &Handler{script: s, Error: &handlerError{err: err}}
}

// handlerError is a broken handler's Error.
type handlerError struct {
	err error
}

func (e *handlerError) Error() string { return e.err.Error() }

func (e *handlerError) Unwrap() error { return e.err }

// MarshalJSON renders e as an httputil.Err.
func (e *handlerError) MarshalJSON() ([]byte, error) {
	return json.Marshal(httputil.Err{Code: http.StatusUnprocessableEntity, Msg: e.err.Error()})
}
