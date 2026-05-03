package checks

import (
	"context"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// TestLintGapRefinerThreshold pins the recurrence behaviour: below
// MinHits, no proposal; at or above, exactly one proposal per
// linter, sorted by linter name.
func TestLintGapRefinerThreshold(t *testing.T) {
	r := LintGapRefiner{MinHits: 3}
	findings := []audit.Finding{
		mkLintFinding("errcheck"),
		mkLintFinding("errcheck"),
		mkLintFinding("errcheck"),
		mkLintFinding("staticcheck"), // single hit — below threshold
		mkLintFinding("gosec"),
		mkLintFinding("gosec"),
		mkLintFinding("gosec"),
		mkLintFinding("gosec"),
	}
	props, err := r.Propose(context.Background(), findings)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if len(props) != 2 {
		t.Fatalf("got %d proposals, want 2", len(props))
	}
	if !strings.Contains(props[0].Title, "errcheck") {
		t.Errorf("first proposal should be errcheck (sorted): %s", props[0].Title)
	}
	if !strings.Contains(props[1].Title, "gosec") {
		t.Errorf("second proposal should be gosec: %s", props[1].Title)
	}
	for _, p := range props {
		if p.Layer != audit.ProposalEnforce {
			t.Errorf("proposal layer = %s, want enforce", p.Layer)
		}
		if p.Patch == "" {
			t.Errorf("proposal patch should be non-empty for %s", p.Title)
		}
	}
}

func TestLintGapRefinerIgnoresOtherChecks(t *testing.T) {
	r := LintGapRefiner{MinHits: 1}
	findings := []audit.Finding{
		{
			CheckID:     "vuln.govulncheck",
			Fingerprint: "fp",
			Severity:    audit.SeverityP1,
			Title:       "CVE",
			Evidence:    map[string]any{"linter": "irrelevant"},
		},
	}
	props, err := r.Propose(context.Background(), findings)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("non-lint findings should not produce proposals; got %d", len(props))
	}
}

func mkLintFinding(linter string) audit.Finding {
	return audit.Finding{
		CheckID:     "lint.go",
		Fingerprint: linter + "-fp",
		Severity:    audit.SeverityP2,
		Title:       linter + " issue",
		Evidence:    map[string]any{"linter": linter},
	}
}
