package script

import "github.com/dop251/goja"

// ScheduledJob is a job a script exports in module.exports.jobs. The callback
// is unexported so it can only run through Handler.RunJob, on its handler's
// runtime.
type ScheduledJob struct {
	// CronExpr is the job's cron expression, validated by Compile.
	CronExpr string
	fn       goja.Callable
}
