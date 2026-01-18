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

	if config.DefaultAgent != "claude" {
		t.Errorf("Expected default agent 'claude', got '%s'", config.DefaultAgent)
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

	if config.DefaultAgent != "claude" {
		t.Errorf("Expected default agent when no file exists, got '%s'", config.DefaultAgent)
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
default_agent: gemini
preferences:
  - keywords: [creative, design]
    agent: gemini
  - keywords: [code, debug]
    agent: claude
`
	os.WriteFile("AGENTS.md", []byte(content), 0644)

	config := LoadConfig()

	if config.DefaultAgent != "gemini" {
		t.Errorf("Expected default agent 'gemini', got '%s'", config.DefaultAgent)
	}

	if len(config.Preferences) != 2 {
		t.Fatalf("Expected 2 preferences, got %d", len(config.Preferences))
	}

	if config.Preferences[0].Agent != "gemini" {
		t.Errorf("Expected first preference agent 'gemini', got '%s'", config.Preferences[0].Agent)
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
default_agent: claude
preferences:
  - keywords [creative]  # Missing colon
    agent: gemini
`
	os.WriteFile("AGENTS.md", []byte(content), 0644)

	config := LoadConfig()

	// Should fallback to defaults with warning
	if config.DefaultAgent != "claude" {
		t.Errorf("Expected default agent 'claude' after parse error, got '%s'", config.DefaultAgent)
	}

	if len(config.Preferences) != 0 {
		t.Errorf("Expected empty preferences after parse error, got %d items", len(config.Preferences))
	}
}

func TestLoadConfig_MissingDefaultAgent(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	// Create YAML without default_agent
	content := `
preferences:
  - keywords: [creative]
    agent: gemini
`
	os.WriteFile("AGENTS.md", []byte(content), 0644)

	config := LoadConfig()

	// Should use system default
	if config.DefaultAgent != "claude" {
		t.Errorf("Expected system default 'claude' when field missing, got '%s'", config.DefaultAgent)
	}
}

func TestLoadConfig_InvalidPreferences(t *testing.T) {
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldDir)

	// Create YAML with invalid preferences
	content := `
default_agent: claude
preferences:
  - keywords: []  # Empty keywords (invalid)
    agent: gemini
  - keywords: [code]
    agent: ""  # Empty agent (invalid)
  - keywords: [design]
    agent: gemini  # Valid
`
	os.WriteFile("AGENTS.md", []byte(content), 0644)

	config := LoadConfig()

	// Should skip invalid preferences
	if len(config.Preferences) != 1 {
		t.Errorf("Expected 1 valid preference (2 skipped), got %d", len(config.Preferences))
	}

	if config.Preferences[0].Agent != "gemini" {
		t.Errorf("Expected valid preference agent 'gemini', got '%s'", config.Preferences[0].Agent)
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

	globalContent := `default_agent: gpt4`
	os.WriteFile(globalPath, []byte(globalContent), 0644)

	// Create local config (should take precedence)
	localContent := `default_agent: gemini`
	os.WriteFile("AGENTS.md", []byte(localContent), 0644)

	config := LoadConfig()

	// Local config should win
	if config.DefaultAgent != "gemini" {
		t.Errorf("Expected local config to take precedence, got '%s'", config.DefaultAgent)
	}
}
