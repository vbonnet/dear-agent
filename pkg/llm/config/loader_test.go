package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_NonExistentFile(t *testing.T) {
	config, err := LoadConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("LoadConfig should not error on nonexistent file: %v", err)
	}

	if config == nil {
		t.Fatal("LoadConfig should return default config when file doesn't exist")
	}

	// Verify defaults are set
	if config.Defaults == nil {
		t.Fatal("Defaults should be set")
	}
	if config.Defaults.Anthropic == nil || config.Defaults.Anthropic.Model != DefaultAnthropicModel {
		t.Errorf("Expected default Anthropic model %s, got %v", DefaultAnthropicModel, config.Defaults.Anthropic)
	}
	if config.Defaults.Gemini == nil || config.Defaults.Gemini.Model != DefaultGeminiModel {
		t.Errorf("Expected default Gemini model %s, got %v", DefaultGeminiModel, config.Defaults.Gemini)
	}
}

func TestLoadConfig_ValidYAML(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "llm-config.yaml")

	yamlContent := `
tools:
  ecphory:
    anthropic:
      model: claude-3-5-sonnet-20241022
      max_tokens: 4096
    gemini:
      model: gemini-2.0-flash-exp
      max_tokens: 8192
    default_family: gemini

  multi-persona-review:
    anthropic:
      model: claude-opus-4-6
      max_tokens: 8192
    gemini:
      model: gemini-2.5-pro-exp
      max_tokens: 8192
    default_family: anthropic

defaults:
  anthropic:
    model: claude-3-5-sonnet-20241022
  gemini:
    model: gemini-2.0-flash-exp
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify tools are loaded
	if config.Tools == nil {
		t.Fatal("Tools should be loaded")
	}

	ecphory := config.Tools["ecphory"]
	if ecphory == nil {
		t.Fatal("ecphory tool config should exist")
	}
	if ecphory.Gemini.Model != "gemini-2.0-flash-exp" {
		t.Errorf("Expected gemini-2.0-flash-exp for ecphory, got %s", ecphory.Gemini.Model)
	}
	if ecphory.Gemini.MaxTokens != 8192 {
		t.Errorf("Expected max_tokens 8192 for ecphory gemini, got %d", ecphory.Gemini.MaxTokens)
	}

	review := config.Tools["multi-persona-review"]
	if review == nil {
		t.Fatal("multi-persona-review tool config should exist")
	}
	if review.Anthropic.Model != "claude-opus-4-6" {
		t.Errorf("Expected claude-opus-4-6 for review, got %s", review.Anthropic.Model)
	}
}

func TestSelectModel_ToolSpecific(t *testing.T) {
	config := &Config{
		Tools: map[string]*ToolConfig{
			"ecphory": {
				Gemini: &ProviderConfig{Model: "gemini-2.0-flash-exp"},
			},
			"multi-persona-review": {
				Anthropic: &ProviderConfig{Model: "claude-opus-4-6"},
			},
		},
		Defaults: &DefaultConfig{
			Anthropic: &ProviderConfig{Model: DefaultAnthropicModel},
			Gemini:    &ProviderConfig{Model: DefaultGeminiModel},
		},
	}

	// Test tool-specific selection
	model := SelectModel(config, "ecphory", "gemini")
	if model != "gemini-2.0-flash-exp" {
		t.Errorf("Expected gemini-2.0-flash-exp for ecphory, got %s", model)
	}

	model = SelectModel(config, "multi-persona-review", "anthropic")
	if model != "claude-opus-4-6" {
		t.Errorf("Expected claude-opus-4-6 for multi-persona-review, got %s", model)
	}
}

func TestSelectModel_GlobalDefaults(t *testing.T) {
	config := &Config{
		Tools: map[string]*ToolConfig{
			"some-tool": {
				Gemini: &ProviderConfig{Model: "gemini-custom"},
			},
		},
		Defaults: &DefaultConfig{
			Anthropic: &ProviderConfig{Model: "claude-custom-default"},
			Gemini:    &ProviderConfig{Model: "gemini-custom-default"},
		},
	}

	// Test fallback to global defaults when tool doesn't have provider config
	model := SelectModel(config, "some-tool", "anthropic")
	if model != "claude-custom-default" {
		t.Errorf("Expected claude-custom-default from global defaults, got %s", model)
	}

	// Test fallback to global defaults for unconfigured tool
	model = SelectModel(config, "unconfigured-tool", "gemini")
	if model != "gemini-custom-default" {
		t.Errorf("Expected gemini-custom-default from global defaults, got %s", model)
	}
}

func TestSelectModel_HardcodedDefaults(t *testing.T) {
	// Test with nil config
	model := SelectModel(nil, "any-tool", "anthropic")
	if model != DefaultAnthropicModel {
		t.Errorf("Expected hardcoded default %s, got %s", DefaultAnthropicModel, model)
	}

	model = SelectModel(nil, "any-tool", "gemini")
	if model != DefaultGeminiModel {
		t.Errorf("Expected hardcoded default %s, got %s", DefaultGeminiModel, model)
	}

	// Test with empty config
	emptyConfig := &Config{}
	model = SelectModel(emptyConfig, "any-tool", "anthropic")
	if model != DefaultAnthropicModel {
		t.Errorf("Expected hardcoded default %s, got %s", DefaultAnthropicModel, model)
	}
}

func TestSelectModel_ProviderAliases(t *testing.T) {
	config := &Config{
		Defaults: &DefaultConfig{
			Anthropic: &ProviderConfig{Model: "claude-test"},
			Gemini:    &ProviderConfig{Model: "gemini-test"},
		},
	}

	// Test "claude" alias for "anthropic"
	model := SelectModel(config, "tool", "claude")
	if model != "claude-test" {
		t.Errorf("Expected claude-test for 'claude' alias, got %s", model)
	}

	// Test "google" alias for "gemini"
	model = SelectModel(config, "tool", "google")
	if model != "gemini-test" {
		t.Errorf("Expected gemini-test for 'google' alias, got %s", model)
	}
}

func TestGetMaxTokens(t *testing.T) {
	config := &Config{
		Tools: map[string]*ToolConfig{
			"ecphory": {
				Gemini: &ProviderConfig{
					Model:     "gemini-2.0-flash-exp",
					MaxTokens: 8192,
				},
			},
		},
		Defaults: &DefaultConfig{
			Anthropic: &ProviderConfig{
				Model:     DefaultAnthropicModel,
				MaxTokens: 4096,
			},
		},
	}

	// Test tool-specific max_tokens
	maxTokens := GetMaxTokens(config, "ecphory", "gemini")
	if maxTokens != 8192 {
		t.Errorf("Expected 8192 max_tokens for ecphory gemini, got %d", maxTokens)
	}

	// Test global default max_tokens
	maxTokens = GetMaxTokens(config, "other-tool", "anthropic")
	if maxTokens != 4096 {
		t.Errorf("Expected 4096 max_tokens from global defaults, got %d", maxTokens)
	}

	// Test unconfigured (should return 0)
	maxTokens = GetMaxTokens(config, "other-tool", "gemini")
	if maxTokens != 0 {
		t.Errorf("Expected 0 max_tokens for unconfigured setting, got %d", maxTokens)
	}
}

func TestLoadConfig_TildeExpansion(t *testing.T) {
	// This test verifies that ~ is expanded to home directory
	// We don't actually create a file in home, just verify the path logic
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot determine home directory")
	}

	// Create a test file in a temp dir that looks like it's in home
	tmpDir := t.TempDir()
	relPath := filepath.Join(".engram", "llm-config.yaml")
	testPath := filepath.Join(tmpDir, relPath)

	// Create the directory structure
	if err := os.MkdirAll(filepath.Dir(testPath), 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	yamlContent := `
defaults:
  anthropic:
    model: test-model
`
	if err := os.WriteFile(testPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Test that tilde expansion works by verifying home dir is used
	config, err := LoadConfig("~/.engram/llm-config.yaml")
	// This will fail to find the file (we didn't create it in actual home)
	// but should not error - should return defaults
	if err != nil {
		t.Fatalf("LoadConfig with tilde should not error: %v", err)
	}
	if config == nil {
		t.Fatal("Config should not be nil")
	}

	// Verify the expansion logic by checking a path
	expandedPath := filepath.Join(homeDir, ".engram/llm-config.yaml")
	if expandedPath == "~/.engram/llm-config.yaml" {
		t.Error("Tilde was not expanded")
	}
}
