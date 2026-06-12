// Package sqlite provides a canonical SQLite opener with standard pragmas.
package sqlite

import (
	"database/sql"

	_ "modernc.org/sqlite" // register the sqlite3 database/sql driver
)

// Open opens a SQLite database at path with standard pragma settings:
// busy_timeout(5000) and foreign_keys(on).
func Open(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
}
