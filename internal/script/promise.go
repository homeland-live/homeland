package script

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/dop251/goja"
)

// trackRejection is the runtime's promise rejection tracker. It records a
// promise rejected with no handler attached, and forgets it once one is
// attached, so settle sees only the rejections nothing handled. goja calls it
// on the runtime's goroutine, during a run.
func (ctx *Context) trackRejection(p *goja.Promise, op goja.PromiseRejectionOperation) {
	switch op {
	case goja.PromiseRejectionReject:
		ctx.unhandled = append(ctx.unhandled, p)
	case goja.PromiseRejectionHandle:
		ctx.unhandled = slices.DeleteFunc(ctx.unhandled, func(q *goja.Promise) bool { return q == p })
	}
}

// settle returns the error a run's promises amount to, ret being what the
// run's function returned. goja runs every queued promise job before a call
// returns, and there is no event loop, so whatever is still pending then
// never settles. The error joins, in order:
//   - the rejection of a returned promise, or that it never settled;
//   - each other promise rejected with no handler, as "unhandled promise
//     rejection".
//
// Rendering a rejection reason can run script code (a custom toString or
// stack getter), so a throw or timeout there is returned as the error.
func (ctx *Context) settle(ret goja.Value) error {
	unhandled := ctx.unhandled
	var errs []error
	if err := ctx.try(func() {
		if p, ok := exportPromise(ret); ok {
			switch p.State() {
			case goja.PromiseStateRejected:
				errs = append(errs, rejectionError(p.Result()))
				unhandled = slices.DeleteFunc(unhandled, func(q *goja.Promise) bool { return q == p })
			case goja.PromiseStatePending:
				errs = append(errs, errors.New("returned a promise that never settled"))
			}
		}
		for _, p := range unhandled {
			errs = append(errs, fmt.Errorf("unhandled promise rejection: %w", rejectionError(p.Result())))
		}
	}); err != nil {
		return err
	}
	return errors.Join(errs...)
}

// promiseType is what a promise exports to.
var promiseType = reflect.TypeFor[*goja.Promise]()

// exportPromise returns v as a promise, when it is one. Only a promise is
// exported: exporting another object copies it deeply and runs its getters.
func exportPromise(v goja.Value) (*goja.Promise, bool) {
	if v == nil || v.ExportType() != promiseType {
		return nil, false
	}
	p, ok := v.Export().(*goja.Promise)
	return p, ok
}

// rejectionError renders a rejection reason the way goja renders a thrown
// exception: its string form, followed by " at <frame>" for an error whose
// stack names where it was thrown.
func rejectionError(reason goja.Value) error {
	msg := reason.String()
	if obj, ok := reason.(*goja.Object); ok {
		if stack := obj.Get("stack"); stack != nil && !goja.IsUndefined(stack) {
			lines := strings.SplitN(stack.String(), "\n", 3)
			if len(lines) > 1 {
				if frame, ok := strings.CutPrefix(lines[1], "\tat "); ok {
					msg += " at " + frame
				}
			}
		}
	}
	return errors.New(msg)
}
