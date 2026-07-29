package wayfinderparity

import (
	"os"
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
		if got, want := surface.DiscoverySurface, expectedDiscoverySurfaces[harness]; got != want {
			t.Errorf("surface discovery for %q = %q, want %q", harness, got, want)
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

func TestValidatePiSkillDiscoveryRequiresResolvedSkillTrees(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"skills":["../agm/plugins","../wayfinder/skills","../research-pipeline/skills"]}`
	if err := os.WriteFile(filepath.Join(root, ".pi", "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, entrypoint := range []string{
		"agm/plugins/agm/SKILL.md",
		"wayfinder/skills/wayfinder/SKILL.md",
		"research-pipeline/skills/research-pipeline/SKILL.md",
	} {
		path := filepath.Join(root, filepath.FromSlash(entrypoint))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# Skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidatePiSkillDiscovery(root); err != nil {
		t.Fatalf("complete skill trees: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "agm", "plugins", "agm", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePiSkillDiscovery(root); err == nil {
		t.Fatal("expected missing AGM Pi skill entrypoint to fail")
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
