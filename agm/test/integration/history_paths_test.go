package integration

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/history"
)

// TestGetHistoryPaths_Integration tests end-to-end path construction
func TestGetHistoryPaths_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name       string
		agent      string
		uuid       string
		workingDir string
		verify     bool
		wantErr    bool
	}{
		{
			name:       "Claude agent with real working directory",
			agent:      "claude",
			uuid:       "test-uuid-1234-5678-9abc-def012345678",
			workingDir: "~/src",
			verify:     false,
			wantErr:    false,
		},
		{
			name:       "Gemini agent with project directory",
			agent:      "gemini",
			uuid:       "ses_test123",
			workingDir: "~/project",
			verify:     false,
			wantErr:    false,
		},
		{
			name:       "OpenCode agent without working directory",
			agent:      "opencode",
			uuid:       "ses_opencode456",
			workingDir: "",
			verify:     false,
			wantErr:    false,
		},
		{
			name:       "Codex agent with date in UUID",
			agent:      "codex",
			uuid:       "rollout-2026-03-18-test789",
			workingDir: "",
			verify:     false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			location, err := history.GetHistoryPaths(tt.agent, tt.uuid, tt.workingDir, tt.verify)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetHistoryPaths() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("GetHistoryPaths() unexpected error: %v", err)
			}

			// Verify location is not nil
			if location == nil {
				t.Fatal("GetHistoryPaths() returned nil location")
			}

			// Verify harness matches
			if location.Harness != tt.agent {
				t.Errorf("location.Harness = %q, want %q", location.Harness, tt.agent)
			}

			// Verify UUID matches
			if location.UUID != tt.uuid {
				t.Errorf("location.UUID = %q, want %q", location.UUID, tt.uuid)
			}

			// Verify paths are returned
			if len(location.Paths) == 0 {
				t.Error("location.Paths is empty, expected at least one path")
			}

			// Verify paths are absolute
			for _, path := range location.Paths {
				if !filepath.IsAbs(path) && !filepath.IsAbs(filepath.Clean(path)) {
					// Allow wildcards
					if filepath.IsAbs(filepath.Dir(path)) {
						continue
					}
					t.Errorf("path %q is not absolute", path)
				}
			}

			// Verify metadata exists
			if location.Metadata == nil {
				t.Error("location.Metadata is nil, expected metadata")
			}

			// Verify harness metadata
			harness, ok := location.Metadata["harness"]
			if !ok {
				t.Error("metadata missing 'harness' key")
			} else if harness != tt.agent {
				t.Errorf("metadata[harness] = %q, want %q", harness, tt.agent)
			}
		})
	}
}

// TestGetHistoryPaths_WithVerification tests path verification
func TestGetHistoryPaths_WithVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	testWorkingDir := filepath.Join(tmpDir, "test", "project")
	if err := os.MkdirAll(testWorkingDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	testUUID := "test-verify-uuid-1234-5678-9abc"

	// Test with verification enabled (paths won't exist)
	location, err := history.GetHistoryPaths("claude", testUUID, testWorkingDir, true)
	if err != nil {
		t.Fatalf("GetHistoryPaths() error = %v", err)
	}

	// Verify that exists is false (since we didn't create the actual files)
	if location.Exists {
		t.Error("location.Exists = true, want false (files don't exist)")
	}
}

// TestGetHistoryPaths_ErrorCases tests error handling
func TestGetHistoryPaths_ErrorCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name        string
		agent       string
		uuid        string
		workingDir  string
		wantErrCode string
	}{
		{
			name:        "empty agent",
			agent:       "",
			uuid:        "test-uuid",
			workingDir:  "~/src",
			wantErrCode: "HARNESS_UNKNOWN",
		},
		{
			name:        "empty UUID",
			agent:       "claude",
			uuid:        "",
			workingDir:  "~/src",
			wantErrCode: "UUID_MISSING",
		},
		{
			name:        "unknown agent type",
			agent:       "invalid",
			uuid:        "test-uuid",
			workingDir:  "~/src",
			wantErrCode: "HARNESS_UNKNOWN",
		},
		{
			name:        "Claude without working directory",
			agent:       "claude",
			uuid:        "test-uuid",
			workingDir:  "",
			wantErrCode: "WORKING_DIR_MISSING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := history.GetHistoryPaths(tt.agent, tt.uuid, tt.workingDir, false)

			if err == nil {
				t.Fatal("GetHistoryPaths() expected error, got nil")
			}

			// Check if error is LocationError
			locErr := &history.LocationError{}
			ok := errors.As(err, &locErr)
			if !ok {
				t.Fatalf("GetHistoryPaths() error type = %T, want *history.LocationError", err)
			}

			if locErr.Code != tt.wantErrCode {
				t.Errorf("error.Code = %q, want %q", locErr.Code, tt.wantErrCode)
			}
		})
	}
}

// TestGetHistoryPaths_CustomOpenCodeDataDir tests OpenCode with custom data directory
func TestGetHistoryPaths_CustomOpenCodeDataDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Set custom OPENCODE_DATA_DIR
	customDir := "/tmp/custom-opencode-test"
	originalEnv := os.Getenv("OPENCODE_DATA_DIR")
	t.Setenv("OPENCODE_DATA_DIR", customDir)
	defer func() {
		if originalEnv != "" {
			t.Setenv("OPENCODE_DATA_DIR", originalEnv)
		} else {
			os.Unsetenv("OPENCODE_DATA_DIR")
		}
	}()

	testUUID := "ses_custom_test"

	location, err := history.GetHistoryPaths("opencode", testUUID, "", false)
	if err != nil {
		t.Fatalf("GetHistoryPaths() error = %v", err)
	}

	// Verify custom directory is used
	if location.Metadata["base_dir"] != customDir {
		t.Errorf("metadata[base_dir] = %q, want %q", location.Metadata["base_dir"], customDir)
	}

	if location.Metadata["env_override"] != "true" {
		t.Errorf("metadata[env_override] = %q, want 'true'", location.Metadata["env_override"])
	}

	// Verify paths use custom directory
	for _, path := range location.Paths {
		relPath, err := filepath.Rel(customDir, path)
		if err != nil || filepath.IsAbs(relPath) {
			t.Errorf("path %q is not under %q", path, customDir)
		}
	}
}
