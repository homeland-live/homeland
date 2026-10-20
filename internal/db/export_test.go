package db

// ExecSQL runs raw SQL against the repo's database, so tests can break it in
// ways the public API cannot.
func (r *Repo) ExecSQL(sql string) error {
	return r.db.Conn().Exec(sql).Error
}
