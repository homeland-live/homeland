package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/avakarev/go-util/gormutil/model"
)

// Event is a stored device event. It is keyed by IEEE address, not friendly
// name, so it survives a rename. Payload is stored as JSON.
type Event struct {
	model.Timeseries
	IEEEAddress string                     `json:"ieeeAddress" validate:"required"`
	Payload     map[string]json.RawMessage `json:"payload" gorm:"serializer:json;type:blob"`
}

// stamp assigns e's id, sets an unset CreatedAt to now, and converts
// CreatedAt to UTC, so stored times sort by instant (SQLite compares them as
// strings).
func (e *Event) stamp() error {
	if err := e.EnsureID(); err != nil {
		return fmt.Errorf("db: event id: %w", err)
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	e.CreatedAt = e.CreatedAt.UTC()
	return nil
}

// CreateEvent stores e synchronously, stamping it first (see stamp). An empty
// IEEEAddress is rejected. Hot paths use EnqueueEvent.
func (r *Repo) CreateEvent(e *Event) error {
	if err := r.ready(); err != nil {
		return err
	}
	if err := e.stamp(); err != nil {
		return err
	}
	if err := r.db.Create(e); err != nil {
		return fmt.Errorf("db: persist event for %q: %w", e.IEEEAddress, err)
	}
	return nil
}

// QueryEvents returns one page of a device's events, latest first (never nil),
// and sets p.Count to the device's total. ieeeAddress and a valid p are
// required. A database failure is returned, not reported as an empty page.
func (r *Repo) QueryEvents(ieeeAddress string, p *Page) ([]Event, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	if ieeeAddress == "" {
		return nil, errors.New("db: ieee address is required")
	}
	if p == nil {
		return nil, errors.New("db: page is required")
	}
	if err := r.db.Validate(p); err != nil {
		return nil, fmt.Errorf("db: invalid page: %w", err)
	}

	var count int64
	if err := r.db.Conn().Model(&Event{}).
		Where("ieee_address = ?", ieeeAddress).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("db: count events for %q: %w", ieeeAddress, err)
	}
	p.Count = int(count)

	events := make([]Event, 0, p.Size)
	if err := r.db.Conn().
		Where("ieee_address = ?", ieeeAddress).
		Order("created_at DESC, rowid DESC").
		Limit(p.Size).
		Offset(p.Offset()).
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("db: query events for %q: %w", ieeeAddress, err)
	}
	return events, nil
}
