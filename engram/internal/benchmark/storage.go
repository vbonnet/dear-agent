package benchmark

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3" // sqlite3 driver
)

// BenchmarkRun represents a single benchmark execution.
type BenchmarkRun struct {
	ID          int64
	RunID       string
	Timestamp   time.Time
	Variant     string // "raw", "engram", "wayfinder"
	ProjectSize string // "small", "medium", "large", "xl"
	ProjectName string

	// Metrics (nullable fields use pointers)
	QualityScore    *float64
	CostUSD         *float64
	TokensInput     *int64
	TokensOutput    *int64
	DurationMs      *int64
	FileCount       *int
	DocumentationKB *float64
	PhasesCompleted *int

	Successful bool
	Metadata   map[string]interface{} // JSON
	CreatedAt  time.Time
}

// Storage provides SQLite persistence for benchmark data.
type Storage struct {
	db *sql.DB
}

// NewStorage creates a Storage instance at ~/.engram/benchmarks.db
func NewStorage() (*Storage, error) {
	return NewStorageAt(DefaultDatabasePath())
}

// NewStorageAt creates a Storage instance at the specified path.
// The database file and parent directories are created if they don't exist.
// WAL mode is enabled for better concurrent write performance.
func NewStorageAt(path string) (*Storage, error) {
	// Expand ~ to home directory
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", path, err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil { //nolint:noctx // TODO(context): plumb ctx through this layer
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	storage := &Storage{db: db}

	// Initialize schema
	if err := storage.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return storage, nil
}

// DefaultDatabasePath returns the default database path.
func DefaultDatabasePath() string {
	return "~/.engram/benchmarks.db"
}

// initSchema creates the database schema if it doesn't exist.
func (s *Storage) initSchema() error {
	if _, err := s.db.Exec(schema); err != nil { //nolint:noctx // TODO(context): plumb ctx through this layer
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (s *Storage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// InsertRun inserts a new benchmark run into the database.
func (s *Storage) InsertRun(run BenchmarkRun) error {
	// Validation
	if run.RunID == "" {
		return fmt.Errorf("run_id cannot be empty")
	}
	if run.Variant == "" {
		return fmt.Errorf("variant cannot be empty")
	}
	if !isValidVariant(run.Variant) {
		return fmt.Errorf("invalid variant %q (must be raw, engram, or wayfinder)", run.Variant)
	}
	if run.ProjectSize == "" {
		return fmt.Errorf("project_size cannot be empty")
	}
	if !isValidProjectSize(run.ProjectSize) {
		return fmt.Errorf("invalid project_size %q (must be small, medium, large, or xl)", run.ProjectSize)
	}

	// Serialize metadata to JSON
	var metadataJSON []byte
	var err error
	if run.Metadata != nil {
		metadataJSON, err = json.Marshal(run.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO benchmark_runs (
			run_id, timestamp, variant, project_size, project_name,
			quality_score, cost_usd, tokens_input, tokens_output,
			duration_ms, file_count, documentation_kb, phases_completed,
			successful, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		run.RunID,
		run.Timestamp.Format(time.RFC3339),
		run.Variant,
		run.ProjectSize,
		nullString(run.ProjectName),
		run.QualityScore,
		run.CostUSD,
		run.TokensInput,
		run.TokensOutput,
		run.DurationMs,
		run.FileCount,
		run.DocumentationKB,
		run.PhasesCompleted,
		boolToInt(run.Successful),
		nullString(string(metadataJSON)),
	)

	if err != nil {
		return fmt.Errorf("failed to insert benchmark run: %w", err)
	}

	return nil
}

// Helper functions

func isValidVariant(variant string) bool {
	return variant == "raw" || variant == "engram" || variant == "wayfinder"
}

func isValidProjectSize(size string) bool {
	return size == "small" || size == "medium" || size == "large" || size == "xl"
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
