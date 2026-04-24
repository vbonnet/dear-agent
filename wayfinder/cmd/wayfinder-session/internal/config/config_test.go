package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Storage.Mode != ModeCentralized {
		t.Errorf("expected mode=%s, got %s", ModeCentralized, cfg.Storage.Mode)
	}
	if cfg.Storage.Workspace != DefaultWorkspace {
		t.Errorf("expected workspace=%s, got %s", DefaultWorkspace, cfg.Storage.Workspace)
	}
	if cfg.Storage.RelativePath != DefaultRelativePath {
		t.Errorf("expected relative_path=%s, got %s", DefaultRelativePath, cfg.Storage.RelativePath)
	}
	if !cfg.Storage.AutoSymlink {
		t.Error("expected auto_symlink=true")
	}
}

func TestValidate_ValidConfigs(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "dotfile mode",
			config: &Config{
				Storage: StorageConfig{
					Mode:         ModeDotfile,
					RelativePath: "wf",
				},
			},
		},
		{
			name: "centralized mode with workspace",
			config: &Config{
				Storage: StorageConfig{
					Mode:         ModeCentralized,
					Workspace:    "engram-research",
					RelativePath: "wf",
				},
			},
		},
		{
			name: "centralized mode with explicit path",
			config: &Config{
				Storage: StorageConfig{
					Mode:            ModeCentralized,
					CentralizedPath: "/tmp/test/workspace/wf",
					RelativePath:    "wf",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); err != nil {
				t.Errorf("validation failed: %v", err)
			}
		})
	}
}

func TestValidate_InvalidConfigs(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name: "invalid mode",
			config: &Config{
				Storage: StorageConfig{
					Mode: "invalid",
				},
			},
			expectedErr: "invalid storage.mode",
		},
		{
			name: "centralized without workspace or path",
			config: &Config{
				Storage: StorageConfig{
					Mode:         ModeCentralized,
					Workspace:    "",
					RelativePath: "wf",
				},
			},
			expectedErr: "storage.workspace required",
		},
		{
			name: "relative path with traversal",
			config: &Config{
				Storage: StorageConfig{
					Mode:         ModeDotfile,
					RelativePath: "../wf",
				},
			},
			expectedErr: "cannot contain '../'",
		},
		{
			name: "absolute relative path",
			config: &Config{
				Storage: StorageConfig{
					Mode:         ModeCentralized,
					Workspace:    "engram-research",
					RelativePath: "/absolute/path",
				},
			},
			expectedErr: "must be relative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil {
				t.Error("expected validation error, got nil")
				return
			}
			if tt.expectedErr != "" && !contains(err.Error(), tt.expectedErr) {
				t.Errorf("expected error containing '%s', got '%v'", tt.expectedErr, err)
			}
		})
	}
}

func TestGetStoragePath_DotfileMode(t *testing.T) {
	cfg := &Config{
		Storage: StorageConfig{
			Mode: ModeDotfile,
		},
	}

	path, err := cfg.GetStoragePath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".wayfinder")
	if path != expected {
		t.Errorf("expected path=%s, got %s", expected, path)
	}
}

func TestGetStoragePath_CentralizedWithExplicitPath(t *testing.T) {
	testPath := "/tmp/test-workspace/wf"
	cfg := &Config{
		Storage: StorageConfig{
			Mode:            ModeCentralized,
			CentralizedPath: testPath,
		},
	}

	path, err := cfg.GetStoragePath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path != testPath {
		t.Errorf("expected path=%s, got %s", testPath, path)
	}
}

func TestDetectWorkspace_AbsolutePath(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Test with absolute path
	workspace, err := DetectWorkspace(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workspace != tmpDir {
		t.Errorf("expected workspace=%s, got %s", tmpDir, workspace)
	}
}

func TestDetectWorkspace_TestMode(t *testing.T) {
	// Set test mode environment variables
	os.Setenv("ENGRAM_TEST_MODE", "1")
	os.Setenv("ENGRAM_TEST_WORKSPACE", "/tmp/test-workspace")
	defer func() {
		os.Unsetenv("ENGRAM_TEST_MODE")
		os.Unsetenv("ENGRAM_TEST_WORKSPACE")
	}()

	workspace, err := DetectWorkspace("any-name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workspace != "/tmp/test-workspace" {
		t.Errorf("expected test workspace, got %s", workspace)
	}
}

func TestDetectWorkspace_EnvironmentVariable(t *testing.T) {
	// Set environment variable
	os.Setenv("ENGRAM_WORKSPACE", "/custom/workspace")
	defer os.Unsetenv("ENGRAM_WORKSPACE")

	workspace, err := DetectWorkspace("any-name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workspace != "/custom/workspace" {
		t.Errorf("expected custom workspace, got %s", workspace)
	}
}

func TestDetectWorkspace_SearchUpward(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()
	workspaceDir := filepath.Join(tmpDir, "engram-research")
	gitDir := filepath.Join(workspaceDir, ".git")

	// Create .git marker
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git directory: %v", err)
	}

	// Create subdirectory to search from
	subDir := filepath.Join(workspaceDir, "wf", "test-project")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	// Change to subdirectory
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(subDir)

	// Search upward for workspace
	workspace := searchUpwardForWorkspace(subDir, "engram-research")
	if workspace != workspaceDir {
		t.Errorf("expected workspace=%s, got %s", workspaceDir, workspace)
	}
}

func TestHasWorkspaceMarker(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(string)
		expected bool
	}{
		{
			name: ".git directory",
			setup: func(dir string) {
				os.MkdirAll(filepath.Join(dir, ".git"), 0755)
			},
			expected: true,
		},
		{
			name: "WORKSPACE.yaml file",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "WORKSPACE.yaml"), []byte(""), 0644)
			},
			expected: true,
		},
		{
			name:     "no markers",
			setup:    func(dir string) {},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			result := hasWorkspaceMarker(tmpDir)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "tilde expansion",
			path:     "~/test/path",
			expected: filepath.Join(home, "test/path"),
		},
		{
			name:     "no expansion needed",
			path:     "/absolute/path",
			expected: "/absolute/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPath(tt.path)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestLoadAndSave(t *testing.T) {
	// Create temporary directory for config
	tmpHome := t.TempDir()
	configDir := filepath.Join(tmpHome, ".wayfinder")
	configPath := filepath.Join(configDir, "config.yaml")

	// Override home directory for test
	originalHomeVar := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHomeVar)

	// Test 1: Load when config doesn't exist (should return default)
	cfg1, err := Load()
	if err != nil {
		t.Fatalf("unexpected error loading default config: %v", err)
	}
	if cfg1.Storage.Mode != ModeCentralized {
		t.Errorf("expected default mode=centralized, got %s", cfg1.Storage.Mode)
	}

	// Test 2: Save config
	cfg2 := &Config{
		Storage: StorageConfig{
			Mode:         ModeDotfile,
			RelativePath: "wf",
			AutoSymlink:  false,
		},
	}
	if err := cfg2.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}

	// Test 3: Load saved config
	cfg3, err := Load()
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}
	if cfg3.Storage.Mode != ModeDotfile {
		t.Errorf("expected mode=dotfile, got %s", cfg3.Storage.Mode)
	}
	if cfg3.Storage.AutoSymlink {
		t.Error("expected auto_symlink=false, got true")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
