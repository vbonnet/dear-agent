package health

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckCoreEngramsAccessible(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T, workspace string)
		expectedStatus string
		expectedMsg    string
	}{
		{
			name: "symlink_missing",
			setup: func(t *testing.T, workspace string) {
				// Don't create core symlink
			},
			expectedStatus: "error",
			expectedMsg:    "Core symlink missing",
		},
		{
			name: "broken_symlink",
			setup: func(t *testing.T, workspace string) {
				createSymlink(t, "/nonexistent/path", filepath.Join(workspace, "core"))
			},
			expectedStatus: "error",
			expectedMsg:    "Broken symlink",
		},
		{
			name: "not_a_symlink",
			setup: func(t *testing.T, workspace string) {
				createDir(t, filepath.Join(workspace, "core"))
			},
			expectedStatus: "error",
			expectedMsg:    "not a symlink",
		},
		{
			name: "valid_symlink_no_engrams",
			setup: func(t *testing.T, workspace string) {
				targetDir := filepath.Join(workspace, "engram-repo")
				createDir(t, targetDir)
				createDir(t, filepath.Join(targetDir, "engrams"))
				createSymlink(t, targetDir, filepath.Join(workspace, "core"))
			},
			expectedStatus: "warning",
			expectedMsg:    "No .ai.md files found",
		},
		{
			name: "valid_symlink_with_engrams",
			setup: func(t *testing.T, workspace string) {
				targetDir := filepath.Join(workspace, "engram-repo")
				engramsDir := filepath.Join(targetDir, "engrams")
				createDir(t, targetDir)
				createDir(t, engramsDir)
				createEngramFile(t, filepath.Join(engramsDir, "test-pattern.ai.md"))
				createSymlink(t, targetDir, filepath.Join(workspace, "core"))
			},
			expectedStatus: "ok",
			expectedMsg:    "engrams found",
		},
		{
			name: "symlink_target_missing_engrams_dir",
			setup: func(t *testing.T, workspace string) {
				targetDir := filepath.Join(workspace, "engram-repo")
				createDir(t, targetDir)
				createSymlink(t, targetDir, filepath.Join(workspace, "core"))
			},
			expectedStatus: "warning",
			expectedMsg:    "Core engrams directory missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			tt.setup(t, workspace)

			hc := &HealthChecker{workspace: workspace}
			result := hc.checkCoreEngramsAccessible()

			if result.Status != tt.expectedStatus {
				t.Errorf("Expected status %q, got %q", tt.expectedStatus, result.Status)
			}

			assertMessageContains(t, result, tt.expectedMsg)

			if (result.Status == "error" || result.Status == "warning") && result.Fix == "" {
				t.Logf("Warning: Fix field is empty for status %q", result.Status)
			}
		})
	}
}

// equalFold performs case-insensitive string comparison
func equalFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c1, c2 := s[i], t[i]
		// Convert to lowercase
		if c1 >= 'A' && c1 <= 'Z' {
			c1 += 'a' - 'A'
		}
		if c2 >= 'A' && c2 <= 'Z' {
			c2 += 'a' - 'A'
		}
		if c1 != c2 {
			return false
		}
	}
	return true
}

// Test helpers for TestCheckCoreEngramsAccessible

// createDir creates a directory or fails the test
func createDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", path, err)
	}
}

// createSymlink creates a symlink or fails the test
func createSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Failed to create symlink %s → %s: %v", link, target, err)
	}
}

// createEngramFile creates an engram file with test content or fails the test
func createEngramFile(t *testing.T, path string) {
	t.Helper()
	content := []byte("---\ntitle: Test Pattern\n---\nTest content")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("Failed to create engram file %s: %v", path, err)
	}
}

// assertMessageContains checks if result message contains expected substring (case-insensitive)
func assertMessageContains(t *testing.T, result CheckResult, expected string) {
	t.Helper()
	if expected == "" {
		return
	}
	if result.Message == "" {
		t.Errorf("Expected message to contain %q, but message was empty", expected)
		return
	}
	for i := 0; i <= len(result.Message)-len(expected); i++ {
		substr := result.Message[i : i+len(expected)]
		if equalFold(substr, expected) {
			return
		}
	}
	t.Errorf("Expected message to contain %q, got %q", expected, result.Message)
}

// TestCheckHookExtensionMatch tests the hook extension mismatch detection
func TestCheckHookExtensionMatch(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T) (settingsPath string, cleanup func())
		expectedStatus string
		expectedMsg    string
	}{
		{
			name: "no_mismatches",
			setup: func(t *testing.T) (string, func()) {
				tmpDir := t.TempDir()
				hooksDir := filepath.Join(tmpDir, ".claude", "hooks")
				createDir(t, hooksDir)

				// Create hook without extension
				hookPath := filepath.Join(hooksDir, "test-hook")
				createEngramFile(t, hookPath)

				// Create settings referencing the hook (without extension)
				settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
				settings := `{"hooks":{"SessionStart":[{"hooks":[{"command":"` + hookPath + `"}],"matcher":".*"}]}}`
				os.WriteFile(settingsPath, []byte(settings), 0644)

				// Set HOME for test
				oldHome := os.Getenv("HOME")
				t.Setenv("HOME", tmpDir)

				return settingsPath, func() { t.Setenv("HOME", oldHome) }
			},
			expectedStatus: "ok",
			expectedMsg:    "",
		},
		{
			name: "extension_mismatch_py",
			setup: func(t *testing.T) (string, func()) {
				tmpDir := t.TempDir()
				hooksDir := filepath.Join(tmpDir, ".claude", "hooks")
				createDir(t, hooksDir)

				// Create hook binary WITHOUT .py extension
				hookPath := filepath.Join(hooksDir, "test-hook")
				createEngramFile(t, hookPath)

				// Create settings referencing the hook WITH .py extension
				settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
				settings := `{"hooks":{"SessionStart":[{"hooks":[{"command":"` + hookPath + `.py"}],"matcher":".*"}]}}`
				os.WriteFile(settingsPath, []byte(settings), 0644)

				// Set HOME for the test
				oldHome := os.Getenv("HOME")
				t.Setenv("HOME", tmpDir)

				return settingsPath, func() { t.Setenv("HOME", oldHome) }
			},
			expectedStatus: "warning",
			expectedMsg:    "Extension mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingsPath, cleanup := tt.setup(t)
			defer cleanup()

			workspace := filepath.Dir(filepath.Dir(settingsPath))
			hc := &HealthChecker{workspace: workspace}
			result := hc.checkHookExtensionMatch()

			if result.Status != tt.expectedStatus {
				t.Errorf("Expected status %q, got %q", tt.expectedStatus, result.Status)
			}

			if tt.expectedMsg != "" {
				assertMessageContains(t, result, tt.expectedMsg)
			}
		})
	}
}

// TestCheckHookPathsValid tests the hook path validation
func TestCheckHookPathsValid(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T) (settingsPath string, cleanup func())
		expectedStatus string
		expectedMsg    string
	}{
		{
			name: "all_paths_valid",
			setup: func(t *testing.T) (string, func()) {
				tmpDir := t.TempDir()
				hooksDir := filepath.Join(tmpDir, ".claude", "hooks")
				createDir(t, hooksDir)

				// Create hook that exists
				hookPath := filepath.Join(hooksDir, "valid-hook")
				createEngramFile(t, hookPath)

				// Create settings referencing valid hook
				settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
				settings := `{"hooks":{"SessionStart":[{"hooks":[{"command":"` + hookPath + `"}],"matcher":".*"}]}}`
				os.WriteFile(settingsPath, []byte(settings), 0644)

				oldHome := os.Getenv("HOME")
				t.Setenv("HOME", tmpDir)

				return settingsPath, func() { t.Setenv("HOME", oldHome) }
			},
			expectedStatus: "ok",
			expectedMsg:    "",
		},
		{
			name: "path_needs_correction",
			setup: func(t *testing.T) (string, func()) {
				tmpDir := t.TempDir()

				// Create correct hook location
				correctDir := filepath.Join(tmpDir, "src/ws/oss/repos/engram/hooks")
				os.MkdirAll(correctDir, 0755)
				hookPath := filepath.Join(correctDir, "test-hook")
				createEngramFile(t, hookPath)

				// Create settings with WRONG path (using /main/hooks/)
				wrongPath := filepath.Join(tmpDir, "src/ws/oss/repos/engram/main/hooks/test-hook")
				settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
				os.MkdirAll(filepath.Dir(settingsPath), 0755)
				settings := `{"hooks":{"SessionStart":[{"hooks":[{"command":"` + wrongPath + `"}],"matcher":".*"}]}}`
				os.WriteFile(settingsPath, []byte(settings), 0644)

				oldHome := os.Getenv("HOME")
				t.Setenv("HOME", tmpDir)

				return settingsPath, func() { t.Setenv("HOME", oldHome) }
			},
			expectedStatus: "warning",
			expectedMsg:    "Hook paths need correction",
		},
		{
			name: "path_missing_no_correction",
			setup: func(t *testing.T) (string, func()) {
				tmpDir := t.TempDir()
				settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
				os.MkdirAll(filepath.Dir(settingsPath), 0755)

				// Reference hook that doesn't exist anywhere
				missingPath := filepath.Join(tmpDir, ".claude", "hooks", "nonexistent-hook")
				settings := `{"hooks":{"SessionStart":[{"hooks":[{"command":"` + missingPath + `"}],"matcher":".*"}]}}`
				os.WriteFile(settingsPath, []byte(settings), 0644)

				oldHome := os.Getenv("HOME")
				t.Setenv("HOME", tmpDir)

				return settingsPath, func() { t.Setenv("HOME", oldHome) }
			},
			expectedStatus: "warning",
			expectedMsg:    "Hook paths missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settingsPath, cleanup := tt.setup(t)
			defer cleanup()

			workspace := filepath.Dir(filepath.Dir(settingsPath))
			hc := &HealthChecker{workspace: workspace}
			result := hc.checkHookPathsValid()

			if result.Status != tt.expectedStatus {
				t.Errorf("Expected status %q, got %q (message: %s)", tt.expectedStatus, result.Status, result.Message)
			}

			if tt.expectedMsg != "" {
				assertMessageContains(t, result, tt.expectedMsg)
			}
		})
	}
}

// TestCheckMarketplaceConfigValid tests marketplace config validation
func TestCheckMarketplaceConfigValid(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T) (tmpDir string)
		expectedStatus string
		expectedMsg    string
	}{
		{
			name: "no_marketplace_config",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				oldHome := os.Getenv("HOME")
				t.Setenv("HOME", tmpDir)
				t.Cleanup(func() { t.Setenv("HOME", oldHome) })
				return tmpDir
			},
			expectedStatus: "ok",
			expectedMsg:    "No marketplace config",
		},
		{
			name: "invalid_source_directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				pluginsDir := filepath.Join(tmpDir, ".claude", "plugins")
				os.MkdirAll(pluginsDir, 0755)

				// Create marketplace config with source="directory"
				mktPath := filepath.Join(pluginsDir, "known_marketplaces.json")
				invalidConfig := `{"test":{"source":{"source":"directory","path":"/test"}}}`
				os.WriteFile(mktPath, []byte(invalidConfig), 0644)

				oldHome := os.Getenv("HOME")
				t.Setenv("HOME", tmpDir)
				t.Cleanup(func() { t.Setenv("HOME", oldHome) })
				return tmpDir
			},
			expectedStatus: "error",
			expectedMsg:    "Invalid marketplace entries",
		},
		{
			name: "valid_marketplace_config",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				pluginsDir := filepath.Join(tmpDir, ".claude", "plugins")
				os.MkdirAll(pluginsDir, 0755)

				// Create valid marketplace config with proper structure
				mktPath := filepath.Join(pluginsDir, "known_marketplaces.json")
				validConfig := `{
					"anthropic": {
						"source": {
							"source": "github",
							"owner": "anthropics",
							"repo": "plugins"
						},
						"installLocation": "~/.claude/plugins"
					}
				}`
				os.WriteFile(mktPath, []byte(validConfig), 0644)

				oldHome := os.Getenv("HOME")
				t.Setenv("HOME", tmpDir)
				t.Cleanup(func() { t.Setenv("HOME", oldHome) })
				return tmpDir
			},
			expectedStatus: "ok",
			expectedMsg:    "marketplace(s) configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := tt.setup(t)

			hc := &HealthChecker{workspace: tmpDir}
			result := hc.checkMarketplaceConfigValid()

			if result.Status != tt.expectedStatus {
				t.Errorf("Expected status %q, got %q (message: %s)", tt.expectedStatus, result.Status, result.Message)
			}

			if tt.expectedMsg != "" {
				assertMessageContains(t, result, tt.expectedMsg)
			}
		})
	}
}
