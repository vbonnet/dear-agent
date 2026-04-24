package harnesseffort

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// GenerateGemini
// ---------------------------------------------------------------------------

func TestGenerateGemini_ContainsAllTiers(t *testing.T) {
	cfg, err := LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	cfg = ResolveAliases(cfg)

	out := GenerateGemini(cfg)

	for _, tier := range []string{"lookup", "operational", "analysis", "deep"} {
		alias := "alias gemini-" + tier + "="
		if !strings.Contains(out, alias) {
			t.Errorf("expected %q in Gemini output, got:\n%s", alias, out)
		}
	}
}

func TestGenerateGemini_ContainsComment(t *testing.T) {
	cfg, err := LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	out := GenerateGemini(cfg)
	if !strings.Contains(out, "# Gemini effort tier aliases") {
		t.Errorf("expected header comment in Gemini output, got:\n%s", out)
	}
	if !strings.Contains(out, "~/.bashrc") {
		t.Errorf("expected ~/.bashrc reference in Gemini output, got:\n%s", out)
	}
}

func TestGenerateGemini_DefaultModels(t *testing.T) {
	// Empty config should fall back to geminiTierModelMap defaults.
	out := GenerateGemini(HarnessEffortConfig{})

	if !strings.Contains(out, "gemini-2.5-flash") {
		t.Errorf("expected gemini-2.5-flash in output, got:\n%s", out)
	}
	if !strings.Contains(out, "gemini-2.5-pro") {
		t.Errorf("expected gemini-2.5-pro in output, got:\n%s", out)
	}
}

func TestGenerateGemini_ConfigOverridesModel(t *testing.T) {
	cfg := makeConfig(
		withTier("lookup", "Lookup", map[string]ProviderConfig{
			"google": {Model: "gemini-custom-model"},
		}),
	)
	out := GenerateGemini(cfg)
	if !strings.Contains(out, "gemini-custom-model") {
		t.Errorf("expected custom model in Gemini output, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// geminiTierModelFromCfg
// ---------------------------------------------------------------------------

func TestGeminiTierModelFromCfg_FallsBackToMap(t *testing.T) {
	// Config with no google provider — should return the map default.
	got := geminiTierModelFromCfg(HarnessEffortConfig{}, "lookup")
	if got != "gemini-2.5-flash" {
		t.Errorf("expected gemini-2.5-flash, got %q", got)
	}
}

func TestGeminiTierModelFromCfg_FallsBackToFlashForUnknownTier(t *testing.T) {
	got := geminiTierModelFromCfg(HarnessEffortConfig{}, "nonexistent-tier")
	if got != "gemini-2.5-flash" {
		t.Errorf("expected gemini-2.5-flash for unknown tier, got %q", got)
	}
}

func TestGeminiTierModelFromCfg_UsesConfigGoogleModel(t *testing.T) {
	cfg := makeConfig(
		withTier("deep", "Deep", map[string]ProviderConfig{
			"google": {Model: "gemini-ultra"},
		}),
	)
	got := geminiTierModelFromCfg(cfg, "deep")
	if got != "gemini-ultra" {
		t.Errorf("expected gemini-ultra, got %q", got)
	}
}

func TestGeminiTierModelFromCfg_EmptyModelFallsBack(t *testing.T) {
	// google provider present but model is empty — should fall back to map.
	cfg := makeConfig(
		withTier("analysis", "Analysis", map[string]ProviderConfig{
			"google": {Model: ""},
		}),
	)
	got := geminiTierModelFromCfg(cfg, "analysis")
	if got != "gemini-2.5-pro" {
		t.Errorf("expected gemini-2.5-pro fallback, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// loadOverrideFile
// ---------------------------------------------------------------------------

func TestLoadOverrideFile_MissingFileReturnsEmpty(t *testing.T) {
	cfg, err := loadOverrideFile("/nonexistent/path/harness-effort.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	// Should return empty config (all zero values).
	if cfg.Tiers != nil || cfg.ModelAliases != nil {
		t.Errorf("expected empty config for missing file, got: %+v", cfg)
	}
}

func TestLoadOverrideFile_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "harness-effort.yaml")
	content := `
model_aliases:
  latest-haiku: claude-haiku-5
tiers:
  lookup:
    description: "Fast"
    providers:
      anthropic:
        model: claude-haiku-5
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := loadOverrideFile(path)
	if err != nil {
		t.Fatalf("loadOverrideFile: %v", err)
	}
	if cfg.ModelAliases["latest-haiku"] != "claude-haiku-5" {
		t.Errorf("alias = %q, want %q", cfg.ModelAliases["latest-haiku"], "claude-haiku-5")
	}
	if cfg.Tiers["lookup"].Providers["anthropic"].Model != "claude-haiku-5" {
		t.Errorf("tier model = %q, want %q", cfg.Tiers["lookup"].Providers["anthropic"].Model, "claude-haiku-5")
	}
}

func TestLoadOverrideFile_InvalidYAMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("tiers: [invalid: yaml: {{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := loadOverrideFile(path)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// ---------------------------------------------------------------------------
// Generate (top-level orchestration)
// ---------------------------------------------------------------------------

func TestGenerate_DefaultOutputsAllHarnesses(t *testing.T) {
	// Point OpenCodeDir to a temp dir to avoid cwd pollution.
	dir := t.TempDir()
	opts := GenerateOpts{
		DryRun:      true,
		OpenCodeDir: dir,
	}

	outputs, gemini, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Expect two output files: codex and opencode.
	if len(outputs) != 2 {
		t.Errorf("expected 2 outputs, got %d", len(outputs))
	}

	// Gemini suggestions must be non-empty.
	if gemini == "" {
		t.Error("expected non-empty Gemini suggestions")
	}
	if !strings.Contains(gemini, "alias gemini-lookup=") {
		t.Errorf("expected gemini-lookup alias, got:\n%s", gemini)
	}
}

func TestGenerate_HarnessFilterCodex(t *testing.T) {
	dir := t.TempDir()
	opts := GenerateOpts{
		DryRun:      true,
		Harness:     "codex",
		OpenCodeDir: dir,
	}

	outputs, gemini, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate(codex): %v", err)
	}
	if len(outputs) != 1 {
		t.Errorf("expected 1 output for codex harness, got %d", len(outputs))
	}
	if !strings.HasSuffix(outputs[0].Path, "config.toml") {
		t.Errorf("expected codex output path to end in config.toml, got %q", outputs[0].Path)
	}
	if gemini != "" {
		t.Errorf("expected empty Gemini output when filtering to codex, got:\n%s", gemini)
	}
}

func TestGenerate_HarnessFilterOpenCode(t *testing.T) {
	dir := t.TempDir()
	opts := GenerateOpts{
		DryRun:      true,
		Harness:     "opencode",
		OpenCodeDir: dir,
	}

	outputs, gemini, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate(opencode): %v", err)
	}
	if len(outputs) != 1 {
		t.Errorf("expected 1 output for opencode harness, got %d", len(outputs))
	}
	if !strings.HasSuffix(outputs[0].Path, "opencode.json") {
		t.Errorf("expected opencode output path to end in opencode.json, got %q", outputs[0].Path)
	}
	if gemini != "" {
		t.Errorf("expected empty Gemini output when filtering to opencode, got:\n%s", gemini)
	}
}

func TestGenerate_HarnessFilterGemini(t *testing.T) {
	dir := t.TempDir()
	opts := GenerateOpts{
		DryRun:      true,
		Harness:     "gemini",
		OpenCodeDir: dir,
	}

	outputs, gemini, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate(gemini): %v", err)
	}
	if len(outputs) != 0 {
		t.Errorf("expected 0 file outputs for gemini harness, got %d", len(outputs))
	}
	if gemini == "" {
		t.Error("expected non-empty Gemini suggestions")
	}
}

func TestGenerate_DryRunDoesNotWriteFiles(t *testing.T) {
	dir := t.TempDir()
	opts := GenerateOpts{
		DryRun:      true,
		OpenCodeDir: dir,
	}

	_, _, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate dry-run: %v", err)
	}

	// opencode.json must NOT exist in dir (DryRun = no writes).
	if _, statErr := os.Stat(filepath.Join(dir, "opencode.json")); statErr == nil {
		t.Error("DryRun=true must not write opencode.json")
	}
}

func TestGenerate_OpenCodeOutputPath(t *testing.T) {
	dir := t.TempDir()
	opts := GenerateOpts{
		DryRun:      true,
		Harness:     "opencode",
		OpenCodeDir: dir,
	}

	outputs, _, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	want := filepath.Join(dir, "opencode.json")
	if outputs[0].Path != want {
		t.Errorf("opencode path = %q, want %q", outputs[0].Path, want)
	}
}

func TestGenerate_UserOverrideApplied(t *testing.T) {
	// Write a user override into a temp HOME, then call Generate with HOME pointing there.
	// Codex generator uses the "openai" provider, so we override that.
	dir := t.TempDir()
	overrideDir := filepath.Join(dir, ".config", "engram")
	if err := os.MkdirAll(overrideDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	overrideContent := `
tiers:
  lookup:
    description: "Overridden"
    providers:
      openai:
        model: o4-mini-custom-override
        reasoning_effort: low
`
	overridePath := filepath.Join(overrideDir, "harness-effort.yaml")
	if err := os.WriteFile(overridePath, []byte(overrideContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Redirect HOME so loadOverrideFile picks up the file.
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	opts := GenerateOpts{
		DryRun:      true,
		Harness:     "codex",
		OpenCodeDir: dir,
	}

	outputs, _, err := Generate(opts)
	if err != nil {
		t.Fatalf("Generate with override: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	content := string(outputs[0].Content)
	if !strings.Contains(content, "o4-mini-custom-override") {
		t.Errorf("expected override model in codex output, got:\n%s", content)
	}
}
