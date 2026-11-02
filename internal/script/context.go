package script

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"runtime/debug"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// MaxCallDepth is how deeply a script's calls may nest, counting the module
// wrapper, handle or run, and built-ins that call back (forEach, map). It
// leaves ample room for helper functions but stops recursion early: scripts
// are meant to be simple, and runaway recursion would otherwise grow memory
// until the run times out. Exceeding it fails the run; try/catch can't catch
// it.
const MaxCallDepth = 64

// Context is the goja-side state of one compiled script: its runtime, the ctx
// object passed to handle and job callbacks, and the jobs read from
// module.exports. Runs into it (Compile's evaluation, then Handle and RunJob
// under the Handler's mutex) never overlap.
type Context struct {
	script  *Script
	runtime *goja.Runtime
	ctxArg  goja.Value
	z2m     *z2mSDK
	mqtt    *mqttSDK

	// timedOut is closed when the current run times out, waking ctx.sleep;
	// nil (never ready) when the run has no timeout.
	timedOut chan struct{}

	// unhandled holds the promises the current run rejected with no handler
	// attached yet (see trackRejection); run clears it.
	unhandled []*goja.Promise

	// jobs is set once by Compile and never changes afterwards.
	jobs []ScheduledJob
}

// recoverPanic, deferred around a run, turns a Go panic escaping it into
// *err and logs the stack. goja converts JS exceptions into errors itself;
// what remains is a host bug (e.g. in a native ctx method), which must fail
// the run, not crash the process.
func (ctx *Context) recoverPanic(what string, err *error) {
	r := recover()
	if r == nil {
		return
	}
	slog.Error("script: recovered panic", "script", ctx.script.FileName, "run", what,
		"panic", r, "stack", string(debug.Stack()))
	*err = fmt.Errorf("%s panicked: %v", what, r)
}

// try runs fn, which may call into script code, and returns what the script
// code throws as an error. Unlike Runtime.Try, it also returns an interrupt
// (the run timing out) or a stack overflow as an error rather than panicking:
// goja re-panics these uncatchable exceptions out of Try, and recoverPanic
// would report them as a host bug. Any other panic propagates.
func (ctx *Context) try(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			switch ex := r.(type) {
			case *goja.InterruptedError:
				err = ex
			case *goja.StackOverflowError:
				err = ex
			default:
				panic(r)
			}
		}
	}()
	if ex := ctx.runtime.Try(fn); ex != nil {
		return ex
	}
	return nil
}

// run runs fn as one run named what: bounded by timeout (see withTimeout),
// with a Go panic recovered (see recoverPanic), and failing when fn returns a
// promise that rejected or never settled, or leaves any promise rejected
// without a handler (see settle). Rejections never carry over to the next
// run. The caller holds the Handler's mutex, or owns ctx (Compile).
func (ctx *Context) run(timeout time.Duration, what string, fn func() (goja.Value, error)) error {
	return ctx.withTimeout(timeout, what, func() (err error) {
		defer ctx.recoverPanic(what, &err)
		defer func() { ctx.unhandled = nil }()

		ret, err := fn()
		if err != nil {
			return describeOverflow(err)
		}
		return describeOverflow(ctx.settle(ret))
	})
}

// describeOverflow turns a stack overflow, which goja reports with only its
// innermost frame (possibly a built-in), into a callDepthError. Other errors
// are returned as is.
func describeOverflow(err error) error {
	if so, ok := errors.AsType[*goja.StackOverflowError](err); ok {
		return &callDepthError{overflow: so}
	}
	return err
}

// callDepthError is a run that exceeded MaxCallDepth. It names the innermost
// script frame, so the author sees which function recursed.
type callDepthError struct {
	overflow *goja.StackOverflowError
}

func (e *callDepthError) Error() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "call stack exceeded %d nested calls; recursion is not supported", MaxCallDepth)
	for _, frame := range e.overflow.Stack() {
		if frame.SrcName() != "<native>" {
			b.WriteString(" at ")
			frame.Write(&b)
			break
		}
	}
	return b.String()
}

func (e *callDepthError) Unwrap() error { return e.overflow }

// withTimeout runs fn, bounded by timeout (<= 0 disables it), and returns its
// error. On timeout the runtime is interrupted with "<what> timed out after
// <timeout>" and a pending ctx.sleep wakes, so fn fails promptly. The
// interrupt is cleared before withTimeout returns, after waiting for a timer
// that is already firing, so it never reaches the next run.
func (ctx *Context) withTimeout(timeout time.Duration, what string, fn func() error) error {
	if timeout <= 0 {
		return fn()
	}

	timedOut := make(chan struct{})
	fired := make(chan struct{})
	ctx.timedOut = timedOut
	timer := time.AfterFunc(timeout, func() {
		defer close(fired)
		ctx.runtime.Interrupt(fmt.Sprintf("%s timed out after %s", what, timeout))
		close(timedOut)
	})
	defer func() {
		if !timer.Stop() {
			<-fired
		}
		ctx.runtime.ClearInterrupt()
		ctx.timedOut = nil
	}()

	return fn()
}

// log implements ctx.log(...): it logs the arguments, space-separated, at Info
// through slog, tagged with the script name, so LOG_LEVEL applies.
func (ctx *Context) log(call goja.FunctionCall) goja.Value {
	args := make([]string, len(call.Arguments))
	for i, a := range call.Arguments {
		args[i] = formatLogArg(a)
	}
	slog.Info("script: log", "script", ctx.script.FileName, "text", strings.Join(args, " "))
	return goja.Undefined()
}

// formatLogArg renders a ctx.log argument the way console.log would: strings
// verbatim, errors as "Name: message", objects and arrays as JSON (without
// escaping &, < and >), everything else in its JS form.
func formatLogArg(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if goja.IsNull(v) {
		return "null"
	}
	// An Error's props aren't enumerable, so as JSON it would be {}.
	if obj, ok := v.(*goja.Object); ok && obj.ClassName() == "Error" {
		return v.String()
	}
	switch exp := v.Export().(type) {
	case string:
		return exp
	case map[string]any, []any:
		var b strings.Builder
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(exp); err == nil {
			return strings.TrimSuffix(b.String(), "\n")
		}
	}
	return v.String()
}

// sleep implements ctx.sleep(ms). It blocks the calling goroutine until ms
// elapse or the run times out, whichever comes first; on timeout the run then
// fails with the interrupt. An argument that does not convert to a positive
// number returns immediately.
func (ctx *Context) sleep(call goja.FunctionCall) goja.Value {
	ms := call.Argument(0).ToInteger()
	if ms <= 0 {
		return goja.Undefined()
	}
	d := time.Duration(math.MaxInt64)
	if ms < int64(math.MaxInt64/time.Millisecond) {
		d = time.Duration(ms) * time.Millisecond
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.timedOut:
	}
	return goja.Undefined()
}

// install builds the ctx object (log, sleep, z2m, mqtt) passed
// to handle and job callbacks. Nothing is installed as a global. The SDK
// structs' exported methods are bound by goja and lower-cased by the runtime's
// field-name mapper; a returned Go error is thrown as a JS exception.
func (ctx *Context) install() error {
	obj := ctx.runtime.NewObject()
	if err := obj.Set("log", ctx.log); err != nil {
		return fmt.Errorf("script: install ctx.log: %w", err)
	}
	if err := obj.Set("sleep", ctx.sleep); err != nil {
		return fmt.Errorf("script: install ctx.sleep: %w", err)
	}
	if err := obj.Set("z2m", ctx.z2m); err != nil {
		return fmt.Errorf("script: install ctx.z2m: %w", err)
	}
	if err := obj.Set("mqtt", ctx.mqtt); err != nil {
		return fmt.Errorf("script: install ctx.mqtt: %w", err)
	}
	ctx.ctxArg = obj
	return nil
}

// newContext returns a Context for script, with a fresh runtime and ctx
// installed. A nil z2m or mqtt makes the corresponding API throw "not
// available".
func newContext(script *Script, z2m Z2M, mqtt MQTT) (*Context, error) {
	ctx := &Context{
		script:  script,
		runtime: goja.New(),
		z2m:     &z2mSDK{client: z2m},
		mqtt:    &mqttSDK{client: mqtt},
	}

	// Must be set before any Go struct is handed to the runtime.
	ctx.runtime.SetFieldNameMapper(goja.UncapFieldNameMapper())
	ctx.runtime.SetMaxCallStackSize(MaxCallDepth)
	ctx.runtime.SetPromiseRejectionTracker(ctx.trackRejection)

	if err := ctx.install(); err != nil {
		return nil, err
	}
	return ctx, nil
}
