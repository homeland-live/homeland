// Package scheduler runs the cron jobs device scripts export in
// module.exports.jobs.
package scheduler

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"

	"github.com/homeland-live/homeland/internal/script"
	"github.com/homeland-live/homeland/internal/svc/scripts"
)

// Scripts is the part of svc/scripts the scheduler uses. Satisfied by
// *scripts.Service.
type Scripts interface {
	// Subscribe registers fn for every handler change, including the initial
	// set.
	Subscribe(fn scripts.ChangeFunc)
	// InConflict reports whether devID's device also has a script under its
	// other id, so neither script may run.
	InConflict(devID string) bool
}

// Service keeps gocron jobs in sync with the handlers svc/scripts reports. Each
// job runs through Handler.RunJob, so it never overlaps with event handling on
// the same handler, and at most one run of a job is in progress: a tick that
// fires while the previous run is still going is skipped.
type Service struct {
	scripts Scripts

	sched gocron.Scheduler

	// mu serializes Sync and Shutdown, held across the gocron calls, so a
	// device's jobs are replaced in one step and none is registered after
	// Shutdown. It also guards jobs and stopped.
	mu sync.Mutex
	// jobs maps a device id to its registered jobs.
	jobs map[string][]gocron.Job
	// stopped makes Sync a no-op after Shutdown.
	stopped bool
}

// New builds the gocron scheduler and subscribes Sync to svc/scripts. Call it
// before svc/scripts.Start so the initial handler set is registered, then
// Start to run jobs.
func New(scripts Scripts) (*Service, error) {
	if scripts == nil {
		return nil, errors.New("svc/scheduler: script service is required, but missing")
	}

	// time.Local follows TZ; a cron expression may override it with a
	// TZ=/CRON_TZ= prefix.
	sched, err := gocron.NewScheduler(
		gocron.WithLocation(time.Local),
		gocron.WithMonitor(monitor{}),
	)
	if err != nil {
		return nil, fmt.Errorf("svc/scheduler: create scheduler: %w", err)
	}

	svc := &Service{
		scripts: scripts,
		sched:   sched,
		jobs:    make(map[string][]gocron.Job),
	}

	svc.scripts.Subscribe(svc.Sync)

	return svc, nil
}

// Start begins running the registered jobs.
func (svc *Service) Start() error {
	svc.sched.Start()
	slog.Info("svc/scheduler: started", "jobs", svc.JobCount())
	return nil
}

// Sync replaces devID's jobs with h's, or removes them when h is nil. It is
// the svc/scripts subscriber and safe for concurrent use: concurrent Syncs
// apply one at a time, each replacing the device's jobs in one step, and Sync
// is a no-op after Shutdown.
func (svc *Service) Sync(devID string, h *script.Handler) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.stopped {
		return
	}
	svc.unregister(devID)
	if h != nil {
		svc.register(devID, h)
	}
}

// register registers h's jobs for devID, which has none. Compile has already
// validated each cron expression; a job gocron still rejects is logged and
// skipped without dropping the device's other jobs. The caller holds mu.
func (svc *Service) register(devID string, h *script.Handler) {
	scriptJobs := h.Jobs()
	if len(scriptJobs) == 0 {
		return
	}

	jobs := make([]gocron.Job, 0, len(scriptJobs))
	for _, j := range scriptJobs {
		job, err := svc.sched.NewJob(
			gocron.CronJob(j.CronExpr, false),
			gocron.NewTask(svc.run(devID, h, j)),
			// A tick that arrives while the previous run is still going is
			// skipped (and logged by monitor), so slow runs don't pile up
			// waiting on the handler.
			gocron.WithSingletonMode(gocron.LimitModeReschedule),
			gocron.WithName(devID),
			gocron.WithTags(j.CronExpr),
		)
		if err != nil {
			slog.Error("svc/scheduler: register job failed, skipping",
				"device", devID, "cron", j.CronExpr, "err", err)
			continue
		}
		jobs = append(jobs, job)
	}
	if len(jobs) == 0 {
		return
	}

	svc.jobs[devID] = jobs
	slog.Info("svc/scheduler: registered jobs", "device", devID, "count", len(jobs))
}

// unregister removes every job registered for devID. The caller holds mu.
func (svc *Service) unregister(devID string) {
	jobs := svc.jobs[devID]
	if len(jobs) == 0 {
		return
	}
	delete(svc.jobs, devID)
	for _, job := range jobs {
		if err := svc.sched.RemoveJob(job.ID()); err != nil {
			slog.Error("svc/scheduler: remove job", "device", devID, "err", err)
		}
	}
	slog.Info("svc/scheduler: unregistered jobs", "device", devID, "count", len(jobs))
}

// run returns the gocron task for job j; script errors are logged and the
// schedule continues. A run is skipped while devID's device also has a script
// under its other id (see scripts.Service.InConflict); the conflict is checked
// on every run, so it follows script changes and device renames.
func (svc *Service) run(devID string, h *script.Handler, j script.ScheduledJob) func() {
	return func() {
		if svc.scripts.InConflict(devID) {
			// Debug: the API and UI already report the conflict.
			slog.Debug("svc/scheduler: skip job, device has two scripts",
				"device", devID, "cron", j.CronExpr)
			return
		}
		if err := h.RunJob(j); err != nil {
			slog.Error("svc/scheduler: scheduled job error",
				"device", devID, "cron", j.CronExpr, "err", err)
		}
	}
}

// JobCount returns the number of registered jobs across all devices.
func (svc *Service) JobCount() int {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	n := 0
	for _, jobs := range svc.jobs {
		n += len(jobs)
	}
	return n
}

// Shutdown stops the scheduler, blocking until running jobs complete. A Sync
// in progress finishes first; later ones are no-ops. Safe to call without
// Start.
func (svc *Service) Shutdown() {
	slog.Info("svc/scheduler: shutting down...")
	svc.mu.Lock()
	svc.stopped = true
	svc.mu.Unlock()
	if err := svc.sched.Shutdown(); err != nil {
		slog.Error("svc/scheduler: gocron", "err", err)
	}
}
