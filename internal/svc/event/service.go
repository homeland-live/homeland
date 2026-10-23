// Package event persists z2m device events and runs the matching device
// script handler for each.
package event

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/homeland-live/homeland/internal/db"
	"github.com/homeland-live/homeland/internal/healthcheck"
	"github.com/homeland-live/homeland/internal/script"
	"github.com/homeland-live/homeland/internal/z2m"
)

// DeviceQueueSize bounds the handler runs waiting on one device's worker. When
// it is full, new updates for that device are dropped rather than blocking
// the MQTT delivery goroutine.
const DeviceQueueSize = 64

// Handlers resolves the handler that runs for a device. Satisfied by
// *scripts.Service.
type Handlers interface {
	// Handler returns the device's handler, or nil when it has none.
	Handler(spec *z2m.DeviceSpec) *script.Handler
}

// Z2M is the source of device updates. Satisfied by *z2m.Client.
type Z2M interface {
	// Subscribe registers fn for every device update that propagates.
	Subscribe(fn z2m.UpdateFunc)
}

// Service subscribes to z2m device updates, queues each one for storage, and
// runs the device's handler on that device's worker goroutine. It does not own
// the z2m client, the script service or the database; the caller does.
type Service struct {
	handlers Handlers
	z2m      Z2M
	db       *db.Repo

	// mu guards stopped, workers, and the wg.Add calls that pair with them.
	mu      sync.Mutex
	stopped bool
	workers map[string]chan run
	// quit is closed by Shutdown; workers exit and drop their queued runs.
	quit chan struct{}
	// wg counts in-flight update deliveries and live workers.
	wg sync.WaitGroup
}

// run is one queued handler run: the handler resolved when the update
// arrived, and the device it arrived for.
type run struct {
	handler *script.Handler
	dev     *z2m.Device
}

// handleDevice is the z2m device-update subscriber that Start registers, so
// it is called on the MQTT delivery goroutine. It queues the event for
// storage (written in arrival order, asynchronously, so the handler may run
// before the event is stored), then queues a run of the device's handler and
// returns without waiting for it. A device without state or without a
// handler has no run. Updates delivered after Shutdown are dropped.
func (svc *Service) handleDevice(dev *z2m.Device) {
	if !svc.enter() {
		return
	}
	defer svc.wg.Done()

	// Debug, not Info: this fires on every update. Events are stored and
	// visible in the API, so Info stays for actions and errors.
	slog.Debug("svc/event: device update", "device", dev.Spec.FriendlyName, "state", dev.State)

	svc.persist(dev)

	if svc.handlers == nil {
		return
	}
	if dev.State == nil {
		slog.Debug("svc/event: ignore event, device has no reported state",
			"device", dev.Spec.FriendlyName,
		)
		return
	}
	handler := svc.handlers.Handler(dev.Spec)
	if handler == nil {
		slog.Debug("svc/event: ignore device event with no handler",
			"device", dev.Spec.FriendlyName,
		)
		return
	}
	svc.enqueue(run{handler: handler, dev: dev})
}

// enqueue hands r to its device's worker, starting the worker on the device's
// first run. Workers are keyed by IEEE address, so a renamed device keeps its
// queue. It never blocks: when the queue is full, r is dropped and logged.
func (svc *Service) enqueue(r run) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.stopped {
		return
	}
	addr := r.dev.Spec.IEEEAddress
	runs, ok := svc.workers[addr]
	if !ok {
		runs = make(chan run, DeviceQueueSize)
		svc.workers[addr] = runs
		svc.wg.Add(1)
		go svc.work(runs)
	}
	select {
	case runs <- r:
	default:
		slog.Warn("svc/event: device queue full, dropping update",
			"device", r.dev.Spec.FriendlyName,
			"queueSize", DeviceQueueSize,
		)
	}
}

// work runs a device's queued handler runs one at a time, in arrival order,
// until Shutdown. Runs still queued at Shutdown are dropped.
func (svc *Service) work(runs <-chan run) {
	defer svc.wg.Done()
	for {
		select {
		case <-svc.quit:
			return
		case r := <-runs:
			// select picks randomly among ready cases, so check quit again
			// to stop promptly instead of draining the queue.
			select {
			case <-svc.quit:
				return
			default:
			}
			if err := r.handler.Handle(r.dev.State.Props()); err != nil {
				slog.Error("svc/event: script error", "device", r.dev.Spec.FriendlyName, "err", err)
			}
		}
	}
}

// persist queues the device's raw state for storage, keyed by IEEE address and
// stamped with the time the state was parsed. Failures are logged, so they
// never stop the handler from running.
func (svc *Service) persist(s *z2m.Device) {
	if svc.db == nil || s.State == nil {
		return
	}
	event := &db.Event{IEEEAddress: s.Spec.IEEEAddress, Payload: s.State.Raw}
	event.CreatedAt = s.State.CreatedAt
	if err := svc.db.EnqueueEvent(event); err != nil {
		slog.Error("svc/event: event persist error", "device", s.Spec.FriendlyName, "err", err)
	}
}

// Start subscribes to the z2m client's device updates. It does not connect the
// broker.
func (svc *Service) Start() error {
	svc.z2m.Subscribe(svc.handleDevice)
	return nil
}

// HealthCheck is unhealthy only when no script service is wired.
func (svc *Service) HealthCheck() healthcheck.HealthCheck {
	if svc.handlers == nil {
		return healthcheck.New("svc/event", fmt.Errorf("svc/event: svc/scripts is not wired"))
	}
	return healthcheck.New("svc/event", nil)
}

// enter registers an in-flight update, or reports false once Shutdown has run.
func (svc *Service) enter() bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.stopped {
		return false
	}
	svc.wg.Add(1)
	return true
}

// Shutdown stops handling updates, drops queued handler runs, and blocks until
// the runs in progress (at most one per device) and any update being queued
// finish; later updates are dropped. Call it before closing what handlers
// use, since disconnecting the broker does not wait for a running handler.
// Safe to call more than once.
func (svc *Service) Shutdown() {
	slog.Info("svc/event: shutting down...")
	svc.mu.Lock()
	if !svc.stopped {
		svc.stopped = true
		close(svc.quit)
	}
	svc.mu.Unlock()
	svc.wg.Wait()
}

// New returns a service over the given dependencies; call Start to subscribe.
func New(handlers Handlers, z2mClient Z2M, repo *db.Repo) *Service {
	return &Service{
		handlers: handlers,
		z2m:      z2mClient,
		db:       repo,
		workers:  make(map[string]chan run),
		quit:     make(chan struct{}),
	}
}
