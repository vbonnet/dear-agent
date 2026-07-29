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

func TestValidateRequiredPluginsRejectsMissingResearchPipeline(t *testing.T) {
	catalog := Catalog{Plugins: []PluginEntry{
		{Name: "agm"},
		{Name: "wayfinder"},
		{Name: "youtube"},
	}}
	if err := validateRequiredPlugins(catalog); err == nil || !strings.Contains(err.Error(), "research-pipeline") {
		t.Fatalf("validateRequiredPlugins() error = %v, want missing research-pipeline", err)
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
	surface := HarnessSurface{Mode: "native-codex-skill", Catalog: ".agents/skills"}
	catalog := Catalog{Plugins: []PluginEntry{
		{Name: "agm", Source: "./plugins/agm", Capabilities: []string{"commands", "skills"}},
		{Name: "wayfinder", Source: "./plugins/wayfinder", Capabilities: []string{"skills"}},
		{Name: "youtube", Capabilities: []string{"commands"}},
	}}
	skills := map[string]string{"agm": "scan-health", "wayfinder": "wayfinder"}
	canonicalBySkill := make(map[string]string)
	for pluginName, skillName := range skills {
		source := filepath.Join(root, "plugins", pluginName)
		canonicalPath := filepath.Join(source, "skills", skillName, "SKILL.md")
		canonical, err := filepath.Rel(root, canonicalPath)
		if err != nil {
			t.Fatal(err)
		}
		canonicalBySkill[skillName] = filepath.ToSlash(canonical)
		if err := os.MkdirAll(filepath.Join(source, ".claude-plugin"), 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := `{"name":"` + pluginName + `","skills":["./skills/"]}`
		if err := os.WriteFile(filepath.Join(source, ".claude-plugin", "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(canonicalPath), 0o755); err != nil {
			t.Fatal(err)
		}
		canonicalContent := "---\nname: " + skillName + "\ndescription: Use when testing the canonical exported workflow.\n---\n" +
			"# Canonical\n\n## Workflow\n\n1. Inspect the requested resource.\n2. Report the typed result.\n\n## Verification\n\nVerify the result is complete.\n"
		if err := os.WriteFile(canonicalPath, []byte(canonicalContent), 0o644); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, surface.Catalog, skillName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + skillName + "\ndescription: Use when native discovery needs the canonical workflow.\n---\n" +
			"# Native entrypoint\n\n## Workflow\n\n1. Read `../../../" + filepath.ToSlash(canonical) + "` completely.\n" +
			"2. Follow the canonical workflow and all of its gates.\n\n## Verification\n\nVerify every canonical exit condition.\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err != nil {
		t.Fatalf("complete native catalog: %v", err)
	}

	// The plugin name is not the export name. A synthesized agm/SKILL.md must
	// not satisfy the manifest's scan-health export.
	scanHealthEntrypoint := filepath.Join(root, surface.Catalog, "scan-health", "SKILL.md")
	scanHealthData, err := os.ReadFile(scanHealthEntrypoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(scanHealthEntrypoint); err != nil {
		t.Fatal(err)
	}
	pluginWrapperDir := filepath.Join(root, surface.Catalog, "agm")
	if err := os.MkdirAll(pluginWrapperDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginWrapperDir, "SKILL.md"), scanHealthData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil || !strings.Contains(err.Error(), "scan-health") {
		t.Fatalf("plugin-name wrapper unexpectedly satisfied exported skill: %v", err)
	}
	if err := os.WriteFile(scanHealthEntrypoint, scanHealthData, 0o644); err != nil {
		t.Fatal(err)
	}

	agmCanonical := filepath.Join(root, filepath.FromSlash(canonicalBySkill["scan-health"]))
	canonicalData, err := os.ReadFile(agmCanonical)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(agmCanonical); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil {
		t.Fatal("expected missing canonical skill entrypoint to fail")
	}
	if err := os.WriteFile(agmCanonical, canonicalData, 0o644); err != nil {
		t.Fatal(err)
	}
	agmEntrypoint := scanHealthEntrypoint
	data, err := os.ReadFile(agmEntrypoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agmEntrypoint, []byte(strings.Replace(string(data), "name: scan-health", "name: wrong", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil {
		t.Fatal("expected mismatched native skill name to fail")
	}
	if err := os.WriteFile(agmEntrypoint, data, 0o644); err != nil {
		t.Fatal(err)
	}
	withoutCanonical := strings.Replace(string(data), "../../../"+canonicalBySkill["scan-health"], "../../../wrong/SKILL.md", 1)
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
	source := filepath.Join(root, "plugins", "agm")
	canonical := filepath.Join(source, "skills", "scan-health", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canonical, []byte("---\nname: scan-health\ndescription: Use for test health scans.\n---\n# Scan\n\n## Workflow\n\n1. Inspect sessions.\n2. Report health.\n\n## Verification\n\nVerify every session.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".claude-plugin", "plugin.json"), []byte(`{"name":"agm","skills":["./skills/"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "native-target.md")
	if err := os.WriteFile(target, []byte("---\nname: scan-health\ndescription: Use for native health scans.\n---\n# Scan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, surface.Catalog, "scan-health")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{Plugins: []PluginEntry{{Name: "agm", Source: "./plugins/agm", Capabilities: []string{"skills"}}}}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil {
		t.Fatal("expected symlinked native entrypoint to fail")
	}
}
