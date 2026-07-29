package marketplaceparity

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
		canonical := canonicalSkillEntrypoints[name]
		canonicalPath := filepath.Join(root, filepath.FromSlash(canonical))
		if err := os.MkdirAll(filepath.Dir(canonicalPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(canonicalPath, []byte("# canonical\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, surface.Catalog, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + name + "\ndescription: Use when native discovery needs the canonical workflow.\n---\n" +
			"# Native entrypoint\n\n## Workflow\n\n1. Read `../../../" + canonical + "` completely.\n" +
			"2. Follow the canonical workflow and all of its gates.\n\n## Verification\n\nVerify every canonical exit condition.\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err != nil {
		t.Fatalf("complete native catalog: %v", err)
	}
	agmEntrypoint := filepath.Join(root, surface.Catalog, "agm", "SKILL.md")
	data, err := os.ReadFile(agmEntrypoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agmEntrypoint, []byte(strings.Replace(string(data), "name: agm", "name: wrong", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil {
		t.Fatal("expected mismatched native skill name to fail")
	}
	if err := os.WriteFile(agmEntrypoint, data, 0o644); err != nil {
		t.Fatal(err)
	}
	withoutCanonical := strings.Replace(string(data), "../../../"+canonicalSkillEntrypoints["agm"], "../../../wrong/SKILL.md", 1)
	if err := os.WriteFile(agmEntrypoint, []byte(withoutCanonical), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil {
		t.Fatal("expected missing canonical skill reference to fail")
	}
	if err := os.WriteFile(agmEntrypoint, data, 0o644); err != nil {
		t.Fatal(err)
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
