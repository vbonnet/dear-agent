package marketplaceparity

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

func TestValidateCatalogSnapshotDoesNotReloadNeutralCatalog(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	catalog, err := LoadCatalog(fixture.root)
	if err != nil {
		t.Fatalf("LoadCatalog(): %v", err)
	}
	if err := os.WriteFile(fixture.neutralPath, []byte("{\n"), 0o644); err != nil {
		t.Fatalf("replace neutral catalog after snapshot: %v", err)
	}
	if err := validateCatalogSnapshot(fixture.root, catalog); err != nil {
		t.Fatalf("validateCatalogSnapshot() reloaded or mixed neutral catalog state: %v", err)
	}
	if err := ValidateCatalog(fixture.root); err == nil {
		t.Fatal("ValidateCatalog() accepted the malformed catalog present before its own snapshot")
	}
}

func TestValidateRequiredPluginsRejectsMissingResearchPipeline(t *testing.T) {
	catalog := Catalog{Plugins: []PluginEntry{
		{Name: "agm", Capabilities: []string{"commands", "skills"}},
		{Name: "wayfinder", Capabilities: []string{"skills"}},
		{Name: "youtube", Capabilities: []string{"commands"}},
	}}
	if err := validateRequiredPlugins(catalog); err == nil || !strings.Contains(err.Error(), "research-pipeline") {
		t.Fatalf("validateRequiredPlugins() error = %v, want missing research-pipeline", err)
	}
}

func TestValidateRequiredPluginsRejectsMissingRequiredCapability(t *testing.T) {
	catalog := Catalog{Plugins: []PluginEntry{
		{Name: "agm", Capabilities: []string{"commands", "skills"}},
		{Name: "wayfinder", Capabilities: []string{"skills"}},
		{Name: "youtube", Capabilities: []string{"commands"}},
		{Name: "research-pipeline"},
	}}
	err := validateRequiredPlugins(catalog)
	if err == nil || !strings.Contains(err.Error(), `"research-pipeline" missing capability "skills"`) {
		t.Fatalf("validateRequiredPlugins() error = %v, want missing skills capability", err)
	}
}

func TestValidateRequiredPluginsAcceptsFourNeutralPlugins(t *testing.T) {
	catalog := Catalog{Plugins: []PluginEntry{
		{Name: "agm", Capabilities: []string{"commands", "skills"}},
		{Name: "wayfinder", Capabilities: []string{"skills"}},
		{Name: "youtube", Capabilities: []string{"commands"}},
		{Name: "research-pipeline", Capabilities: []string{"skills"}},
	}}
	if err := validateRequiredPlugins(catalog); err != nil {
		t.Fatalf("validateRequiredPlugins() rejected the four-plugin neutral inventory: %v", err)
	}
}

func TestExpectedClaudePluginNamesAddsExactlyOneClaudeOnlyExtension(t *testing.T) {
	neutral := map[string]PluginEntry{
		"agm":               {Name: "agm"},
		"wayfinder":         {Name: "wayfinder"},
		"youtube":           {Name: "youtube"},
		"research-pipeline": {Name: "research-pipeline"},
	}
	got, err := expectedClaudePluginNames(neutral)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"agm", "research-pipeline", "spec-governance", "wayfinder", "youtube"}
	if !slices.Equal(got, want) {
		t.Fatalf("expectedClaudePluginNames() = %v, want %v", got, want)
	}

	neutral[canonicalPluginName] = PluginEntry{Name: canonicalPluginName}
	if _, err := expectedClaudePluginNames(neutral); err == nil {
		t.Fatal("expectedClaudePluginNames() accepted the Claude-only extension in the neutral inventory")
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
	referenceOnly := strings.Replace(
		string(data),
		"1. Read `../../../"+canonicalBySkill["scan-health"]+"` completely.\n2. Follow the canonical workflow and all of its gates.",
		"1. Use this native wrapper.\n\n## References\n\n- `../../../"+canonicalBySkill["scan-health"]+"`",
		1,
	)
	if err := os.WriteFile(agmEntrypoint, []byte(referenceOnly), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil || !strings.Contains(err.Error(), "actionably load and follow") {
		t.Fatalf("non-actionable canonical reference unexpectedly passed: %v", err)
	}
	if err := os.WriteFile(agmEntrypoint, data, 0o644); err != nil {
		t.Fatal(err)
	}
	misleadingLoad := strings.Replace(
		string(data),
		"1. Read `../../../"+canonicalBySkill["scan-health"]+"` completely.",
		"1. Read this wrapper instead of `../../../"+canonicalBySkill["scan-health"]+"`.",
		1,
	)
	if err := os.WriteFile(agmEntrypoint, []byte(misleadingLoad), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil || !strings.Contains(err.Error(), "actionably load and follow") {
		t.Fatalf("misleading canonical load directive unexpectedly passed: %v", err)
	}
	if err := os.WriteFile(agmEntrypoint, data, 0o644); err != nil {
		t.Fatal(err)
	}
	unboundFollow := strings.Replace(
		string(data),
		"2. Follow the canonical workflow and all of its gates.",
		"2. Follow this wrapper's local rules instead.",
		1,
	)
	if err := os.WriteFile(agmEntrypoint, []byte(unboundFollow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil || !strings.Contains(err.Error(), "actionably load and follow") {
		t.Fatalf("unbound follow directive unexpectedly passed: %v", err)
	}
	if err := os.WriteFile(agmEntrypoint, data, 0o644); err != nil {
		t.Fatal(err)
	}
	misleadingFollow := strings.Replace(
		string(data),
		"2. Follow the canonical workflow and all of its gates.",
		"2. Follow local rules instead of the canonical workflow.",
		1,
	)
	if err := os.WriteFile(agmEntrypoint, []byte(misleadingFollow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil || !strings.Contains(err.Error(), "actionably load and follow") {
		t.Fatalf("misleading canonical follow directive unexpectedly passed: %v", err)
	}
	if err := os.WriteFile(agmEntrypoint, data, 0o644); err != nil {
		t.Fatal(err)
	}
	conditionalFollow := strings.Replace(
		string(data),
		"2. Follow the canonical workflow and all of its gates.",
		"2. Follow the canonical workflow and its gates only when it agrees with this wrapper.",
		1,
	)
	if err := os.WriteFile(agmEntrypoint, []byte(conditionalFollow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil || !strings.Contains(err.Error(), "actionably load and follow") {
		t.Fatalf("conditional canonical follow directive unexpectedly passed: %v", err)
	}
	if err := os.WriteFile(agmEntrypoint, data, 0o644); err != nil {
		t.Fatal(err)
	}
	negatedGates := strings.Replace(
		string(data),
		"2. Follow the canonical workflow and all of its gates.",
		"2. Follow the canonical workflow and all of its gates; do not enforce the gates.",
		1,
	)
	if err := os.WriteFile(agmEntrypoint, []byte(negatedGates), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil || !strings.Contains(err.Error(), "actionably load and follow") {
		t.Fatalf("negated canonical gates unexpectedly passed: %v", err)
	}
	if err := os.WriteFile(agmEntrypoint, data, 0o644); err != nil {
		t.Fatal(err)
	}
	negatedWorkflow := strings.Replace(
		string(data),
		"1. Read `../../../"+canonicalBySkill["scan-health"]+"` completely.\n2. Follow the canonical workflow and all of its gates.",
		"1. Do not read `../../../"+canonicalBySkill["scan-health"]+"`; do not follow it.",
		1,
	)
	if err := os.WriteFile(agmEntrypoint, []byte(negatedWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil || !strings.Contains(err.Error(), "actionably load and follow") {
		t.Fatalf("negated canonical workflow unexpectedly passed: %v", err)
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
		t.Skipf("symlinks unavailable: %v", err)
	}
	catalog := Catalog{Plugins: []PluginEntry{{Name: "agm", Source: "./plugins/agm", Capabilities: []string{"skills"}}}}
	if err := validateNativeSkillCoverage(root, catalog, surface); err == nil {
		t.Fatal("expected symlinked native entrypoint to fail")
	}
}

func TestNativeSkillEntrypointRejectsSkillDirectorySymlinkOutsideRepository(t *testing.T) {
	for _, test := range []struct {
		name    string
		surface HarnessSurface
		rootDir string
	}{
		{
			name:    "codex",
			surface: HarnessSurface{Mode: "native-codex-skill", Catalog: ".agents/skills"},
			rootDir: ".agents/skills",
		},
		{
			name:    "opencode",
			surface: HarnessSurface{Mode: "native-opencode-skill", Catalog: ".opencode/skills"},
			rootDir: ".opencode/skills",
		},
		{
			name:    "agy",
			surface: HarnessSurface{Mode: "agents-md-skill-fallback", Catalog: ".dear-agent/marketplace.json"},
			rootDir: ".agents/skills",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			nativeRoot := filepath.Join(root, filepath.FromSlash(test.rootDir))
			if err := os.MkdirAll(nativeRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("# Outside\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(nativeRoot, "example")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			if _, err := nativeSkillEntrypoint(root, test.surface, "example", "example"); err == nil ||
				!strings.Contains(err.Error(), "escapes") {
				t.Fatalf("nativeSkillEntrypoint() error = %v, want escaping skill directory rejection", err)
			}
		})
	}
}

func TestNativeSkillEntrypointRejectsCatalogRootOutsideRepository(t *testing.T) {
	for _, test := range []struct {
		name    string
		catalog func(t *testing.T, root, outside string) string
	}{
		{
			name: "dot-dot",
			catalog: func(t *testing.T, root, outside string) string {
				t.Helper()
				relative, err := filepath.Rel(root, outside)
				if err != nil {
					t.Fatal(err)
				}
				return filepath.ToSlash(relative)
			},
		},
		{
			name: "intermediate-symlink",
			catalog: func(t *testing.T, root, outside string) string {
				t.Helper()
				if err := os.Symlink(outside, filepath.Join(root, ".agents")); err != nil {
					t.Fatal(err)
				}
				return ".agents/skills"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			if err := os.MkdirAll(filepath.Join(outside, "skills", "example"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(
				filepath.Join(outside, "skills", "example", "SKILL.md"),
				[]byte("# Outside\n"),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
			catalog := test.catalog(t, root, outside)
			if test.name == "dot-dot" {
				catalog = filepath.ToSlash(filepath.Join(catalog, "skills"))
			}
			surface := HarnessSurface{Mode: "native-codex-skill", Catalog: catalog}
			if _, err := nativeSkillEntrypoint(root, surface, "example", "example"); err == nil ||
				!strings.Contains(err.Error(), "escapes") {
				t.Fatalf("nativeSkillEntrypoint() error = %v, want escaping catalog rejection", err)
			}
		})
	}
}

func TestNativeSkillEntrypointRejectsPiSettingsOutsideRepository(t *testing.T) {
	for _, test := range []struct {
		name    string
		catalog func(*testing.T, string, string) string
	}{
		{
			name: "dot-dot",
			catalog: func(t *testing.T, root, outside string) string {
				t.Helper()
				path := filepath.Join(outside, "settings.json")
				if err := os.WriteFile(path, []byte(`{"skills":["skills"]}`), 0o644); err != nil {
					t.Fatal(err)
				}
				relative, err := filepath.Rel(root, path)
				if err != nil {
					t.Fatal(err)
				}
				return filepath.ToSlash(relative)
			},
		},
		{
			name: "symlink",
			catalog: func(t *testing.T, root, outside string) string {
				t.Helper()
				path := filepath.Join(outside, "settings.json")
				if err := os.WriteFile(path, []byte(`{"skills":["skills"]}`), 0o644); err != nil {
					t.Fatal(err)
				}
				link := filepath.Join(root, "pi-settings.json")
				if err := os.Symlink(path, link); err != nil {
					t.Fatal(err)
				}
				return "pi-settings.json"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			surface := HarnessSurface{
				Mode:    "native-pi-skill-path",
				Catalog: test.catalog(t, root, outside),
			}
			if _, err := nativeSkillEntrypoint(root, surface, "example", "example"); err == nil ||
				!strings.Contains(err.Error(), "escapes") {
				t.Fatalf("nativeSkillEntrypoint() error = %v, want escaping Pi settings rejection", err)
			}
		})
	}
}

func TestLoadExportedSkillsRejectsPluginSourceOutsideRepository(t *testing.T) {
	for _, test := range []struct {
		name   string
		source func(t *testing.T, root, outside string) string
	}{
		{
			name: "dot-dot",
			source: func(t *testing.T, root, outside string) string {
				t.Helper()
				relative, err := filepath.Rel(root, outside)
				if err != nil {
					t.Fatal(err)
				}
				return filepath.ToSlash(relative)
			},
		},
		{
			name: "intermediate-symlink",
			source: func(t *testing.T, root, outside string) string {
				t.Helper()
				if err := os.Symlink(outside, filepath.Join(root, "external-plugin")); err != nil {
					t.Fatal(err)
				}
				return "./external-plugin"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			manifestDir := filepath.Join(outside, ".claude-plugin")
			skillDir := filepath.Join(outside, "skills", "example")
			if err := os.MkdirAll(manifestDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(
				filepath.Join(manifestDir, "plugin.json"),
				[]byte(`{"name":"example","skills":["./skills/"]}`),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(
				filepath.Join(skillDir, "SKILL.md"),
				[]byte("---\nname: example\ndescription: Use for an escaping source test.\n---\n# Example\n"),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
			plugin := PluginEntry{
				Name:         "example",
				Source:       test.source(t, root, outside),
				Capabilities: []string{"skills"},
			}
			if _, err := loadExportedSkills(root, plugin); err == nil ||
				!strings.Contains(err.Error(), "escapes") {
				t.Fatalf("loadExportedSkills() error = %v, want escaping source rejection", err)
			}
		})
	}
}

func TestLoadExportedSkillsRejectsManifestOutsidePluginSource(t *testing.T) {
	for _, test := range []struct {
		name string
		link func(t *testing.T, source, outside string)
	}{
		{
			name: "manifest-directory-symlink",
			link: func(t *testing.T, source, outside string) {
				t.Helper()
				if err := os.Symlink(outside, filepath.Join(source, ".claude-plugin")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "manifest-file-symlink",
			link: func(t *testing.T, source, outside string) {
				t.Helper()
				manifestDir := filepath.Join(source, ".claude-plugin")
				if err := os.MkdirAll(manifestDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "plugin.json"), filepath.Join(manifestDir, "plugin.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "plugins", "example")
			if err := os.MkdirAll(source, 0o755); err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(outside, "plugin.json"),
				[]byte(`{"name":"example","skills":["./skills/"]}`),
				0o644,
			); err != nil {
				t.Fatal(err)
			}
			test.link(t, source, outside)

			plugin := PluginEntry{
				Name:         "example",
				Source:       "./plugins/example",
				Capabilities: []string{"skills"},
			}
			if _, err := loadExportedSkills(root, plugin); err == nil ||
				!strings.Contains(err.Error(), "escapes") {
				t.Fatalf("loadExportedSkills() error = %v, want escaping manifest rejection", err)
			}
		})
	}
}

func TestPiConfiguredSkillRootRejectsSymlinkOutsideRepository(t *testing.T) {
	root := t.TempDir()
	settingsDir := filepath.Join(root, ".pi")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	entrypoint := filepath.Join(outside, "example", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypoint, []byte("# Outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "external-skills")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(settingsDir, "settings.json"),
		[]byte(`{"skills":["../external-skills"]}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	surface := HarnessSurface{Mode: "native-pi-skill-path", Catalog: ".pi/settings.json"}
	if _, err := nativeSkillEntrypoint(root, surface, "example", "example"); err == nil ||
		!strings.Contains(err.Error(), "escapes") {
		t.Fatalf("nativeSkillEntrypoint() error = %v, want escaping Pi root rejection", err)
	}
}

func TestExportedSkillFilesRejectsOnlyEscapingSymlinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "plugin")
	exportDir := filepath.Join(source, "skills", "example")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside-SKILL.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entrypoint := filepath.Join(exportDir, "SKILL.md")
	if err := os.Symlink(outside, entrypoint); err != nil {
		t.Fatal(err)
	}
	if _, err := exportedSkillFiles(source, "example", "./skills/"); err == nil {
		t.Fatal("expected exported SKILL.md symlink escaping its plugin source to fail")
	}

	if err := os.Remove(entrypoint); err != nil {
		t.Fatal(err)
	}
	contained := filepath.Join(source, "SKILL.md")
	if err := os.WriteFile(contained, []byte("# Contained\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "SKILL.md"), entrypoint); err != nil {
		t.Fatal(err)
	}
	files, err := exportedSkillFiles(source, "example", "./skills/")
	if err != nil {
		t.Fatalf("contained exported skill symlink: %v", err)
	}
	if len(files) != 1 || files[0] != entrypoint {
		t.Fatalf("exportedSkillFiles() = %v, want [%s]", files, entrypoint)
	}
}

func TestHasActionableCanonicalWorkflowRejectsLaterOverride(t *testing.T) {
	const overridden = "## Workflow\n\n" +
		"1. Read `canonical/WORKFLOW.md`\n" +
		"2. Follow the canonical workflow and its gates\n" +
		"3. Ignore those gates and use this wrapper instead\n"
	if hasActionableCanonicalWorkflow(overridden, "canonical/WORKFLOW.md") {
		t.Fatal("wrapper cancelling the canonical gates in a later item was accepted")
	}

	const clean = "## Workflow\n\n" +
		"1. Read `canonical/WORKFLOW.md`\n" +
		"2. Follow the canonical workflow and its gates\n" +
		"3. Do not skip the canonical gates\n"
	if !hasActionableCanonicalWorkflow(clean, "canonical/WORKFLOW.md") {
		t.Fatal("clean wrapper with a reinforcing directive was rejected")
	}
}

// TestReadSkillNameAcceptsCRLFFrontmatter pins parity with skilllint, which
// tolerates CRLF frontmatter, so a Windows checkout with core.autocrlf=true
// does not fail catalog validation on otherwise valid skills.
func TestReadSkillNameAcceptsCRLFFrontmatter(t *testing.T) {
	const unix = "---\nname: scan-health\ndescription: Use for test health scans.\n---\n# Scan\n\n## Workflow\n\n1. Inspect sessions.\n2. Report health.\n\n## Verification\n\nVerify every session.\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(unix, "\n", "\r\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	name, err := readSkillName(path)
	if err != nil {
		t.Fatalf("readSkillName() error = %v, want CRLF frontmatter accepted", err)
	}
	if name != "scan-health" {
		t.Fatalf("readSkillName() = %q, want %q", name, "scan-health")
	}
}

func TestHasActionableCanonicalWorkflowAcceptsCRLF(t *testing.T) {
	const clean = "## Workflow\n\n" +
		"1. Read `canonical/WORKFLOW.md`\n" +
		"2. Follow the canonical workflow and its gates\n" +
		"3. Do not skip the canonical gates\n"
	if !hasActionableCanonicalWorkflow(strings.ReplaceAll(clean, "\n", "\r\n"), "canonical/WORKFLOW.md") {
		t.Fatal("CRLF wrapper with an actionable canonical handoff was rejected")
	}
}
