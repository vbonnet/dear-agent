package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// OrphanReclaimer is the production ResourceReclaimer. It remediates FD / vnode
// / gopls pressure by invoking the orphan reaper CLI
// (`agm session reap-orphans --json`), which kills target processes reparented
// to PID 1 (their owning session died). The reaper is conservative by
// construction — only PPID==1 processes matching the allowlist are killed — so
// no session-to-process mapping is required and a live process is never touched.
//
// We shell out rather than import the reaper package directly because the reaper
// lives under agm/internal/ and Go's internal rule forbids importing it from
// pkg/vroom. The process boundary also keeps a single source of truth for the
// kill safety logic: the same CLI the Stop hook and launchd backstop run.
type OrphanReclaimer struct {
	// AGMBin is the agm binary to invoke. Empty means "agm" (resolved on PATH).
	AGMBin string

	// Targets is the process allowlist passed to --targets. Nil/empty leaves
	// the flag off so the CLI applies its own default (gopls, agm-mcp-server).
	Targets []string

	// Timeout bounds a single reclaim pass. Zero means defaultReclaimTimeout.
	Timeout time.Duration

	// run executes the command and returns stdout. It is a seam for tests;
	// nil means the real exec.CommandContext path.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

const defaultReclaimTimeout = 30 * time.Second

// orphanReapResult mirrors the --json contract emitted by
// `agm session reap-orphans --json` (see agm/cmd/agm/session_reap_orphans.go,
// orphanReapJSON). Keep the field tags in sync.
type orphanReapResult struct {
	Targets      []string `json:"targets"`
	DryRun       bool     `json:"dry_run"`
	OrphansFound int      `json:"orphans_found"`
	Killed       int      `json:"killed"`
	Failed       int      `json:"failed"`
	KilledPIDs   []int    `json:"killed_pids"`
	FailedPIDs   []int    `json:"failed_pids"`
}

// Reclaim runs one reaper pass and maps its JSON output into a ReclaimResult.
// It is best-effort: as long as the CLI emits parseable JSON the counts are
// returned even on a non-zero exit (a partial reap is still progress). Only an
// unparseable failure (binary missing, timeout, garbage output) returns an
// error, which the Overseer records in the reclaim trail without failing Tick.
func (r *OrphanReclaimer) Reclaim(ctx context.Context) (ReclaimResult, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultReclaimTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin := r.AGMBin
	if bin == "" {
		bin = "agm"
	}
	args := []string{"session", "reap-orphans", "--json"}
	if len(r.Targets) > 0 {
		args = append(args, "--targets", strings.Join(r.Targets, ","))
	}

	out, runErr := r.exec(ctx, bin, args...)

	// If the context deadline tripped (or a parent cancelled us), the run error
	// is just a side effect of the kill — surface the context cause directly
	// rather than reporting truncated/empty stdout as "unparseable JSON".
	if ctx.Err() != nil {
		return ReclaimResult{}, fmt.Errorf("orphan reaper: %w", ctx.Err())
	}

	// Parse whatever JSON we got first: a non-zero exit can still carry a valid
	// summary (e.g. some kills failed). Prefer the structured result.
	var parsed orphanReapResult
	if jsonErr := json.Unmarshal(out, &parsed); jsonErr != nil {
		if runErr != nil {
			return ReclaimResult{}, fmt.Errorf("orphan reaper: %w (output: %q)", runErr, truncateOutput(out))
		}
		return ReclaimResult{}, fmt.Errorf("orphan reaper: unparseable JSON: %w (output: %q)", jsonErr, truncateOutput(out))
	}

	return ReclaimResult{
		OrphansFound:  parsed.OrphansFound,
		OrphansKilled: parsed.Killed,
		OrphansFailed: parsed.Failed,
	}, nil
}

func (r *OrphanReclaimer) exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.run != nil {
		return r.run(ctx, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...) //#nosec G204 -- name is a fixed binary, args are constant flags
	// Capture stdout only; the reaper writes slog progress to stderr, which we
	// intentionally drop so JSON parsing sees clean stdout.
	return cmd.Output()
}

func truncateOutput(b []byte) string {
	const maxLen = 200
	s := strings.TrimSpace(string(b))
	// Truncate on rune boundaries so a multi-byte UTF-8 sequence is never cut
	// in half (which would yield an invalid-UTF-8 string in the error message).
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "…"
}
