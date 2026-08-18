package steps

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/earsbdd"
)

func TestDuplicateHarnessRequirementIDsRecognizesSupportedMarkdownPrefixes(t *testing.T) {
	requirements, err := earsbdd.Extract("SPEC.md", strings.NewReader(`
**AGP-01** When a bare requirement is checked, the system shall count its identifier.
- **AGP-01** When a list requirement is checked, the system shall count its identifier.
  **AGP-02** When an indented requirement is checked, the system shall count its identifier.
1. **AGP-02** When an ordered requirement is checked, the system shall count its identifier.
**OTHER-01** When an unrelated requirement is checked, the system shall ignore its identifier.
`))
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	got := duplicateHarnessRequirementIDs(requirements)
	if len(got) != 2 || got[0] != "AGP-01" || got[1] != "AGP-02" {
		t.Fatalf("duplicateHarnessRequirementIDs() = %v, want [AGP-01 AGP-02]", got)
	}
}

func TestLifecycleSurfacesDelegateToSharedOperations(t *testing.T) {
	if err := requireLifecycleDelegation(packageSpecBDDRepoRoot()); err != nil {
		t.Fatal(err)
	}
}
