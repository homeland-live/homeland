package scheduler

import (
	"log/slog"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

// monitor is the scheduler's gocron.Monitor. It logs each run singleton mode
// skips; jobs are named by device id and tagged with their cron expression.
type monitor struct{}

// IncrementJob logs a skipped run and ignores every other status.
func (monitor) IncrementJob(_ uuid.UUID, name string, tags []string, status gocron.JobStatus) {
	if status == gocron.SingletonRescheduled {
		slog.Warn("svc/scheduler: skip job run, previous run still in progress",
			"device", name, "cron", strings.Join(tags, " "))
	}
}

// RecordJobTiming is a no-op.
func (monitor) RecordJobTiming(_, _ time.Time, _ uuid.UUID, _ string, _ []string) {}
