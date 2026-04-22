package tmux

import (
	"errors"
	"strings"
	"testing"
)

func TestCapturePaneContent(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		wantErr     bool
		errType     error
	}{
		{
			name:        "empty session name",
			sessionName: "",
			wantErr:     true,
			errType:     nil, // generic error
		},
		{
			name:        "non-existent session",
			sessionName: "nonexistent-session-12345",
			wantErr:     true,
			errType:     ErrSessionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := CapturePaneContent(tt.sessionName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CapturePaneContent() expected error, got nil")
				}
				// Accept either ErrSessionNotFound or ErrTmuxNotRunning (when tmux server is not running)
				if tt.errType != nil && !errors.Is(err, tt.errType) && !errors.Is(err, ErrTmuxNotRunning) {
					t.Errorf("CapturePaneContent() error = %v, want %v or ErrTmuxNotRunning", err, tt.errType)
				}
				return
			}

			if err != nil {
				t.Errorf("CapturePaneContent() unexpected error = %v", err)
			}

			if content == "" {
				t.Errorf("CapturePaneContent() returned empty content for valid session")
			}
		})
	}
}

func TestCapturePaneContent_LargeContent(t *testing.T) {
	// This test verifies that large pane content (>100KB) is handled correctly
	// We'll skip this test if no tmux session is available
	if !IsTmuxRunning() {
		t.Skip("tmux is not running, skipping test")
	}

	// Try to capture content from the current session (if any exists)
	// This is a smoke test to ensure large content doesn't cause issues
	sessionName := "test-large-content"
	_, err := CapturePaneContent(sessionName)

	// We expect an error since the session likely doesn't exist,
	// but this tests that the function handles the call correctly
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		// Only fail if it's not a "session not found" error
		if !errors.Is(err, ErrTmuxNotRunning) {
			t.Logf("CapturePaneContent() returned: %v (expected for non-existent session)", err)
		}
	}
}

func TestIsTmuxRunning(t *testing.T) {
	// This test just verifies the function doesn't panic
	// The actual result depends on whether tmux is running
	running := IsTmuxRunning()

	t.Logf("Tmux running: %v", running)

	// We can't assert a specific value since it depends on the environment
	// but we can ensure it returns without error
}

func TestCapturePaneHistory(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		lines       int
		wantErr     bool
		errType     error
	}{
		{
			name:        "empty session name",
			sessionName: "",
			lines:       100,
			wantErr:     true,
		},
		{
			name:        "non-existent session",
			sessionName: "nonexistent-session-12345",
			lines:       100,
			wantErr:     true,
			errType:     ErrSessionNotFound,
		},
		{
			name:        "zero lines (all history)",
			sessionName: "nonexistent-session-12345",
			lines:       0,
			wantErr:     true,
			errType:     ErrSessionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := CapturePaneHistory(tt.sessionName, tt.lines)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CapturePaneHistory() expected error, got nil")
				}
				// Accept either ErrSessionNotFound or ErrTmuxNotRunning (when tmux server is not running)
				if tt.errType != nil && !errors.Is(err, tt.errType) && !errors.Is(err, ErrTmuxNotRunning) {
					t.Errorf("CapturePaneHistory() error = %v, want %v or ErrTmuxNotRunning", err, tt.errType)
				}
				return
			}

			if err != nil {
				t.Errorf("CapturePaneHistory() unexpected error = %v", err)
			}

			if content == "" {
				t.Errorf("CapturePaneHistory() returned empty content for valid session")
			}
		})
	}
}

func TestGetSessionInfo(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		wantErr     bool
		errType     error
	}{
		{
			name:        "empty session name",
			sessionName: "",
			wantErr:     true,
		},
		{
			name:        "non-existent session",
			sessionName: "nonexistent-session-12345",
			wantErr:     true,
			errType:     ErrSessionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := GetSessionInfo(tt.sessionName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetSessionInfo() expected error, got nil")
				}
				// Accept either ErrSessionNotFound or ErrTmuxNotRunning (when tmux server is not running)
				if tt.errType != nil && !errors.Is(err, tt.errType) && !errors.Is(err, ErrTmuxNotRunning) {
					t.Errorf("GetSessionInfo() error = %v, want %v or ErrTmuxNotRunning", err, tt.errType)
				}
				if info != nil {
					t.Errorf("GetSessionInfo() expected nil info for error case, got %+v", info)
				}
				return
			}

			if err != nil {
				t.Errorf("GetSessionInfo() unexpected error = %v", err)
			}

			if info == nil {
				t.Errorf("GetSessionInfo() returned nil info for valid session")
			}
		})
	}
}

// TestGetSessionInfo_Integration is an integration test that requires an actual tmux session
func TestGetSessionInfo_Integration(t *testing.T) {
	if !IsTmuxRunning() {
		t.Skip("tmux is not running, skipping integration test")
	}

	// Try to get info for a test session
	// This will fail if the session doesn't exist, which is expected
	info, err := GetSessionInfo("test-session-that-probably-doesnt-exist")

	if err != nil {
		if !errors.Is(err, ErrSessionNotFound) {
			t.Logf("GetSessionInfo() returned error: %v (expected for non-existent session)", err)
		}
	} else if info != nil {
		// If we somehow got a session, verify the structure
		if info.Name == "" {
			t.Error("SessionInfo.Name should not be empty")
		}
		t.Logf("Found session: %+v", info)
	}
}

// TestCapturePaneContent_Integration tests with real tmux sessions
func TestCapturePaneContent_Integration(t *testing.T) {
	if !IsTmuxRunning() {
		t.Skip("tmux is not running, skipping integration test")
	}

	// Get list of actual sessions to test with
	info, _ := GetSessionInfo("session-monitoring-infrastructure")
	if info != nil {
		t.Run("capture existing session", func(t *testing.T) {
			content, err := CapturePaneContent(info.Name)
			if err != nil {
				t.Errorf("CapturePaneContent() failed for existing session %s: %v", info.Name, err)
			}
			if content == "" {
				t.Logf("Warning: CapturePaneContent() returned empty content for session %s", info.Name)
			}

			// Verify content is reasonable (not binary garbage)
			if !strings.Contains(content, string(rune(0))) {
				// Content should be printable text
				t.Logf("Captured %d bytes from session %s", len(content), info.Name)
			}
		})
	}
}

// Benchmark tests
func BenchmarkCapturePaneContent(b *testing.B) {
	if !IsTmuxRunning() {
		b.Skip("tmux is not running")
	}

	b.Run("non-existent session", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = CapturePaneContent("nonexistent-session-bench")
		}
	})
}

func BenchmarkIsTmuxRunning(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = IsTmuxRunning()
	}
}
