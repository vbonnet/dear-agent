package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// FindingState is the lifecycle of one Finding row in the
// audit_findings table. Mirrors the CHECK constraint in schema.sql.
//
//	open → acknowledged → resolved
//	             ↓
//	          reopened (next audit run sees the fingerprint again
//	                     after a resolved entry — the runner flips it
//	                     back to open and bumps last_seen)
type FindingState string

// Canonical finding states.
const (
	FindingOpen         FindingState = "open"
	FindingAcknowledged FindingState = "acknowledged"
	FindingResolved     FindingState = "resolved"
	FindingReopened     FindingState = "reopened"
)

// IsValid reports whether s names a known finding state.
func (s FindingState) IsValid() bool {
	switch s {
	case FindingOpen, FindingAcknowledged, FindingResolved, FindingReopened:
		return true
	}
	return false
}

// IsTerminal reports whether s is a settled state (resolved). Used
// by the runner to decide whether re-emitting a fingerprint should
// reopen the finding.
func (s FindingState) IsTerminal() bool {
	return s == FindingResolved
}

// Finding is one defect a Check discovered. The fields divide into
// three groups:
//   - identity: CheckID + Fingerprint uniquely identify the
//     conceptual finding across runs; the FindingID is the row id
//     (assigned by the store) for one occurrence in one repo.
//   - description: Title / Detail / Path / Line are what humans read.
//   - lifecycle / metadata: Severity, Suggested, Evidence drive the
//     remediation pipeline.
//
// A Check builds Findings and returns them in Result.Findings; it
// never sets FindingID, FirstSeen, LastSeen, ResolvedAt, or State —
// the store owns those. Suggested may be left zero-valued if no
// remediation is known; the runner falls back to the severity-policy
// default in .dear-agent.yml.
type Finding struct {
	// FindingID is the store's row id. Set by Store.Upsert; checks
	// must leave this empty.
	FindingID string

	// Repo is the logical repository name (e.g. "dear-agent"). Set by
	// the runner from the audit run's Env, not by the check.
	Repo string

	CheckID     string
	Fingerprint string
	Severity    Severity

	Title  string
	Detail string
	Path   string
	Line   int

	Suggested Remediation

	// Evidence carries the raw tool output keyed for debugging
	// ("stdout", "rule_id", "vuln_id", ...). The store JSON-encodes
	// this; values must round-trip through encoding/json without loss.
	// Keep payloads small — large blobs belong in node_outputs, not
	// here.
	Evidence map[string]any

	// Lifecycle. Set by the store; checks must leave these zero.
	State      FindingState
	FirstSeen  time.Time
	LastSeen   time.Time
	ResolvedAt time.Time
}

// Validate returns a non-nil error if the finding is missing fields
// the store requires. It does NOT validate lifecycle fields — those
// are the store's concern.
func (f Finding) Validate() error {
	if f.CheckID == "" {
		return fmt.Errorf("audit: Finding.CheckID is empty")
	}
	if f.Fingerprint == "" {
		return fmt.Errorf("audit: Finding.Fingerprint is empty (check %q)", f.CheckID)
	}
	if !f.Severity.IsValid() {
		return fmt.Errorf("audit: Finding.Severity %q invalid (check %q)", f.Severity, f.CheckID)
	}
	if f.Title == "" {
		return fmt.Errorf("audit: Finding.Title is empty (check %q, fp %q)", f.CheckID, f.Fingerprint)
	}
	return nil
}

// Fingerprint computes a stable hash from the parts a check chooses
// as identity-bearing. The contract is: two findings of "the same
// underlying defect across two runs" must produce the same string;
// two findings of "different defects" must produce different strings.
// Checks call this helper rather than rolling their own SHA so the
// algorithm stays uniform — that is what the de-dup test in §D9
// asserts.
//
// Empty parts are skipped; the order of remaining parts is stable
// (joined with U+001E "Record Separator" so colons in paths are not
// confused with separators). The output is the first 16 bytes of
// SHA-256, hex-encoded — 32 chars, fits a SQLite TEXT key cleanly.
func Fingerprint(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	h := sha256.Sum256([]byte(strings.Join(out, "\x1e")))
	return hex.EncodeToString(h[:16])
}

// ClampSeverity returns the more permissive (numerically larger Rank)
// of want and ceiling. Used by the runner to prevent a check from
// emitting a severity higher than its declared SeverityCeiling.
// Returns ceiling when want is invalid so a misconfigured check
// degrades to its ceiling rather than crashing the run.
func ClampSeverity(want, ceiling Severity) Severity {
	if !want.IsValid() {
		return ceiling
	}
	if !ceiling.IsValid() {
		return want
	}
	if want.Rank() < ceiling.Rank() {
		return ceiling
	}
	return want
}
