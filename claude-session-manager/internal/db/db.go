package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite" // Pure Go SQLite driver with FTS5 support
)

//go:embed schema.sql
var schemaSQL string

// DB wraps the SQLite database connection
type DB struct {
	conn *sql.DB
}

// Open opens a SQLite database at the given path and applies the schema.
// If path is ":memory:", it creates an in-memory database (useful for testing).
func Open(path string) (*DB, error) {
	// Open SQLite database connection (modernc.org/sqlite registers as "sqlite")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Apply schema
	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to apply schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn == nil {
		return nil
	}
	return db.conn.Close()
}

// BeginTx starts a new transaction
func (db *DB) BeginTx() (*sql.Tx, error) {
	return db.conn.Begin()
}

// Conn returns the underlying database connection for direct access
func (db *DB) Conn() *sql.DB {
	return db.conn
}
