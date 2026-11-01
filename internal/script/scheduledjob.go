package script

import "github.com/dop251/goja"

// ScheduledJob is a job a script exports in module.exports.jobs. The callback
// is unexported and belongs to the handler that compiled it, so it runs only
// through that handler's RunJob, which refuses any other job.
type ScheduledJob struct {
	// CronExpr is the job's cron expression, validated by Compile.
	CronExpr string
	fn       goja.Callable
	// owner is the handler whose Jobs returned this job, only compared by
	// RunJob; nil for a hand-built job.
	owner *Handler
}
