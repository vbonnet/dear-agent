package consolidation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Test getDefaultConfig directly instead of LoadConfig
	// (LoadConfig would search for config files from current directory)
	config := getDefaultConfig()

	if config.ProviderType != "simple" {
		t.Errorf("Default ProviderType = %s, want simple", config.ProviderType)
	}

	if config.Options["storage_path"] == nil {
		t.Error("Default config missing storage_path")
	}
}

func TestLoadConfigFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create test config file
	configContent := `memory:
  provider: simple
  simple:
    storage_path: /test/storage
`
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	config, err := loadConfigFile(configPath)
	if err != nil {
		t.Fatalf("loadConfigFile failed: %v", err)
	}

	if config.ProviderType != "simple" {
		t.Errorf("ProviderType = %s, want simple", config.ProviderType)
	}

	storagePath, ok := config.Options["storage_path"].(string)
	if !ok {
		t.Fatal("storage_path not found or wrong type")
	}

	if storagePath != "/test/storage" {
		t.Errorf("storage_path = %s, want /test/storage", storagePath)
	}
}

func TestLoadConfigFile_InvalidYAML(t *testing.T) {
	tempDir := t.TempDir()

	// Create invalid YAML
	configPath := filepath.Join(tempDir, "bad.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	_, err := loadConfigFile(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestLoadConfigFile_Nonexistent(t *testing.T) {
	_, err := loadConfigFile("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestGetDefaultConfig(t *testing.T) {
	config := getDefaultConfig()

	if config.ProviderType != "simple" {
		t.Errorf("Default ProviderType = %s, want simple", config.ProviderType)
	}

	if config.Options == nil {
		t.Fatal("Default Options is nil")
	}

	if config.Options["storage_path"] == nil {
		t.Error("Default config missing storage_path")
	}
}
