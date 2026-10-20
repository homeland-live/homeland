package config

import (
	"time"

	"github.com/avakarev/go-util/envutil"
)

// DB is the event database configuration.
type DB struct {
	// Path is the SQLite file path. Its parent directory must exist and be on
	// a local filesystem (the database uses WAL).
	Path string

	// Retention controls pruning of stored events.
	Retention DBRetention
}

// DBRetention configures event pruning, which runs at startup and then every
// Interval. A zero value disables pruning.
type DBRetention struct {
	// MaxAge, when > 0, deletes events older than this.
	MaxAge time.Duration
	// MaxPerDevice, when > 0, keeps only the newest N events per device.
	MaxPerDevice int
	// Interval is how often pruning runs; <= 0 means one hour.
	Interval time.Duration
}

// Enabled reports whether any retention policy is active.
func (r DBRetention) Enabled() bool {
	return r.MaxAge > 0 || r.MaxPerDevice > 0
}

// loadDB reads HL_DB_PATH (default "data/homeland.db") and the retention
// policy.
func loadDB() (*DB, error) {
	r, err := loadDBRetention()
	if err != nil {
		return nil, err
	}
	return &DB{
		Path:      envutil.StrDef("HL_DB_PATH", "data/homeland.db"),
		Retention: r,
	}, nil
}

// loadDBRetention reads HL_DB_RETENTION_MAX_AGE (default 30 days),
// HL_DB_RETENTION_MAX_PER_DEVICE (default 0, unlimited) and
// HL_DB_RETENTION_INTERVAL (default 1h).
func loadDBRetention() (DBRetention, error) {
	maxAge, err := envutil.DurDef("HL_DB_RETENTION_MAX_AGE", 30*24*time.Hour)
	if err != nil {
		return DBRetention{}, err
	}
	maxPerDevice, err := envutil.IntDef("HL_DB_RETENTION_MAX_PER_DEVICE", 0)
	if err != nil {
		return DBRetention{}, err
	}
	interval, err := envutil.DurDef("HL_DB_RETENTION_INTERVAL", time.Hour)
	if err != nil {
		return DBRetention{}, err
	}
	return DBRetention{
		MaxAge:       maxAge,
		MaxPerDevice: maxPerDevice,
		Interval:     interval,
	}, nil
}
