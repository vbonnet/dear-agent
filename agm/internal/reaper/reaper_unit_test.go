package reaper

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Constructor tests ---

func TestNew_AllFieldsInitialized(t *testing.T) {
	r := New("my-session", "/data/sessions")

	if r.SessionName != "my-session" {
		t.Errorf("SessionName = %q, want %q", r.SessionName, "my-session")
	}
	if r.SessionsDir != "/data/sessions" {
		t.Errorf("SessionsDir = %q, want %q", r.SessionsDir, "/data/sessions")
	}
	if r.SocketPath == "" {
		t.Error("SocketPath should not be empty")
	}
	if r.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestNew_EmptySessionName(t *testing.T) {
	r := New("", "/data/sessions")
	if r.SessionName != "" {
		t.Errorf("SessionName should be empty, got %q", r.SessionName)
	}
	// Should still have logger and socket path
	if r.logger == nil {
		t.Error("logger should be initialized even with empty session name")
	}
	if r.SocketPath == "" {
		t.Error("SocketPath should be set even with empty session name")
	}
}

func TestNew_EmptySessionsDir(t *testing.T) {
	r := New("test", "")
	if r.SessionsDir != "" {
		t.Errorf("SessionsDir should be empty, got %q", r.SessionsDir)
	}
}

// --- Constants tests ---

func TestConstants(t *testing.T) {
	// Verify constants are reasonable values
	if PromptDetectionTimeout <= 0 {
		t.Error("PromptDetectionTimeout should be positive")
	}
	if PromptDetectionTimeout > 5*time.Minute {
		t.Errorf("PromptDetectionTimeout = %v, seems too high", PromptDetectionTimeout)
	}

	if PaneCloseTimeout <= 0 {
		t.Error("PaneCloseTimeout should be positive")
	}
	if PaneCloseTimeout > 5*time.Minute {
		t.Errorf("PaneCloseTimeout = %v, seems too high", PaneCloseTimeout)
	}

	if FallbackWaitTime <= 0 {
		t.Error("FallbackWaitTime should be positive")
	}
	if FallbackWaitTime > 5*time.Minute {
		t.Errorf("FallbackWaitTime = %v, seems too high", FallbackWaitTime)
	}

	// PromptDetectionTimeout should be >= FallbackWaitTime
	if PromptDetectionTimeout < FallbackWaitTime {
		t.Logf("Note: PromptDetectionTimeout (%v) > FallbackWaitTime (%v)",
			PromptDetectionTimeout, FallbackWaitTime)
	}
}

func TestConstantValues(t *testing.T) {
	if PromptDetectionTimeout != 90*time.Second {
		t.Errorf("PromptDetectionTimeout = %v, want 90s", PromptDetectionTimeout)
	}
	if PaneCloseTimeout != 60*time.Second {
		t.Errorf("PaneCloseTimeout = %v, want 60s", PaneCloseTimeout)
	}
	if FallbackWaitTime != 60*time.Second {
		t.Errorf("FallbackWaitTime = %v, want 60s", FallbackWaitTime)
	}
	if SIGTERMGracePeriod != 10*time.Second {
		t.Errorf("SIGTERMGracePeriod = %v, want 10s", SIGTERMGracePeriod)
	}
	if PostKillPaneTimeout != 5*time.Second {
		t.Errorf("PostKillPaneTimeout = %v, want 5s", PostKillPaneTimeout)
	}
}

// --- getSessionsDir tests ---

func TestGetSessionsDir_WithConfiguredDir(t *testing.T) {
	r := New("test", "/custom/sessions")
	dir, err := r.getSessionsDir()
	if err != nil {
		t.Fatalf("getSessionsDir() error: %v", err)
	}
	if dir != "/custom/sessions" {
		t.Errorf("getSessionsDir() = %q, want %q", dir, "/custom/sessions")
	}
}

func TestGetSessionsDir_WithDefault(t *testing.T) {
	r := New("test", "")
	dir, err := r.getSessionsDir()
	if err != nil {
		t.Fatalf("getSessionsDir() error: %v", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error: %v", err)
	}

	expected := filepath.Join(homeDir, ".claude", "sessions")
	if dir != expected {
		t.Errorf("getSessionsDir() = %q, want %q", dir, expected)
	}
}

func TestGetSessionsDir_ReturnsAbsolutePath(t *testing.T) {
	r := New("test", "")
	dir, err := r.getSessionsDir()
	if err != nil {
		t.Fatalf("getSessionsDir() error: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("getSessionsDir() should return absolute path, got %q", dir)
	}
}

// --- Run method existence / interface test ---

func TestReaperMethodsExist(t *testing.T) {
	r := New("test", "/tmp/sessions")

	// Verify methods exist via function values (compile-time check)
	var _ func() error = r.Run
	var _ func(time.Duration) error = r.waitForPrompt
	var _ func() error = r.sendExit
	var _ func(time.Duration) error = r.waitForPaneClose
	var _ func() error = r.archiveSession
	var _ func() error = r.markReaping
	var _ func() = r.forceKillPaneProcess
	var _ func(time.Time) time.Duration = r.timeRemaining
}

// --- Multiple reapers test ---

func TestMultipleReapers_Independent(t *testing.T) {
	r1 := New("session-1", "/path/1")
	r2 := New("session-2", "/path/2")

	if r1.SessionName == r2.SessionName {
		t.Error("Reapers should have independent session names")
	}
	if r1.SessionsDir == r2.SessionsDir {
		t.Error("Reapers should have independent session dirs")
	}

	// Both should share the same socket path (global tmux socket)
	if r1.SocketPath != r2.SocketPath {
		t.Errorf("Socket paths should match: %q vs %q", r1.SocketPath, r2.SocketPath)
	}
}
