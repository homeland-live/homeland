package db_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/db"
)

// countEvents returns how many events are stored for device, or -1 when the
// query fails. It doesn't assert, so it is safe inside Eventually conditions,
// which run off the test goroutine.
func countEvents(_ *testing.T, s *db.Repo, device string) int {
	p := &db.Page{Num: 1, Size: 1}
	if _, err := s.QueryEvents(device, p); err != nil {
		return -1
	}
	return p.Count
}

func TestEnqueueEventIsWrittenAsync(t *testing.T) {
	s := openTemp(t)
	require.NoError(t, s.EnqueueEvent(event(t, "lamp", `{"n":1}`)))

	// Written by the background event writer within a flush interval or so.
	assert.Eventually(t, func() bool { return countEvents(t, s, "lamp") == 1 },
		2*time.Second, 10*time.Millisecond)
}

func TestEnqueueEventKeepsArrivalOrder(t *testing.T) {
	s := openTemp(t)
	const n = 250 // spans several batches
	base := time.Now().UTC()
	for i := range n {
		require.NoError(t, s.EnqueueEvent(eventAt(t, "lamp", fmt.Sprintf(`{"n":%d}`, i), base.Add(time.Duration(i)*time.Millisecond))))
	}
	require.Eventually(t, func() bool { return countEvents(t, s, "lamp") == n },
		3*time.Second, 10*time.Millisecond)

	latest, err := s.QueryEvents("lamp", &db.Page{Num: 1, Size: 3})
	require.NoError(t, err)
	require.Len(t, latest, 3)
	for i, e := range latest {
		assert.Equal(t, n-1-i, nFromEvent(t, e), "latest first")
	}
}

func TestCloseFlushesQueuedEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flush.db")
	s, err := db.New(&config.DB{Path: path})
	require.NoError(t, err)
	const n = 150
	for i := range n {
		require.NoError(t, s.EnqueueEvent(event(t, "lamp", fmt.Sprintf(`{"n":%d}`, i))))
	}
	s.Close() // must write everything still queued

	assert.Equal(t, n, countEvents(t, openAt(t, path), "lamp"))
}

func TestEnqueueEventStampsUTCTime(t *testing.T) {
	s := openTemp(t)
	e := &db.Event{IEEEAddress: "lamp", Payload: map[string]json.RawMessage{"n": json.RawMessage(`1`)}}
	require.NoError(t, s.EnqueueEvent(e))
	assert.NotEmpty(t, e.ID)
	assert.Equal(t, time.UTC, e.CreatedAt.Location())
}

func TestEnqueueEventRejects(t *testing.T) {
	s, err := db.New(&config.DB{Path: filepath.Join(t.TempDir(), "reject.db")})
	require.NoError(t, err)

	require.Error(t, s.EnqueueEvent(event(t, "", `{"n":1}`)), "an event without IEEE address is invalid")
	s.Close()
	assert.Error(t, s.EnqueueEvent(event(t, "lamp", `{"n":1}`)), "a closed repo accepts no events")
}

// TestEnqueueEventRacesClose pins the teardown contract under -race: enqueues
// racing Close either succeed (and are written) or return an error; none
// panics on the closed queue.
func TestEnqueueEventRacesClose(t *testing.T) {
	s, err := db.New(&config.DB{Path: filepath.Join(t.TempDir(), "race.db")})
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				// Built inline: helpers assert, which must stay on the test
				// goroutine. Errors after Close are expected.
				_ = s.EnqueueEvent(&db.Event{
					IEEEAddress: "lamp",
					Payload:     map[string]json.RawMessage{"n": json.RawMessage(`1`)},
				})
			}
		})
	}
	s.Close()
	wg.Wait()
}

func TestEventWriterKeepsGoodRowsOfAFailedBatch(t *testing.T) {
	s := openTemp(t)
	taken := event(t, "lamp", `{"n":0}`)
	require.NoError(t, s.CreateEvent(taken))

	// A duplicate primary key fails the multi-row INSERT; the row-by-row
	// retry must still store the batch's other events.
	dup := event(t, "lamp", `{"n":1}`)
	dup.ID = taken.ID
	require.NoError(t, s.EnqueueEvent(dup))
	require.NoError(t, s.EnqueueEvent(event(t, "lamp", `{"n":2}`)))
	require.NoError(t, s.EnqueueEvent(event(t, "lamp", `{"n":3}`)))

	// The writer stores the good rows before it records the loss, so wait
	// for the loss.
	require.Eventually(t, func() bool { return !s.HealthCheck().IsHealthy },
		3*time.Second, 10*time.Millisecond, "a lost event must make the db unhealthy")
	assert.Contains(t, s.HealthCheck().Error, "lost 1 event(s)")
	assert.Equal(t, 3, countEvents(t, s, "lamp"))

	// A later fully written batch clears it.
	require.NoError(t, s.EnqueueEvent(event(t, "lamp", `{"n":4}`)))
	assert.Eventually(t, func() bool { return s.HealthCheck().IsHealthy },
		3*time.Second, 10*time.Millisecond)
	assert.Equal(t, 4, countEvents(t, s, "lamp"))
}

// countInserts counts the INSERTs s attempts from now on.
func countInserts(t *testing.T, s *db.Repo) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	require.NoError(t, s.OnInsert(func() { n.Add(1) }))
	return &n
}

func TestEventWriterGivesUpOnAFailingDatabase(t *testing.T) {
	s := openTemp(t)
	require.NoError(t, s.ExecSQL("DROP TABLE events"))
	inserts := countInserts(t, s)

	// Enqueued well within a flush interval, so they form one batch.
	for i := range 10 {
		require.NoError(t, s.EnqueueEvent(event(t, "lamp", fmt.Sprintf(`{"n":%d}`, i))))
	}

	// Every write fails: the writer must report it rather than drop silently.
	require.Eventually(t, func() bool { return !s.HealthCheck().IsHealthy },
		3*time.Second, 10*time.Millisecond)
	assert.Contains(t, s.HealthCheck().Error, "event writer lost 10 event(s)")
	assert.Equal(t, int64(1+3), inserts.Load(),
		"the batch, then rows until 3 fail in a row; the rest are given up untried")
	assert.Equal(t, -1, countEvents(t, s, "lamp"), "nothing can be stored without the table")
}

func TestEventWriterRetriesEveryRowBetweenSuccesses(t *testing.T) {
	s := openTemp(t)
	// Four events whose ids the batch below reuses, so those rows fail.
	taken := make([]*db.Event, 4)
	for i := range taken {
		taken[i] = event(t, "lamp", fmt.Sprintf(`{"taken":%d}`, i))
		require.NoError(t, s.CreateEvent(taken[i]))
	}
	inserts := countInserts(t, s)

	// Failing rows alternate with good ones, so failures never run 3 in a
	// row and every row is tried.
	for i, e := range taken {
		dup := event(t, "lamp", fmt.Sprintf(`{"dup":%d}`, i))
		dup.ID = e.ID
		require.NoError(t, s.EnqueueEvent(dup))
		if i < len(taken)-1 {
			require.NoError(t, s.EnqueueEvent(event(t, "lamp", fmt.Sprintf(`{"good":%d}`, i))))
		}
	}

	require.Eventually(t, func() bool { return !s.HealthCheck().IsHealthy },
		3*time.Second, 10*time.Millisecond)
	assert.Contains(t, s.HealthCheck().Error, "event writer lost 4 event(s)")
	assert.Equal(t, int64(1+7), inserts.Load(), "the batch, then each of its 7 rows")
	assert.Equal(t, 4+3, countEvents(t, s, "lamp"), "every good row is stored")
}
