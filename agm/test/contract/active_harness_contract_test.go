//go:build contract

package contract

import (
	"slices"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

func TestActiveHarnessRegistryContract(t *testing.T) {
	t.Parallel()

	want := []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}
	if got := agent.ActiveHarnesses(); !slices.Equal(got, want) {
		t.Fatalf("ActiveHarnesses() = %v, want %v", got, want)
	}
	if slices.Contains(agent.ActiveHarnesses(), "gemini-cli") {
		t.Fatal("deprecated gemini-cli must not appear in the active parity contract")
	}

	for _, harness := range want {
		harness := harness
		t.Run(harness, func(t *testing.T) {
			t.Parallel()
			adapter, err := agent.GetHarness(harness)
			if err != nil {
				t.Fatalf("GetHarness(%q): %v", harness, err)
			}
			if got := adapter.Name(); got != harness {
				t.Fatalf("GetHarness(%q).Name() = %q", harness, got)
			}
		})
	}
}
