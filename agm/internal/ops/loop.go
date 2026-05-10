package ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const loopStoreOpenOptions = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"

const loopStoreSchema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS loops (
    loop_id     TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    cmd         TEXT NOT NULL,
    cadence     TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'paused')),
    created_at  TIMESTAMP NOT NULL,
    last_run_at TIMESTAMP,
    next_run_at TIMESTAMP,
    run_count   INTEGER NOT NULL DEFAULT 0,
    UNIQUE (name)
);

CREATE INDEX IF NOT EXISTS idx_loops_status      ON loops (status);
CREATE INDEX IF NOT EXISTS idx_loops_next_run_at ON loops (next_run_at);

CREATE TABLE IF NOT EXISTS loop_runs (
    run_id      TEXT PRIMARY KEY,
    loop_id     TEXT NOT NULL REFERENCES loops (loop_id) ON DELETE CASCADE,
    started_at  TIMESTAMP NOT NULL,
    finished_at TIMESTAMP,
    exit_code   INTEGER,
    stdout      TEXT NOT NULL DEFAULT '',
    stderr      TEXT NOT NULL DEFAULT '',
    success     INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_loop_runs_loop_id ON loop_runs (loop_id, started_at);
`

// LoopStatus is the lifecycle state of a persistent loop.
type LoopStatus string

// Loop lifecycle states.
const (
	LoopStatusActive LoopStatus = "active"
	LoopStatusPaused LoopStatus = "paused"
)

// Loop is a named, recurring bash command stored in the loop store.
type Loop struct {
	LoopID      string
	Name        string
	Description string
	Cmd         string
	Cadence     time.Duration
	Status      LoopStatus
	CreatedAt   time.Time
	LastRunAt   *time.Time
	NextRunAt   *time.Time
	RunCount    int
}

// LoopRun is one execution record for a loop.
type LoopRun struct {
	RunID      string
	LoopID     string
	StartedAt  time.Time
	FinishedAt *time.Time
	ExitCode   *int
	Stdout     string
	Stderr     string
	Success    bool
}

// LoopStore is a SQLite-backed store for loop definitions and their runs.
// It is safe for concurrent use across multiple goroutines (WAL mode serialises
// writers; readers proceed concurrently).
type LoopStore struct {
	db    *sql.DB
	now   func() time.Time
	idGen func() string
}

// LoopStorePath returns the canonical path for the loop store database
// (~/.agm/loops.db).
func LoopStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agm/loops.db"
	}
	return filepath.Join(home, ".agm", "loops.db")
}

// OpenLoopStore opens (or creates) the loop store at path and applies the
// schema. The directory is created if it does not exist.
func OpenLoopStore(path string) (*LoopStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("loop store: mkdir %s: %w", filepath.Dir(path), err)
	}
	db, err := sql.Open("sqlite", path+loopStoreOpenOptions)
	if err != nil {
		return nil, fmt.Errorf("loop store: open %s: %w", path, err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("loop store: ping: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), loopStoreSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("loop store: apply schema: %w", err)
	}
	return &LoopStore{
		db:    db,
		now:   time.Now,
		idGen: uuid.NewString,
	}, nil
}

// Close releases the database connection.
func (s *LoopStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// CreateLoop inserts a new loop definition. Returns an error if a loop with
// the same name already exists.
//
// The first run is scheduled cadence from now; callers can trigger an
// immediate first run with RunLoop.
func (s *LoopStore) CreateLoop(ctx context.Context, name, description, cmd string, cadence time.Duration) (*Loop, error) {
	if name == "" {
		return nil, fmt.Errorf("loop: name is required")
	}
	if cmd == "" {
		return nil, fmt.Errorf("loop: cmd is required")
	}
	if cadence <= 0 {
		return nil, fmt.Errorf("loop: cadence must be positive")
	}

	id := s.idGen()
	now := s.now()
	nextRunAt := now.Add(cadence)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO loops (loop_id, name, description, cmd, cadence, status, created_at, next_run_at)
		VALUES (?, ?, ?, ?, ?, 'active', ?, ?)
	`, id, name, description, cmd, cadence.String(), now, nextRunAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("loop %q already exists", name)
		}
		return nil, fmt.Errorf("loop store: create %q: %w", name, err)
	}

	return &Loop{
		LoopID:      id,
		Name:        name,
		Description: description,
		Cmd:         cmd,
		Cadence:     cadence,
		Status:      LoopStatusActive,
		CreatedAt:   now,
		NextRunAt:   &nextRunAt,
	}, nil
}

// GetLoop retrieves a loop by name.
func (s *LoopStore) GetLoop(ctx context.Context, name string) (*Loop, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT loop_id, name, description, cmd, cadence, status,
		       created_at, last_run_at, next_run_at, run_count
		FROM loops WHERE name = ?
	`, name)
	l, err := scanLoop(row)
	if err != nil {
		if strings.Contains(err.Error(), "loop not found") {
			return nil, fmt.Errorf("loop %q not found", name)
		}
		return nil, err
	}
	return l, nil
}

// ListLoops returns all loops ordered by name.
func (s *LoopStore) ListLoops(ctx context.Context) ([]*Loop, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT loop_id, name, description, cmd, cadence, status,
		       created_at, last_run_at, next_run_at, run_count
		FROM loops ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("loop store: list: %w", err)
	}
	defer rows.Close()

	var loops []*Loop
	for rows.Next() {
		l, err := scanLoop(rows)
		if err != nil {
			return nil, err
		}
		loops = append(loops, l)
	}
	return loops, rows.Err()
}

// DueLoops returns active loops whose next_run_at is at or before now.
// These are the loops agm loop tick should execute.
func (s *LoopStore) DueLoops(ctx context.Context) ([]*Loop, error) {
	now := s.now()
	rows, err := s.db.QueryContext(ctx, `
		SELECT loop_id, name, description, cmd, cadence, status,
		       created_at, last_run_at, next_run_at, run_count
		FROM loops
		WHERE status = 'active' AND (next_run_at IS NULL OR next_run_at <= ?)
		ORDER BY next_run_at ASC
	`, now)
	if err != nil {
		return nil, fmt.Errorf("loop store: due loops: %w", err)
	}
	defer rows.Close()

	var loops []*Loop
	for rows.Next() {
		l, err := scanLoop(rows)
		if err != nil {
			return nil, err
		}
		loops = append(loops, l)
	}
	return loops, rows.Err()
}

// SetStatus updates a loop's status (active/paused).
func (s *LoopStore) SetStatus(ctx context.Context, name string, status LoopStatus) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE loops SET status = ? WHERE name = ?`, string(status), name)
	if err != nil {
		return fmt.Errorf("loop store: set status %q: %w", name, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("loop %q not found", name)
	}
	return nil
}

// DeleteLoop removes a loop and all its run history (cascade).
func (s *LoopStore) DeleteLoop(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM loops WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("loop store: delete %q: %w", name, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("loop %q not found", name)
	}
	return nil
}

// RunLoop executes a loop's command immediately, records the result, and
// updates last_run_at / next_run_at. The command runs via /bin/sh -c,
// inheriting the caller's environment.
func (s *LoopStore) RunLoop(ctx context.Context, name string) (*LoopRun, error) {
	l, err := s.GetLoop(ctx, name)
	if err != nil {
		return nil, err
	}

	runID := s.idGen()
	now := s.now()

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO loop_runs (run_id, loop_id, started_at)
		VALUES (?, ?, ?)
	`, runID, l.LoopID, now); err != nil {
		return nil, fmt.Errorf("loop store: start run: %w", err)
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", l.Cmd)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	finished := s.now()
	exitCode := 0
	success := runErr == nil
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	nextRunAt := finished.Add(l.Cadence)

	tx, txErr := s.db.BeginTx(ctx, nil)
	if txErr != nil {
		return nil, fmt.Errorf("loop store: begin tx: %w", txErr)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE loop_runs
		SET finished_at = ?, exit_code = ?, stdout = ?, stderr = ?, success = ?
		WHERE run_id = ?
	`, finished, exitCode, stdout.String(), stderr.String(), boolToInt(success), runID); err != nil {
		return nil, fmt.Errorf("loop store: finish run: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE loops
		SET last_run_at = ?, next_run_at = ?, run_count = run_count + 1
		WHERE loop_id = ?
	`, finished, nextRunAt, l.LoopID); err != nil {
		return nil, fmt.Errorf("loop store: update loop metadata: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("loop store: commit run: %w", err)
	}

	lr := &LoopRun{
		RunID:      runID,
		LoopID:     l.LoopID,
		StartedAt:  now,
		FinishedAt: &finished,
		ExitCode:   &exitCode,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		Success:    success,
	}
	return lr, nil
}

// GetRuns returns the most recent run records for a loop, newest first.
// If limit <= 0, it defaults to 20.
func (s *LoopStore) GetRuns(ctx context.Context, name string, limit int) ([]*LoopRun, error) {
	l, err := s.GetLoop(ctx, name)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, loop_id, started_at, finished_at, exit_code, stdout, stderr, success
		FROM loop_runs
		WHERE loop_id = ?
		ORDER BY started_at DESC
		LIMIT ?
	`, l.LoopID, limit)
	if err != nil {
		return nil, fmt.Errorf("loop store: get runs: %w", err)
	}
	defer rows.Close()

	var runs []*LoopRun
	for rows.Next() {
		r, err := scanLoopRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// -- scan helpers --

type loopScanner interface {
	Scan(dest ...any) error
}

func scanLoop(r loopScanner) (*Loop, error) {
	var (
		cadenceStr string
		lastRunAt  sql.NullTime
		nextRunAt  sql.NullTime
		l          Loop
	)
	err := r.Scan(
		&l.LoopID, &l.Name, &l.Description, &l.Cmd, &cadenceStr, &l.Status,
		&l.CreatedAt, &lastRunAt, &nextRunAt, &l.RunCount,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("loop not found")
		}
		return nil, fmt.Errorf("loop store: scan: %w", err)
	}
	d, err := time.ParseDuration(cadenceStr)
	if err != nil {
		return nil, fmt.Errorf("loop store: invalid cadence %q: %w", cadenceStr, err)
	}
	l.Cadence = d
	if lastRunAt.Valid {
		t := lastRunAt.Time
		l.LastRunAt = &t
	}
	if nextRunAt.Valid {
		t := nextRunAt.Time
		l.NextRunAt = &t
	}
	return &l, nil
}

func scanLoopRun(r loopScanner) (*LoopRun, error) {
	var (
		finishedAt sql.NullTime
		exitCode   sql.NullInt64
		successInt int
		lr         LoopRun
	)
	if err := r.Scan(
		&lr.RunID, &lr.LoopID, &lr.StartedAt, &finishedAt, &exitCode,
		&lr.Stdout, &lr.Stderr, &successInt,
	); err != nil {
		return nil, fmt.Errorf("loop store: scan run: %w", err)
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		lr.FinishedAt = &t
	}
	if exitCode.Valid {
		c := int(exitCode.Int64)
		lr.ExitCode = &c
	}
	lr.Success = successInt == 1
	return &lr, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
