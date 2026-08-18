//go:build integration

package portable_test

import (
	"slices"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// TestActiveHarnessParityContract is deliberately credential-free. It remains
// runnable when no harness binary or remote service is available and therefore
// prevents one optional prerequisite from hiding every adapter assertion.
func TestActiveHarnessParityContract(t *testing.T) {
	t.Parallel()

	want := []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}
	if got := agent.ActiveHarnesses(); !slices.Equal(got, want) {
		t.Fatalf("active harnesses = %v, want %v", got, want)
	}
	if !slices.Contains(agent.ActiveHarnesses(), "codex-cli") {
		t.Fatal("codex-cli is missing from the active adapter parity matrix")
	}
	for _, finding := range agent.ValidateActiveHarnessConformance() {
		t.Error(finding.Error())
	}
}

// TestHarnessPrerequisitesAreScoped records host coverage without turning one
// missing binary, credential, or service into a suite-wide skip. The portable
// contract above always runs; each host-dependent branch owns its own skip.
func TestHarnessPrerequisitesAreScoped(t *testing.T) {
	for _, harness := range agent.ActiveHarnesses() {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()
			if err := agent.ValidateHarnessAvailability(harness); err != nil {
				t.Skipf("%s host prerequisite unavailable: %v", harness, err)
			}
		})
	}
}
