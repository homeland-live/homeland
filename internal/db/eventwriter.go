package db

import (
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	// eventQueueSize bounds the events waiting to be written. A full queue
	// makes EnqueueEvent block, so the caller slows down rather than losing
	// events.
	eventQueueSize = 1024
	// eventBatchSize is the most events written in one transaction.
	eventBatchSize = 100
	// eventFlushInterval is the longest an event waits in a partial batch.
	eventFlushInterval = 250 * time.Millisecond
	// maxRowFailures is how many consecutive rows may fail in writeEventBatch's
	// row-by-row retry before it gives up on the rest of the batch. It bounds
	// how long a failing database (each row may wait out busy_timeout) holds
	// up the queue.
	maxRowFailures = 3
)

// EnqueueEvent stamps e (see Event.stamp), validates it, and queues it for
// the background event writer, which stores events in arrival order, in
// batches. It returns before e is stored: a crash can lose events still
// queued (at most about eventFlushInterval's worth under normal load), and a
// query may not see e for up to eventFlushInterval. It blocks only when the
// queue is full. Invalid events and calls after Close return an error.
func (r *Repo) EnqueueEvent(e *Event) error {
	if err := r.ready(); err != nil {
		return err
	}
	if err := e.stamp(); err != nil {
		return err
	}
	if err := r.db.Validate(e); err != nil {
		return fmt.Errorf("db: invalid event for %q: %w", e.IEEEAddress, err)
	}

	r.eventQueueMu.RLock()
	defer r.eventQueueMu.RUnlock()
	if r.closed.Load() {
		return errors.New("db: closed")
	}
	r.eventQueue <- e
	return nil
}

// startEventWriter starts the goroutine that drains the event queue.
func (r *Repo) startEventWriter() {
	r.eventQueue = make(chan *Event, eventQueueSize)
	r.eventWriterWG.Go(r.eventWriteLoop)
}

// stopEventWriter stops accepting events, then waits until the event queue is
// drained. It must run after closed is set and before the database is closed.
func (r *Repo) stopEventWriter() {
	r.eventQueueMu.Lock()
	close(r.eventQueue)
	r.eventQueueMu.Unlock()
	r.eventWriterWG.Wait()
}

// eventWriteLoop waits for an event, collects more until eventBatchSize or
// eventFlushInterval, and writes the batch, until the queue is closed and
// empty.
func (r *Repo) eventWriteLoop() {
	for first := range r.eventQueue {
		batch := []*Event{first}
		timer := time.NewTimer(eventFlushInterval)
	collect:
		for len(batch) < eventBatchSize {
			select {
			case e, ok := <-r.eventQueue:
				if !ok {
					break collect
				}
				batch = append(batch, e)
			case <-timer.C:
				break collect
			}
		}
		timer.Stop()
		r.writeEventBatch(batch)
	}
}

// writeFailure is the event writer's last unrecovered write, reported by
// HealthCheck.
type writeFailure struct {
	lost int
	err  error
}

// writeEventBatch stores batch with one multi-row INSERT. When that fails it
// retries row by row, so one bad row costs only itself and a transient error
// gets a second chance. After maxRowFailures consecutive row failures the
// database itself is failing, so the rest of the batch is given up. Lost
// events are logged and reported by HealthCheck until a later batch is fully
// written.
func (r *Repo) writeEventBatch(batch []*Event) {
	err := r.db.Conn().Create(batch).Error
	if err == nil {
		r.lastWriteFailure.Store(nil)
		return
	}
	slog.Warn("db: write event batch failed, retrying row by row", "count", len(batch), "err", err)

	lost, streak := 0, 0
	for i, e := range batch {
		if streak >= maxRowFailures {
			lost += len(batch) - i
			break
		}
		if rowErr := r.db.Conn().Create(e).Error; rowErr != nil {
			lost++
			streak++
			err = rowErr
			continue
		}
		streak = 0
	}

	if lost == 0 {
		r.lastWriteFailure.Store(nil)
		return
	}
	slog.Error("db: write events failed, events lost", "lost", lost, "count", len(batch), "err", err)
	r.lastWriteFailure.Store(&writeFailure{lost: lost, err: err})
}
