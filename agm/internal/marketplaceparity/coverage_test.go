package marketplaceparity

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

func TestValidateCatalog(t *testing.T) {
	if err := ValidateCatalog(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestActiveHarnessesHaveMarketplaceSurfaces(t *testing.T) {
	root := repoRoot(t)
	catalog, err := LoadCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(catalog, harness)
		if !ok {
			t.Fatalf("SurfaceForHarness(%q) not found", harness)
		}
		if surface.Catalog == "" {
			t.Errorf("%s marketplace surface has empty catalog", harness)
		}
		if want := ExpectedMarketplaceMode(harness); surface.Mode != want {
			t.Errorf("%s mode = %q, want %s", harness, surface.Mode, want)
		}
	}
}

func TestClaudeMarketplaceMirrorsNeutralCatalog(t *testing.T) {
	if err := ValidateClaudeMarketplaceMirror(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNativeSkillCoverageRequiresEverySkillPlugin(t *testing.T) {
	root := t.TempDir()
	surface := HarnessSurface{Mode: "native-codex-skill", Catalog: ".codex/skills"}
	catalog := Catalog{Plugins: []PluginEntry{
		{Name: "agm", Capabilities: []string{"commands", "skills"}},
		{Name: "wayfinder", Capabilities: []string{"skills"}},
		{Name: "youtube", Capabilities: []string{"commands"}},
	}}
	for _, name := range []string{"agm", "wayfinder"} {
		dir := filepath.Join(root, surface.Catalog, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err != nil {
		t.Fatalf("complete native catalog: %v", err)
	}
	if err := os.Remove(filepath.Join(root, surface.Catalog, "wayfinder", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil {
		t.Fatal("expected missing wayfinder native entrypoint to fail")
	}
}

func TestValidateNativeSkillCoverageRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	surface := HarnessSurface{Mode: "native-opencode-skill", Catalog: ".opencode/skills"}
	target := filepath.Join(root, "canonical.md")
	if err := os.WriteFile(target, []byte("---\nname: agm\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, surface.Catalog, "agm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{Plugins: []PluginEntry{{Name: "agm", Capabilities: []string{"skills"}}}}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil {
		t.Fatal("expected symlinked native entrypoint to fail")
	}
}
