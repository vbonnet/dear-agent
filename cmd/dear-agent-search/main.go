// Command dear-agent-search is the user-facing surface over
// pkg/source.Adapter. It runs Fetch and renders results either as
// human-readable text or as JSON; the SQLite adapter joins through to
// the workflow runs table so each result is annotated with the
// run/node that produced it.
//
// Usage:
//
//	dear-agent-search "deep research routing"
//	dear-agent-search --since 30d "wayfinder"
//	dear-agent-search --cue research --json "topic"
//	dear-agent-search --db ./runs.db --sources ./sources.db "topic"
//
// Exit codes: 0 = ok (zero or more results), 1 = adapter / IO error,
// 2 = bad usage. Phase 3.5 ships the read path; --watch and pagination
// are not in scope.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vbonnet/dear-agent/pkg/source"
	sqliteadapter "github.com/vbonnet/dear-agent/pkg/source/sqlite"
)

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable entry point: takes args/stdout/stderr so tests
// can drive it without touching real os.* state.
func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("dear-agent-search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		dbPath     = fs.String("db", "runs.db", "path to runs.db")
		sourcePath = fs.String("sources", "", "path to sources.db (default: same as --db)")
		asJSON     = fs.Bool("json", false, "emit machine-readable JSON")
		k          = fs.Int("k", 10, "max results")
		since      = fs.String("since", "", "only return sources indexed after this (e.g. 30d, 24h, 2026-04-01)")
		until      = fs.String("until", "", "only return sources indexed before this")
		workItem   = fs.String("work-item", "", "filter by work item id (run_id or run_id/node_id)")
		cues       stringList
	)
	fs.Var(&cues, "cue", "filter by cue; repeat for AND")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s [flags] <query>\n\nFlags:\n", "dear-agent-search")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 && *workItem == "" && len(cues) == 0 && *since == "" {
		fs.Usage()
		return 2
	}
	query := strings.Join(fs.Args(), " ")

	// Sources DB defaults to the runs DB so the JOIN against `runs`
	// works without any extra wiring (see source.go in dear-agent-mcp
	// for the same convention).
	srcDB := *sourcePath
	if srcDB == "" {
		srcDB = *dbPath
	}
	db, err := openDB(srcDB)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()

	a, err := sqliteadapter.New(db)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer a.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	q := source.FetchQuery{
		Query: query,
		K:     *k,
		Filters: source.Filters{
			Cues:     cues,
			WorkItem: *workItem,
		},
	}
	if *since != "" {
		ts, err := parseSince(*since, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(stderr, "invalid --since: %v\n", err)
			return 2
		}
		q.Filters.After = &ts
	}
	if *until != "" {
		ts, err := parseSince(*until, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(stderr, "invalid --until: %v\n", err)
			return 2
		}
		q.Filters.Before = &ts
	}

	got, err := a.Fetch(ctx, q)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	annotated := joinWithRuns(ctx, db, got)
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"results": annotated})
		return 0
	}
	fmt.Fprint(stdout, formatText(query, annotated))
	return 0
}

// openDB returns a *sql.DB for path with the shared pragma block. The
// search CLI is read-mostly; busy_timeout still helps when concurrent
// runs are appending sources rows in the background.
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}

// result is the wire shape emitted by --json and rendered as text.
type result struct {
	URI       string    `json:"uri"`
	Title     string    `json:"title"`
	Snippet   string    `json:"snippet"`
	Score     float64   `json:"score"`
	IndexedAt time.Time `json:"indexed_at"`
	WorkItem  string    `json:"work_item,omitempty"`
	RunID     string    `json:"run_id,omitempty"`
	NodeID    string    `json:"node_id,omitempty"`
	RunState  string    `json:"run_state,omitempty"`
	Cues      []string  `json:"cues,omitempty"`
}

// joinWithRuns annotates each Source with its workflow context. The
// JOIN is best-effort — if a Source's WorkItem doesn't match a known
// run (e.g. user-added via AddSource MCP), the run-side fields stay
// empty rather than failing the request.
func joinWithRuns(ctx context.Context, db *sql.DB, sources []source.Source) []result {
	out := make([]result, 0, len(sources))
	for _, s := range sources {
		r := result{
			URI: s.URI, Title: s.Title, Snippet: s.Snippet,
			Score: s.Score, IndexedAt: s.IndexedAt,
			WorkItem: s.Metadata.WorkItem,
			Cues:     s.Metadata.Cues,
		}
		runID, nodeID := splitWorkItem(s.Metadata.WorkItem)
		r.RunID = runID
		r.NodeID = nodeID
		if runID != "" {
			row := db.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, runID)
			var state string
			if err := row.Scan(&state); err == nil {
				r.RunState = state
			}
		}
		out = append(out, r)
	}
	return out
}

func splitWorkItem(w string) (runID, nodeID string) {
	if w == "" {
		return "", ""
	}
	if i := strings.Index(w, "/"); i >= 0 {
		return w[:i], w[i+1:]
	}
	return w, ""
}

// formatText renders the result list as compact, scannable text. Two
// lines per result: header (rank, title, work item) and indented
// snippet.
func formatText(query string, rs []result) string {
	var sb strings.Builder
	if query != "" {
		fmt.Fprintf(&sb, "query: %q\n", query)
	}
	if len(rs) == 0 {
		sb.WriteString("no results\n")
		return sb.String()
	}
	for i, r := range rs {
		header := fmt.Sprintf("[%d] %s", i+1, displayTitle(r))
		if r.RunID != "" {
			header += fmt.Sprintf("  (run %s", short(r.RunID))
			if r.NodeID != "" {
				header += "/" + r.NodeID
			}
			if r.RunState != "" {
				header += " " + r.RunState
			}
			header += ")"
		}
		fmt.Fprintln(&sb, header)
		fmt.Fprintf(&sb, "    %s\n", r.URI)
		if r.Snippet != "" {
			fmt.Fprintf(&sb, "    %s\n", r.Snippet)
		}
	}
	return sb.String()
}

func displayTitle(r result) string {
	if r.Title != "" {
		return r.Title
	}
	return r.URI
}

func short(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8]
}

// parseSince accepts three forms: a Go duration ("24h", "30d"), a
// date ("2026-04-01"), or a full RFC3339 timestamp. "d" is treated as
// 24h since Go's time.ParseDuration doesn't accept it natively.
func parseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty value")
	}
	// Days shortcut: "30d" → 720h ago.
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
			return now.Add(-time.Duration(n) * 24 * time.Hour), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognised time %q (try 30d, 24h, or 2026-04-01)", s)
}
