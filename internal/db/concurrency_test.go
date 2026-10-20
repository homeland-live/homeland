package db_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/db"
)

// rawOpen opens a second, independent connection to the database file, the
// way another process (or another pooled connection) would.
func rawOpen(t *testing.T, path string) *sql.DB {
	t.Helper()
	raw, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	return raw
}

func TestJournalModeWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.db")
	openAt(t, path)

	// journal_mode=WAL is persistent, so any later connection reports it.
	var mode string
	require.NoError(t, rawOpen(t, path).QueryRow("PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "wal", mode)
}

// TestInsertNotBlockedByOpenRead holds a read transaction open on another
// connection and requires an insert to finish meanwhile. Under the old
// rollback journal the insert failed with "database is locked".
func TestInsertNotBlockedByOpenRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "read.db")
	s := openAt(t, path)
	require.NoError(t, s.CreateEvent(event(t, "lamp", `{"n":1}`)))

	tx, err := rawOpen(t, path).Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	var n int
	require.NoError(t, tx.QueryRow("SELECT count(*) FROM events").Scan(&n), "start the read snapshot")

	// Built here: the helper asserts, which must stay on the test goroutine.
	next := event(t, "lamp", `{"n":2}`)
	done := make(chan error, 1)
	go func() { done <- s.CreateEvent(next) }()
	select {
	case err := <-done:
		require.NoError(t, err, "insert must succeed while a read is open")
	case <-time.After(2 * time.Second):
		t.Fatal("insert blocked by an open read")
	}
}

// seed inserts count events for device with a payload big enough that the
// table spans many pages.
func seed(t *testing.T, s *db.Repo, device string, count int) {
	t.Helper()
	pad, err := json.Marshal(strings.Repeat("x", 512))
	require.NoError(t, err)
	for i := range count {
		require.NoError(t, s.CreateEvent(&db.Event{
			IEEEAddress: device,
			Payload: map[string]json.RawMessage{
				"n":   json.RawMessage(fmt.Sprint(i)),
				"pad": pad,
			},
		}), "seed %d", i)
	}
}

func TestPruneByAgeAcrossBatches(t *testing.T) {
	s := openTemp(t)
	old := time.Now().UTC().Add(-48 * time.Hour)
	// More than two prune batches of old events, plus fresh ones that stay.
	const oldCount = 1100
	for i := range oldCount {
		require.NoError(t, s.CreateEvent(eventAt(t, "lamp", `{"n":1}`, old.Add(-time.Duration(i)*time.Second))))
	}
	for range 3 {
		require.NoError(t, s.CreateEvent(event(t, "lamp", `{"n":2}`)))
	}

	deleted, err := s.Prune(config.DBRetention{MaxAge: 24 * time.Hour})
	require.NoError(t, err)
	assert.Equal(t, int64(oldCount), deleted)

	p := &db.Page{Num: 1, Size: 10}
	_, err = s.QueryEvents("lamp", p)
	require.NoError(t, err)
	assert.Equal(t, 3, p.Count)
}

func TestPrunePerDeviceCapAcrossBatches(t *testing.T) {
	s := openTemp(t)
	base := time.Now().UTC().Add(-time.Hour)
	const count, keep = 1100, 5
	for i := range count {
		payload := fmt.Sprintf(`{"n":%d}`, i)
		require.NoError(t, s.CreateEvent(eventAt(t, "lamp", payload, base.Add(time.Duration(i)*time.Second))))
	}
	require.NoError(t, s.CreateEvent(event(t, "sensor", `{"n":1}`)))

	deleted, err := s.Prune(config.DBRetention{MaxPerDevice: keep})
	require.NoError(t, err)
	assert.Equal(t, int64(count-keep), deleted)

	// The newest events are the ones kept, latest first.
	lamp, err := s.QueryEvents("lamp", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, lamp, keep)
	for i, e := range lamp {
		assert.Equal(t, count-1-i, nFromEvent(t, e))
	}
	sensor, err := s.QueryEvents("sensor", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	assert.Len(t, sensor, 1, "a device under the cap is untouched")
}

func TestPruneReclaimsFreePages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vacuum.db")
	s := openAt(t, path)
	seed(t, s, "lamp", 600)

	deleted, err := s.Prune(config.DBRetention{MaxPerDevice: 1})
	require.NoError(t, err)
	require.Equal(t, int64(599), deleted)

	var free int
	require.NoError(t, rawOpen(t, path).QueryRow("PRAGMA freelist_count").Scan(&free))
	assert.Zero(t, free, "prune must reclaim every freed page")
}
