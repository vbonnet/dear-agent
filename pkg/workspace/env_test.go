package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvManagerLoadSave(t *testing.T) {
	tmpDir := t.TempDir()
	envMgr := NewEnvManager("")

	// Override env file path to use temp dir
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create env vars
	envVars := map[string]string{
		"WORKSPACE_ROOT": "/test/path",
		"OPENAI_API_KEY": "sk-test-key",
		"LOG_LEVEL":      "debug",
	}

	// Save env file
	if err := envMgr.SaveEnvFile("test", envVars); err != nil {
		t.Fatalf("Failed to save env file: %v", err)
	}

	// Load env file
	loaded, err := envMgr.LoadEnvFile("test")
	if err != nil {
		t.Fatalf("Failed to load env file: %v", err)
	}

	// Verify
	if len(loaded) != len(envVars) {
		t.Errorf("Expected %d env vars, got %d", len(envVars), len(loaded))
	}

	for k, v := range envVars {
		if loaded[k] != v {
			t.Errorf("Expected %s=%s, got %s", k, v, loaded[k])
		}
	}
}

func TestGenerateActivationScript(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.yaml")

	// Create registry
	registry := &Registry{
		Version:         1,
		ProtocolVersion: "1.0.0",
		DefaultSettings: map[string]interface{}{
			"log_level": "info",
		},
		Workspaces: []Workspace{
			{
				Name:    "test",
				Root:    tmpDir,
				Enabled: true,
				Settings: map[string]interface{}{
					"log_level": "debug",
				},
			},
		},
	}

	if err := SaveRegistry(registryPath, registry); err != nil {
		t.Fatalf("Failed to save registry: %v", err)
	}

	// Generate activation script
	envMgr := NewEnvManager(registryPath)
	script, err := envMgr.GenerateActivationScript("test")
	if err != nil {
		t.Fatalf("Failed to generate activation script: %v", err)
	}

	// Verify script contains expected variables
	if !strings.Contains(script, "WORKSPACE_ROOT=") {
		t.Error("Script missing WORKSPACE_ROOT")
	}
	if !strings.Contains(script, "WORKSPACE_NAME=") {
		t.Error("Script missing WORKSPACE_NAME")
	}
	if !strings.Contains(script, "Activated workspace: test") {
		t.Error("Script missing activation message")
	}
}

func TestMaskSensitiveValue(t *testing.T) {
	tests := []struct {
		key      string
		value    string
		expected string
	}{
		{"OPENAI_API_KEY", "sk-1234567890", "sk-***"},
		{"PASSWORD", "secret123", "sec***"},
		{"USERNAME", "john", "john"},
		{"API_TOKEN", "token123456", "tok***"},
		{"NORMAL_VAR", "value", "value"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := MaskSensitiveValue(tt.key, tt.value)
			if result != tt.expected {
				t.Errorf("Expected masked value '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
