package checks

import (
	"context"
	"fmt"
	"sort"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// LintGapRefiner is the v1 refiner that closes the DEAR loop. It
// notices when a single linter fires more than `MinHits` times in
// one audit run and proposes promoting the rule's *underlying class*
// (e.g. "we keep finding `errcheck` issues" → "make sure errcheck
// stays enabled and considered an error").
//
// In v1 the proposal is informational — the patch field carries a
// suggested diff against .golangci.yml that the operator can apply
// or ignore. The refiner does not try to be clever about already-
// enabled rules; the operator reviews. This matches ADR-011's
// "refiner output is suggestions, not auto-applied" stance.
type LintGapRefiner struct {
	// MinHits is the recurrence threshold: a refiner only fires when
	// a single linter rule has at least this many findings in one
	// run. Default 5.
	MinHits int
}

// Name is the stable identifier surfaced in `workflow audit propose`.
func (LintGapRefiner) Name() string { return "lint-gap" }

// Propose scans findings for runs of the same linter and emits one
// Proposal per linter that crosses the threshold.
func (r LintGapRefiner) Propose(_ context.Context, findings []audit.Finding) ([]audit.Proposal, error) {
	threshold := r.MinHits
	if threshold <= 0 {
		threshold = 5
	}

	hits := map[string]int{}
	for _, f := range findings {
		if f.CheckID != "lint.go" {
			continue
		}
		linter, _ := f.Evidence["linter"].(string)
		if linter == "" {
			continue
		}
		hits[linter]++
	}

	if len(hits) == 0 {
		return nil, nil
	}

	// Sort linters for stable output; tests assert ordering.
	linters := make([]string, 0, len(hits))
	for l := range hits {
		linters = append(linters, l)
	}
	sort.Strings(linters)

	var props []audit.Proposal
	for _, l := range linters {
		if hits[l] < threshold {
			continue
		}
		props = append(props, audit.Proposal{
			Layer: audit.ProposalEnforce,
			Title: fmt.Sprintf("Audit: linter %q produced %d findings — review .golangci.yml policy", l, hits[l]),
			Rationale: fmt.Sprintf(
				"Linter %q produced %d findings in one audit run. "+
					"Either the rule is mis-tuned (consider raising severity in "+
					"`linters-settings`), the codebase has accumulated debt that needs a "+
					"focused cleanup, or this rule should be promoted to error severity "+
					"so CI blocks regressions.",
				l, hits[l]),
			Patch: fmt.Sprintf(`# Suggestion for .golangci.yml:
# Confirm %q is enabled with severity=error so future occurrences fail CI.
linters-settings:
  severity:
    rules:
      - linters: [%s]
        severity: error
`, l, l),
		})
	}
	return props, nil
}
