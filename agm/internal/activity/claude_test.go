package activity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeActivityTracker_GetLastActivity(t *testing.T) {
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
			historyData: `{"display":"test 1","pastedContents":{},"timestamp":1704067200000,"project":"/test","sessionId":"test-session-1"}
{"display":"test 2","pastedContents":{},"timestamp":1704067260000,"project":"/test","sessionId":"test-session-1"}
{"display":"test 3","pastedContents":{},"timestamp":1704067320000,"project":"/test","sessionId":"test-session-1"}`,
			sessionID:   "test-session-1",
			expectError: false,
			expectTime:  true,
			timeValidator: func(t time.Time) bool {
				// Should return 1704067320000 ms = 2024-01-01 00:02:00 UTC
				expected := time.Unix(1704067320, 0).UTC()
				return t.Equal(expected)
			},
		},
		{
			name:        "valid session with single message",
			historyData: `{"display":"test 1","pastedContents":{},"timestamp":1704067200000,"project":"/test","sessionId":"test-session-2"}`,
			sessionID:   "test-session-2",
			expectError: false,
			expectTime:  true,
			timeValidator: func(t time.Time) bool {
				expected := time.Unix(1704067200, 0).UTC()
				return t.Equal(expected)
			},
		},
		{
			name:        "session not found in history",
			historyData: `{"display":"test","pastedContents":{},"timestamp":1704067200000,"project":"/test","sessionId":"other-session"}`,
			sessionID:   "test-session-404",
			expectError: true,
			errorType:   ErrHistoryNotFound,
		},
		{
			name:        "empty history file",
			historyData: "",
			sessionID:   "test-session-empty",
			expectError: true,
			errorType:   ErrHistoryNotFound,
		},
		{
			name:        "session with empty entries list",
			historyData: "", // Simulated by having no matching sessionID
			sessionID:   "test-session-no-match",
			expectError: true,
			errorType:   ErrHistoryNotFound,
		},
		{
			name:        "malformed JSON",
			historyData: `{"display":"test","timestamp":INVALID}`,
			sessionID:   "test-session-bad",
			expectError: true,
			errorType:   ErrHistoryCorrupted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test history file
			historyFile := filepath.Join(tmpDir, tt.name+".jsonl")
			if tt.historyData != "" {
				if err := os.WriteFile(historyFile, []byte(tt.historyData), 0644); err != nil {
					t.Fatalf("Failed to write test history file: %v", err)
				}
			}

			// Create tracker with custom path
			tracker := NewClaudeActivityTrackerWithPath(historyFile)

			// Call GetLastActivity
			timestamp, err := tracker.GetLastActivity(tt.sessionID)

			// Verify error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
					return
				}
				// Check error type using errors.Is if needed
				// For now, just check it's not nil
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

func TestClaudeActivityTracker_HistoryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentFile := filepath.Join(tmpDir, "nonexistent.jsonl")

	tracker := NewClaudeActivityTrackerWithPath(nonExistentFile)
	_, err := tracker.GetLastActivity("test-session")

	if err == nil {
		t.Errorf("Expected error for nonexistent file, got nil")
	}
}

func TestClaudeActivityTracker_PermissionDenied(t *testing.T) {
	// This test is platform-specific and may not work on all systems
	// Skip on systems where we can't test permission denied
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tmpDir := t.TempDir()
	historyFile := filepath.Join(tmpDir, "noperm.jsonl")

	// Create file with no read permissions
	if err := os.WriteFile(historyFile, []byte("test"), 0000); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Chmod(historyFile, 0644) // Restore permissions for cleanup

	tracker := NewClaudeActivityTrackerWithPath(historyFile)
	_, err := tracker.GetLastActivity("test-session")

	if err == nil {
		t.Errorf("Expected permission denied error, got nil")
	}
}

func TestNewClaudeActivityTracker_DefaultPath(t *testing.T) {
	tracker := NewClaudeActivityTracker()
	if tracker == nil {
		t.Errorf("Expected tracker, got nil")
	}

	// Check that historyPath is set (should be ~/.claude/history.jsonl)
	home, _ := os.UserHomeDir()
	expectedPath := filepath.Join(home, ".claude", "history.jsonl")
	if tracker.historyPath != expectedPath {
		t.Errorf("Expected historyPath=%s, got %s", expectedPath, tracker.historyPath)
	}
}
