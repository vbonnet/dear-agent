package wayfinderparity

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestValidateActiveHarnessSurfaces(t *testing.T) {
	if err := ValidateActiveHarnessSurfaces(); err != nil {
		t.Fatal(err)
	}
	got := ActiveHarnessSurfaces()
	if len(got) != len(agent.ActiveHarnesses()) {
		t.Fatalf("Wayfinder surfaces = %d, want %d: %+v", len(got), len(agent.ActiveHarnesses()), got)
	}
}

func TestActiveHarnessesHaveWayfinderSurfaces(t *testing.T) {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			t.Fatalf("SurfaceForHarness(%q) not found", harness)
		}
		if surface.Harness != harness {
			t.Errorf("surface.Harness = %q, want %q", surface.Harness, harness)
		}
		if surface.ExecutionSurface == "" || surface.StatusSurface == "" {
			t.Errorf("surface for %q is incomplete: %+v", harness, surface)
		}
	}
}

func TestValidateAssets(t *testing.T) {
	if err := ValidateAssets(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePiSkillDiscovery(t *testing.T) {
	if err := ValidatePiSkillDiscovery(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateMCPOperations(t *testing.T) {
	if err := ValidateMCPOperations(); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePhaseEngramCoverage(t *testing.T) {
	if err := ValidatePhaseEngramCoverage(); err != nil {
		t.Fatal(err)
	}
}
