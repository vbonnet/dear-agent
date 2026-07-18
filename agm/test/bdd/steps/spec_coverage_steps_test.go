package steps

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/internal/speccoverage"
)

func TestSpecCoverageFindingSelectionUsesKind(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), specCoverageStateKey{}, &specCoverageState{
		findings: []speccoverage.Finding{{
			Kind:    speccoverage.FindingKindMissingEARS,
			Surface: "example",
			Path:    "BDD feature",
			Message: "invalid EARS syntax should not classify this finding",
		}},
	})

	if err := specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindInvalidEARS); err != nil {
		t.Fatalf("message text reclassified finding: %v", err)
	}
	if err := specCoverageShouldHaveNoFindings(ctx, speccoverage.FindingKindMissingEARS); err == nil {
		t.Fatal("finding kind was not selected")
	}
}
