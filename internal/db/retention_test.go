package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/db"
)

// openWithRetention opens the database at path with retention policy.
func openWithRetention(t *testing.T, path string, policy config.DBRetention) *db.Repo {
	t.Helper()
	s, err := db.New(&config.DB{Path: path, Retention: policy})
	require.NoError(t, err)
	t.Cleanup(s.Close)
	return s
}

func TestRetentionPrunesAtStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retention.db")
	s, err := db.New(&config.DB{Path: path})
	require.NoError(t, err)
	require.NoError(t, s.CreateEvent(eventAt(t, "lamp", `{"n":1}`, time.Now().UTC().Add(-48*time.Hour))))
	require.NoError(t, s.CreateEvent(event(t, "lamp", `{"n":2}`)))
	s.Close()

	// The interval is far off, so only the startup pass can prune.
	s = openWithRetention(t, path, config.DBRetention{MaxAge: 24 * time.Hour, Interval: time.Hour})

	assert.Eventually(t, func() bool { return countEvents(t, s, "lamp") == 1 },
		2*time.Second, 10*time.Millisecond, "the event older than MaxAge is pruned on startup")
}

func TestRetentionPrunesEveryInterval(t *testing.T) {
	s := openWithRetention(t, filepath.Join(t.TempDir(), "retention.db"),
		config.DBRetention{MaxAge: 24 * time.Hour, Interval: 20 * time.Millisecond})
	require.NoError(t, s.CreateEvent(event(t, "lamp", `{"recent":true}`)))

	// Two rounds: the startup pass runs once, so a second prune proves the
	// interval.
	for i := range 2 {
		require.NoError(t, s.CreateEvent(eventAt(t, "lamp", `{"old":true}`, time.Now().UTC().Add(-48*time.Hour))))
		assert.Eventually(t, func() bool { return countEvents(t, s, "lamp") == 1 },
			2*time.Second, 10*time.Millisecond, "round %d: the old event is pruned", i+1)
	}
}

func TestRetentionStopsOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retention.db")
	s, err := db.New(&config.DB{Path: path, Retention: config.DBRetention{MaxAge: 24 * time.Hour, Interval: time.Millisecond}})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		s.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return: the retention loop kept running")
	}
	_, err = s.Prune(config.DBRetention{MaxAge: time.Hour})
	assert.ErrorContains(t, err, "db: closed")
}
