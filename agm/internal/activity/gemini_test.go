package activity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGeminiActivityTracker_GetLastActivity(t *testing.T) {
	// Create temporary directory for test files
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		historyData   string
		sessionID     string
		expectError   bool
		errorType     error
		expectTime    bool
		timeValidator func(time.Time) bool
	}{
		{
			name: "valid session with multiple messages",
			historyData: `{"id":"msg-1","role":"user","content":"test 1","timestamp":"2024-01-01T00:00:00Z"}
{"id":"msg-2","role":"assistant","content":"test 2","timestamp":"2024-01-01T00:01:00Z"}
{"id":"msg-3","role":"user","content":"test 3","timestamp":"2024-01-01T00:02:00Z"}`,
			sessionID:   "test-session-1",
			expectError: false,
			expectTime:  true,
			timeValidator: func(t time.Time) bool {
				expected, _ := time.Parse(time.RFC3339, "2024-01-01T00:02:00Z")
				return t.Equal(expected.UTC())
			},
		},
		{
			name:        "valid session with single message",
			historyData: `{"id":"msg-1","role":"user","content":"test 1","timestamp":"2024-01-01T00:00:00Z"}`,
			sessionID:   "test-session-2",
			expectError: false,
			expectTime:  true,
			timeValidator: func(t time.Time) bool {
				expected, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
				return t.Equal(expected.UTC())
			},
		},
		{
			name:        "empty history file",
			historyData: "",
			sessionID:   "test-session-empty",
			expectError: true,
			errorType:   ErrEmptyHistory,
		},
		{
			name: "history with only empty lines",
			historyData: `

`,
			sessionID:   "test-session-whitespace",
			expectError: true,
			errorType:   ErrEmptyHistory,
		},
		{
			name:        "malformed JSON",
			historyData: `{"id":"msg-1","timestamp":INVALID}`,
			sessionID:   "test-session-bad",
			expectError: true,
			errorType:   ErrHistoryCorrupted,
		},
		{
			name: "messages with different timestamp formats",
			historyData: `{"id":"msg-1","role":"user","content":"test 1","timestamp":"2024-01-01T00:00:00Z"}
{"id":"msg-2","role":"assistant","content":"test 2","timestamp":"2024-01-01T00:05:00+00:00"}`,
			sessionID:   "test-session-formats",
			expectError: false,
			expectTime:  true,
			timeValidator: func(t time.Time) bool {
				// Should return the later timestamp
				expected, _ := time.Parse(time.RFC3339, "2024-01-01T00:05:00Z")
				return t.Equal(expected.UTC())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test session directory structure
			sessionDir := filepath.Join(tmpDir, tt.sessionID)
			if err := os.MkdirAll(sessionDir, 0755); err != nil {
				t.Fatalf("Failed to create session directory: %v", err)
			}

			// Create test history file
			historyFile := filepath.Join(sessionDir, "history.jsonl")
			if err := os.WriteFile(historyFile, []byte(tt.historyData), 0644); err != nil {
				t.Fatalf("Failed to write test history file: %v", err)
			}

			// Create tracker with custom base directory
			tracker := NewGeminiActivityTrackerWithPath(tmpDir)

			// Call GetLastActivity
			timestamp, err := tracker.GetLastActivity(tt.sessionID)

			// Verify error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				// Check error type if needed
				return
			}

			// Verify no error
			if err != nil {
				t.Errorf("Expected no error, got: %v", err)
				return
			}

			// Verify timestamp
			if tt.expectTime && tt.timeValidator != nil {
				if !tt.timeValidator(timestamp) {
					t.Errorf("Timestamp validation failed: got %v", timestamp)
				}
			}
		})
	}
}

func TestGeminiActivityTracker_HistoryNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	tracker := NewGeminiActivityTrackerWithPath(tmpDir)
	_, err := tracker.GetLastActivity("nonexistent-session")

	if err == nil {
		t.Errorf("Expected error for nonexistent session, got nil")
	}
}

func TestGeminiActivityTracker_PermissionDenied(t *testing.T) {
	// Skip on systems where we can't test permission denied
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tmpDir := t.TempDir()
	sessionID := "noperm-session"
	sessionDir := filepath.Join(tmpDir, sessionID)
	historyFile := filepath.Join(sessionDir, "history.jsonl")

	// Create session directory
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session directory: %v", err)
	}

	// Create file with no read permissions
	if err := os.WriteFile(historyFile, []byte("test"), 0000); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Chmod(historyFile, 0644) // Restore permissions for cleanup

	tracker := NewGeminiActivityTrackerWithPath(tmpDir)
	_, err := tracker.GetLastActivity(sessionID)

	if err == nil {
		t.Errorf("Expected permission denied error, got nil")
	}
}

func TestNewGeminiActivityTracker_DefaultPath(t *testing.T) {
	tracker := NewGeminiActivityTracker()
	if tracker == nil {
		t.Errorf("Expected tracker, got nil")
	}

	// Check that baseDir is set (should be ~/.agm/gemini)
	home, _ := os.UserHomeDir()
	expectedPath := filepath.Join(home, ".agm", "gemini")
	if tracker.baseDir != expectedPath {
		t.Errorf("Expected baseDir=%s, got %s", expectedPath, tracker.baseDir)
	}
}
