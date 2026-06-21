package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// SessionGardener is the seam the Overseer drives once per tick to archive
// completed/stopped worker sessions that no longer have a live tmux pane.
//
// It is deliberately separate from SessionArchiver (the memory-pressure
// reaper's critical-tier action): hygiene runs on every tick as routine
// maintenance and is age-filtered (`--older-than`), whereas SessionArchiver
// fires only under memory pressure. GC is idempotent and conservative by
// construction — active sessions, sessions with a live tmux pane, and
// protected supervisor roles are always skipped — so it is safe to invoke on
// a cadence without a session-to-pane mapping in the Overseer.
type SessionGardener interface {
	// GC archives stale stopped sessions and reports what it scanned and
	// archived. It is best-effort: a partial pass (some archived, some
	// errored) is reported in SessionGCStats and does not fail the tick.
	GC(ctx context.Context) (SessionGCStats, error)
}

// SessionGCStats reports the outcome of one session-hygiene pass. The fields
// mirror the `agm session gc` summary contract.
type SessionGCStats struct {
	// Scanned is the total number of session manifests examined.
	Scanned int
	// Archived is the number of stale sessions archived this pass.
	Archived int
	// Skipped is the number left untouched (active, live tmux pane,
	// protected role, or too recent).
	Skipped int
	// Errors is the number of sessions that failed to archive.
	Errors int
}

// GCSessionGardener is the production SessionGardener. It shells out to
// `agm session gc --older-than=<dur> --json`, which archives stopped sessions
// whose owning work is done. We shell out rather than import the gc op
// directly because it lives under agm/internal/ and Go's internal rule forbids
// importing it from pkg/vroom; the process boundary also keeps a single source
// of truth for the archive safety checks (the same CLI an operator runs).
type GCSessionGardener struct {
	// AGMBin is the agm binary to invoke. Empty means "agm" (resolved on PATH).
	AGMBin string

	// OlderThan filters to sessions inactive for at least this duration.
	// Zero means defaultHygieneOlderThan.
	OlderThan time.Duration

	// Timeout bounds a single gc pass. Zero means defaultHygieneTimeout.
	Timeout time.Duration

	// run executes the command and returns stdout. It is a seam for tests;
	// nil means the real exec.CommandContext path.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

const (
	defaultHygieneOlderThan = 10 * time.Minute
	defaultHygieneTimeout   = 30 * time.Second
)

// gcCLIResult mirrors the --json contract emitted by `agm session gc --json`
// (see agm/internal/ops/session_gc.go, GCResult). Keep the field tags in sync.
type gcCLIResult struct {
	Scanned  int `json:"scanned"`
	Archived int `json:"archived"`
	Skipped  int `json:"skipped"`
	Errors   int `json:"errors"`
}

// GC runs one `agm session gc` pass and maps its JSON output into
// SessionGCStats. It is best-effort: as long as the CLI emits parseable JSON
// the counts are returned even on a non-zero exit. Only an unparseable failure
// (binary missing, timeout, garbage output) returns an error, which the
// Overseer records in the hygiene trail without failing the tick.
func (g *GCSessionGardener) GC(ctx context.Context) (SessionGCStats, error) {
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = defaultHygieneTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	older := g.OlderThan
	if older <= 0 {
		older = defaultHygieneOlderThan
	}

	bin := g.AGMBin
	if bin == "" {
		bin = "agm"
	}
	args := []string{"session", "gc", "--json", "--older-than", older.String()}

	out, runErr := g.exec(ctx, bin, args...)

	// If the context deadline tripped (or a parent cancelled us), surface the
	// context cause directly rather than reporting truncated stdout as
	// "unparseable JSON".
	if ctx.Err() != nil {
		return SessionGCStats{}, fmt.Errorf("session gc: %w", ctx.Err())
	}

	// Parse whatever JSON we got first: a non-zero exit can still carry a valid
	// summary (e.g. some archives failed). Prefer the structured result.
	var parsed gcCLIResult
	if jsonErr := json.Unmarshal(out, &parsed); jsonErr != nil {
		if runErr != nil {
			return SessionGCStats{}, fmt.Errorf("session gc: %w (output: %q)", runErr, truncateOutput(out))
		}
		return SessionGCStats{}, fmt.Errorf("session gc: unparseable JSON: %w (output: %q)", jsonErr, truncateOutput(out))
	}

	return SessionGCStats(parsed), nil
}

func (g *GCSessionGardener) exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	if g.run != nil {
		return g.run(ctx, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...) //#nosec G204 -- name is a fixed binary, args are constant flags
	// Capture stdout only; gc writes progress/warnings to stderr, which we
	// intentionally drop so JSON parsing sees clean stdout.
	return cmd.Output()
}
