package aggregator

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, matches pkg/workflow
)

//go:embed schema.sql
var sqliteSchema string

// sqliteOpenOptions matches pkg/workflow.sqliteOpenOptions: WAL mode for
// reader/writer concurrency, busy_timeout for retrying SQLITE_BUSY,
// foreign_keys on so the schema's PRAGMA is honored.
const sqliteOpenOptions = "?_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=foreign_keys(on)"

// Store is the persistence interface the Aggregator writes to and the
// Scorer/report CLI read from. SQLiteStore is the only implementation
// shipped in Phase 1.
type Store interface {
	Insert(ctx context.Context, sigs []Signal) error
	Recent(ctx context.Context, kind Kind, limit int) ([]Signal, error)
	Range(ctx context.Context, kind Kind, since time.Time) ([]Signal, error)
	Kinds(ctx context.Context) ([]Kind, error)
	Close() error
}

// SQLiteStore is the SQLite-backed Store. It is the substrate for the
// signals.db file the recommendation engine and the report CLI both
// read.
type SQLiteStore struct {
	db     *sql.DB
	ownsDB bool
}

// OpenSQLiteStore opens (or creates) signals.db at path and applies the
// schema. The returned store owns the *sql.DB; callers that want to
// share a DB across packages should use NewSQLiteStore.
func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("aggregator: OpenSQLiteStore: empty path")
	}
	db, err := sql.Open("sqlite", path+sqliteOpenOptions)
	if err != nil {
		return nil, fmt.Errorf("aggregator: open %s: %w", path, err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("aggregator: ping %s: %w", path, err)
	}
	if _, err := db.ExecContext(context.Background(), sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("aggregator: apply schema: %w", err)
	}
	return &SQLiteStore{db: db, ownsDB: true}, nil
}

// NewSQLiteStore wraps an already-open *sql.DB. The schema is applied
// on construction (idempotently); the caller retains ownership of the
// DB and Close becomes a no-op.
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("aggregator: NewSQLiteStore: nil db")
	}
	if _, err := db.ExecContext(context.Background(), sqliteSchema); err != nil {
		return nil, fmt.Errorf("aggregator: apply schema: %w", err)
	}
	return &SQLiteStore{db: db, ownsDB: false}, nil
}

// Close releases the underlying *sql.DB if the store owns it. Safe to
// call multiple times; subsequent calls return nil.
func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil || !s.ownsDB {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// Insert persists the given signals in a single transaction. Empty
// input is a no-op (returns nil). Every signal is validated before
// the transaction begins; a single invalid signal aborts the batch.
func (s *SQLiteStore) Insert(ctx context.Context, sigs []Signal) error {
	if len(sigs) == 0 {
		return nil
	}
	for i := range sigs {
		if err := sigs[i].Validate(); err != nil {
			return fmt.Errorf("aggregator: insert: signal %d: %w", i, err)
		}
		if sigs[i].Metadata != "" && !json.Valid([]byte(sigs[i].Metadata)) {
			return fmt.Errorf("aggregator: insert: signal %d: metadata is not valid JSON",
				i)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("aggregator: insert: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after commit

	const q = `INSERT INTO signals
        (signal_id, kind, subject, value, metadata_json, collected_at)
        VALUES (?, ?, ?, ?, ?, ?)`
	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("aggregator: insert: prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range sigs {
		md := sigs[i].Metadata
		if md == "" {
			md = "{}"
		}
		if _, err := stmt.ExecContext(ctx,
			sigs[i].ID,
			string(sigs[i].Kind),
			sigs[i].Subject,
			sigs[i].Value,
			md,
			sigs[i].CollectedAt.UTC(),
		); err != nil {
			return fmt.Errorf("aggregator: insert: exec signal %d: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("aggregator: insert: commit: %w", err)
	}
	return nil
}

// Recent returns the most recent `limit` signals of the given kind,
// ordered most-recent first. Limit 0 or negative returns all rows.
func (s *SQLiteStore) Recent(ctx context.Context, kind Kind, limit int) ([]Signal, error) {
	if err := kind.Validate(); err != nil {
		return nil, err
	}
	q := `SELECT signal_id, kind, subject, value, metadata_json, collected_at
        FROM signals WHERE kind = ?
        ORDER BY collected_at DESC, signal_id DESC`
	args := []any{string(kind)}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	return s.queryRows(ctx, q, args...)
}

// Range returns every signal of the given kind collected at or after
// since, ordered most-recent first.
func (s *SQLiteStore) Range(ctx context.Context, kind Kind, since time.Time) ([]Signal, error) {
	if err := kind.Validate(); err != nil {
		return nil, err
	}
	const q = `SELECT signal_id, kind, subject, value, metadata_json, collected_at
        FROM signals WHERE kind = ? AND collected_at >= ?
        ORDER BY collected_at DESC, signal_id DESC`
	return s.queryRows(ctx, q, string(kind), since.UTC())
}

// Kinds returns every distinct Kind currently present in the store.
// Useful for the report CLI's "show me what's there" mode.
func (s *SQLiteStore) Kinds(ctx context.Context) ([]Kind, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT kind FROM signals ORDER BY kind`)
	if err != nil {
		return nil, fmt.Errorf("aggregator: kinds: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Kind
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("aggregator: kinds: scan: %w", err)
		}
		out = append(out, Kind(k))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("aggregator: kinds: iter: %w", err)
	}
	return out, nil
}

// queryRows runs a SELECT and decodes the rows into Signals. The query
// must select the six signal columns in the canonical order.
func (s *SQLiteStore) queryRows(ctx context.Context, q string, args ...any) ([]Signal, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("aggregator: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Signal
	for rows.Next() {
		var sig Signal
		var kindStr string
		if err := rows.Scan(
			&sig.ID, &kindStr, &sig.Subject, &sig.Value,
			&sig.Metadata, &sig.CollectedAt,
		); err != nil {
			return nil, fmt.Errorf("aggregator: scan: %w", err)
		}
		sig.Kind = Kind(kindStr)
		if sig.Metadata == "{}" {
			sig.Metadata = ""
		}
		out = append(out, sig)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("aggregator: iter: %w", err)
	}
	return out, nil
}

