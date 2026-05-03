package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDetectWorkspacePriority verifies the workspace detection priority algorithm
// Priority 1: $WORKSPACE env var
// Priority 2: AGM session query
// Priority 3: Path pattern detection
func TestDetectWorkspacePriority(t *testing.T) {
	// Save original env var and restore after test
	originalWorkspace := os.Getenv("WORKSPACE")
	defer func() {
		if originalWorkspace != "" {
			t.Setenv("WORKSPACE", originalWorkspace)
		} else {
			os.Unsetenv("WORKSPACE")
		}
	}()

	t.Run("Priority1_EnvironmentVariable", func(t *testing.T) {
		// Set $WORKSPACE env var
		t.Setenv("WORKSPACE", "env-workspace")
		defer os.Unsetenv("WORKSPACE")

		// Should return env var value even with path that suggests different workspace
		testPath := "/tmp/different-workspace/wf/project"
		workspace := DetectWorkspace(testPath)

		if workspace != "env-workspace" {
			t.Errorf("Expected workspace 'env-workspace' from env var, got '%s'", workspace)
		}
	})

	t.Run("Priority2_AGMSessionQuery", func(t *testing.T) {
		// Unset env var so AGM query takes precedence
		os.Unsetenv("WORKSPACE")

		// Create temporary AGM session structure
		tmpDir := t.TempDir()
		agmDir := filepath.Join(tmpDir, ".agm")
		sessionDir := filepath.Join(agmDir, "test-session")
		os.MkdirAll(sessionDir, 0755)

		// Create manifest with workspace field
		manifest := map[string]interface{}{
			"workspace":  "agm-workspace",
			"session_id": "test-session",
		}
		manifestData, _ := json.Marshal(manifest)
		manifestFile := filepath.Join(sessionDir, "manifest.json")
		os.WriteFile(manifestFile, manifestData, 0644)

		// Create current-session symlink
		currentSessionLink := filepath.Join(agmDir, "current-session")
		os.Symlink(sessionDir, currentSessionLink)

		// Temporarily set HOME to tmpDir for this test
		t.Setenv("HOME", tmpDir)

		// Should return AGM workspace
		testPath := "/tmp/different-workspace/wf/project"
		workspace := DetectWorkspace(testPath)

		if workspace != "agm-workspace" {
			t.Errorf("Expected workspace 'agm-workspace' from AGM, got '%s'", workspace)
		}
	})

	t.Run("Priority3_PathDetection", func(t *testing.T) {
		// Unset env var
		os.Unsetenv("WORKSPACE")

		// Use path with workspace pattern
		testPath := "/tmp/test/oss/wf/project"
		workspace := DetectWorkspace(testPath)

		if workspace != "oss" {
			t.Errorf("Expected workspace 'oss' from path, got '%s'", workspace)
		}
	})

	t.Run("PriorityOrder_EnvOverridesAGM", func(t *testing.T) {
		// Set up both env var and AGM manifest
		t.Setenv("WORKSPACE", "env-wins")
		defer os.Unsetenv("WORKSPACE")

		// Create AGM manifest with different workspace
		tmpDir := t.TempDir()
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

		t.Setenv("HOME", tmpDir)

		// Env var should win
		testPath := "/tmp/path-loses/wf/project"
		workspace := DetectWorkspace(testPath)

		if workspace != "env-wins" {
			t.Errorf("Expected env var 'env-wins' to override AGM, got '%s'", workspace)
		}
	})

	t.Run("PriorityOrder_AGMOverridesPath", func(t *testing.T) {
		// Unset env var so AGM takes precedence
		os.Unsetenv("WORKSPACE")

		// Create AGM manifest
		tmpDir := t.TempDir()
		agmDir := filepath.Join(tmpDir, ".agm")
		sessionDir := filepath.Join(agmDir, "test-session")
		os.MkdirAll(sessionDir, 0755)

		manifest := map[string]interface{}{
			"workspace":  "agm-wins",
			"session_id": "test-session",
		}
		manifestData, _ := json.Marshal(manifest)
		manifestFile := filepath.Join(sessionDir, "manifest.json")
		os.WriteFile(manifestFile, manifestData, 0644)

		currentSessionLink := filepath.Join(agmDir, "current-session")
		os.Symlink(sessionDir, currentSessionLink)

		t.Setenv("HOME", tmpDir)

		// AGM should win over path
		testPath := "/tmp/path-loses/wf/project"
		workspace := DetectWorkspace(testPath)

		if workspace != "agm-wins" {
			t.Errorf("Expected AGM 'agm-wins' to override path, got '%s'", workspace)
		}
	})

	t.Run("Fallback_NoWorkspaceDetected", func(t *testing.T) {
		// Unset env var
		os.Unsetenv("WORKSPACE")

		// Use invalid path (no workspace pattern)
		testPath := "/tmp/invalid/project"
		workspace := DetectWorkspace(testPath)

		if workspace != "" {
			t.Errorf("Expected empty workspace for invalid path, got '%s'", workspace)
		}
	})

	t.Run("InvalidWorkspaceName_Rejected", func(t *testing.T) {
		// Set invalid workspace name in env var
		t.Setenv("WORKSPACE", "invalid workspace")
		defer os.Unsetenv("WORKSPACE")

		testPath := "/tmp/test/wf/project"
		workspace := DetectWorkspace(testPath)

		// Should fall back to path detection since env var is invalid
		if workspace == "invalid workspace" {
			t.Error("Invalid workspace name should be rejected")
		}
	})
}

// TestQueryAGMWorkspace tests the AGM workspace query function directly
func TestQueryAGMWorkspace(t *testing.T) {
	// Save original HOME

	t.Run("SuccessfulQuery_JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// Create AGM session with JSON manifest
		agmDir := filepath.Join(tmpDir, ".agm")
		sessionDir := filepath.Join(agmDir, "test-session")
		os.MkdirAll(sessionDir, 0755)

		manifest := map[string]interface{}{
			"workspace":  "test-workspace",
			"session_id": "test-session",
		}
		manifestData, _ := json.Marshal(manifest)
		manifestFile := filepath.Join(sessionDir, "manifest.json")
		os.WriteFile(manifestFile, manifestData, 0644)

		currentSessionLink := filepath.Join(agmDir, "current-session")
		os.Symlink(sessionDir, currentSessionLink)

		workspace := queryAGMWorkspace()
		if workspace != "test-workspace" {
			t.Errorf("Expected 'test-workspace', got '%s'", workspace)
		}
	})

	t.Run("NoCurrentSession", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// No .agm directory, should return empty
		workspace := queryAGMWorkspace()
		if workspace != "" {
			t.Errorf("Expected empty workspace when no AGM session, got '%s'", workspace)
		}
	})

	t.Run("ManifestMissingWorkspaceField", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		agmDir := filepath.Join(tmpDir, ".agm")
		sessionDir := filepath.Join(agmDir, "test-session")
		os.MkdirAll(sessionDir, 0755)

		// Manifest without workspace field
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
			t.Errorf("Expected empty workspace when manifest has no workspace field, got '%s'", workspace)
		}
	})

	t.Run("FallbackToClaudeDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// Create session in ~/.claude instead of ~/.agm
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
			t.Errorf("Expected 'claude-workspace' from ~/.claude, got '%s'", workspace)
		}
	})
}

// TestDetectWorkspaceFromPath tests the path detection fallback
func TestDetectWorkspaceFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "ProductionPath_OSS",
			path:     "/tmp/test-ws/oss/wf/test-project",
			expected: "oss",
		},
		{
			name:     "ProductionPath_Acme",
			path:     "/tmp/test-ws/acme/wf/test-project",
			expected: "acme",
		},
		{
			name:     "TestPath_OSS",
			path:     "/tmp/test-run/oss/wf/test-project",
			expected: "oss",
		},
		{
			name:     "TestPath_Acme",
			path:     "/tmp/test-run/acme/wf/test-project",
			expected: "acme",
		},
		{
			name:     "InvalidPath_NoWorkspace",
			path:     "/tmp/invalid/project",
			expected: "",
		},
		{
			name:     "InvalidPath_NoWf",
			path:     "/tmp/test-ws/oss/project",
			expected: "", // No /ws/ or /wf/ path component to match
		},
		{
			name:     "CustomWorkspace",
			path:     "/tmp/test-ws/my-custom-workspace/wf/project",
			expected: "my-custom-workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unset env var to test path detection only
			t.Setenv("WORKSPACE", "") // restored on test cleanup
			os.Unsetenv("WORKSPACE")

			workspace := detectWorkspaceFromPath(tt.path)
			if workspace != tt.expected {
				t.Errorf("Expected '%s', got '%s' for path '%s'", tt.expected, workspace, tt.path)
			}
		})
	}
}
