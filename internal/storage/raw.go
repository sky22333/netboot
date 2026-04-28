package storage

import "database/sql"

func (s *Store) RawDB() *sql.DB {
	return s.db
}
