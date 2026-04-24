package tmux

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// --- CapturePaneContent unit tests ---

func TestCapturePaneContent_EmptySessionName(t *testing.T) {
	_, err := CapturePaneContent("")
	if err == nil {
		t.Error("CapturePaneContent('') should return error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestCapturePaneHistory_EmptySessionName(t *testing.T) {
	_, err := CapturePaneHistory("", 100)
	if err == nil {
		t.Error("CapturePaneHistory('') should return error")
	}
}

func TestCapturePaneHistory_ZeroLines(t *testing.T) {
	// With zero lines, should attempt to capture all history
	// Will fail with session not found or tmux not running, which is expected
	_, err := CapturePaneHistory("nonexistent-test-session-xyz", 0)
	if err == nil {
		t.Error("CapturePaneHistory for nonexistent session should return error")
	}
	// Should be either ErrSessionNotFound or ErrTmuxNotRunning
	if !errors.Is(err, ErrSessionNotFound) && !errors.Is(err, ErrTmuxNotRunning) {
		t.Logf("Got error: %v (acceptable for nonexistent session)", err)
	}
}

func TestCapturePaneLines_EmptySessionName(t *testing.T) {
	_, err := CapturePaneLines("", 10)
	if err == nil {
		t.Error("CapturePaneLines('') should return error")
	}
}

func TestCapturePaneLines_NonexistentSession(t *testing.T) {
	_, err := CapturePaneLines("nonexistent-test-session-xyz", 10)
	if err == nil {
		t.Error("CapturePaneLines for nonexistent session should return error")
	}
	if !errors.Is(err, ErrSessionNotFound) && !errors.Is(err, ErrTmuxNotRunning) {
		t.Logf("Got error: %v (acceptable)", err)
	}
}

func TestCapturePaneHistoryLines_EmptySessionName(t *testing.T) {
	_, err := CapturePaneHistoryLines("", 10)
	if err == nil {
		t.Error("CapturePaneHistoryLines('') should return error")
	}
}

func TestCapturePaneHistoryLines_NonexistentSession(t *testing.T) {
	lines, err := CapturePaneHistoryLines("nonexistent-test-session-xyz", 50)
	if err == nil {
		// If tmux happens to be running but session doesn't exist
		if len(lines) != 0 {
			t.Error("Expected empty lines for nonexistent session")
		}
	} else {
		if !errors.Is(err, ErrSessionNotFound) && !errors.Is(err, ErrTmuxNotRunning) {
			t.Logf("Got error: %v (acceptable)", err)
		}
	}
}

// --- SessionExists tests ---

func TestSessionExists_EmptyName(t *testing.T) {
	_, err := SessionExists("")
	if err == nil {
		t.Error("SessionExists('') should return error")
	}
}

func TestSessionExists_NonexistentSession(t *testing.T) {
	exists, err := SessionExists("nonexistent-test-session-xyz-12345")
	if err != nil {
		// tmux not running is acceptable
		t.Logf("SessionExists error: %v", err)
		return
	}
	if exists {
		t.Error("SessionExists should return false for nonexistent session")
	}
}

// --- GetSessionInfo tests ---

func TestGetSessionInfo_EmptyName(t *testing.T) {
	_, err := GetSessionInfo("")
	if err == nil {
		t.Error("GetSessionInfo('') should return error")
	}
}

func TestGetSessionInfo_NonexistentSession(t *testing.T) {
	info, err := GetSessionInfo("nonexistent-test-session-xyz-12345")
	if err == nil {
		t.Error("GetSessionInfo for nonexistent session should return error")
	}
	if info != nil {
		t.Error("info should be nil for error case")
	}
	// Accept ErrSessionNotFound or ErrTmuxNotRunning
	if !errors.Is(err, ErrSessionNotFound) && !errors.Is(err, ErrTmuxNotRunning) {
		// Could be a different tmux error, still acceptable
		t.Logf("Got error: %v", err)
	}
}

// --- Error sentinel tests ---

func TestErrorSentinels(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrSessionNotFound", ErrSessionNotFound, "session not found"},
		{"ErrTmuxNotRunning", ErrTmuxNotRunning, "not running"},
		{"ErrPermissionDenied", ErrPermissionDenied, "permission denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Error("sentinel error should not be nil")
			}
			if !strings.Contains(tt.err.Error(), tt.msg) {
				t.Errorf("error %q should contain %q", tt.err.Error(), tt.msg)
			}
		})
	}
}

// --- Now() tests ---

func TestNow(t *testing.T) {
	before := time.Now().UnixMilli()
	got := Now()
	after := time.Now().UnixMilli()

	if got < before || got > after {
		t.Errorf("Now() = %d, want between %d and %d", got, before, after)
	}
}

func TestNow_Monotonic(t *testing.T) {
	first := Now()
	second := Now()
	if second < first {
		t.Errorf("Now() should be monotonically increasing: first=%d, second=%d", first, second)
	}
}

// --- SessionInfo struct test ---

func TestSessionInfo_Fields(t *testing.T) {
	info := SessionInfo{
		Name:     "test-session",
		Windows:  3,
		Created:  "1234567890",
		Attached: true,
	}

	if info.Name != "test-session" {
		t.Errorf("Name = %q, want %q", info.Name, "test-session")
	}
	if info.Windows != 3 {
		t.Errorf("Windows = %d, want 3", info.Windows)
	}
	if !info.Attached {
		t.Error("Attached should be true")
	}
}
