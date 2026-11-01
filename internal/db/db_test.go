package db_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/db"
)

func openAt(t *testing.T, path string) *db.Repo {
	t.Helper()
	s, err := db.New(&config.DB{Path: path})
	require.NoError(t, err)
	t.Cleanup(s.Close)
	return s
}

func openTemp(t *testing.T) *db.Repo {
	t.Helper()
	return openAt(t, filepath.Join(t.TempDir(), "test.db"))
}

// event builds an Event for the given device from a JSON object literal.
func event(t *testing.T, ieeeAddress, payload string) *db.Event {
	t.Helper()
	var props map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(payload), &props), "invalid test payload")
	return &db.Event{IEEEAddress: ieeeAddress, Payload: props}
}

// eventAt is like event but stamps the Event with an explicit created_at, used
// to exercise time-based ordering and pruning without waiting real time.
func eventAt(t *testing.T, ieeeAddress, payload string, createdAt time.Time) *db.Event {
	t.Helper()
	e := event(t, ieeeAddress, payload)
	e.CreatedAt = createdAt
	return e
}

func TestPersistAndQuery(t *testing.T) {
	s := openTemp(t)

	payloads := []string{
		`{"state":"ON","battery":80,"linkquality":120}`,
		`{"state":"OFF","battery":79,"linkquality":118}`,
		`{"state":"ON","battery":78,"linkquality":121}`,
	}
	for _, p := range payloads {
		require.NoError(t, s.CreateEvent(event(t, "workshop-lamp", p)))
	}
	// A different device, to confirm device filtering.
	require.NoError(t, s.CreateEvent(event(t, "kitchen-sensor", `{"occupancy":true}`)))

	// Results are scoped to the requested device.
	lamp, err := s.QueryEvents("workshop-lamp", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	assert.Len(t, lamp, 3)
	sensor, err := s.QueryEvents("kitchen-sensor", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	assert.Len(t, sensor, 1)

	// Payload must round-trip and stay queryable by property value.
	require.Contains(t, lamp[0].Payload, "state")
	var state string
	require.NoError(t, json.Unmarshal(lamp[0].Payload["state"], &state))
	assert.Equal(t, "ON", state)
}

func TestHealthCheck(t *testing.T) {
	s := openTemp(t)
	hc := s.HealthCheck()
	assert.Truef(t, hc.IsHealthy, "expected healthy db, got error %q", hc.Error)
}

// nFromEvent reads the "n" property from an event's payload.
func nFromEvent(t *testing.T, e db.Event) int {
	t.Helper()
	raw, ok := e.Payload["n"]
	require.True(t, ok, "payload missing n")
	var n int
	require.NoError(t, json.Unmarshal(raw, &n), "unmarshal n")
	return n
}

// TestQueryEventsPagination writes a known sequence, then walks it a page at a
// time, asserting latest-first ordering and that pages don't overlap. Each
// event is written at a distinct, increasing timestamp so ordering is stable.
func TestQueryEventsPagination(t *testing.T) {
	s := openTemp(t)

	base := time.Now().UTC().Add(-time.Hour)
	for i := 1; i <= 5; i++ {
		payload := fmt.Sprintf(`{"n":%d}`, i)
		require.NoError(t, s.CreateEvent(eventAt(t, "lamp", payload, base.Add(time.Duration(i)*time.Minute))))
	}

	// Page 1, size 2: newest two (n=5, n=4). Count reports the device total.
	p1 := &db.Page{Num: 1, Size: 2}
	page1, err := s.QueryEvents("lamp", p1)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, 5, nFromEvent(t, page1[0]))
	assert.Equal(t, 4, nFromEvent(t, page1[1]))
	assert.Equal(t, 5, p1.Count)

	// Page 2, size 2: next two (n=3, n=2).
	p2 := &db.Page{Num: 2, Size: 2}
	page2, err := s.QueryEvents("lamp", p2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, 3, nFromEvent(t, page2[0]))
	assert.Equal(t, 2, nFromEvent(t, page2[1]))
	assert.Equal(t, 5, p2.Count)

	// Page 3, size 2: the last one (n=1). Count still reports the full total.
	p3 := &db.Page{Num: 3, Size: 2}
	page3, err := s.QueryEvents("lamp", p3)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	assert.Equal(t, 1, nFromEvent(t, page3[0]))
	assert.Equal(t, 5, p3.Count)

	// A page beyond the data: no rows, but Count still reports the total.
	p4 := &db.Page{Num: 4, Size: 2}
	page4, err := s.QueryEvents("lamp", p4)
	require.NoError(t, err)
	assert.Empty(t, page4)
	assert.Equal(t, 5, p4.Count)
}

// TestQueryEventsPaginationWithEqualTimestamps covers events stored with the
// same created_at: arrival order breaks the tie, so pages are latest first
// and never repeat or skip an event.
func TestQueryEventsPaginationWithEqualTimestamps(t *testing.T) {
	s := openTemp(t)
	at := time.Now().UTC().Add(-time.Hour)
	for i := 1; i <= 5; i++ {
		require.NoError(t, s.CreateEvent(eventAt(t, "lamp", fmt.Sprintf(`{"n":%d}`, i), at)))
	}

	var got []int
	for num := 1; num <= 3; num++ {
		page, err := s.QueryEvents("lamp", &db.Page{Num: num, Size: 2})
		require.NoError(t, err)
		for _, e := range page {
			got = append(got, nFromEvent(t, e))
		}
	}
	assert.Equal(t, []int{5, 4, 3, 2, 1}, got)
}

// TestEventTimesAreStoredInUTC covers a CreatedAt in another zone: it is
// stored in UTC, so events sort by instant even though SQLite compares the
// stored times as strings.
func TestEventTimesAreStoredInUTC(t *testing.T) {
	s := openTemp(t)
	berlin := time.FixedZone("CEST", 2*60*60)
	base := time.Now().UTC().Add(-time.Hour)

	// As strings, 10:30+02:00 would sort after 09:00+00:00, though it is 30
	// minutes earlier.
	earlier := eventAt(t, "lamp", `{"n":1}`, base.Add(-30*time.Minute).In(berlin))
	later := eventAt(t, "lamp", `{"n":2}`, base)
	require.NoError(t, s.CreateEvent(earlier))
	require.NoError(t, s.CreateEvent(later))
	assert.Equal(t, time.UTC, earlier.CreatedAt.Location(), "stamping converts CreatedAt to UTC")

	events, err := s.QueryEvents("lamp", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, []int{2, 1}, []int{nFromEvent(t, events[0]), nFromEvent(t, events[1])}, "latest first")

	// Pruning compares the same strings: only the earlier event is old enough.
	deleted, err := s.Prune(config.DBRetention{MaxAge: time.Since(base) + 15*time.Minute})
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
}

// TestQueryEventsRequiresArgs confirms device and page are mandatory and that
// an out-of-range page is rejected by validation rather than silently coerced.
func TestQueryEventsRequiresArgs(t *testing.T) {
	s := openTemp(t)
	_, err := s.QueryEvents("", &db.Page{Num: 1, Size: 10})
	require.Error(t, err, "expected error for empty ieee address")
	_, err = s.QueryEvents("lamp", nil)
	require.Error(t, err, "expected error for nil page")
	_, err = s.QueryEvents("lamp", &db.Page{Num: 0, Size: 10})
	require.Error(t, err, "expected error for invalid page (Num=0)")
	_, err = s.QueryEvents("lamp", &db.Page{Num: 1, Size: 100})
	require.Error(t, err, "expected error for invalid page (Size>25)")
}

func TestQueryEventsReturnsEmptyPageNotNil(t *testing.T) {
	s := openTemp(t)
	events, err := s.QueryEvents("lamp", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	assert.NotNil(t, events, "an empty page must serialize as [], not null")
	assert.Empty(t, events)
}

func TestQueryEventsReportsDatabaseErrors(t *testing.T) {
	s := openTemp(t)
	require.NoError(t, s.ExecSQL("DROP TABLE events"))

	events, err := s.QueryEvents("lamp", &db.Page{Num: 1, Size: 10})
	require.Error(t, err, "a failing query must not look like an empty page")
	assert.Nil(t, events)
}

func TestPrunePerDeviceCap(t *testing.T) {
	s := openTemp(t)

	// 5 events for lamp, 2 for sensor, each at a distinct increasing time so
	// the per-device cap keeps a well-defined newest N.
	base := time.Now().UTC().Add(-time.Hour)
	for i := range 5 {
		require.NoError(t, s.CreateEvent(eventAt(t, "lamp", `{"n":1}`, base.Add(time.Duration(i)*time.Minute))))
	}
	for i := range 2 {
		require.NoError(t, s.CreateEvent(eventAt(t, "sensor", `{"n":1}`, base.Add(time.Duration(i)*time.Minute))))
	}

	// Keep newest 2 per device: lamp drops 3, sensor unchanged.
	deleted, err := s.Prune(config.DBRetention{MaxPerDevice: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), deleted)

	lamp, err := s.QueryEvents("lamp", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	assert.Len(t, lamp, 2)
	sensor, err := s.QueryEvents("sensor", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	assert.Len(t, sensor, 2)
}

func TestPruneByAge(t *testing.T) {
	s := openTemp(t)

	// A fresh event, then one back-dated well past the cutoff.
	require.NoError(t, s.CreateEvent(event(t, "lamp", `{"n":1}`)))
	old := time.Now().UTC().Add(-48 * time.Hour)
	require.NoError(t, s.CreateEvent(eventAt(t, "lamp", `{"n":2}`, old)))

	// Keep last 24h: the back-dated row is removed, the fresh one stays.
	deleted, err := s.Prune(config.DBRetention{MaxAge: 24 * time.Hour})
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
	remaining, err := s.QueryEvents("lamp", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	assert.Len(t, remaining, 1)
}

func TestPruneDisabledIsNoop(t *testing.T) {
	s := openTemp(t)
	require.NoError(t, s.CreateEvent(event(t, "lamp", `{"n":1}`)))
	deleted, err := s.Prune(config.DBRetention{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted, "expected no deletions for disabled policy")
}

// TestMigrationsIdempotent opens the same database file twice: the second New
// must re-run migrations against an already-migrated schema without error, and
// data written before the reopen must survive.
func TestMigrationsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")

	s1, err := db.New(&config.DB{Path: path})
	require.NoError(t, err, "first New")
	require.NoError(t, s1.CreateEvent(event(t, "dev", `{"state":"ON"}`)))
	s1.Close()

	s2 := openAt(t, path)

	events, err := s2.QueryEvents("dev", &db.Page{Num: 1, Size: 10})
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

// TestCreateEventRequiresIEEEAddress confirms an event cannot be stored
// unattributable.
func TestCreateEventRequiresIEEEAddress(t *testing.T) {
	s := openTemp(t)
	assert.Error(t, s.CreateEvent(event(t, "", `{"n":1}`)),
		"expected empty ieee address to be rejected")
}

// TestMigrationVersion asserts the head migration id, so adding a migration
// without noticing this reports the wrong version is caught here.
func TestMigrationVersion(t *testing.T) {
	s := openTemp(t)
	assert.Equal(t, "202104092114", s.MigrationVersion())
}

// TestAutoVacuumIncremental confirms open set auto_vacuum=INCREMENTAL, which is
// what lets Prune reclaim freed pages.
func TestAutoVacuumIncremental(t *testing.T) {
	path := filepath.Join(t.TempDir(), "av.db")
	s := openAt(t, path)
	require.NoError(t, s.CreateEvent(event(t, "lamp", `{"n":1}`)))

	raw, err := sql.Open("sqlite3", path)
	require.NoError(t, err, "raw open")
	t.Cleanup(func() {
		require.NoError(t, raw.Close())
	})

	var mode int
	require.NoError(t, raw.QueryRow("PRAGMA auto_vacuum").Scan(&mode))
	assert.Equal(t, 2, mode, "expected auto_vacuum=2 (INCREMENTAL)")
}

// TestCloseIsSafeUnderConcurrentUse pins the teardown contract: Close is
// idempotent, and callers racing it get an error rather than a panic. Run under
// -race this fails if Close ever clears the handle out from under readers.
func TestCloseIsSafeUnderConcurrentUse(t *testing.T) {
	s, err := db.New(&config.DB{Path: filepath.Join(t.TempDir(), "close.db")})
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Events are built inline rather than via the event helper: the
			// helper asserts, and testify assertions may only run on the test
			// goroutine. Errors are expected here and deliberately ignored;
			// this test is about not panicking or racing.
			for range 50 {
				_ = s.CreateEvent(&db.Event{
					IEEEAddress: "lamp",
					Payload:     map[string]json.RawMessage{"n": json.RawMessage(`1`)},
				})
				_, _ = s.QueryEvents("lamp", &db.Page{Num: 1, Size: 10})
				s.HealthCheck()
				s.MigrationVersion()
			}
		}()
	}

	s.Close()
	s.Close() // must be idempotent
	wg.Wait()

	require.Error(t, s.CreateEvent(event(t, "lamp", `{"n":1}`)),
		"expected a closed repo to reject writes")
	assert.False(t, s.HealthCheck().IsHealthy, "expected a closed repo to be unhealthy")
}
