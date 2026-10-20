package db

import (
	"encoding/json"

	"github.com/avakarev/go-util/gormutil/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// MigrationVersion returns the latest applied migration id, or "" when it
// cannot be read.
func (r *Repo) MigrationVersion() string {
	if r.ready() != nil {
		return ""
	}
	var id string
	r.db.Conn().
		Table(gormigrate.DefaultOptions.TableName).
		Select(gormigrate.DefaultOptions.IDColumnName).
		Order(gormigrate.DefaultOptions.IDColumnName + " DESC").
		Limit(1).
		Scan(&id)
	return id
}

// migrate applies pending migrations. Migrations are append-only: never edit
// a shipped one. Each declares its own local type so later changes to Event
// do not alter it (model.Timeseries is still shared with go-util).
func migrate(db *gorm.DB) error {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{{
		ID: "202103212212", // create `events` table
		Migrate: func(tx *gorm.DB) error {
			type event struct {
				model.Timeseries
				IEEEAddress string                     `gorm:"index"`
				Payload     map[string]json.RawMessage `gorm:"serializer:json;type:blob"`
			}
			return tx.Migrator().CreateTable(&event{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("events")
		},
	}, {
		ID: "202104092114", // index events by (ieee_address, created_at)
		Migrate: func(tx *gorm.DB) error {
			// Serves QueryEvents' filter and order from one index; it also
			// covers every lookup of the single-column index it replaces.
			if err := tx.Exec(
				`CREATE INDEX IF NOT EXISTS idx_events_ieee_address_created_at
					ON events(ieee_address, created_at)`,
			).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP INDEX IF EXISTS idx_events_ieee_address`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(
				`CREATE INDEX IF NOT EXISTS idx_events_ieee_address ON events(ieee_address)`,
			).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP INDEX IF EXISTS idx_events_ieee_address_created_at`).Error
		},
	}}).Migrate()
}
