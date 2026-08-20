package wayfinderparity

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestValidatePiSkillDiscoveryRequiresConfiguredSharedSkillTree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"skills":["../.agents/skills","../agm/plugins"]}`
	if err := os.WriteFile(filepath.Join(root, ".pi", "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	entrypoint := filepath.Join(root, ".agents", "skills", "wayfinder", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypoint, []byte("# Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agmEntrypoint := filepath.Join(root, "agm", "plugins", "agm", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(agmEntrypoint), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agmEntrypoint, []byte("# AGM\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePiSkillDiscovery(root); err != nil {
		t.Fatalf("complete skill trees: %v", err)
	}
	if err := os.Remove(entrypoint); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePiSkillDiscovery(root); err == nil {
		t.Fatal("expected empty configured Pi skill root to fail")
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

func TestValidatePiSkillDiscoveryRejectsSymlinkedRootOutsideRepository(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, ".pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"skills":["../.agents/skills","../agm/plugins"]}`
	if err := os.WriteFile(filepath.Join(root, ".pi", "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	// A real skill tree that lives entirely outside the repository.
	externalEntrypoint := filepath.Join(outside, "skills", "wayfinder", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(externalEntrypoint), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalEntrypoint, []byte("# External\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// .agents/skills is lexically inside the repo but symlinks outside it.
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "skills"), filepath.Join(root, ".agents", "skills")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	agmEntrypoint := filepath.Join(root, "agm", "plugins", "agm", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(agmEntrypoint), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agmEntrypoint, []byte("# AGM\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ValidatePiSkillDiscovery(root)
	if err == nil {
		t.Fatal("symlinked Pi skill root escaping the repository was accepted")
	}
	if !strings.Contains(err.Error(), "escapes the repository") {
		t.Fatalf("err = %v, want an escapes-the-repository rejection", err)
	}
}

func TestValidatePiSkillDiscoveryRejectsSettingsSymlinkOutsideRepository(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	externalSettings := filepath.Join(outside, "settings.json")
	if err := os.WriteFile(externalSettings, []byte(`{"skills":["../.agents/skills","../agm/plugins"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalSettings, filepath.Join(root, ".pi", "settings.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	for _, entrypoint := range []string{
		filepath.Join(root, ".agents", "skills", "wayfinder", "SKILL.md"),
		filepath.Join(root, "agm", "plugins", "agm", "SKILL.md"),
	} {
		if err := os.MkdirAll(filepath.Dir(entrypoint), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(entrypoint, []byte("# Skill\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	err := ValidatePiSkillDiscovery(root)
	if err == nil {
		t.Fatal("Pi settings symlink escaping the repository was accepted")
	}
	if !strings.Contains(err.Error(), "settings escape the repository") {
		t.Fatalf("err = %v, want settings containment rejection", err)
	}
}

func TestValidateAssetsRejectsSymlinkedAsset(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	assets := []string{
		"wayfinder/SPEC.md",
		"wayfinder/SKILL.md",
		"wayfinder/.claude-plugin/plugin.json",
		"wayfinder/ARCHITECTURE.md",
		"wayfinder/cmd/wayfinder-session/SPEC.md",
	}
	// Everything real except the last asset, which is a repository-local
	// symlink to a file a clean clone would not carry.
	for _, rel := range assets[:len(assets)-1] {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	external := filepath.Join(outside, "SKILL.md")
	if err := os.WriteFile(external, []byte("# External\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, assets[len(assets)-1])
	if err := os.MkdirAll(filepath.Dir(linked), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, linked); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := ValidateAssets(root)
	if err == nil {
		t.Fatal("ValidateAssets accepted a symlinked external Wayfinder asset")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v, want a regular-file rejection", err)
	}
}
