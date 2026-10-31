package scripts_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/homeland-live/homeland/internal/svc/scripts"
)

const delay = 100 * time.Millisecond

// newDebouncer returns a debouncer released when the test ends.
func newDebouncer(t *testing.T) *scripts.Debouncer {
	t.Helper()
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	return scripts.NewDebouncer(delay, done)
}

func TestDebouncerFiresOnceQuiet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := newDebouncer(t)
		start := time.Now()

		d.Touch("lamp.js")
		time.Sleep(delay / 2)
		d.Touch("lamp.js") // restarts the delay

		f := d.Next()
		assert.Equal(t, "lamp.js", f.Name)
		assert.Equal(t, delay/2+delay, time.Since(start), "fires one delay after the last touch")
		assert.True(t, d.Take(f))

		time.Sleep(10 * delay)
		synctest.Wait()
		_, fired := d.TryNext()
		assert.False(t, fired, "a burst fires once")
	})
}

func TestDebouncerSupersedesFiredTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := newDebouncer(t)

		d.Touch("lamp.js")
		time.Sleep(delay)
		synctest.Wait() // the timer fired, and its firing waits to be received

		// An event arrives before the loop receives that firing: the file is
		// mid-burst, so the stale firing must not sync it.
		d.Touch("lamp.js")
		assert.False(t, d.Take(d.Next()), "a superseded firing must not sync")

		time.Sleep(delay)
		f := d.Next()
		assert.True(t, d.Take(f), "the latest timer syncs once quiet")

		time.Sleep(10 * delay)
		synctest.Wait()
		_, fired := d.TryNext()
		assert.False(t, fired, "nothing else fires")
	})
}

func TestDebouncerTracksFilesIndependently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := newDebouncer(t)

		d.Touch("lamp.js")
		d.Touch("door.js")
		time.Sleep(delay)
		synctest.Wait()

		names := map[string]bool{}
		for range 2 {
			f := d.Next()
			names[f.Name] = d.Take(f)
		}
		assert.Equal(t, map[string]bool{"lamp.js": true, "door.js": true}, names)
	})
}
