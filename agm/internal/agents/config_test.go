package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.SchemaVersion != "1.0" {
		t.Errorf("Expected schema version '1.0', got '%s'", config.SchemaVersion)
	}

	if config.DefaultHarness != "claude-code" {
		t.Errorf("Expected default harness 'claude-code', got '%s'", config.DefaultHarness)
	}

	if len(config.Preferences) != 0 {
		t.Errorf("Expected empty preferences, got %d items", len(config.Preferences))
	}
}

func TestLoadConfig_NoFile(t *testing.T) {
	// Change to a temp directory where no AGENTS.md exists
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	config := LoadConfig()

	if config.DefaultHarness != "claude-code" {
		t.Errorf("Expected default harness when no file exists, got '%s'", config.DefaultHarness)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	// Create valid AGENTS.md
	content := `
schema_version: "1.0"
default_harness: gemini
preferences:
  - keywords: [creative, design]
    harness: gemini
  - keywords: [code, debug]
    harness: claude
`
	os.WriteFile("AGENTS.md", []byte(content), 0644)

	config := LoadConfig()

	if config.DefaultHarness != "gemini" {
		t.Errorf("Expected default harness 'gemini', got '%s'", config.DefaultHarness)
	}

	if len(config.Preferences) != 2 {
		t.Fatalf("Expected 2 preferences, got %d", len(config.Preferences))
	}

	if config.Preferences[0].Harness != "gemini" {
		t.Errorf("Expected first preference harness 'gemini', got '%s'", config.Preferences[0].Harness)
	}

	if len(config.Preferences[0].Keywords) != 2 {
		t.Errorf("Expected 2 keywords in first preference, got %d", len(config.Preferences[0].Keywords))
	}
}

func TestLoadConfig_MalformedYAML(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	// Create malformed YAML
	content := `
default_harness: claude
preferences:
  - keywords [creative]  # Missing colon
    harness: gemini
`
	os.WriteFile("AGENTS.md", []byte(content), 0644)

	config := LoadConfig()

	// Should fallback to defaults with warning
	if config.DefaultHarness != "claude-code" {
		t.Errorf("Expected default harness 'claude-code' after parse error, got '%s'", config.DefaultHarness)
	}

	if len(config.Preferences) != 0 {
		t.Errorf("Expected empty preferences after parse error, got %d items", len(config.Preferences))
	}
}

func TestLoadConfig_MissingDefaultHarness(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	// Create YAML without default_harness
	content := `
preferences:
  - keywords: [creative]
    harness: gemini
`
	os.WriteFile("AGENTS.md", []byte(content), 0644)

	config := LoadConfig()

	// Should use system default
	if config.DefaultHarness != "claude-code" {
		t.Errorf("Expected system default 'claude-code' when field missing, got '%s'", config.DefaultHarness)
	}
}

func TestLoadConfig_InvalidPreferences(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	// Create YAML with invalid preferences
	content := `
default_harness: claude
preferences:
  - keywords: []  # Empty keywords (invalid)
    harness: gemini
  - keywords: [code]
    harness: ""  # Empty agent (invalid)
  - keywords: [design]
    harness: gemini  # Valid
`
	os.WriteFile("AGENTS.md", []byte(content), 0644)

	config := LoadConfig()

	// Should skip invalid preferences
	if len(config.Preferences) != 1 {
		t.Errorf("Expected 1 valid preference (2 skipped), got %d", len(config.Preferences))
	}

	if config.Preferences[0].Harness != "gemini" {
		t.Errorf("Expected valid preference harness 'gemini', got '%s'", config.Preferences[0].Harness)
	}
}

func TestLoadConfig_MultiPath(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	// Create global config
	homeDir, _ := os.UserHomeDir()
	globalDir := filepath.Join(homeDir, ".config", "agm")
	os.MkdirAll(globalDir, 0755)
	globalPath := filepath.Join(globalDir, "AGENTS.md")
	defer os.Remove(globalPath)

	globalContent := `default_harness: gpt4`
	os.WriteFile(globalPath, []byte(globalContent), 0644)

	// Create local config (should take precedence)
	localContent := `default_harness: gemini`
	os.WriteFile("AGENTS.md", []byte(localContent), 0644)

	config := LoadConfig()

	// Local config should win
	if config.DefaultHarness != "gemini" {
		t.Errorf("Expected local config to take precedence, got '%s'", config.DefaultHarness)
	}
}
