package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// SessionInboxCounter is the seam the Overseer drives once per tick to count
// non-archived ("inbox") sessions — those an operator still has to triage.
//
// It is deliberately separate from SessionGardener: the gardener *remediates*
// (archives stale stopped sessions via `agm session gc`), whereas the counter
// only *observes* the inbox depth so the Overseer can alert when sessions
// accumulate faster than hygiene reclaims them. Counting is a read-only probe;
// it never mutates session state.
//
// The default `agm session list` (no --all) shows only sessions with a live
// tmux pane, hiding stopped-but-unarchived sessions. A correct inbox count must
// include those, so implementations query the --all set and subtract archived
// sessions rather than trusting the default list.
type SessionInboxCounter interface {
	// InboxCount returns the number of non-archived sessions (active + stopped,
	// excluding archived). It is read-only.
	InboxCount(ctx context.Context) (int, error)
}

// defaultSessionInboxAlertThreshold is the inbox depth above which the Overseer
// escalates. More than this many non-archived sessions means stopped sessions
// are accumulating without being archived — a signal to run `agm session gc`.
const defaultSessionInboxAlertThreshold = 15

// defaultInboxCountTimeout bounds a single `agm session list` invocation.
const defaultInboxCountTimeout = 30 * time.Second

// AGMSessionInboxCounter is the production SessionInboxCounter. It shells out to
// `agm session list --all --json` and counts sessions whose status is not
// "archived". We shell out (rather than import the list op directly) for the
// same reason GCSessionGardener does: the op lives under agm/internal/ and Go's
// internal rule forbids importing it from pkg/vroom, and the process boundary
// keeps a single source of truth for the list/archive contract.
type AGMSessionInboxCounter struct {
	// AGMBin is the agm binary to invoke. Empty means "agm" (resolved on PATH).
	AGMBin string

	// Timeout bounds a single list pass. Zero means defaultInboxCountTimeout.
	Timeout time.Duration

	// run executes the command and returns stdout. It is a seam for tests;
	// nil means the real exec.CommandContext path.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// inboxListResult mirrors the subset of the JSON contract emitted by
// `agm session list --json` that the counter needs (see
// agm/internal/ops/session_list.go, ListSessionsResult / SessionSummary).
// Status is "active", "stopped", or "archived".
type inboxListResult struct {
	Sessions []struct {
		Status string `json:"status"`
	} `json:"sessions"`
}

// InboxCount runs one `agm session list --all --json` pass and counts the
// sessions whose status is not "archived". An unparseable failure (binary
// missing, timeout, garbage output) returns an error, which the Overseer records
// in the inbox-alert trail without failing the tick.
func (c *AGMSessionInboxCounter) InboxCount(ctx context.Context) (int, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultInboxCountTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin := c.AGMBin
	if bin == "" {
		bin = "agm"
	}
	// --all is required: the default list hides stopped-but-unarchived sessions.
	args := []string{"session", "list", "--all", "--json"}

	out, runErr := c.exec(ctx, bin, args...)

	// If the context deadline tripped (or a parent cancelled us), surface the
	// context cause directly rather than reporting truncated stdout as
	// "unparseable JSON".
	if ctx.Err() != nil {
		return 0, fmt.Errorf("session list: %w", ctx.Err())
	}

	var parsed inboxListResult
	if jsonErr := json.Unmarshal(out, &parsed); jsonErr != nil {
		if runErr != nil {
			return 0, fmt.Errorf("session list: %w (output: %q)", runErr, truncateOutput(out))
		}
		return 0, fmt.Errorf("session list: unparseable JSON: %w (output: %q)", jsonErr, truncateOutput(out))
	}

	count := 0
	for _, s := range parsed.Sessions {
		if s.Status != "archived" {
			count++
		}
	}
	return count, nil
}

func (c *AGMSessionInboxCounter) exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	if c.run != nil {
		return c.run(ctx, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...) //#nosec G204 -- name is a fixed binary, args are constant flags
	// Capture stdout only; list writes warnings/hints to stderr, which we
	// intentionally drop so JSON parsing sees clean stdout.
	return cmd.Output()
}
