package script

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/robfig/cron/v3"
)

// Keys a script may set on module.exports.
const (
	exportHandle = "handle"
	exportJobs   = "jobs"
)

// Keys a job object in module.exports.jobs may have.
const (
	jobCron = "cron"
	jobRun  = "run"
)

// readExports validates module.exports and returns its handle function (nil
// when not exported) and its jobs. It accepts an object with an optional
// handle function and an optional jobs array, at least one of the two, and no
// other keys, so a typo such as "job" is an error rather than ignored.
func readExports(v goja.Value) (goja.Callable, []ScheduledJob, error) {
	obj, ok := plainObject(v)
	if !ok {
		return nil, nil, errors.New("module.exports must be an object, e.g. module.exports = { handle, jobs }")
	}
	if err := checkKeys("module.exports", obj, exportHandle, exportJobs); err != nil {
		return nil, nil, err
	}

	var handle goja.Callable
	if hv := obj.Get(exportHandle); isSet(hv) {
		if handle, ok = goja.AssertFunction(hv); !ok {
			return nil, nil, errors.New("module.exports.handle must be a function")
		}
	}

	jobs, err := readJobs(obj.Get(exportJobs))
	if err != nil {
		return nil, nil, err
	}

	if handle == nil && len(jobs) == 0 {
		return nil, nil, errors.New("module.exports must export a handle function, at least one job, or both")
	}
	return handle, jobs, nil
}

// MaxJobs is the most jobs one script may export.
const MaxJobs = 20

// readJobs validates module.exports.jobs: an array of at most MaxJobs
// { cron, run } objects, each with a cron expression the scheduler accepts
// and a run function. Undefined means no jobs.
func readJobs(v goja.Value) ([]ScheduledJob, error) {
	if !isSet(v) {
		return nil, nil
	}
	arr, ok := v.(*goja.Object)
	if !ok || arr.ClassName() != "Array" {
		return nil, errors.New("module.exports.jobs must be an array of { cron, run } objects")
	}

	// Check length before allocating: the script controls it, and a sparse
	// array can claim up to 2^32-1 elements.
	n := arr.Get("length").ToInteger()
	if n > MaxJobs {
		return nil, fmt.Errorf("module.exports.jobs has %d jobs; at most %d are allowed", n, MaxJobs)
	}
	jobs := make([]ScheduledJob, 0, n)
	for i := range n {
		where := fmt.Sprintf("module.exports.jobs[%d]", i)
		job, ok := plainObject(arr.Get(strconv.FormatInt(i, 10)))
		if !ok {
			return nil, fmt.Errorf("%s must be a { cron, run } object", where)
		}
		if err := checkKeys(where, job, jobCron, jobRun); err != nil {
			return nil, err
		}

		var expr string
		if cv := job.Get(jobCron); isSet(cv) {
			expr, _ = cv.Export().(string)
		}
		if strings.TrimSpace(expr) == "" {
			return nil, fmt.Errorf("%s.cron must be a cron expression string", where)
		}
		if err := validateCron(expr); err != nil {
			return nil, fmt.Errorf("%s.cron %q is invalid: %w", where, expr, err)
		}

		run, ok := goja.AssertFunction(job.Get(jobRun))
		if !ok {
			return nil, fmt.Errorf("%s.run must be a function", where)
		}

		jobs = append(jobs, ScheduledJob{CronExpr: expr, fn: run})
	}
	return jobs, nil
}

// validateCron parses expr the way the scheduler does (standard 5-field
// syntax or a descriptor, optionally TZ=/CRON_TZ= prefixed) and rejects one
// that never fires.
func validateCron(expr string) error {
	sched, err := cron.ParseStandard(expr)
	if err != nil {
		return err
	}
	if sched.Next(time.Now()).IsZero() {
		return errors.New("it never fires")
	}
	return nil
}

// plainObject returns v as an object when it is one that is neither a
// function nor an array.
func plainObject(v goja.Value) (*goja.Object, bool) {
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil, false
	}
	if _, isFn := goja.AssertFunction(obj); isFn || obj.ClassName() == "Array" {
		return nil, false
	}
	return obj, true
}

// checkKeys returns an error naming obj's keys outside allowed.
func checkKeys(where string, obj *goja.Object, allowed ...string) error {
	var unknown []string
	for _, k := range obj.Keys() {
		if !slices.Contains(allowed, k) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return fmt.Errorf("%s has unknown key(s) %s; allowed: %s",
		where, strings.Join(unknown, ", "), strings.Join(allowed, ", "))
}

// isSet reports whether v holds a value other than undefined.
func isSet(v goja.Value) bool {
	return v != nil && !goja.IsUndefined(v)
}
