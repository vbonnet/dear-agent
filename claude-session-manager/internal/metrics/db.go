package metrics

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	// Database location
	defaultDBPath = "~/.engram/deadlock-metrics.db"

	// Retention period (90 days default)
	defaultRetentionDays = 90
)

// DB wraps the SQLite database for deadlock metrics
type DB struct {
	conn           *sql.DB
	path           string
	retentionDays  int
}

// Open opens or creates the metrics database
func Open(path string) (*DB, error) {
	if path == "" {
		path = expandPath(defaultDBPath)
	} else {
		path = expandPath(path)
	}

	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create metrics dir: %w", err)
	}

	// Open database
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db := &DB{
		conn:          conn,
		path:          path,
		retentionDays: defaultRetentionDays,
	}

	// Initialize schema
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// initSchema creates tables if they don't exist
func (db *DB) initSchema() error {
	schema := `
	-- Deadlock incidents table
	CREATE TABLE IF NOT EXISTS incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL,
		session_name TEXT NOT NULL,
		pid INTEGER NOT NULL,
		process_state TEXT NOT NULL,
		cpu_percent REAL NOT NULL,
		runtime_seconds INTEGER NOT NULL,
		wchan TEXT,
		recovery_method TEXT NOT NULL,
		time_to_recovery_seconds INTEGER NOT NULL,
		swarm_size INTEGER,
		hardware_cpu_percent REAL,
		hardware_ram_percent REAL,
		hardware_load_avg REAL,
		notes TEXT
	);

	-- Swarm operations table
	CREATE TABLE IF NOT EXISTS swarms (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL,
		agent_count INTEGER NOT NULL,
		batch_size INTEGER NOT NULL,
		wave_count INTEGER NOT NULL,
		start_time TEXT NOT NULL,
		end_time TEXT,
		duration_seconds INTEGER,
		deadlock_occurred INTEGER NOT NULL DEFAULT 0,
		hardware_cpu_percent REAL,
		hardware_ram_percent REAL,
		hardware_load_avg REAL,
		notes TEXT
	);

	-- System health snapshots table
	CREATE TABLE IF NOT EXISTS health_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL,
		active_sessions INTEGER NOT NULL,
		deadlocked_processes INTEGER NOT NULL,
		cpu_percent REAL NOT NULL,
		ram_percent REAL NOT NULL,
		disk_io_percent REAL,
		load_avg_1min REAL,
		load_avg_5min REAL,
		load_avg_15min REAL,
		api_rate_limit_remaining INTEGER,
		notes TEXT
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_incidents_timestamp ON incidents(timestamp);
	CREATE INDEX IF NOT EXISTS idx_incidents_session ON incidents(session_name);
	CREATE INDEX IF NOT EXISTS idx_swarms_timestamp ON swarms(timestamp);
	CREATE INDEX IF NOT EXISTS idx_swarms_deadlock ON swarms(deadlock_occurred);
	CREATE INDEX IF NOT EXISTS idx_health_timestamp ON health_snapshots(timestamp);
	`

	_, err := db.conn.Exec(schema)
	return err
}

// CleanupOldRecords removes records older than retention period
func (db *DB) CleanupOldRecords() error {
	cutoff := time.Now().AddDate(0, 0, -db.retentionDays).Format(time.RFC3339)

	tables := []string{"incidents", "swarms", "health_snapshots"}
	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE timestamp < ?", table)
		if _, err := db.conn.Exec(query, cutoff); err != nil {
			return fmt.Errorf("cleanup %s: %w", table, err)
		}
	}

	return nil
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
