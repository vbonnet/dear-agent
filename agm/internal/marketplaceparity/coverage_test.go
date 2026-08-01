package marketplaceparity

import (
	"encoding/json"
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
		if harness == "claude-code" && surface.Mode != "native-claude-plugin-marketplace" {
			t.Errorf("claude-code mode = %q, want native-claude-plugin-marketplace", surface.Mode)
		}
		if harness != "claude-code" && surface.Mode != "agents-md-skill-fallback" {
			t.Errorf("%s mode = %q, want agents-md-skill-fallback", harness, surface.Mode)
		}
	}
}

func TestClaudeMarketplaceMirrorsNeutralCatalog(t *testing.T) {
	if err := ValidateClaudeMarketplaceMirror(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeMarketplaceRejectsNonCanonicalPluginSet(t *testing.T) {
	root := t.TempDir()
	writeMarketplaceFixture(t, root, NeutralCatalogPath, Catalog{
		SchemaVersion: "dear-agent.marketplace/v1",
		Name:          "test",
		Plugins:       []PluginEntry{{Name: "canonical", Source: "./canonical", Version: "1.0.0"}},
	})
	writeMarketplaceFixture(t, root, ClaudeCatalogPath, claudeCatalog{
		Name: "test",
		Plugins: []claudePluginEntry{
			{Name: "canonical", Source: "./canonical", Version: "1.0.0"},
			{Name: "claude-only", Source: "./unowned", Version: "1.0.0"},
		},
	})

	err := ValidateClaudeMarketplaceMirror(root)
	if err == nil || !strings.Contains(err.Error(), "want exact neutral set [canonical]") {
		t.Fatalf("ValidateClaudeMarketplaceMirror() error = %v, want exact-set rejection", err)
	}
}

func TestClaudeMarketplaceRejectsInvalidExpandedSkillBundle(t *testing.T) {
	tests := []struct {
		name            string
		plugin          claudePluginEntry
		neutralSource   string
		capabilities    []string
		duplicatePlugin bool
		wantErr         string
	}{
		{
			name:    "implicit strict mode",
			plugin:  claudePluginEntry{Name: "example", Source: ".", Version: "1.0.0", Skills: []string{"./plugin/skills/example"}},
			wantErr: "strict to false",
		},
		{
			name:    "escaping skill path",
			plugin:  claudePluginEntry{Name: "example", Source: ".", Version: "1.0.0", Strict: new(false), Skills: []string{"./other/skills/example"}},
			wantErr: "exact canonical set",
		},
		{
			name:    "duplicate skill path",
			plugin:  claudePluginEntry{Name: "example", Source: ".", Version: "1.0.0", Strict: new(false), Skills: []string{"./plugin/skills/example", "./plugin/skills/example"}},
			wantErr: "exact canonical set",
		},
		{
			name:    "unauthenticated expanded source",
			plugin:  claudePluginEntry{Name: "example", Source: "./other", Version: "1.0.0", Strict: new(false), Skills: []string{"./plugin/skills/example"}},
			wantErr: "authenticated marketplace root",
		},
		{
			name:          "escaping neutral source",
			plugin:        claudePluginEntry{Name: "example", Source: ".", Version: "1.0.0", Strict: new(false), Skills: []string{"./outside/skills/example"}},
			neutralSource: "../outside",
			wantErr:       "repository subtree",
		},
		{
			name:         "mixed neutral capabilities",
			plugin:       claudePluginEntry{Name: "example", Source: ".", Version: "1.0.0", Strict: new(false), Skills: []string{"./plugin/skills/example"}},
			capabilities: []string{"commands", "skills"},
			wantErr:      "exact skills capability",
		},
		{
			name:            "duplicate plugin entry",
			plugin:          claudePluginEntry{Name: "example", Source: ".", Version: "1.0.0", Strict: new(false), Skills: []string{"./plugin/skills/example"}},
			duplicatePlugin: true,
			wantErr:         "duplicate plugin",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			neutralSource := test.neutralSource
			if neutralSource == "" {
				neutralSource = "./plugin"
			}
			capabilities := test.capabilities
			if capabilities == nil {
				capabilities = []string{"skills"}
			}
			writeMarketplaceFixture(t, root, NeutralCatalogPath, Catalog{
				SchemaVersion: "dear-agent.marketplace/v1",
				Name:          "test",
				Plugins: []PluginEntry{{
					Name: "example", Source: neutralSource, Version: "1.0.0", Capabilities: capabilities,
				}},
			})
			plugins := []claudePluginEntry{test.plugin}
			if test.duplicatePlugin {
				plugins = append(plugins, test.plugin)
			}
			writeMarketplaceFixture(t, root, ClaudeCatalogPath, claudeCatalog{
				Name: "test", Plugins: plugins,
			})
			skill := filepath.Join(root, "plugin", "skills", "example", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(skill, []byte("---\nname: example\ndescription: test\n---\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			if err := ValidateClaudeMarketplaceMirror(root); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateClaudeMarketplaceMirror() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestClaudeMarketplaceRejectsSymlinkedCanonicalSkillSubtree(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	skill := filepath.Join(outside, "skills", "example", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("---\nname: example\ndescription: test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "plugin")); err != nil {
		t.Fatal(err)
	}
	writeMarketplaceFixture(t, root, NeutralCatalogPath, Catalog{
		SchemaVersion: "dear-agent.marketplace/v1",
		Name:          "test",
		Plugins: []PluginEntry{{
			Name: "example", Source: "./plugin", Version: "1.0.0", Capabilities: []string{"skills"},
		}},
	})
	writeMarketplaceFixture(t, root, ClaudeCatalogPath, claudeCatalog{
		Name: "test",
		Plugins: []claudePluginEntry{{
			Name: "example", Source: ".", Version: "1.0.0", Strict: new(false), Skills: []string{"./plugin/skills/example"},
		}},
	})

	if err := ValidateClaudeMarketplaceMirror(root); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("ValidateClaudeMarketplaceMirror() error = %v, want symlink rejection", err)
	}
}

func TestClaudeMarketplaceRejectsSpecGovernanceNativeSurfaceExpansion(t *testing.T) {
	for _, test := range []struct {
		name            string
		manifest        string
		extraPath       string
		extraSkill      string
		expandedCatalog bool
		wantErr         string
	}{
		{
			name:     "plugin-level hooks",
			manifest: `{"name":"spec-governance","version":"0.1.0","description":"test","author":{"name":"test"},"skills":["./skills/audit-specs","./skills/write-spec"],"hooks":{}}`,
			wantErr:  "unknown field",
		},
		{
			name:      "MCP asset",
			manifest:  `{"name":"spec-governance","version":"0.1.0","description":"test","author":{"name":"test"},"skills":["./skills/audit-specs","./skills/write-spec"]}`,
			extraPath: "spec-governance/.mcp.json",
			wantErr:   "must not expose",
		},
		{
			name:            "fully declared third canonical skill",
			manifest:        `{"name":"spec-governance","version":"0.1.0","description":"test","author":{"name":"test"},"skills":["./skills/audit-specs","./skills/review-spec","./skills/write-spec"]}`,
			extraSkill:      "review-spec",
			expandedCatalog: true,
			wantErr:         "want fixed set [audit-specs write-spec]",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeMarketplaceFixture(t, root, NeutralCatalogPath, Catalog{
				SchemaVersion: "dear-agent.marketplace/v1", Name: "test",
				Plugins: []PluginEntry{{Name: "spec-governance", Source: "./spec-governance", Version: "0.1.0", Capabilities: []string{"skills"}}},
			})
			claudePlugin := claudePluginEntry{Name: "spec-governance", Source: "./spec-governance", Version: "0.1.0"}
			if test.expandedCatalog {
				claudePlugin.Strict = new(false)
				claudePlugin.Skills = []string{"./skills/audit-specs", "./skills/review-spec", "./skills/write-spec"}
			}
			writeMarketplaceFixture(t, root, ClaudeCatalogPath, claudeCatalog{Name: "test", Plugins: []claudePluginEntry{claudePlugin}})
			writeValidSpecGovernanceDistribution(t, root, test.manifest)
			if test.extraSkill != "" {
				path := filepath.Join(root, "spec-governance", "skills", test.extraSkill, "SKILL.md")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("---\nname: "+test.extraSkill+"\ndescription: test\n---\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if test.extraPath != "" {
				writeMarketplaceFixture(t, root, test.extraPath, map[string]any{})
			}
			if err := ValidateClaudeMarketplaceMirror(root); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateClaudeMarketplaceMirror() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestSpecGovernanceDistributionClosureRejectsUnsafeEntries(t *testing.T) {
	const manifest = `{"name":"spec-governance","version":"0.1.0","description":"test","author":{"name":"test"},"skills":["./skills/audit-specs","./skills/write-spec"]}`
	tests := []struct {
		name    string
		mutate  func(*testing.T, string)
		wantErr string
	}{
		{
			name: "missing required file",
			mutate: func(t *testing.T, pluginRoot string) {
				t.Helper()
				if err := os.Remove(filepath.Join(pluginRoot, "skills", "audit-specs", "scripts", "specaudit", "main.go")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "main.go",
		},
		{
			name: "symlinked required ancestor",
			mutate: func(t *testing.T, pluginRoot string) {
				t.Helper()
				original := filepath.Join(pluginRoot, "skills", "audit-specs")
				outside := filepath.Join(t.TempDir(), "audit-specs")
				if err := os.Rename(original, outside); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, original); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "must not be a symlink",
		},
		{
			name: "non-regular required file",
			mutate: func(t *testing.T, pluginRoot string) {
				t.Helper()
				path := filepath.Join(pluginRoot, "go.mod")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "must be a regular file",
		},
		{
			name: "hard-linked required file",
			mutate: func(t *testing.T, pluginRoot string) {
				t.Helper()
				path := filepath.Join(pluginRoot, "go.mod")
				outside := filepath.Join(t.TempDir(), "go.mod")
				if err := os.WriteFile(outside, []byte("module example.invalid/test\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(outside, path); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "exactly one hard link",
		},
		{
			name: "non-executable launcher",
			mutate: func(t *testing.T, pluginRoot string) {
				t.Helper()
				if err := os.Chmod(filepath.Join(pluginRoot, "scripts", "specaudit"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "must be executable",
		},
		{
			name: "unsafe-extra-Go",
			mutate: func(t *testing.T, pluginRoot string) {
				t.Helper()
				path := filepath.Join(pluginRoot, "skills", "audit-specs", "scripts", "specaudit", "stale.go")
				if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "undeclared executable Go source",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			pluginRoot := writeValidSpecGovernanceDistribution(t, root, manifest)
			test.mutate(t, pluginRoot)
			neutral := PluginEntry{Name: "spec-governance", Source: "./spec-governance", Version: "0.1.0", Capabilities: []string{"skills"}}
			claude := claudePluginEntry{Name: "spec-governance", Source: "./spec-governance", Version: "0.1.0"}
			err := validateNativeSpecGovernanceSurface(root, neutral, claude)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateNativeSpecGovernanceSurface() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestDistributionClosureRejectsEscapingManifestPath(t *testing.T) {
	_, err := requireContainedRelativePath(t.TempDir(), "../outside")
	if err == nil || !strings.Contains(err.Error(), "inside the distribution root") {
		t.Fatalf("requireContainedRelativePath() error = %v, want escape rejection", err)
	}
}

func writeValidSpecGovernanceDistribution(t *testing.T, root, manifest string) string {
	t.Helper()
	pluginRoot := filepath.Join(root, "spec-governance")
	for _, entry := range specGovernanceDistributionManifest {
		path := filepath.Join(pluginRoot, entry.path)
		switch entry.kind {
		case distributionDirectory:
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		case distributionRegularFile:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			mode := os.FileMode(0o644)
			if entry.executable {
				mode = 0o755
			}
			if err := os.WriteFile(path, []byte("fixture\n"), mode); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return pluginRoot
}

func writeMarketplaceFixture(t *testing.T, root, relative string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
