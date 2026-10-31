package scripts

import "time"

// debouncer schedules one sync per name, once the name has been quiet for
// delay. Its methods are for one goroutine (the watcher's loop): touch on
// every event, and take on every firing received from due. A touch supersedes
// the name's earlier timer even when that timer already fired and its firing
// is waiting on due, so take reports only the latest timer of each name.
type debouncer struct {
	delay time.Duration
	// due receives the firing of a name's timer. It is unbuffered, so a fired
	// timer waits for the loop to receive it, or for done.
	due  chan firing
	done <-chan struct{}

	// pending holds each name's latest timer that has not been taken.
	pending map[string]pendingTimer
	// gen numbers timers, so a superseded firing is told apart.
	gen uint64
}

// firing is a name's timer having fired.
type firing struct {
	name string
	gen  uint64
}

type pendingTimer struct {
	timer *time.Timer
	gen   uint64
}

// newDebouncer returns a debouncer whose fired timers stop waiting on due once
// done is closed.
func newDebouncer(delay time.Duration, done <-chan struct{}) *debouncer {
	return &debouncer{
		delay:   delay,
		due:     make(chan firing),
		done:    done,
		pending: make(map[string]pendingTimer),
	}
}

// touch (re)starts name's timer, superseding any earlier one, fired or not.
func (d *debouncer) touch(name string) {
	if p, ok := d.pending[name]; ok {
		p.timer.Stop()
	}
	d.gen++
	f := firing{name: name, gen: d.gen}
	d.pending[name] = pendingTimer{
		gen: f.gen,
		timer: time.AfterFunc(d.delay, func() {
			select {
			case d.due <- f:
			case <-d.done:
			}
		}),
	}
}

// take reports whether f is its name's latest timer, which is then no longer
// pending. A superseded firing reports false.
func (d *debouncer) take(f firing) bool {
	if p, ok := d.pending[f.name]; !ok || p.gen != f.gen {
		return false
	}
	delete(d.pending, f.name)
	return true
}

// stop cancels every pending timer. One that already fired stops waiting once
// done is closed.
func (d *debouncer) stop() {
	for _, p := range d.pending {
		p.timer.Stop()
	}
}
