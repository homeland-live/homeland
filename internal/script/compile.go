package script

import (
	"errors"
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
)

// A script runs as the body of a strict-mode function, CommonJS style: its
// declarations stay local to the function, and only what it assigns to
// module.exports reaches the host. The prefix has no newline, so line numbers
// in errors match the script file (only columns on line 1 shift).
const (
	modulePrefix = "(function (module, exports) {'use strict';"
	moduleSuffix = "\n})"
)

// Compile evaluates the script as a module and returns its Handler. A valid
// script sets module.exports to { handle, jobs } with a handle function, at
// least one job, or both (see readExports). z2m and mqtt back ctx.z2m and
// ctx.mqtt; a nil one makes its API throw in the script.
//
// Script-author mistakes (a syntax error, a throw, timeout or unhandled
// promise rejection at top level, or invalid exports, including an invalid
// cron expression) yield a broken handler with Handler.Error set, not an
// error. The returned error is reserved for host-side setup failures.
//
// timeout bounds the top-level evaluation and every later Handle and RunJob
// call; <= 0 disables it.
func Compile(script *Script, z2m Z2M, mqtt MQTT, timeout time.Duration) (*Handler, error) {
	ctx, err := newContext(script, z2m, mqtt)
	if err != nil {
		return nil, err
	}

	broken := func(err error) *Handler {
		return NewBrokenHandler(script, fmt.Errorf("script: compile: %w", err))
	}

	prg, err := compileModule(script)
	if err != nil {
		return broken(err), nil
	}

	// Exports are read once, so changing module.exports later has no effect.
	// Reading them can run script code (a getter or a Proxy), so it shares the
	// evaluation's run (timeout, panic guard and promise check).
	var handle goja.Callable
	err = ctx.run(timeout, "top-level evaluation", func() (goja.Value, error) {
		exports, err := ctx.runModule(prg)
		if err != nil {
			return nil, err
		}
		// Try turns a throw from script code run by the read into an error.
		if ex := ctx.runtime.Try(func() { handle, ctx.jobs, err = readExports(exports) }); ex != nil {
			return nil, ex
		}
		return nil, err
	})
	if err != nil {
		return broken(err), nil
	}

	return &Handler{
		script:    script,
		ctx:       ctx,
		handle:    handle,
		timeout:   timeout,
		HasHandle: handle != nil,
		JobCount:  len(ctx.jobs),
	}, nil
}

// compileModule parses the script wrapped as a module function and compiles
// it. The wrapped source must parse to that one function: a stray "})" in the
// script would otherwise close the wrapper early and run the rest at global
// scope.
func compileModule(script *Script) (*goja.Program, error) {
	prg, err := goja.Parse(script.FileName, modulePrefix+script.Content+moduleSuffix)
	if err != nil {
		return nil, err
	}
	if !isSingleFunction(prg) {
		return nil, errors.New("unbalanced brackets: a closing } or ) ends the script early")
	}
	return goja.CompileAST(prg, true)
}

// isSingleFunction reports whether prg is exactly one function expression.
func isSingleFunction(prg *ast.Program) bool {
	if len(prg.Body) != 1 {
		return false
	}
	stmt, ok := prg.Body[0].(*ast.ExpressionStatement)
	if !ok {
		return false
	}
	_, ok = stmt.Expression.(*ast.FunctionLiteral)
	return ok
}

// runModule runs the compiled module function with fresh module and exports
// objects and returns module.exports.
func (ctx *Context) runModule(prg *goja.Program) (goja.Value, error) {
	fnVal, err := ctx.runtime.RunProgram(prg)
	if err != nil {
		return nil, err
	}
	fn, ok := goja.AssertFunction(fnVal)
	if !ok {
		return nil, errors.New("module wrapper did not evaluate to a function")
	}

	module := ctx.runtime.NewObject()
	exports := ctx.runtime.NewObject()
	if err := module.Set("exports", exports); err != nil {
		return nil, err
	}
	if _, err := fn(goja.Undefined(), module, exports); err != nil {
		return nil, err
	}
	return module.Get("exports"), nil
}
