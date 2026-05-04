package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSessionName(t *testing.T) {
	existing := map[string]bool{
		"existing-session": true,
		"another-one":      true,
	}

	tests := []struct {
		name        string
		input       string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "valid name",
			input:       "my-new-session",
			shouldError: false,
		},
		{
			name:        "valid with underscores",
			input:       "my_new_session",
			shouldError: false,
		},
		{
			name:        "valid with numbers",
			input:       "session-123",
			shouldError: false,
		},
		{
			name:        "valid all caps",
			input:       "MY-SESSION",
			shouldError: false,
		},
		{
			name:        "empty name",
			input:       "",
			shouldError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name:        "whitespace only",
			input:       "   ",
			shouldError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name:        "too short",
			input:       "a",
			shouldError: true,
			errorMsg:    "at least 2 characters",
		},
		{
			name:        "too long",
			input:       "this-is-a-very-long-session-name-that-exceeds-the-maximum-allowed-length-limit",
			shouldError: true,
			errorMsg:    "64 characters or less",
		},
		{
			name:        "invalid characters - spaces",
			input:       "my session",
			shouldError: true,
			errorMsg:    "only letters, numbers, hyphens, and underscores",
		},
		{
			name:        "invalid characters - special chars",
			input:       "my-session!",
			shouldError: true,
			errorMsg:    "only letters, numbers, hyphens, and underscores",
		},
		{
			name:        "invalid characters - dots",
			input:       "my.session",
			shouldError: true,
			errorMsg:    "only letters, numbers, hyphens, and underscores",
		},
		{
			name:        "conflicts with existing",
			input:       "existing-session",
			shouldError: true,
			errorMsg:    "already exists",
		},
		{
			name:        "no conflict - different name",
			input:       "brand-new-session",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionName(tt.input, existing)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestValidateProjectPath(t *testing.T) {
	// Create temp directory for testing
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(tempFile, []byte("test"), 0644)

	tests := []struct {
		name        string
		input       string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "valid existing directory",
			input:       tempDir,
			shouldError: false,
		},
		{
			name:        "valid with tilde expansion",
			input:       "~/",
			shouldError: false,
		},
		{
			name:        "empty path",
			input:       "",
			shouldError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name:        "whitespace only",
			input:       "   ",
			shouldError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name:        "nonexistent path",
			input:       "/tmp/this-directory-does-not-exist-12345",
			shouldError: true,
			errorMsg:    "does not exist",
		},
		{
			name:        "file instead of directory",
			input:       tempFile,
			shouldError: true,
			errorMsg:    "must be a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectPath(tt.input)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde expansion",
			input:    "~/test",
			expected: filepath.Join(home, "test"),
		},
		{
			name:     "tilde only",
			input:    "~/",
			expected: home,
		},
		{
			name:     "absolute path unchanged",
			input:    "/tmp/test",
			expected: "/tmp/test",
		},
		{
			name:     "relative path to absolute",
			input:    ".",
			expected: cwd,
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace trimmed",
			input:    "  ~/test  ",
			expected: filepath.Join(home, "test"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPath(tt.input)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestNewSessionForm_ValidatesExistingSessions(t *testing.T) {
	// This test verifies the validation integration
	// We can't test the full form interactively without a TTY,
	// but we can verify the validation logic is correctly wired

	existingNames := map[string]bool{
		"test-session":    true,
		"another-session": true,
	}

	// Should fail for existing session
	err := validateSessionName("test-session", existingNames)
	if err == nil {
		t.Error("expected error for duplicate session name")
	}

	// Should succeed for new session
	err = validateSessionName("new-session", existingNames)
	if err != nil {
		t.Errorf("expected no error for new session, got: %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
