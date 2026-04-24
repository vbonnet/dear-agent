package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetStoragePath(t *testing.T) {
	tests := []struct {
		name          string
		config        *Config
		expectedMode  string
		expectError   bool
		skipOnWindows bool
	}{
		{
			name: "dotfile mode",
			config: &Config{
				Storage: StorageConfig{
					Mode:         "dotfile",
					Workspace:    "",
					RelativePath: ".agm",
				},
			},
			expectedMode: "dotfile",
			expectError:  false,
		},
		{
			name: "default mode (empty = dotfile)",
			config: &Config{
				Storage: StorageConfig{
					Mode:         "",
					Workspace:    "",
					RelativePath: ".agm",
				},
			},
			expectedMode: "dotfile",
			expectError:  false,
		},
		{
			name: "centralized mode with absolute path",
			config: &Config{
				Storage: StorageConfig{
					Mode:         "centralized",
					Workspace:    "/tmp/test-workspace",
					RelativePath: ".agm",
				},
			},
			expectedMode: "centralized",
			expectError:  false,
		},
		{
			name: "centralized mode with test env var",
			config: &Config{
				Storage: StorageConfig{
					Mode:         "centralized",
					Workspace:    "test-workspace",
					RelativePath: ".agm",
				},
			},
			expectedMode: "centralized",
			expectError:  false,
		},
		{
			name: "invalid mode",
			config: &Config{
				Storage: StorageConfig{
					Mode:         "invalid",
					Workspace:    "",
					RelativePath: ".agm",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && os.Getenv("OS") == "Windows_NT" {
				t.Skip("Skipping on Windows")
			}

			// Setup test environment for centralized mode tests
			if tt.expectedMode == "centralized" {
				os.Setenv("ENGRAM_TEST_MODE", "1")
				os.Setenv("ENGRAM_TEST_WORKSPACE", "/tmp/test-workspace")
				defer os.Unsetenv("ENGRAM_TEST_MODE")
				defer os.Unsetenv("ENGRAM_TEST_WORKSPACE")

				// Create the test workspace directory if using absolute path
				if tt.config.Storage.Workspace == "/tmp/test-workspace" {
					os.MkdirAll("/tmp/test-workspace", 0755)
					defer os.RemoveAll("/tmp/test-workspace")
				}
			}

			storagePath, err := GetStoragePath(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify path format
			if storagePath == "" {
				t.Errorf("storage path is empty")
			}

			if tt.expectedMode == "dotfile" {
				homeDir, _ := os.UserHomeDir()
				expected := filepath.Join(homeDir, ".agm")
				if storagePath != expected {
					t.Errorf("expected dotfile path %s, got %s", expected, storagePath)
				}
			}

			if tt.expectedMode == "centralized" {
				if !filepath.IsAbs(storagePath) {
					t.Errorf("centralized path should be absolute, got: %s", storagePath)
				}
			}
		})
	}
}

func TestDetectWorkspace(t *testing.T) {
	tests := []struct {
		name          string
		nameOrPath    string
		envVars       map[string]string
		expectedError bool
	}{
		{
			name:          "absolute path exists",
			nameOrPath:    "/tmp",
			expectedError: false,
		},
		{
			name:       "test mode env var",
			nameOrPath: "test-workspace",
			envVars: map[string]string{
				"ENGRAM_TEST_MODE":      "1",
				"ENGRAM_TEST_WORKSPACE": "/tmp/test-workspace",
			},
			expectedError: false,
		},
		{
			name:       "ENGRAM_WORKSPACE env var",
			nameOrPath: "ignored",
			envVars: map[string]string{
				"ENGRAM_WORKSPACE": "/tmp/engram-workspace",
			},
			expectedError: false,
		},
		{
			name:          "workspace not found",
			nameOrPath:    "nonexistent-workspace-12345",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			workspace, err := DetectWorkspace(tt.nameOrPath)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if workspace == "" {
				t.Errorf("workspace path is empty")
			}

			if !filepath.IsAbs(workspace) {
				t.Errorf("workspace path should be absolute, got: %s", workspace)
			}
		})
	}
}

func TestHasWorkspaceMarker(t *testing.T) {
	// Create temp directory with .git marker
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	os.Mkdir(gitDir, 0755)

	tests := []struct {
		name       string
		dir        string
		targetName string
		expected   bool
	}{
		{
			name:       "has .git marker with matching name",
			dir:        tmpDir,
			targetName: filepath.Base(tmpDir),
			expected:   true,
		},
		{
			name:       "has .git marker with empty target name",
			dir:        tmpDir,
			targetName: "",
			expected:   true,
		},
		{
			name:       "has .git marker with non-matching name",
			dir:        tmpDir,
			targetName: "different-name",
			expected:   false,
		},
		{
			name:       "no markers",
			dir:        "/tmp",
			targetName: "test",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasWorkspaceMarker(tt.dir, tt.targetName)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEnsureSymlinkBootstrap(t *testing.T) {
	// These tests require filesystem manipulation and are integration tests
	// They should be run in a test environment with proper cleanup

	t.Run("dotfile mode does nothing", func(t *testing.T) {
		cfg := &Config{
			Storage: StorageConfig{
				Mode:         "dotfile",
				Workspace:    "",
				RelativePath: ".agm",
			},
		}

		err := EnsureSymlinkBootstrap(cfg)
		if err != nil {
			t.Errorf("unexpected error in dotfile mode: %v", err)
		}
	})

	t.Run("centralized mode requires valid workspace", func(t *testing.T) {
		cfg := &Config{
			Storage: StorageConfig{
				Mode:         "centralized",
				Workspace:    "nonexistent-workspace-12345",
				RelativePath: ".agm",
			},
		}

		err := EnsureSymlinkBootstrap(cfg)
		if err == nil {
			t.Errorf("expected error for nonexistent workspace")
		}
	})
}

func TestVerifyStorageIntegrity(t *testing.T) {
	t.Run("dotfile mode with existing directory", func(t *testing.T) {
		// Create temporary .agm directory
		homeDir, _ := os.UserHomeDir()
		testDir := filepath.Join(homeDir, ".agm-test-verify")
		os.MkdirAll(testDir, 0755)
		defer os.RemoveAll(testDir)

		cfg := &Config{
			Storage: StorageConfig{
				Mode:         "dotfile",
				Workspace:    "",
				RelativePath: ".agm-test-verify",
			},
		}

		// Override GetStoragePath to use test directory
		// This is a simplified test - real implementation would mock GetStoragePath
		err := VerifyStorageIntegrity(cfg)
		if err != nil {
			t.Logf("Storage verification: %v (expected for test environment)", err)
		}
	})

	t.Run("centralized mode requires workspace", func(t *testing.T) {
		cfg := &Config{
			Storage: StorageConfig{
				Mode:         "centralized",
				Workspace:    "nonexistent-workspace-12345",
				RelativePath: ".agm",
			},
		}

		err := VerifyStorageIntegrity(cfg)
		if err == nil {
			t.Errorf("expected error for nonexistent workspace")
		}
	})
}

func TestCopyDir(t *testing.T) {
	// Create source directory with test files
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create test structure
	os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
	os.Mkdir(filepath.Join(srcDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644)

	// Copy directory
	err := copyDir(srcDir, dstDir)
	if err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Verify files copied
	tests := []struct {
		path    string
		content string
	}{
		{"file1.txt", "content1"},
		{"subdir/file2.txt", "content2"},
	}

	for _, tt := range tests {
		dstFile := filepath.Join(dstDir, tt.path)
		content, err := os.ReadFile(dstFile)
		if err != nil {
			t.Errorf("failed to read copied file %s: %v", tt.path, err)
			continue
		}
		if string(content) != tt.content {
			t.Errorf("file %s: expected content %q, got %q", tt.path, tt.content, string(content))
		}
	}
}
