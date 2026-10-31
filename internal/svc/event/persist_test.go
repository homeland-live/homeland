package event_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	slogtest "github.com/avakarev/go-util/testutil/slog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/homeland-live/homeland/internal/db"
)

// recordingStore is an event.Store fake: it records every queued event and
// fails each call with err, when set.
type recordingStore struct {
	mu     sync.Mutex
	events []*db.Event
	err    error
}

func (r *recordingStore) EnqueueEvent(e *db.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.events = append(r.events, e)
	return nil
}

func (r *recordingStore) recorded() []*db.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*db.Event(nil), r.events...)
}

func TestUpdateQueuesEventByIEEEAddress(t *testing.T) {
	store := &recordingStore{}
	_, deliver := serviceWithStore(t, handlerMap{}, store)
	dev := deviceWithN("0x01", "lamp", 7)
	dev.State.CreatedAt = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

	// Queued before the subscriber returns, even for a device without a
	// handler.
	deliver(dev)

	got := store.recorded()
	require.Len(t, got, 1)
	assert.Equal(t, "0x01", got[0].IEEEAddress, "events survive a rename, so they are keyed by IEEE address")
	assert.Equal(t, map[string]json.RawMessage{"n": json.RawMessage("7")}, got[0].Payload)
	assert.Equal(t, dev.State.CreatedAt, got[0].CreatedAt, "stamped with the time the state was parsed")
}

func TestUpdateWithoutStateQueuesNoEvent(t *testing.T) {
	store := &recordingStore{}
	_, deliver := serviceWithStore(t, handlerMap{}, store)
	dev := device("0x01", "lamp")
	dev.State = nil

	deliver(dev)

	assert.Empty(t, store.recorded())
}

func TestUpdateAfterShutdownQueuesNoEvent(t *testing.T) {
	store := &recordingStore{}
	svc, deliver := serviceWithStore(t, handlerMap{}, store)
	svc.Shutdown()

	deliver(device("0x01", "lamp"))

	assert.Empty(t, store.recorded())
}

func TestEventQueueFailureIsLoggedAndHandlerStillRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs := slogtest.Capture(t, slog.LevelError)
		z2mRec := &recordingZ2M{}
		store := &recordingStore{err: errors.New("db: closed")}
		svc, deliver := serviceWithStore(t, handlerMap{"lamp": markerHandler(t, z2mRec, "lamp")}, store)

		deliver(device("0x01", "lamp"))
		synctest.Wait()
		svc.Shutdown()

		assert.Contains(t, logs.String(), `msg="svc/event: event persist error" device=lamp err="db: closed"`)
		assert.Equal(t, []string{"lamp.ran"}, z2mRec.recorded(), "a storage failure must not stop the handler")
	})
}
