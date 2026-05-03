// Package sqlite is the default pkg/source.Adapter implementation:
// SQLite + FTS5. It backs `dear-agent search`, the FetchSource /
// AddSource MCP tools, and the engram_indexed durability tier.
//
// The schema is two tables — sources and sources_fts — defined in
// schema.sql and embedded into the binary. The FTS5 table mirrors
// uri, title, snippet, and content so MATCH queries cover all three.
// Cues, work_item, role, and indexed_at are filtered in WHERE because
// they are exact-match, not full-text.
//
// Performance target (per ROADMAP / BACKLOG): Fetch P95 < 50 ms on
// 10K rows. Validated by adapter_perf_test.go.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver with database/sql

	"github.com/vbonnet/dear-agent/pkg/source"
)

//go:embed schema.sql
var schemaSQL string

// Name is the backend identifier the MCP layer matches against
// FetchQuery.Filters.Backend.
const Name = "sqlite"

// defaultK is the cap Fetch applies when FetchQuery.K is zero. Matches
// the synthesis doc's "k=10 by default" recommendation.
const defaultK = 10

// openOptions are the pragmas every sources.db needs. Same shape as
// pkg/workflow's runs.db: WAL for concurrent readers, busy_timeout so
// concurrent writers retry instead of failing, foreign_keys for the
// cascade rules.
const openOptions = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"

// Adapter is the SQLite + FTS5 implementation of source.Adapter.
type Adapter struct {
	db     *sql.DB
	ownsDB bool
	now    func() time.Time
}

// Open opens (or creates) sources.db at path, applies the schema, and
// returns a ready-to-use Adapter. The caller is responsible for Close.
func Open(path string) (*Adapter, error) {
	db, err := sql.Open("sqlite", path+openOptions)
	if err != nil {
		return nil, fmt.Errorf("source/sqlite: open %s: %w", path, err)
	}
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("source/sqlite: ping %s: %w", path, err)
	}
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("source/sqlite: apply schema: %w", err)
	}
	return &Adapter{db: db, ownsDB: true, now: time.Now}, nil
}

// New wraps a caller-provided *sql.DB. Useful when the workflow
// database and the sources database live in the same file (so JOINs
// across runs ↔ sources are cheap). The schema is applied if the
// tables don't already exist; the caller retains ownership of db.
func New(db *sql.DB) (*Adapter, error) {
	if db == nil {
		return nil, fmt.Errorf("source/sqlite: New: db is nil")
	}
	if _, err := db.ExecContext(context.Background(), schemaSQL); err != nil {
		return nil, fmt.Errorf("source/sqlite: apply schema: %w", err)
	}
	return &Adapter{db: db, ownsDB: false, now: time.Now}, nil
}

// Name returns the backend identifier.
func (a *Adapter) Name() string { return Name }

// HealthCheck pings the database.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("source/sqlite: closed")
	}
	if err := a.db.PingContext(ctx); err != nil {
		return fmt.Errorf("source/sqlite: ping: %w", err)
	}
	return nil
}

// Close releases the underlying connection if the Adapter owns it.
func (a *Adapter) Close() error {
	if a == nil || a.db == nil || !a.ownsDB {
		return nil
	}
	err := a.db.Close()
	a.db = nil
	return err
}

// DB returns the underlying *sql.DB so the search CLI can join
// `sources` against `runs` / `nodes` from the workflow schema. Callers
// must NOT close it — the Adapter owns lifecycle.
func (a *Adapter) DB() *sql.DB { return a.db }

// Add stores or updates s. If a row with the same URI already exists,
// content + metadata are overwritten in place and indexed_at refreshed.
// The FTS5 mirror is updated atomically with the row.
func (a *Adapter) Add(ctx context.Context, s source.Source) (source.Ref, error) {
	if s.URI == "" {
		return source.Ref{}, fmt.Errorf("source/sqlite: Add: URI is required")
	}
	indexed := s.IndexedAt
	if indexed.IsZero() {
		indexed = a.now().UTC()
	}

	customJSON := ""
	if len(s.Metadata.Custom) > 0 {
		b, err := json.Marshal(s.Metadata.Custom)
		if err != nil {
			return source.Ref{}, fmt.Errorf("source/sqlite: Add: marshal custom: %w", err)
		}
		customJSON = string(b)
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return source.Ref{}, fmt.Errorf("source/sqlite: Add: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sources (uri, title, snippet, content, cues, work_item, role, confidence, src_origin, custom_json, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (uri) DO UPDATE SET
		    title       = excluded.title,
		    snippet     = excluded.snippet,
		    content     = excluded.content,
		    cues        = excluded.cues,
		    work_item   = excluded.work_item,
		    role        = excluded.role,
		    confidence  = excluded.confidence,
		    src_origin  = excluded.src_origin,
		    custom_json = excluded.custom_json,
		    indexed_at  = excluded.indexed_at
	`,
		s.URI, s.Title, s.Snippet, s.Content,
		encodeCues(s.Metadata.Cues), s.Metadata.WorkItem, s.Metadata.Role, s.Metadata.Confidence,
		s.Metadata.Source, customJSON, indexed,
	); err != nil {
		return source.Ref{}, fmt.Errorf("source/sqlite: Add: upsert: %w", err)
	}

	// FTS5 mirror — DELETE + INSERT keeps the index in sync on update
	// without depending on FTS5-specific UPSERT semantics, which differ
	// across builds.
	if _, err := tx.ExecContext(ctx, `DELETE FROM sources_fts WHERE uri = ?`, s.URI); err != nil {
		return source.Ref{}, fmt.Errorf("source/sqlite: Add: fts delete: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sources_fts (uri, title, snippet, content) VALUES (?, ?, ?, ?)
	`, s.URI, s.Title, s.Snippet, string(s.Content)); err != nil {
		return source.Ref{}, fmt.Errorf("source/sqlite: Add: fts insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return source.Ref{}, fmt.Errorf("source/sqlite: Add: commit: %w", err)
	}
	return source.Ref{URI: s.URI, Backend: Name, IndexedAt: indexed}, nil
}

// Fetch returns up to q.K Sources matching q. Ordering is descending
// FTS5 rank when Query is non-empty, descending IndexedAt otherwise.
func (a *Adapter) Fetch(ctx context.Context, q source.FetchQuery) ([]source.Source, error) {
	k := q.K
	if k <= 0 {
		k = defaultK
	}

	var (
		sb        strings.Builder
		args      []any
		hasQuery  = strings.TrimSpace(q.Query) != ""
		whereJoin = "WHERE"
	)

	if hasQuery {
		// Join through FTS5; rank is exposed as bm25() — lower is better,
		// so we ORDER BY bm25 ASC. Score in the result is -bm25 so
		// callers can sort descending if they want.
		sb.WriteString(`
			SELECT s.uri, s.title, s.snippet, s.content, s.cues, s.work_item,
			       s.role, s.confidence, s.src_origin, s.custom_json, s.indexed_at,
			       bm25(sources_fts) AS rank
			FROM sources_fts
			JOIN sources s ON s.uri = sources_fts.uri
			WHERE sources_fts MATCH ?
		`)
		args = append(args, escapeFTSQuery(q.Query))
		whereJoin = "AND"
	} else {
		sb.WriteString(`
			SELECT s.uri, s.title, s.snippet, s.content, s.cues, s.work_item,
			       s.role, s.confidence, s.src_origin, s.custom_json, s.indexed_at,
			       0 AS rank
			FROM sources s
		`)
	}

	for _, cue := range q.Filters.Cues {
		sb.WriteString(" ")
		sb.WriteString(whereJoin)
		// cues are stored TAB-separated with leading+trailing tabs so
		// LIKE matches on the exact token boundary.
		sb.WriteString(" s.cues LIKE ?")
		args = append(args, "%\t"+cue+"\t%")
		whereJoin = "AND"
	}
	if q.Filters.WorkItem != "" {
		sb.WriteString(" ")
		sb.WriteString(whereJoin)
		sb.WriteString(" (s.work_item = ? OR s.work_item LIKE ?)")
		args = append(args, q.Filters.WorkItem, q.Filters.WorkItem+"/%")
		whereJoin = "AND"
	}
	if q.Filters.After != nil {
		sb.WriteString(" ")
		sb.WriteString(whereJoin)
		sb.WriteString(" s.indexed_at >= ?")
		args = append(args, q.Filters.After.UTC())
		whereJoin = "AND"
	}
	if q.Filters.Before != nil {
		sb.WriteString(" ")
		sb.WriteString(whereJoin)
		sb.WriteString(" s.indexed_at < ?")
		args = append(args, q.Filters.Before.UTC())
	}

	if hasQuery {
		sb.WriteString(" ORDER BY rank ASC, s.indexed_at DESC LIMIT ?")
	} else {
		sb.WriteString(" ORDER BY s.indexed_at DESC LIMIT ?")
	}
	args = append(args, k)

	rows, err := a.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("source/sqlite: Fetch: %w", err)
	}
	defer rows.Close()

	out := make([]source.Source, 0, k)
	for rows.Next() {
		var (
			s          source.Source
			cues       string
			customJSON string
			rank       float64
		)
		if err := rows.Scan(
			&s.URI, &s.Title, &s.Snippet, &s.Content, &cues, &s.Metadata.WorkItem,
			&s.Metadata.Role, &s.Metadata.Confidence, &s.Metadata.Source, &customJSON, &s.IndexedAt,
			&rank,
		); err != nil {
			return nil, fmt.Errorf("source/sqlite: Fetch: scan: %w", err)
		}
		s.Metadata.Cues = decodeCues(cues)
		if customJSON != "" {
			_ = json.Unmarshal([]byte(customJSON), &s.Metadata.Custom)
		}
		// FTS5 bm25 lower = better. Surface as -rank so a higher Score
		// always means "more relevant".
		s.Score = -rank
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("source/sqlite: Fetch: rows: %w", err)
	}
	return out, nil
}

// FetchByURI is a convenience used by the search CLI's --uri flag and
// by tests. Returns ErrNotFound if no row matches.
func (a *Adapter) FetchByURI(ctx context.Context, uri string) (source.Source, error) {
	got, err := a.Fetch(ctx, source.FetchQuery{Filters: source.Filters{}, K: 1, Query: ""})
	if err != nil {
		return source.Source{}, err
	}
	for _, s := range got {
		if s.URI == uri {
			return s, nil
		}
	}
	// Fall back to a direct lookup so callers who pass a URI that
	// happens to be off the LIMIT-1 head still get the right answer.
	row := a.db.QueryRowContext(ctx, `
		SELECT uri, title, snippet, content, cues, work_item, role, confidence, src_origin, custom_json, indexed_at
		FROM sources WHERE uri = ?
	`, uri)
	var (
		s          source.Source
		cues       string
		customJSON string
	)
	if err := row.Scan(&s.URI, &s.Title, &s.Snippet, &s.Content, &cues, &s.Metadata.WorkItem,
		&s.Metadata.Role, &s.Metadata.Confidence, &s.Metadata.Source, &customJSON, &s.IndexedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return source.Source{}, source.ErrNotFound
		}
		return source.Source{}, err
	}
	s.Metadata.Cues = decodeCues(cues)
	if customJSON != "" {
		_ = json.Unmarshal([]byte(customJSON), &s.Metadata.Custom)
	}
	return s, nil
}

// encodeCues stores cues as TAB-separated with leading and trailing tabs
// so a LIKE '%\tCUE\t%' query matches on token boundaries.
func encodeCues(cues []string) string {
	if len(cues) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteByte('\t')
	for _, c := range cues {
		// drop tabs from cue content to keep the boundary unambiguous
		c = strings.ReplaceAll(c, "\t", " ")
		sb.WriteString(c)
		sb.WriteByte('\t')
	}
	return sb.String()
}

func decodeCues(s string) []string {
	s = strings.Trim(s, "\t")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\t")
}

// escapeFTSQuery wraps the user query so FTS5's MATCH parser doesn't
// trip on stray punctuation. Multi-word queries are AND'd; phrases are
// preserved when the user wraps them in double quotes themselves.
//
// Strategy: split on whitespace, drop tokens shorter than 1 char, wrap
// each remaining token in double quotes. This sacrifices the ability
// to write FTS5 operators (NEAR, *, OR) — fine for the engine's
// internal Fetch surface; if we expose a power-user query later we'll
// add a "raw" mode.
func escapeFTSQuery(q string) string {
	tokens := strings.Fields(q)
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		// strip embedded double-quotes; FTS5 phrase syntax doesn't
		// accept escaped quotes inside a phrase
		t = strings.ReplaceAll(t, `"`, "")
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		out = append(out, `"`+t+`"`)
	}
	if len(out) == 0 {
		return `""`
	}
	return strings.Join(out, " ")
}
