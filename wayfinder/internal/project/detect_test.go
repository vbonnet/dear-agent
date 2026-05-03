package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDetermineWorkspace tests the workspace detection priority algorithm
func TestDetermineWorkspace(t *testing.T) {
	// Save original env vars
	originalWorkspace := os.Getenv("WORKSPACE")
	originalHome := os.Getenv("HOME")
	defer func() {
		if originalWorkspace != "" {
			t.Setenv("WORKSPACE", originalWorkspace)
		} else {
			os.Unsetenv("WORKSPACE")
		}
		t.Setenv("HOME", originalHome)
	}()

	t.Run("Priority1_EnvironmentVariable", func(t *testing.T) {
		t.Setenv("WORKSPACE", "test-workspace")
		defer os.Unsetenv("WORKSPACE")

		workspace, err := DetermineWorkspace()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if workspace != "test-workspace" {
			t.Errorf("Expected 'test-workspace' from env var, got '%s'", workspace)
		}
	})

	t.Run("Priority2_AGMQuery", func(t *testing.T) {
		os.Unsetenv("WORKSPACE")

		// Create temporary AGM session
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		agmDir := filepath.Join(tmpDir, ".agm")
		sessionDir := filepath.Join(agmDir, "test-session")
		os.MkdirAll(sessionDir, 0755)

		manifest := map[string]interface{}{
			"workspace":  "agm-workspace",
			"session_id": "test-session",
		}
		manifestData, _ := json.Marshal(manifest)
		manifestFile := filepath.Join(sessionDir, "manifest.json")
		os.WriteFile(manifestFile, manifestData, 0644)

		currentSessionLink := filepath.Join(agmDir, "current-session")
		os.Symlink(sessionDir, currentSessionLink)

		workspace, err := DetermineWorkspace()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if workspace != "agm-workspace" {
			t.Errorf("Expected 'agm-workspace' from AGM, got '%s'", workspace)
		}
	})

	t.Run("Priority3_CwdDetection", func(t *testing.T) {
		os.Unsetenv("WORKSPACE")

		// Set HOME to temp dir (no AGM session)
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// Create workspace directory structure
		workspaceDir := filepath.Join(tmpDir, "src", "ws", "cwd-workspace", "wf", "test-project")
		os.MkdirAll(workspaceDir, 0755)

		// Change to workspace directory
		originalCwd, _ := os.Getwd()
		defer os.Chdir(originalCwd)
		os.Chdir(workspaceDir)

		workspace, err := DetermineWorkspace()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if workspace != "cwd-workspace" {
			t.Errorf("Expected 'cwd-workspace' from cwd, got '%s'", workspace)
		}
	})

	t.Run("Priority4_DefaultFallback", func(t *testing.T) {
		os.Unsetenv("WORKSPACE")

		// Set HOME to temp dir (no AGM session, not in workspace directory)
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		workspace, err := DetermineWorkspace()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if workspace != "oss" {
			t.Errorf("Expected default 'oss', got '%s'", workspace)
		}
	})

	t.Run("EnvOverridesAGM", func(t *testing.T) {
		t.Setenv("WORKSPACE", "env-wins")
		defer os.Unsetenv("WORKSPACE")

		// Create AGM session with different workspace
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		agmDir := filepath.Join(tmpDir, ".agm")
		sessionDir := filepath.Join(agmDir, "test-session")
		os.MkdirAll(sessionDir, 0755)

		manifest := map[string]interface{}{
			"workspace":  "agm-loses",
			"session_id": "test-session",
		}
		manifestData, _ := json.Marshal(manifest)
		manifestFile := filepath.Join(sessionDir, "manifest.json")
		os.WriteFile(manifestFile, manifestData, 0644)

		currentSessionLink := filepath.Join(agmDir, "current-session")
		os.Symlink(sessionDir, currentSessionLink)

		workspace, err := DetermineWorkspace()
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if workspace != "env-wins" {
			t.Errorf("Expected env var 'env-wins' to override AGM, got '%s'", workspace)
		}
	})

	t.Run("InvalidEnvVar", func(t *testing.T) {
		t.Setenv("WORKSPACE", "invalid workspace!")
		defer os.Unsetenv("WORKSPACE")

		_, err := DetermineWorkspace()
		if err == nil {
			t.Error("Expected error for invalid workspace name")
		}
	})
}

// TestIsValidWorkspace tests workspace name validation
func TestIsValidWorkspace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Valid_Lowercase", "oss", true},
		{"Valid_Mixed", "Acme", true},
		{"Valid_WithHyphen", "my-workspace", true},
		{"Valid_WithUnderscore", "my_workspace", true},
		{"Valid_WithNumbers", "workspace123", true},
		{"Invalid_Empty", "", false},
		{"Invalid_Spaces", "my workspace", false},
		{"Invalid_Special", "workspace!", false},
		{"Invalid_Slash", "workspace/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidWorkspace(tt.input)
			if result != tt.expected {
				t.Errorf("isValidWorkspace(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestDetectWorkspaceFromCwd tests current directory detection
func TestDetectWorkspaceFromCwd(t *testing.T) {
	// Save original HOME and CWD
	originalHome := os.Getenv("HOME")
	originalCwd, _ := os.Getwd()
	defer func() {
		t.Setenv("HOME", originalHome)
		os.Chdir(originalCwd)
	}()

	tests := []struct {
		name     string
		setupCwd func(tmpDir string) string
		expected string
	}{
		{
			name: "InWorkspaceDir",
			setupCwd: func(tmpDir string) string {
				workspaceDir := filepath.Join(tmpDir, "src", "ws", "test-ws", "wf", "project")
				os.MkdirAll(workspaceDir, 0755)
				return workspaceDir
			},
			expected: "test-ws",
		},
		{
			name: "InWorkspaceRoot",
			setupCwd: func(tmpDir string) string {
				workspaceDir := filepath.Join(tmpDir, "src", "ws", "another-ws")
				os.MkdirAll(workspaceDir, 0755)
				return workspaceDir
			},
			expected: "another-ws",
		},
		{
			name: "OutsideWorkspace",
			setupCwd: func(tmpDir string) string {
				otherDir := filepath.Join(tmpDir, "other", "directory")
				os.MkdirAll(otherDir, 0755)
				return otherDir
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			cwdPath := tt.setupCwd(tmpDir)
			os.Chdir(cwdPath)

			result := detectWorkspaceFromCwd()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestQueryAGMWorkspace tests AGM manifest reading
func TestQueryAGMWorkspace(t *testing.T) {
	// Save original HOME

	t.Run("ValidJSONManifest", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		agmDir := filepath.Join(tmpDir, ".agm")
		sessionDir := filepath.Join(agmDir, "test-session")
		os.MkdirAll(sessionDir, 0755)

		manifest := map[string]interface{}{
			"workspace":  "json-workspace",
			"session_id": "test-session",
		}
		manifestData, _ := json.Marshal(manifest)
		manifestFile := filepath.Join(sessionDir, "manifest.json")
		os.WriteFile(manifestFile, manifestData, 0644)

		currentSessionLink := filepath.Join(agmDir, "current-session")
		os.Symlink(sessionDir, currentSessionLink)

		workspace := queryAGMWorkspace()
		if workspace != "json-workspace" {
			t.Errorf("Expected 'json-workspace', got '%s'", workspace)
		}
	})

	t.Run("NoCurrentSession", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		workspace := queryAGMWorkspace()
		if workspace != "" {
			t.Errorf("Expected empty workspace, got '%s'", workspace)
		}
	})

	t.Run("ManifestWithoutWorkspace", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		agmDir := filepath.Join(tmpDir, ".agm")
		sessionDir := filepath.Join(agmDir, "test-session")
		os.MkdirAll(sessionDir, 0755)

		manifest := map[string]interface{}{
			"session_id": "test-session",
		}
		manifestData, _ := json.Marshal(manifest)
		manifestFile := filepath.Join(sessionDir, "manifest.json")
		os.WriteFile(manifestFile, manifestData, 0644)

		currentSessionLink := filepath.Join(agmDir, "current-session")
		os.Symlink(sessionDir, currentSessionLink)

		workspace := queryAGMWorkspace()
		if workspace != "" {
			t.Errorf("Expected empty workspace, got '%s'", workspace)
		}
	})

	t.Run("FallbackToClaudeDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		claudeDir := filepath.Join(tmpDir, ".claude")
		sessionDir := filepath.Join(claudeDir, "test-session")
		os.MkdirAll(sessionDir, 0755)

		manifest := map[string]interface{}{
			"workspace":  "claude-workspace",
			"session_id": "test-session",
		}
		manifestData, _ := json.Marshal(manifest)
		manifestFile := filepath.Join(sessionDir, "manifest.json")
		os.WriteFile(manifestFile, manifestData, 0644)

		currentSessionLink := filepath.Join(claudeDir, "current-session")
		os.Symlink(sessionDir, currentSessionLink)

		workspace := queryAGMWorkspace()
		if workspace != "claude-workspace" {
			t.Errorf("Expected 'claude-workspace', got '%s'", workspace)
		}
	})
}

// TestGenerateID tests project ID generation
func TestGenerateID(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected string
	}{
		{
			name:     "Simple",
			prompt:   "Build a REST API",
			expected: "build-a-rest-api",
		},
		{
			name:     "WithSpecialChars",
			prompt:   "Fix bug: user can't login!",
			expected: "fix-bug-user-can-t-login",
		},
		{
			name:     "VeryLong",
			prompt:   "This is a very long project description that needs to be truncated because it exceeds the maximum length",
			expected: "this-is-a-very-long-project-description-that-needs",
		},
		{
			name:     "MultipleSpaces",
			prompt:   "Build    API    with   spaces",
			expected: "build-api-with-spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateID(tt.prompt)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestValidatePath tests project path validation
func TestValidatePath(t *testing.T) {
	home := os.Getenv("HOME")

	tests := []struct {
		name      string
		path      string
		workspace string
		expected  bool
	}{
		{
			name:      "ValidPath_OSS",
			path:      filepath.Join(home, "src", "ws", "oss", "wf", "test-project"),
			workspace: "oss",
			expected:  true,
		},
		{
			name:      "ValidPath_Acme",
			path:      filepath.Join(home, "src", "ws", "acme", "wf", "test-project"),
			workspace: "acme",
			expected:  true,
		},
		{
			name:      "InvalidPath_WrongWorkspace",
			path:      filepath.Join(home, "src", "ws", "oss", "wf", "test-project"),
			workspace: "acme",
			expected:  false,
		},
		{
			name:      "InvalidPath_NotInWs",
			path:      filepath.Join(home, "other", "path"),
			workspace: "oss",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidatePath(tt.path, tt.workspace)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for path %s with workspace %s",
					tt.expected, result, tt.path, tt.workspace)
			}
		})
	}
}
