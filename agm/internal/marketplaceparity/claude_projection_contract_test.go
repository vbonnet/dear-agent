package marketplaceparity

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const projectionTestClaudeOnlyNeutralPlugin = `{
  "name": "spec-governance",
  "source": "./spec-governance",
  "description": "Canonical SPEC.md authoring and read-only consolidation-audit skills",
  "version": "0.1.0",
  "author": {"name": "dear-agent", "url": "https://github.com/vbonnet/dear-agent"},
  "repository": "https://github.com/vbonnet/dear-agent",
  "license": "Apache-2.0",
  "capabilities": ["skills"]
}`

const projectionTestNeutralResearchPlugin = `{"name":"research-pipeline","source":"./research-pipeline","description":"Research","version":"0.1.0","capabilities":["skills"]}`

const projectionTestClaudePlugin = `{
  "name": "spec-governance",
  "source": "./spec-governance",
  "description": "Canonical SPEC.md authoring and read-only consolidation-audit skills",
  "version": "0.1.0",
  "author": {"name": "dear-agent", "url": "https://github.com/vbonnet/dear-agent"},
  "repository": "https://github.com/vbonnet/dear-agent",
  "license": "Apache-2.0",
  "strict": true
}`

const projectionTestManifest = `{
  "name": "spec-governance",
  "version": "0.1.0",
  "description": "Canonical SPEC.md authoring and read-only consolidation-audit skills",
  "author": {"name": "dear-agent", "url": "https://github.com/vbonnet/dear-agent"},
  "repository": "https://github.com/vbonnet/dear-agent",
  "license": "Apache-2.0",
  "skills": ["./skills/audit-specs", "./skills/write-spec"]
}`

type projectionTestFixture struct {
	root          string
	neutralPath   string
	claudePath    string
	manifestPath  string
	ownerPath     string
	skillPath     string
	referencePath string
}

type projectionTestAuthorityTarget struct {
	name     string
	path     func(projectionTestFixture) string
	validate func(string) error
}

func projectionTestAuthorityTargets() []projectionTestAuthorityTarget {
	return []projectionTestAuthorityTarget{
		{name: "neutral catalog", path: func(f projectionTestFixture) string { return f.neutralPath }, validate: ValidateCatalog},
		{name: "claude catalog", path: func(f projectionTestFixture) string { return f.claudePath }, validate: ValidateClaudeMarketplaceMirror},
		{name: "manifest", path: func(f projectionTestFixture) string { return f.manifestPath }, validate: ValidateClaudeMarketplaceMirror},
		{name: "SPEC owner", path: func(f projectionTestFixture) string { return f.ownerPath }, validate: ValidateClaudeMarketplaceMirror},
		{name: "skill entrypoint", path: func(f projectionTestFixture) string { return f.skillPath }, validate: ValidateClaudeMarketplaceMirror},
		{name: "exported reference", path: func(f projectionTestFixture) string { return f.referencePath }, validate: ValidateClaudeMarketplaceMirror},
	}
}

func TestSpecGovernanceProjectionAcceptsExactContract(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	if err := ValidateCatalog(fixture.root); err != nil {
		t.Fatalf("ValidateCatalog() exact projection: %v", err)
	}
	if err := ValidateClaudeMarketplaceMirror(fixture.root); err != nil {
		t.Fatalf("ValidateClaudeMarketplaceMirror() exact projection: %v", err)
	}
}

func TestNeutralCatalogRejectsClaudeOnlySpecGovernance(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	projectionTestReplace(t, fixture.neutralPath, "\n  ],\n  \"harnesses\"", ",\n"+projectionTestClaudeOnlyNeutralPlugin+"\n  ],\n  \"harnesses\"")
	err := ValidateCatalog(fixture.root)
	if err == nil || !strings.Contains(err.Error(), "must not advertise Claude-only plugin") {
		t.Fatalf("ValidateCatalog() error = %v, want Claude-only inventory rejection", err)
	}
}

func TestSpecGovernanceManifestAuthorityIsExact(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "name", old: `"name": "spec-governance"`, new: `"name": "Spec-Governance"`},
		{name: "description", old: `"description": "Canonical SPEC.md authoring and read-only consolidation-audit skills"`, new: `"description": "Shadow policy"`},
		{name: "version", old: `"version": "0.1.0"`, new: `"version": "9.9.9"`},
		{name: "author name", old: `"author": {"name": "dear-agent", "url": "https://github.com/vbonnet/dear-agent"}`, new: `"author": {"name": "shadow", "url": "https://github.com/vbonnet/dear-agent"}`},
		{name: "author URL", old: `"author": {"name": "dear-agent", "url": "https://github.com/vbonnet/dear-agent"}`, new: `"author": {"name": "dear-agent", "url": "https://github.com/example/shadow"}`},
		{name: "author email", old: `"author": {"name": "dear-agent", "url": "https://github.com/vbonnet/dear-agent"}`, new: `"author": {"name": "dear-agent", "email": "shadow@example.com", "url": "https://github.com/vbonnet/dear-agent"}`},
		{name: "repository", old: `"repository": "https://github.com/vbonnet/dear-agent"`, new: `"repository": "https://github.com/example/shadow"`},
		{name: "license", old: `"license": "Apache-2.0"`, new: `"license": "MIT"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			projectionTestReplace(t, fixture.manifestPath, test.old, test.new)
			projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error {
				return ValidateClaudeMarketplaceMirror(fixture.root)
			})
		})
	}
}

func TestSpecGovernanceClaudeAuthorityIsExactAndStrict(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "name", old: `"name": "spec-governance"`, new: `"name": "Spec-Governance"`},
		{name: "source literal", old: `"source": "./spec-governance"`, new: `"source": "spec-governance"`},
		{name: "description", old: `"description": "Canonical SPEC.md authoring and read-only consolidation-audit skills"`, new: `"description": "Shadow policy"`},
		{name: "version", old: `"version": "0.1.0"`, new: `"version": "9.9.9"`},
		{name: "author name", old: `"author": {"name": "dear-agent", "url": "https://github.com/vbonnet/dear-agent"}`, new: `"author": {"name": "shadow", "url": "https://github.com/vbonnet/dear-agent"}`},
		{name: "author URL", old: `"author": {"name": "dear-agent", "url": "https://github.com/vbonnet/dear-agent"}`, new: `"author": {"name": "dear-agent", "url": "https://github.com/example/shadow"}`},
		{name: "author email", old: `"author": {"name": "dear-agent", "url": "https://github.com/vbonnet/dear-agent"}`, new: `"author": {"name": "dear-agent", "email": "shadow@example.com", "url": "https://github.com/vbonnet/dear-agent"}`},
		{name: "repository", old: `"repository": "https://github.com/vbonnet/dear-agent"`, new: `"repository": "https://github.com/example/shadow"`},
		{name: "license", old: `"license": "Apache-2.0"`, new: `"license": "MIT"`},
		{name: "strict false", old: `"strict": true`, new: `"strict": false`},
		{name: "strict missing", old: ",\n  \"strict\": true", new: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			projectionTestReplace(t, fixture.claudePath, test.old, test.new)
			projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error {
				return ValidateClaudeMarketplaceMirror(fixture.root)
			})
		})
	}
}

func TestSpecGovernanceManifestAndClaudeEntryUseClosedFields(t *testing.T) {
	manifestFields := []string{
		`"commands": "./commands"`,
		`"defaultEnabled": true`,
		`"metadata": {"owner": "shadow"}`,
		`"strict": true`,
		`"x-future": true`,
	}
	for _, field := range manifestFields {
		t.Run("manifest/"+projectionTestFieldName(field), func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			projectionTestReplace(t, fixture.manifestPath, "\n}", ",\n  "+field+"\n}")
			projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error {
				return ValidateClaudeMarketplaceMirror(fixture.root)
			})
		})
	}

	marketplaceFields := []string{
		`"skills": "./shadow-skills"`,
		`"commands": "./commands"`,
		`"defaultEnabled": true`,
		`"metadata": {"owner": "shadow"}`,
		`"x-future": true`,
	}
	for _, field := range marketplaceFields {
		t.Run("marketplace/"+projectionTestFieldName(field), func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			projectionTestReplace(t, fixture.claudePath, "\n}\n  ]", ",\n  "+field+"\n}\n  ]")
			projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error {
				return ValidateClaudeMarketplaceMirror(fixture.root)
			})
		})
	}

	t.Run("marketplace root", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		projectionTestReplace(t, fixture.claudePath, "\n  ]\n}\n", "\n  ],\n  \"metadata\": {\"pluginRoot\": \"./shadow\"}\n}\n")
		projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error {
			return ValidateClaudeMarketplaceMirror(fixture.root)
		})
	})
}

func TestClaudeSharedMarketplaceEntryRejectsBehaviorField(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	projectionTestReplace(
		t,
		fixture.claudePath,
		`{"name":"agm","source":"./agm/agm-plugin","description":"AGM","version":"0.4.1"}`,
		`{"name":"agm","source":"./agm/agm-plugin","description":"AGM","version":"0.4.1","commands":"./commands"}`,
	)
	err := ValidateClaudeMarketplaceMirror(fixture.root)
	if err == nil || !strings.Contains(err.Error(), `forbidden field "commands"`) {
		t.Fatalf("ValidateClaudeMarketplaceMirror() error = %v, want shared-entry closed-field rejection", err)
	}
}

func TestNeutralCatalogUnknownNonAliasFieldsRemainCompatible(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	projectionTestReplace(t, fixture.neutralPath, "\n  ]\n}\n", "\n  ],\n  \"x-catalog-note\": \"retained\"\n}\n")
	projectionTestReplace(t, fixture.neutralPath, projectionTestNeutralResearchPlugin, strings.TrimSuffix(projectionTestNeutralResearchPlugin, "}")+`,"x-plugin-note":"retained"}`)
	if err := ValidateCatalog(fixture.root); err != nil {
		t.Fatalf("ValidateCatalog() forward-compatible neutral fields: %v", err)
	}
}

func TestProjectionRejectsDuplicateAndCaseAliasAuthorityFields(t *testing.T) {
	tests := []struct {
		name     string
		path     func(projectionTestFixture) string
		old      string
		new      string
		validate func(string) error
	}{
		{
			name:     "neutral duplicate plugin name",
			path:     func(f projectionTestFixture) string { return f.neutralPath },
			old:      `"name":"agm",`,
			new:      `"name":"agm","name":"shadow",`,
			validate: ValidateCatalog,
		},
		{
			name:     "neutral nested duplicate author",
			path:     func(f projectionTestFixture) string { return f.neutralPath },
			old:      `"author":{"name":"dear-agent","url":"https://github.com/vbonnet/dear-agent"}`,
			new:      `"author":{"name":"dear-agent","name":"shadow","url":"https://github.com/vbonnet/dear-agent"}`,
			validate: ValidateCatalog,
		},
		{
			name:     "neutral repository case alias",
			path:     func(f projectionTestFixture) string { return f.neutralPath },
			old:      `"repository":"https://github.com/vbonnet/dear-agent",`,
			new:      `"repository":"https://github.com/vbonnet/dear-agent","Repository":"https://github.com/example/shadow",`,
			validate: ValidateCatalog,
		},
		{
			name:     "manifest duplicate skills",
			path:     func(f projectionTestFixture) string { return f.manifestPath },
			old:      `"skills": ["./skills/audit-specs", "./skills/write-spec"]`,
			new:      `"skills": [], "skills": ["./skills/audit-specs", "./skills/write-spec"]`,
			validate: ValidateClaudeMarketplaceMirror,
		},
		{
			name:     "manifest skills case alias",
			path:     func(f projectionTestFixture) string { return f.manifestPath },
			old:      `"skills": ["./skills/audit-specs", "./skills/write-spec"]`,
			new:      `"skills": ["./skills/audit-specs", "./skills/write-spec"], "Skills": []`,
			validate: ValidateClaudeMarketplaceMirror,
		},
		{
			name:     "claude duplicate strict",
			path:     func(f projectionTestFixture) string { return f.claudePath },
			old:      `"strict": true`,
			new:      `"strict": true, "strict": false`,
			validate: ValidateClaudeMarketplaceMirror,
		},
		{
			name:     "claude strict case alias",
			path:     func(f projectionTestFixture) string { return f.claudePath },
			old:      `"strict": true`,
			new:      `"strict": true, "Strict": false`,
			validate: ValidateClaudeMarketplaceMirror,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			projectionTestReplace(t, test.path(fixture), test.old, test.new)
			projectionTestRequireError(t, test.name, func() error { return test.validate(fixture.root) })
		})
	}
}

func TestSpecGovernanceProjectionInventoriesAreExact(t *testing.T) {
	t.Run("neutral missing", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		projectionTestReplace(t, fixture.neutralPath, ",\n    "+projectionTestNeutralResearchPlugin+"\n", "\n")
		projectionTestRequireError(t, "ValidateCatalog", func() error { return ValidateCatalog(fixture.root) })
	})
	t.Run("neutral duplicate", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		projectionTestReplace(t, fixture.neutralPath, "\n  ],\n  \"harnesses\"", ",\n    "+projectionTestNeutralResearchPlugin+"\n  ],\n  \"harnesses\"")
		projectionTestRequireError(t, "ValidateCatalog", func() error { return ValidateCatalog(fixture.root) })
	})

	for _, test := range []struct {
		name string
		edit func(*testing.T, projectionTestFixture)
	}{
		{
			name: "claude missing neutral plugin",
			edit: func(t *testing.T, f projectionTestFixture) {
				projectionTestReplace(t, f.claudePath, "    {\"name\":\"agm\",\"source\":\"./agm/agm-plugin\",\"description\":\"AGM\",\"version\":\"0.4.1\"},\n", "")
			},
		},
		{
			name: "claude missing extension",
			edit: func(t *testing.T, f projectionTestFixture) {
				projectionTestReplace(t, f.claudePath, ",\n"+projectionTestClaudePlugin, "")
			},
		},
		{
			name: "claude duplicate",
			edit: func(t *testing.T, f projectionTestFixture) {
				projectionTestReplace(t, f.claudePath, "\n  ]\n}", ",\n"+projectionTestClaudePlugin+"\n  ]\n}")
			},
		},
		{
			name: "claude extra",
			edit: func(t *testing.T, f projectionTestFixture) {
				projectionTestReplace(t, f.claudePath, "\n  ]\n}", ",\n    {\"name\":\"shadow\",\"source\":\"./shadow\",\"version\":\"0.1.0\"}\n  ]\n}")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			test.edit(t, fixture)
			projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error {
				return ValidateClaudeMarketplaceMirror(fixture.root)
			})
		})
	}

	for _, test := range []struct {
		name   string
		skills string
	}{
		{name: "manifest missing", skills: `"./skills/write-spec"`},
		{name: "manifest duplicate", skills: `"./skills/audit-specs", "./skills/audit-specs", "./skills/write-spec"`},
		{name: "manifest extra", skills: `"./skills/audit-specs", "./skills/write-spec", "./skills/shadow"`},
		{name: "manifest path alias", skills: `"skills/audit-specs", "./skills/write-spec"`},
		{name: "manifest aggregate root", skills: `"./skills"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			projectionTestReplace(t, fixture.manifestPath,
				`"./skills/audit-specs", "./skills/write-spec"`, test.skills)
			projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error { return ValidateClaudeMarketplaceMirror(fixture.root) })
		})
	}

	for _, test := range []struct {
		name string
		edit func(*testing.T, projectionTestFixture)
	}{
		{
			name: "source missing skill",
			edit: func(t *testing.T, f projectionTestFixture) {
				if err := os.Rename(filepath.Join(f.root, "spec-governance", "skills", "audit-specs"), filepath.Join(f.root, "spec-governance", "audit-specs-hidden")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "source extra skill",
			edit: func(t *testing.T, f projectionTestFixture) {
				projectionTestWriteFile(t, filepath.Join(f.root, "spec-governance", "skills", "shadow", "SKILL.md"), projectionTestSkill("shadow"))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			test.edit(t, fixture)
			projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error { return ValidateClaudeMarketplaceMirror(fixture.root) })
		})
	}

	for _, component := range []string{
		".lsp.json",
		".mcp.json",
		"agents",
		"bin",
		"bun.lock",
		"bun.lockb",
		"channels",
		"commands",
		"hooks",
		"monitors",
		"npm-shrinkwrap.json",
		"output-styles",
		"package-lock.json",
		"package.json",
		"pnpm-lock.yaml",
		"settings.json",
		"themes",
		"workflows",
		"yarn.lock",
	} {
		t.Run("provider component/"+component, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			componentPath := filepath.Join(fixture.root, "spec-governance", component)
			if strings.HasSuffix(component, ".json") {
				projectionTestWriteFile(t, componentPath, "{}\n")
			} else if err := os.Mkdir(componentPath, 0o755); err != nil {
				t.Fatal(err)
			}
			projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error { return ValidateClaudeMarketplaceMirror(fixture.root) })
		})
	}
}

func TestSpecGovernanceTopLevelFileAllowlistIsClosed(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	projectionTestWriteFile(t, filepath.Join(fixture.root, "spec-governance", "README.md"), "# Package documentation\n")
	projectionTestWriteFile(t, filepath.Join(fixture.root, "spec-governance", "SPEC.md"), "# Package contract\n")
	if err := ValidateClaudeMarketplaceMirror(fixture.root); err != nil {
		t.Fatalf("ValidateClaudeMarketplaceMirror() rejected allowed top-level documentation: %v", err)
	}

	projectionTestWriteFile(t, filepath.Join(fixture.root, "spec-governance", "NOTES.md"), "# Unreviewed package file\n")
	err := ValidateClaudeMarketplaceMirror(fixture.root)
	if err == nil {
		t.Fatal("ValidateClaudeMarketplaceMirror() accepted an unexpected top-level file")
	}
	if !strings.Contains(err.Error(), "unexpected top-level file") {
		t.Fatalf("ValidateClaudeMarketplaceMirror() error = %q, want closed top-level allowlist rejection", err)
	}
}

func TestProjectionRejectsSymlinkedAuthorityPaths(t *testing.T) {
	for _, test := range projectionTestAuthorityTargets() {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			projectionTestReplaceWithSymlink(t, test.path(fixture))
			projectionTestRequireError(t, test.name, func() error { return test.validate(fixture.root) })
		})
	}

	t.Run("plugin source directory", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		source := filepath.Join(fixture.root, "spec-governance")
		target := filepath.Join(fixture.root, "real-spec-governance")
		if err := os.Rename(source, target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Base(target), source); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error { return ValidateClaudeMarketplaceMirror(fixture.root) })
	})

	t.Run("skill directory", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		skill := filepath.Join(fixture.root, "spec-governance", "skills", "audit-specs")
		target := filepath.Join(filepath.Dir(skill), "real-audit-specs")
		if err := os.Rename(skill, target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Base(target), skill); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		projectionTestRequireError(t, "ValidateClaudeMarketplaceMirror", func() error { return ValidateClaudeMarketplaceMirror(fixture.root) })
	})
}

func TestProjectionRejectsDirectoryInPlaceOfAuthorityFile(t *testing.T) {
	for _, test := range projectionTestAuthorityTargets() {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			path := test.path(fixture)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			projectionTestRequireError(t, test.name, func() error { return test.validate(fixture.root) })
		})
	}
}

func TestProjectionRejectsHardlinkedAuthorityFiles(t *testing.T) {
	for _, test := range projectionTestAuthorityTargets() {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			projectionTestReplaceWithHardlink(t, test.path(fixture))
			projectionTestRequireError(t, test.name, func() error { return test.validate(fixture.root) })
		})
	}
}

func TestProjectionRejectsOversizedAuthorityJSON(t *testing.T) {
	tests := []struct {
		name     string
		path     func(projectionTestFixture) string
		validate func(string) error
	}{
		{name: "neutral catalog", path: func(f projectionTestFixture) string { return f.neutralPath }, validate: ValidateCatalog},
		{name: "claude catalog", path: func(f projectionTestFixture) string { return f.claudePath }, validate: ValidateClaudeMarketplaceMirror},
		{name: "manifest", path: func(f projectionTestFixture) string { return f.manifestPath }, validate: ValidateClaudeMarketplaceMirror},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			path := test.path(fixture)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			oversized := append(bytes.Repeat([]byte(" "), 2<<20), data...)
			if err := os.WriteFile(path, oversized, 0o644); err != nil {
				t.Fatal(err)
			}
			projectionTestRequireError(t, test.name, func() error { return test.validate(fixture.root) })
		})
	}
}

func newProjectionTestFixture(t *testing.T) projectionTestFixture {
	t.Helper()
	root := t.TempDir()
	for path, directories := range map[string][]string{
		"agm/agm-plugin":     {"commands", "skills"},
		"wayfinder":          {"skills"},
		"agm/youtube-plugin": {"commands"},
		"research-pipeline":  {"skills"},
	} {
		for _, directory := range directories {
			if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(path), directory), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}

	manifestPath := filepath.Join(root, "spec-governance", ".claude-plugin", "plugin.json")
	ownerPath := filepath.Join(root, "spec-governance", ".claude-plugin", "SPEC.owner")
	const fixtureLicense = "Apache License fixture\n"
	projectionTestWriteFile(t, filepath.Join(root, canonicalRepositoryLicense), fixtureLicense)
	projectionTestWriteFile(t, filepath.Join(root, "spec-governance", canonicalPackagedLicense), fixtureLicense)
	projectionTestWriteFile(t, manifestPath, projectionTestManifest+"\n")
	projectionTestWriteFile(t, ownerPath, "agm/internal/marketplaceparity/SPEC.md\n")
	projectionTestWriteFile(t, filepath.Join(root, "agm", "internal", "marketplaceparity", "SPEC.md"), "# Marketplace fixture specification\n")

	var skillPath, referencePath string
	for _, skill := range []string{"audit-specs", "write-spec"} {
		entrypoint := filepath.Join(root, "spec-governance", "skills", skill, "SKILL.md")
		reference := filepath.Join(root, "spec-governance", "skills", skill, "references", "guide.md")
		projectionTestWriteFile(t, entrypoint, projectionTestSkill(skill))
		projectionTestWriteFile(t, reference, "# Canonical reference\n\nUse the canonical workflow.\n")
		if skill == "audit-specs" {
			skillPath = entrypoint
			referencePath = reference
		}
	}

	neutralPath := filepath.Join(root, filepath.FromSlash(NeutralCatalogPath))
	claudePath := filepath.Join(root, filepath.FromSlash(ClaudeCatalogPath))
	projectionTestWriteFile(t, neutralPath, projectionTestNeutralCatalog())
	projectionTestWriteFile(t, claudePath, projectionTestClaudeCatalog())
	return projectionTestFixture{
		root:          root,
		neutralPath:   neutralPath,
		claudePath:    claudePath,
		manifestPath:  manifestPath,
		ownerPath:     ownerPath,
		skillPath:     skillPath,
		referencePath: referencePath,
	}
}

func projectionTestNeutralCatalog() string {
	return `{
  "schema_version": "dear-agent.marketplace/v1",
  "name": "dear-agent",
  "description": "Fixture marketplace",
  "owner": {"name": "dear-agent", "email": "tools@example.com"},
  "plugins": [
    {"name":"agm","source":"./agm/agm-plugin","description":"AGM","version":"0.4.1","author":{"name":"dear-agent","url":"https://github.com/vbonnet/dear-agent"},"repository":"https://github.com/vbonnet/dear-agent","capabilities":["commands","skills"]},
    {"name":"wayfinder","source":"./wayfinder","description":"Wayfinder","version":"0.3.0","capabilities":["skills"]},
    {"name":"youtube","source":"./agm/youtube-plugin","description":"YouTube","version":"0.1.0","capabilities":["commands"]},
    ` + projectionTestNeutralResearchPlugin + `
  ],
  "harnesses": [
    {"name":"claude-code","mode":"native-claude-plugin-marketplace","catalog":".claude-plugin/marketplace.json"},
    {"name":"codex-cli","mode":"agents-md-skill-fallback","catalog":".dear-agent/marketplace.json"},
    {"name":"agy","mode":"agents-md-skill-fallback","catalog":".dear-agent/marketplace.json"},
    {"name":"opencode-cli","mode":"agents-md-skill-fallback","catalog":".dear-agent/marketplace.json"},
    {"name":"pi-cli","mode":"agents-md-skill-fallback","catalog":".dear-agent/marketplace.json"}
  ]
}
`
}

func projectionTestClaudeCatalog() string {
	return `{
  "name": "dear-agent",
  "description": "Fixture marketplace",
  "owner": {"name": "dear-agent", "email": "tools@example.com"},
  "plugins": [
    {"name":"agm","source":"./agm/agm-plugin","description":"AGM","version":"0.4.1"},
    {"name":"wayfinder","source":"./wayfinder","description":"Wayfinder","version":"0.3.0"},
    {"name":"youtube","source":"./agm/youtube-plugin","description":"YouTube","version":"0.1.0"},
    {"name":"research-pipeline","source":"./research-pipeline","description":"Research","version":"0.1.0"},
` + projectionTestClaudePlugin + `
  ]
}
`
}

func projectionTestSkill(name string) string {
	return "---\nname: " + name + "\ndescription: Use when testing the exact canonical marketplace projection.\n---\n" +
		"# " + name + "\n\n## Workflow\n\n1. Inspect the requested source.\n2. Follow the canonical contract.\n\n" +
		"## Verification\n\nVerify the result against the declared contract.\n"
}

func projectionTestWriteFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func projectionTestReplace(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), old); count != 1 {
		t.Fatalf("fixture replacement %q in %s matched %d times, want 1", old, path, count)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func projectionTestRequireError(t *testing.T, operation string, validate func() error) {
	t.Helper()
	if err := validate(); err == nil {
		t.Fatalf("%s accepted an invalid projection", operation)
	}
}

func projectionTestReplaceWithSymlink(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), filepath.Base(path))
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func projectionTestReplaceWithHardlink(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), filepath.Base(path))
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, path); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
}

func projectionTestFieldName(field string) string {
	field = strings.TrimPrefix(field, `"`)
	if end := strings.IndexByte(field, '"'); end >= 0 {
		field = field[:end]
	}
	return field
}
