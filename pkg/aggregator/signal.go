package aggregator

import (
	"fmt"
	"strings"
	"time"
)

// Kind enumerates the categories of project-health signals the aggregator
// stores. The recommendation engine ranks per-kind first, then sums the
// weighted scores across kinds.
type Kind string

// Kind values. Stringly-typed at the schema boundary so adding a kind in a
// later phase is a single constant + scorer-defaults edit.
const (
	KindGitActivity    Kind = "git_activity"
	KindLintTrend      Kind = "lint_trend"
	KindTestCoverage   Kind = "test_coverage"
	KindDepFreshness   Kind = "dep_freshness"
	KindSecurityAlerts Kind = "security_alerts"
)

// AllKinds returns the Kind values shipped in Phase 1, in declaration order.
// Used by report rendering and by tests asserting on coverage of the enum.
func AllKinds() []Kind {
	return []Kind{
		KindGitActivity,
		KindLintTrend,
		KindTestCoverage,
		KindDepFreshness,
		KindSecurityAlerts,
	}
}

// Validate returns nil when k is one of the known Kind values, an error
// otherwise. Used at the Store boundary so unknown kinds never reach SQLite.
func (k Kind) Validate() error {
	switch k {
	case KindGitActivity, KindLintTrend, KindTestCoverage,
		KindDepFreshness, KindSecurityAlerts:
		return nil
	}
	return fmt.Errorf("aggregator: unknown signal kind %q", string(k))
}

// Signal is one observation about a project at a point in time. See
// ADR-015 §D2 for the field contract.
type Signal struct {
	ID          string    `json:"id"`
	Kind        Kind      `json:"kind"`
	Subject     string    `json:"subject"`
	Value       float64   `json:"value"`
	Metadata    string    `json:"metadata,omitempty"` // JSON-encoded; empty == "{}"
	CollectedAt time.Time `json:"collectedAt"`
}

// Validate checks the invariants every signal must satisfy before insertion:
// known Kind, non-empty Subject, non-empty ID, non-zero CollectedAt.
// Metadata, when present, must be valid JSON — but we leave that check to
// the store so this method stays allocation-free.
func (s Signal) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("aggregator: signal: empty ID")
	}
	if err := s.Kind.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(s.Subject) == "" {
		return fmt.Errorf("aggregator: signal: empty subject (kind=%s)", s.Kind)
	}
	if s.CollectedAt.IsZero() {
		return fmt.Errorf("aggregator: signal: zero CollectedAt (kind=%s, subject=%s)",
			s.Kind, s.Subject)
	}
	return nil
}
