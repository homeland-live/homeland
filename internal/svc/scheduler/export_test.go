package scheduler

import (
	"fmt"

	"github.com/go-co-op/gocron/v2"
)

// RunNow queues one run of every job of devID with gocron, without changing
// its schedule, and returns without waiting for the runs. Runs follow the
// usual rules (skipped while the device is in conflict or while the job's
// previous run is in progress). It returns an error when the device has no
// jobs, else the first error gocron reports while queueing a run.
func (svc *Service) RunNow(devID string) error {
	svc.mu.Lock()
	jobs := append([]gocron.Job(nil), svc.jobs[devID]...)
	svc.mu.Unlock()

	if len(jobs) == 0 {
		return fmt.Errorf("svc/scheduler: no jobs registered for %q", devID)
	}

	var firstErr error
	for _, job := range jobs {
		if err := job.RunNow(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
