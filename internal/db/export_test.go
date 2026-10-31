package db

import "gorm.io/gorm"

// ExecSQL runs raw SQL against the repo's database, so tests can break it in
// ways the public API cannot.
func (r *Repo) ExecSQL(sql string) error {
	return r.db.Conn().Exec(sql).Error
}

// OnInsert calls fn before every INSERT the repo attempts, a batch or a single
// row, so tests can count the event writer's attempts. Call it before
// enqueuing events.
func (r *Repo) OnInsert(fn func()) error {
	return r.db.Conn().Callback().Create().Before("gorm:create").
		Register("test:on_insert", func(*gorm.DB) { fn() })
}
