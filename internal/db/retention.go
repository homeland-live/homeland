package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/homeland-live/homeland/internal/config"
)

// pruneBatch is how many rows one prune statement deletes. Each batch is its
// own short write transaction, so event inserts waiting on the write lock get
// in between batches instead of waiting out the whole prune.
const pruneBatch = 500

// vacuumPages is how many free pages one incremental_vacuum step reclaims,
// for the same reason.
const vacuumPages = 256

// Prune deletes events by age, then by the per-device cap, in batches, and
// returns the number of rows removed. A disabled policy is a no-op. It stops
// early, without error, once the repo is closed.
func (r *Repo) Prune(policy config.DBRetention) (int64, error) {
	if err := r.ready(); err != nil {
		return 0, err
	}

	var total int64

	if policy.MaxAge > 0 {
		cutoff := time.Now().UTC().Add(-policy.MaxAge)
		n, err := r.deleteBatches(func(tx *gorm.DB) *gorm.DB {
			return tx.Model(&Event{}).Select("id").
				Where("created_at < ?", cutoff).
				Limit(pruneBatch)
		})
		total += n
		if err != nil {
			return total, fmt.Errorf("db: prune by age: %w", err)
		}
	}

	if policy.MaxPerDevice > 0 {
		var addrs []string
		if err := r.db.Conn().Model(&Event{}).
			Group("ieee_address").
			Having("COUNT(*) > ?", policy.MaxPerDevice).
			Pluck("ieee_address", &addrs).Error; err != nil {
			return total, fmt.Errorf("db: prune per-device cap: %w", err)
		}
		for _, addr := range addrs {
			// Everything past the device's newest MaxPerDevice events, read off
			// the (ieee_address, created_at) index.
			n, err := r.deleteBatches(func(tx *gorm.DB) *gorm.DB {
				return tx.Model(&Event{}).Select("id").
					Where("ieee_address = ?", addr).
					Order("created_at DESC").
					Offset(policy.MaxPerDevice).
					Limit(pruneBatch)
			})
			total += n
			if err != nil {
				return total, fmt.Errorf("db: prune per-device cap for %q: %w", addr, err)
			}
		}
	}

	if total > 0 {
		r.vacuum()
	}

	return total, nil
}

// deleteBatches deletes the event ids that ids selects (at most pruneBatch per
// call), one statement at a time, until a batch comes back short or the repo
// is closed.
func (r *Repo) deleteBatches(ids func(tx *gorm.DB) *gorm.DB) (int64, error) {
	var total int64
	for !r.closed.Load() {
		res := r.db.Conn().Where("id IN (?)", ids(r.db.Conn())).Delete(&Event{})
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if res.RowsAffected < pruneBatch {
			break
		}
	}
	return total, nil
}

// vacuum reclaims free pages vacuumPages at a time until none are left, the
// count stops shrinking, or the repo is closed. A failure only means pages are
// reclaimed on a later prune, so it is logged.
func (r *Repo) vacuum() {
	prev := int64(-1)
	for !r.closed.Load() {
		var free int64
		if err := r.db.Conn().Raw("PRAGMA freelist_count").Scan(&free).Error; err != nil {
			slog.Warn("db: read freelist count failed", "err", err)
			return
		}
		if free == 0 || (prev >= 0 && free >= prev) {
			return
		}
		prev = free
		if err := r.db.Conn().Exec(fmt.Sprintf("PRAGMA incremental_vacuum(%d)", vacuumPages)).Error; err != nil {
			slog.Warn("db: incremental vacuum failed", "err", err)
			return
		}
	}
}

// startRetention prunes once now and then every policy.Interval in a
// goroutine that Close stops. It is a no-op for a disabled policy.
func (r *Repo) startRetention(policy config.DBRetention) {
	if r.ready() != nil || !policy.Enabled() {
		return
	}
	interval := policy.Interval
	if interval <= 0 {
		interval = time.Hour
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.retentionCancel = cancel

	prune := func() {
		n, err := r.Prune(policy)
		if err != nil {
			slog.Error("db: retention prune failed", "err", err)
			return
		}
		if n > 0 {
			slog.Info("db: pruned events", "deleted", n,
				"maxAge", policy.MaxAge, "maxPerDevice", policy.MaxPerDevice)
		}
	}

	r.retentionWG.Go(func() {
		prune() // immediate pass on startup

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prune()
			}
		}
	})
}
