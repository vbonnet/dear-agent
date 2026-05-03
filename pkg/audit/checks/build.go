package checks

import (
	"context"
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// BuildCheck wraps `go build ./...` for the daily cadence. A failure
// is always P0 — a non-building tree blocks every other audit
// downstream and should page operators. The single emitted finding
// fingerprints by the first non-empty line of stderr so successive
// runs of the same break collapse to one row.
type BuildCheck struct{}

// Meta returns the check's identity.
func (BuildCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "build",
		Description:     "go build ./... must succeed across all packages",
		Cadence:         audit.CadenceDaily,
		SeverityCeiling: audit.SeverityP0,
	}
}

// Run shells out to `go build ./...`. Exit 0 → ok, no findings. Any
// other exit → one P0 finding describing the failure. Errors from
// the binary (missing `go` etc.) propagate as Result.Status = error.
func (BuildCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
	res := runCommand(ctx, env.WorkingDir, "go", "build", "./...")
	out := audit.Result{Status: audit.StatusOK, Stdout: res.Stdout, Stderr: res.Stderr}
	if res.Err != nil {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/build: invoke go build: %w", res.Err)
	}
	if res.ExitCode == 0 {
		return out, nil
	}
	title := firstNonEmptyLine(res.Stderr)
	if title == "" {
		title = fmt.Sprintf("go build exited %d", res.ExitCode)
	}
	out.Findings = []audit.Finding{{
		Fingerprint: audit.Fingerprint("build", env.WorkingDir, title),
		Severity:    audit.SeverityP0,
		Title:       title,
		Detail:      truncate(res.Stderr, 4000),
		Path:        env.WorkingDir,
		Suggested: audit.Remediation{
			Strategy: audit.StrategyAuto,
			Command:  "go build ./...",
		},
		Evidence: map[string]any{
			"exit_code": res.ExitCode,
			"stderr":    truncate(res.Stderr, 8000),
		},
	}}
	return out, nil
}

// truncate cuts s to at most n characters, appending an ellipsis when
// it had to cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…(truncated)"
}

func init() {
	audit.Default.MustRegister(BuildCheck{})
}
