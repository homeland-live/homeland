// Package db stores device events in an embedded SQLite database (gorm, with
// gormigrate migrations), keyed by IEEE address and queryable per device,
// latest first. Events are written by one background event writer in batches
// (see EnqueueEvent).
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/avakarev/go-util/gormutil"
	"github.com/avakarev/go-util/gormutil/slogger"
	"gorm.io/driver/sqlite"

	"github.com/homeland-live/homeland/internal/config"
	"github.com/homeland-live/homeland/internal/healthcheck"
)

// Repo owns the event database and its retention loop. Methods called after
// Close return an error.
type Repo struct {
	// db is never reset, so callers racing Close never see a nil handle.
	db *gormutil.DB

	// Zero when retention is not running.
	retentionCancel context.CancelFunc
	retentionWG     sync.WaitGroup

	// eventQueue feeds the background event writer (see EnqueueEvent).
	// eventQueueMu makes closing the queue wait for sends in progress, so none
	// hits a closed channel.
	eventQueue    chan *Event
	eventQueueMu  sync.RWMutex
	eventWriterWG sync.WaitGroup

	// lastWriteFailure is set when the event writer loses events, and cleared
	// once a later batch is fully written.
	lastWriteFailure atomic.Pointer[writeFailure]

	closed atomic.Bool
}

// ready returns an error unless the repo was built by New and is not closed.
func (r *Repo) ready() error {
	if r == nil || r.db == nil {
		return errors.New("db: not initialized")
	}
	if r.closed.Load() {
		return errors.New("db: closed")
	}
	return nil
}

// HealthCheck reports whether the database is reachable and the event writer
// is storing events. After the writer loses events it stays unhealthy until a
// later batch is fully written.
func (r *Repo) HealthCheck() healthcheck.HealthCheck {
	if err := r.ready(); err != nil {
		return healthcheck.New("db", err)
	}
	if err := r.db.Ping(); err != nil {
		return healthcheck.New("db", err)
	}
	if f := r.lastWriteFailure.Load(); f != nil {
		return healthcheck.New("db", fmt.Errorf("db: event writer lost %d event(s): %w", f.lost, f.err))
	}
	return healthcheck.New("db", nil)
}

// Close stops retention (waiting for the current prune batch), writes every
// queued event, and closes the database. Only the first call does anything;
// failures are logged.
func (r *Repo) Close() {
	if r == nil || r.db == nil {
		return
	}
	if !r.closed.CompareAndSwap(false, true) {
		return
	}
	if r.retentionCancel != nil {
		r.retentionCancel()
		r.retentionWG.Wait()
	}
	if r.eventQueue != nil {
		r.stopEventWriter()
	}
	tryClose(r.db)
}

// dsnParams apply to every pooled connection, in this order (the driver's):
//   - auto_vacuum=incremental lets Prune reclaim pages. It only takes on a new
//     database, and switching to WAL initializes the file, so it must come
//     first.
//   - WAL lets readers run alongside the single writer, so an API query no
//     longer blocks the event insert on the MQTT delivery goroutine.
//   - busy_timeout makes a writer wait for another one (e.g. a prune batch)
//     instead of failing with "database is locked".
//   - synchronous=NORMAL is durable under WAL and cuts fsyncs on SD cards.
//   - _txlock=immediate takes the write lock at BEGIN, so waiting happens
//     there and never as a deadlock mid-transaction.
const dsnParams = "_auto_vacuum=incremental&_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_txlock=immediate"

// maxOpenConns bounds the pool. Writes are still serialized by SQLite; the
// extra connections serve concurrent reads.
const maxOpenConns = 4

// dsn appends dsnParams to path, keeping any query the path already has.
func dsn(path string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + dsnParams
}

// open opens the SQLite pool and applies settings that must precede the
// schema.
func open(cfg *config.DB) (*sql.DB, error) {
	sqlDB, err := sql.Open(sqlite.DriverName, dsn(cfg.Path))
	if err != nil {
		return nil, fmt.Errorf("db: failed to open %q: %w", cfg.Path, err)
	}

	// Keep idle connections so their per-connection settings are not
	// re-applied on every query.
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)

	// Open the first connection now, so dsnParams (auto_vacuum before WAL) are
	// applied to the file before migrations create any table, and a bad path
	// fails here.
	if err := sqlDB.Ping(); err != nil {
		tryClose(sqlDB)
		return nil, fmt.Errorf("db: failed to open %q: %w", cfg.Path, err)
	}

	return sqlDB, nil
}

// New opens and migrates the database at cfg.Path, starts the event writer,
// and starts retention when a policy is enabled. The caller must Close the
// returned Repo.
func New(cfg *config.DB) (*Repo, error) {
	if cfg == nil {
		return nil, errors.New("db: config is required, but missing")
	}

	sqlDB, err := open(cfg)
	if err != nil {
		return nil, err
	}

	db, err := gormutil.Open(
		sqlite.New(sqlite.Config{Conn: sqlDB}),
		gormutil.WithLogger(slogger.NewDefault()),
	)
	if err != nil {
		tryClose(sqlDB)
		return nil, err
	}
	slog.Info("db: opened", "path", cfg.Path)

	repo := &Repo{db: db}

	if err := migrate(db.Conn()); err != nil {
		tryClose(db)
		return nil, err
	}
	slog.Info("db: migrated", "version", repo.MigrationVersion())

	repo.startEventWriter()

	if cfg.Retention.Enabled() {
		repo.startRetention(cfg.Retention)
		slog.Info("db: event retention enabled",
			"maxAge", cfg.Retention.MaxAge,
			"maxPerDevice", cfg.Retention.MaxPerDevice,
			"interval", cfg.Retention.Interval)
	}

	return repo, nil
}
