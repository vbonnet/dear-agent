package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDefaultPrompts(t *testing.T) {
	defaults := GetDefaultPrompts()

	if defaults.ExtractPrompt == "" {
		t.Error("ExtractPrompt should not be empty")
	}
	if defaults.AnalyzePrompt == "" {
		t.Error("AnalyzePrompt should not be empty")
	}
	if defaults.ResearchPrompt == "" {
		t.Error("ResearchPrompt should not be empty")
	}
}

func TestLoadPrompts_DefaultsOnly(t *testing.T) {
	// No CLI flags, no config files
	prompts, err := LoadPrompts(CLIPrompts{})
	if err != nil {
		t.Fatalf("LoadPrompts() error = %v, want nil", err)
	}

	defaults := GetDefaultPrompts()
	if prompts.ExtractPrompt != defaults.ExtractPrompt {
		t.Errorf("ExtractPrompt mismatch")
	}
	if prompts.AnalyzePrompt != defaults.AnalyzePrompt {
		t.Errorf("AnalyzePrompt mismatch")
	}
	if prompts.ResearchPrompt != defaults.ResearchPrompt {
		t.Errorf("ResearchPrompt mismatch")
	}
}

func TestLoadPrompts_CLIOverridesDefaults(t *testing.T) {
	cli := CLIPrompts{
		ExtractPrompt:  "Custom extract",
		AnalyzePrompt:  "Custom analyze",
		ResearchPrompt: "Custom research",
	}

	prompts, err := LoadPrompts(cli)
	if err != nil {
		t.Fatalf("LoadPrompts() error = %v, want nil", err)
	}

	if prompts.ExtractPrompt != "Custom extract" {
		t.Errorf("ExtractPrompt = %q, want %q", prompts.ExtractPrompt, "Custom extract")
	}
	if prompts.AnalyzePrompt != "Custom analyze" {
		t.Errorf("AnalyzePrompt = %q, want %q", prompts.AnalyzePrompt, "Custom analyze")
	}
	if prompts.ResearchPrompt != "Custom research" {
		t.Errorf("ResearchPrompt = %q, want %q", prompts.ResearchPrompt, "Custom research")
	}
}

func TestLoadPrompts_PartialCLIFlags(t *testing.T) {
	cli := CLIPrompts{
		AnalyzePrompt: "Custom analyze only",
	}

	prompts, err := LoadPrompts(cli)
	if err != nil {
		t.Fatalf("LoadPrompts() error = %v, want nil", err)
	}

	defaults := GetDefaultPrompts()
	if prompts.ExtractPrompt != defaults.ExtractPrompt {
		t.Errorf("ExtractPrompt should use default")
	}
	if prompts.AnalyzePrompt != "Custom analyze only" {
		t.Errorf("AnalyzePrompt = %q, want %q", prompts.AnalyzePrompt, "Custom analyze only")
	}
	if prompts.ResearchPrompt != defaults.ResearchPrompt {
		t.Errorf("ResearchPrompt should use default")
	}
}

func TestLoadConfigFile_ValidYAML(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "prompts.yaml")

	yamlContent := `prompts:
  extract: "Test extract prompt"
  analyze: "Test analyze prompt"
  research: "Test research prompt"
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := loadConfigFile(configPath)
	if err != nil {
		t.Fatalf("loadConfigFile() error = %v, want nil", err)
	}

	if config.Extract != "Test extract prompt" {
		t.Errorf("Extract = %q, want %q", config.Extract, "Test extract prompt")
	}
	if config.Analyze != "Test analyze prompt" {
		t.Errorf("Analyze = %q, want %q", config.Analyze, "Test analyze prompt")
	}
	if config.Research != "Test research prompt" {
		t.Errorf("Research = %q, want %q", config.Research, "Test research prompt")
	}
}

func TestLoadConfigFile_InvalidYAML(t *testing.T) {
	// Create temp config file with invalid YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "prompts.yaml")

	invalidYAML := `prompts:
  extract: "Missing quote
  analyze: valid
`

	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := loadConfigFile(configPath)
	if err == nil {
		t.Error("loadConfigFile() should return error for invalid YAML")
	}
}

func TestLoadConfigFile_FileNotFound(t *testing.T) {
	_, err := loadConfigFile("/nonexistent/path/prompts.yaml")
	if !os.IsNotExist(err) {
		t.Errorf("loadConfigFile() error should be os.ErrNotExist, got %v", err)
	}
}

func TestLoadConfigFile_PartialConfig(t *testing.T) {
	// Create temp config with only analyze prompt
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "prompts.yaml")

	yamlContent := `prompts:
  analyze: "Only analyze prompt"
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := loadConfigFile(configPath)
	if err != nil {
		t.Fatalf("loadConfigFile() error = %v, want nil", err)
	}

	if config.Extract != "" {
		t.Errorf("Extract should be empty, got %q", config.Extract)
	}
	if config.Analyze != "Only analyze prompt" {
		t.Errorf("Analyze = %q, want %q", config.Analyze, "Only analyze prompt")
	}
	if config.Research != "" {
		t.Errorf("Research should be empty, got %q", config.Research)
	}
}

func TestMergePrompts(t *testing.T) {
	result := MergedPrompts{
		ExtractPrompt:  "Default extract",
		AnalyzePrompt:  "Default analyze",
		ResearchPrompt: "Default research",
	}

	config := PromptConfig{
		Analyze:  "Override analyze",
		Research: "Override research",
	}

	mergePrompts(&result, config)

	if result.ExtractPrompt != "Default extract" {
		t.Errorf("ExtractPrompt should not change, got %q", result.ExtractPrompt)
	}
	if result.AnalyzePrompt != "Override analyze" {
		t.Errorf("AnalyzePrompt = %q, want %q", result.AnalyzePrompt, "Override analyze")
	}
	if result.ResearchPrompt != "Override research" {
		t.Errorf("ResearchPrompt = %q, want %q", result.ResearchPrompt, "Override research")
	}
}
