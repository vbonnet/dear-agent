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

func TestLoadConfig_InvalidProjectConfigReturnsError(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(projectDir, ".engram"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".engram", "config.yaml"), []byte("memory: [invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectDir)

	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want malformed project config error")
	}
}

func TestLoadConfig_InvalidGlobalConfigReturnsError(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	globalDir := filepath.Join(homeDir, ".config", "engram")
	if err := os.MkdirAll(globalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte("memory: [invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workDir)

	if _, err := LoadConfig(); err == nil {
		t.Fatal("LoadConfig() error = nil, want malformed global config error")
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
