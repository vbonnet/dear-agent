package audit

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Cadence names the recommended schedule bucket for a Check. It is a
// recommendation, not a contract — operators override per-check in
// .dear-agent.yml > audits.schedule. The buckets exist so the
// ecosystem agrees on what "daily" or "monthly" *means* in
// cost-vs-value terms (see ADR-011 §D2).
type Cadence string

// Recommended cadence buckets. Ordered cheap → expensive.
const (
	CadenceOnDemand Cadence = "on-demand"
	CadenceDaily    Cadence = "daily"
	CadenceWeekly   Cadence = "weekly"
	CadenceMonthly  Cadence = "monthly"
)

// IsValid reports whether c names a known cadence.
func (c Cadence) IsValid() bool {
	switch c {
	case CadenceOnDemand, CadenceDaily, CadenceWeekly, CadenceMonthly:
		return true
	}
	return false
}

// Severity ranks a Finding. The values are fixed at four levels per
// ADR-011 §D3. Adding a fifth requires an ADR amendment because the
// default-action tables in every consumer are keyed on this enum.
type Severity string

// Severity levels, ordered most → least urgent.
const (
	SeverityP0 Severity = "P0" // build-breaking; security; data-corrupting
	SeverityP1 Severity = "P1" // quality regression; new CVE; broken test
	SeverityP2 Severity = "P2" // drift; stale; minor inefficiency
	SeverityP3 Severity = "P3" // cosmetic; informational
)

// IsValid reports whether s names a known severity level.
func (s Severity) IsValid() bool {
	switch s {
	case SeverityP0, SeverityP1, SeverityP2, SeverityP3:
		return true
	}
	return false
}

// Rank returns 0 (most severe) through 3 (least severe). Used for
// sorting and threshold comparisons. Returns -1 for unknown values
// so a misuse is loud rather than silently sorting last.
func (s Severity) Rank() int {
	switch s {
	case SeverityP0:
		return 0
	case SeverityP1:
		return 1
	case SeverityP2:
		return 2
	case SeverityP3:
		return 3
	}
	return -1
}

// CheckMeta describes a Check's identity and defaults. Keep this small
// and stable: it is the discoverable contract every consumer reads.
//
// Cadence is the *recommended* default; the operator may override it
// per-repo. SeverityCeiling caps the worst severity any single Finding
// from this check can claim — e.g. a docs.dead-links check with a
// ceiling of P2 cannot manufacture P0 findings even if its emitter
// requests one. The runner clamps on read.
type CheckMeta struct {
	ID              string
	Description     string
	Cadence         Cadence
	SeverityCeiling Severity
	// RequiresNetwork is informational — `workflow audit dev` skips
	// network-touching checks unless --live is passed. Not enforced
	// at the package level.
	RequiresNetwork bool
}

// Validate returns a non-nil error if the metadata is malformed.
// IDs use dot-separated lowercase identifiers ("vuln.govulncheck").
// The validator is intentionally permissive on the ID alphabet so
// downstream tooling can extend the namespace; it only rejects empty
// strings, control characters, and obviously bad cadence/severity.
func (m CheckMeta) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("audit: CheckMeta.ID is empty")
	}
	for _, r := range m.ID {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("audit: CheckMeta.ID %q contains control char", m.ID)
		}
	}
	if !m.Cadence.IsValid() {
		return fmt.Errorf("audit: CheckMeta.Cadence %q is not a valid cadence", m.Cadence)
	}
	if !m.SeverityCeiling.IsValid() {
		return fmt.Errorf("audit: CheckMeta.SeverityCeiling %q is not a valid severity", m.SeverityCeiling)
	}
	return nil
}

// Env carries the per-run context every Check receives. RepoRoot is
// the absolute path to the working tree under audit; WorkingDir is
// the subtree the check should operate on (used for monorepo per-tree
// configs — see ADR-011 §5 trees: block). The two are equal for
// single-tree audits.
//
// Config carries check-specific knobs from .dear-agent.yml. Checks
// must tolerate an empty / nil Config without panicking.
//
// Logger is the run-scoped slog logger. Checks should log with
// slog.Debug for routine progress and slog.Warn for surprising state
// that is not yet a Finding.
type Env struct {
	RepoRoot   string
	WorkingDir string
	Config     map[string]any
	Logger     *slog.Logger
	// Now is overridable for deterministic tests. Production callers
	// leave it nil; the runner falls back to time.Now.
	Now func() time.Time
}

// Time returns Env.Now() when set, otherwise time.Now(). Centralised
// so checks never reach for time.Now directly and tests can pin
// timestamps at a single seam.
func (e Env) Time() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Result is what a Check returns: a list of Findings (possibly empty)
// plus a Status describing the *check's* health, distinct from the
// findings' severities. A Check that fails to run (binary missing,
// I/O error) reports StatusError with a non-nil error from Check.Run;
// a Check that ran and found nothing returns StatusOK with empty
// Findings.
type Result struct {
	Findings []Finding
	Status   CheckStatus
	// Duration is observed by the runner, not the check; checks may
	// leave it zero. The runner overwrites it before persistence.
	Duration time.Duration
	// Stdout / Stderr capture the underlying tool's raw output for
	// debugging. The runner may truncate before persisting; the
	// canonical Evidence map on each Finding is the durable surface.
	Stdout string
	Stderr string
}

// CheckStatus describes the check process itself, not its findings.
// A check can be Status=ok with 50 P0 findings (it ran fine and found
// problems) or Status=error with no findings (it failed to run at all).
// Downstream consumers care about both: a perpetually-erroring check
// is a substrate problem, not a code problem.
type CheckStatus string

// Check status values.
const (
	StatusOK      CheckStatus = "ok"
	StatusError   CheckStatus = "error"   // tool errored / couldn't run
	StatusSkipped CheckStatus = "skipped" // skipped via filter / network gate
	StatusTimeout CheckStatus = "timeout" // exceeded the runner's per-check budget
)

// Check is the interface every audit check satisfies. It is small on
// purpose (Meta + Run); accumulated checks should not balloon the
// surface. Stage-specific concerns (remediation, refinement,
// fingerprint normalisation) live in their own interfaces in this
// package.
type Check interface {
	Meta() CheckMeta
	Run(ctx context.Context, env Env) (Result, error)
}
