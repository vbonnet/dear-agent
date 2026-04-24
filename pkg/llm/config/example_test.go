package config_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/pkg/llm/config"
)

// Example_basicUsage demonstrates basic configuration loading and model selection.
func Example_basicUsage() {
	// Create a temporary config file for demonstration
	tmpDir, _ := os.MkdirTemp("", "llm-config-example")
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "llm-config.yaml")
	yamlContent := `
tools:
  ecphory:
    gemini:
      model: gemini-2.0-flash-exp
      max_tokens: 8192
    default_family: gemini

defaults:
  anthropic:
    model: claude-3-5-sonnet-20241022
  gemini:
    model: gemini-2.0-flash-exp
`
	os.WriteFile(configPath, []byte(yamlContent), 0644)

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Select model for ecphory with Gemini (tool-specific config exists)
	model := config.SelectModel(cfg, "ecphory", "gemini")
	fmt.Printf("ecphory + gemini: %s\n", model)

	// Select model for unconfigured tool (falls back to defaults)
	model = config.SelectModel(cfg, "other-tool", "anthropic")
	fmt.Printf("other-tool + anthropic: %s\n", model)

	// Get max_tokens
	maxTokens := config.GetMaxTokens(cfg, "ecphory", "gemini")
	fmt.Printf("ecphory gemini max_tokens: %d\n", maxTokens)

	// Output:
	// ecphory + gemini: gemini-2.0-flash-exp
	// other-tool + anthropic: claude-3-5-sonnet-20241022
	// ecphory gemini max_tokens: 8192
}

// Example_fallbackHierarchy demonstrates the three-tier fallback system.
func Example_fallbackHierarchy() {
	tmpDir, _ := os.MkdirTemp("", "llm-config-example")
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "llm-config.yaml")
	yamlContent := `
tools:
  tool-a:
    gemini:
      model: tool-specific-model

defaults:
  gemini:
    model: global-default-model
`
	os.WriteFile(configPath, []byte(yamlContent), 0644)

	cfg, _ := config.LoadConfig(configPath)

	// Tier 1: Tool-specific config exists
	model := config.SelectModel(cfg, "tool-a", "gemini")
	fmt.Printf("Tier 1 (tool-specific): %s\n", model)

	// Tier 2: Fall back to global defaults
	model = config.SelectModel(cfg, "tool-b", "gemini")
	fmt.Printf("Tier 2 (global default): %s\n", model)

	// Tier 3: Fall back to hardcoded defaults (anthropic not in config)
	model = config.SelectModel(cfg, "tool-b", "anthropic")
	fmt.Printf("Tier 3 (hardcoded): %s\n", model)

	// Output:
	// Tier 1 (tool-specific): tool-specific-model
	// Tier 2 (global default): global-default-model
	// Tier 3 (hardcoded): claude-3-5-sonnet-20241022
}

// Example_providerAliases demonstrates provider family aliases.
func Example_providerAliases() {
	cfg, _ := config.LoadConfig("/nonexistent/path")

	// "anthropic" and "claude" are equivalent
	model1 := config.SelectModel(cfg, "tool", "anthropic")
	model2 := config.SelectModel(cfg, "tool", "claude")
	fmt.Printf("anthropic == claude: %v\n", model1 == model2)

	// "gemini" and "google" are equivalent
	model3 := config.SelectModel(cfg, "tool", "gemini")
	model4 := config.SelectModel(cfg, "tool", "google")
	fmt.Printf("gemini == google: %v\n", model3 == model4)

	// Output:
	// anthropic == claude: true
	// gemini == google: true
}

// Example_noConfigFile demonstrates behavior when config file doesn't exist.
func Example_noConfigFile() {
	// Load from nonexistent path - returns defaults without error
	cfg, err := config.LoadConfig("/nonexistent/path/config.yaml")
	fmt.Printf("Error: %v\n", err)
	fmt.Printf("Config is nil: %v\n", cfg == nil)

	// Hardcoded defaults are used
	model := config.SelectModel(cfg, "any-tool", "anthropic")
	fmt.Printf("Default anthropic model: %s\n", model)

	model = config.SelectModel(cfg, "any-tool", "gemini")
	fmt.Printf("Default gemini model: %s\n", model)

	// Output:
	// Error: <nil>
	// Config is nil: false
	// Default anthropic model: claude-3-5-sonnet-20241022
	// Default gemini model: gemini-2.0-flash-exp
}
