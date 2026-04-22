package tmux

import (
	"testing"
)

func TestSanitizeSessionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid name with alphanumeric and dash",
			input:    "my-session-123",
			expected: "my-session-123",
		},
		{
			name:     "valid name with underscore",
			input:    "my_session_name",
			expected: "my_session_name",
		},
		{
			name:     "name with period (dropped)",
			input:    "gemini-task-1.4",
			expected: "gemini-task-14",
		},
		{
			name:     "name with multiple periods",
			input:    "session.with.many.periods",
			expected: "sessionwithmanyperiods",
		},
		{
			name:     "name with spaces (converted to dash)",
			input:    "my session name",
			expected: "my-session-name",
		},
		{
			name:     "name with special characters (dropped)",
			input:    "my@session!name#123",
			expected: "mysessionname123",
		},
		{
			name:     "name with mixed invalid chars",
			input:    "test.session@2024!",
			expected: "testsession2024",
		},
		{
			name:     "only invalid characters (fallback to 'session')",
			input:    "@#$%^&*()",
			expected: "session",
		},
		{
			name:     "empty string (fallback to 'session')",
			input:    "",
			expected: "session",
		},
		{
			name:     "unicode characters (non-ASCII dropped)",
			input:    "session-café-™",
			expected: "session-caf-", // 'café' has ASCII 'c','a','f' which are preserved, é and ™ dropped
		},
		{
			name:     "path-like name",
			input:    "~/project",
			expected: "project",
		},
		{
			name:     "URL-like name",
			input:    "https://example.com/session",
			expected: "httpsexamplecomsession",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeSessionName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeSessionName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeSessionName_PreservesCasing(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MySession", "MySession"},
		{"MY-SESSION", "MY-SESSION"},
		{"MixedCaseSession123", "MixedCaseSession123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeSessionName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeSessionName(%q) = %q, want %q (casing not preserved)", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeSessionName_RealWorldExamples(t *testing.T) {
	// Real-world examples from AGM usage
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "gemini task with period",
			input:    "gemini-task-1.4",
			expected: "gemini-task-14",
		},
		{
			name:     "UUID with dashes",
			input:    "370980e1-e16c-48a1-9d17-caca0d3910ba",
			expected: "370980e1-e16c-48a1-9d17-caca0d3910ba",
		},
		{
			name:     "session name from project path",
			input:    "claude-session-manager",
			expected: "claude-session-manager",
		},
		{
			name:     "descriptive name with spaces",
			input:    "Fix AGM Bug",
			expected: "Fix-AGM-Bug",
		},
		{
			name:     "version number with dots",
			input:    "v2.0.1-beta",
			expected: "v201-beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeSessionName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeSessionName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNewSession_AutoSanitizesName verifies that NewSession automatically
// sanitizes invalid session names to prevent "exit status 1" errors.
func TestNewSession_AutoSanitizesName(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	skipIfNoTmux(t)
	setupTestSocket(t)
	setupTestState(t)

	// Test that NewSession automatically sanitizes names with periods
	// Before fix: NewSession("gemini-task-1.4", "/tmp") → fails with exit status 1
	// After fix: NewSession("gemini-task-1.4", "/tmp") → creates session "gemini-task-14"

	invalidName := "test-session-1.4.2" // Contains periods (invalid for tmux)
	expectedName := "test-session-142"  // Periods should be stripped

	defer killTestSession(expectedName)

	// Create session with invalid name - should auto-sanitize
	err := NewSession(invalidName, t.TempDir())
	if err != nil {
		t.Fatalf("NewSession should succeed even with invalid name: %v", err)
	}

	// Verify session was created with sanitized name
	exists, err := HasSession(expectedName)
	if err != nil {
		t.Fatalf("Failed to check session existence: %v", err)
	}

	if !exists {
		t.Errorf("Expected sanitized session %q to exist", expectedName)
	}

	t.Logf("Successfully created session with sanitized name: %q → %q", invalidName, expectedName)
}
