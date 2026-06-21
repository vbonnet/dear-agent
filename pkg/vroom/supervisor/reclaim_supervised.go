package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// SupervisedReclaimer is a ResourceReclaimer that remediates memory pressure by
// invoking the AGM-aware process reaper (`agm-aware-reaper --json`). Unlike
// OrphanReclaimer — which only kills processes reparented to PID 1 — this reaper
// consults the AGM session database, protects the entire process subtree of
// every live session, and reaps unsupervised high-RSS / idle processes with a
// graduated SIGTERM→grace→SIGKILL escalation.
//
// As with OrphanReclaimer we shell out rather than import the reaper package:
// it lives under agm/internal/ and Go's internal rule forbids importing it from
// pkg/vroom. The process boundary also keeps the kill-safety logic (session-DB
// consultation, fail-closed behaviour) in a single binary the Overseer and any
// operator share.
type SupervisedReclaimer struct {
	// Bin is the agm-aware-reaper binary to invoke. Empty means
	// "agm-aware-reaper" (resolved on PATH).
	Bin string

	// MaxRSSMB is passed through as --max-rss-mb: only processes above this RSS
	// are candidates. Zero leaves the flag off so the binary applies its
	// default.
	MaxRSSMB int64

	// GracePeriod is passed through as --grace. Zero leaves the flag off.
	GracePeriod time.Duration

	// Timeout bounds a single reclaim pass. Zero uses defaultReclaimTimeout.
	Timeout time.Duration

	// run executes the command and returns stdout. Seam for tests; nil uses
	// exec.CommandContext.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// supervisedReapResult mirrors the --json contract emitted by
// agm-aware-reaper (see agm/cmd/agm-aware-reaper/main.go, reapResultJSON).
// Keep the field tags in sync.
type supervisedReapResult struct {
	DryRun     bool     `json:"dry_run"`
	Candidates int      `json:"candidates"`
	Protected  int      `json:"protected"`
	Reapable   int      `json:"reapable"`
	Terminated int      `json:"terminated"`
	Errors     []string `json:"errors"`
}

// Reclaim runs one AGM-aware reap pass and maps its JSON output into a
// ReclaimResult. Best-effort, like OrphanReclaimer: a parseable summary is
// returned even on a non-zero exit (a partial reap is still progress); only an
// unparseable failure returns an error, which the Overseer records without
// failing Tick.
func (r *SupervisedReclaimer) Reclaim(ctx context.Context) (ReclaimResult, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultReclaimTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin := r.Bin
	if bin == "" {
		bin = "agm-aware-reaper"
	}
	args := []string{"--json"}
	if r.MaxRSSMB > 0 {
		args = append(args, "--max-rss-mb", strconv.FormatInt(r.MaxRSSMB, 10))
	}
	if r.GracePeriod > 0 {
		args = append(args, "--grace", r.GracePeriod.String())
	}

	out, runErr := r.exec(ctx, bin, args...)

	if ctx.Err() != nil {
		return ReclaimResult{}, fmt.Errorf("agm-aware reaper: %w", ctx.Err())
	}

	var parsed supervisedReapResult
	if jsonErr := json.Unmarshal(out, &parsed); jsonErr != nil {
		if runErr != nil {
			return ReclaimResult{}, fmt.Errorf("agm-aware reaper: %w (output: %q)", runErr, truncateOutput(out))
		}
		return ReclaimResult{}, fmt.Errorf("agm-aware reaper: unparseable JSON: %w (output: %q)", jsonErr, truncateOutput(out))
	}

	return ReclaimResult{
		OrphansFound:  parsed.Reapable,
		OrphansKilled: parsed.Terminated,
		OrphansFailed: len(parsed.Errors),
	}, nil
}

func (r *SupervisedReclaimer) exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.run != nil {
		return r.run(ctx, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...) //#nosec G204 -- fixed binary, constant flags
	// Capture stdout only; the reaper writes slog progress to stderr.
	return cmd.Output()
}

// ensure SupervisedReclaimer satisfies the seam at compile time.
var _ ResourceReclaimer = (*SupervisedReclaimer)(nil)
