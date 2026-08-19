package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestAwaitReaperStartupRequiresExactReadinessRecord(t *testing.T) {
	tests := []struct {
		name    string
		record  string
		wantErr string
	}{
		{name: "ready", record: "ready\n"},
		{name: "invalid", record: "not-ready\n", wantErr: "invalid startup acknowledgement"},
		{name: "closed", wantErr: "closed before readiness"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			if tc.record != "" {
				if _, err := writer.WriteString(tc.record); err != nil {
					t.Fatal(err)
				}
			}
			_ = writer.Close()
			defer func() { _ = reader.Close() }()

			err = awaitReaperStartup(reader, time.Second)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("awaitReaperStartup() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("awaitReaperStartup() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestAwaitReaperStartupTimesOutWithoutAcknowledgement(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = writer.Close()
		_ = reader.Close()
	}()
	if err := awaitReaperStartup(reader, 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("awaitReaperStartup() error = %v, want timeout", err)
	}
}

// TestSpawnReaper_SessionNameSanitization tests path traversal protection
func TestSpawnReaper_SessionNameSanitization(t *testing.T) {
	_, _, cleanup := setupArchiveTest(t)
	defer cleanup()

	testCases := []struct {
		name         string
		sessionName  string
		expectedLog  string // Expected log file name (sanitized)
		shouldAccept bool
	}{
		{
			name:         "path traversal attempt",
			sessionName:  "../../../evil-session",
			expectedLog:  "agm-reaper-evil-session.log",
			shouldAccept: true,
		},
		{
			name:         "with forward slash",
			sessionName:  "session/with/slashes",
			expectedLog:  "agm-reaper-slashes.log",
			shouldAccept: true,
		},
		{
			name:         "with backslash",
			sessionName:  "session\\with\\backslash",
			expectedLog:  "agm-reaper-backslash.log",
			shouldAccept: true,
		},
		{
			name:         "normal session name",
			sessionName:  "my-normal-session",
			expectedLog:  "agm-reaper-my-normal-session.log",
			shouldAccept: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Note: spawnReaper() will fail because agm-reaper binary doesn't exist
			// in test environment. We're testing the path sanitization logic.
			err := spawnReaper("stable-session-id", tc.sessionName, "codex-cli", manifest.OutcomeUnknown, false)

			// Should get error about missing binary (expected in tests)
			if err == nil {
				t.Fatal("Expected error about missing binary, got nil")
				return
			}

			// Verify error message mentions expected log path (sanitized)
			if !strings.Contains(err.Error(), tc.expectedLog) {
				t.Errorf("Expected log path with '%s', got error: %v", tc.expectedLog, err)
			}

			// Verify log path is in temp dir (not traversed elsewhere)
			tmpPath := filepath.Join(os.TempDir(), tc.expectedLog)
			if !strings.Contains(err.Error(), tmpPath) {
				t.Errorf("Log path should be in %s, got error: %v", os.TempDir(), err)
			}
		})
	}
}

func TestBuildReaperArgsSeparatesStableAndTmuxIdentities(t *testing.T) {
	args := buildReaperArgs(
		"stable-session-id",
		"resolved-tmux",
		"/tmp/reaper.log",
		"/tmp/sessions",
		"0123456789ab",
		true,
		true,
		true,
		manifest.OutcomeKilled,
	)
	got := strings.Join(args, " ")
	for _, want := range []string{
		"--session-id stable-session-id",
		"--session resolved-tmux",
		"--force",
		"--keep-sandbox",
		"--allow-supervisor-reap",
		"--outcome killed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("reaper args %q do not contain %q", got, want)
		}
	}
}

// panickingReader panics on Read, standing in for whatever could go wrong on
// the far side of the startup pipe.
type panickingReader struct{}

func (panickingReader) Read([]byte) (int, error) { panic("reader exploded") }

// TestAwaitReaperStartupRecoversFromReaderPanic proves the handshake goroutine
// converts a panic into the bounded failure the caller already handles,
// instead of taking the whole agm process down for a startup handshake.
//
// A nil *os.File does not work here: it returns ErrInvalid rather than
// panicking, so the test would pass without the recovery in place.
func TestAwaitReaperStartupRecoversFromReaderPanic(t *testing.T) {
	err := awaitReaperStartup(panickingReader{}, 5*time.Second)
	if err == nil {
		t.Fatal("awaitReaperStartup() = nil error, want the recovered panic")
	}
	if !strings.Contains(err.Error(), "startup acknowledgement reader panicked") {
		t.Errorf("error = %q, want it to name the recovered panic", err)
	}
	if !strings.Contains(err.Error(), "reader exploded") {
		t.Errorf("error = %q, want it to carry the panic value", err)
	}
}

// TestAwaitReaperStartupCapsTheAcknowledgement proves a peer that never sends
// a newline cannot grow the read buffer without bound.
func TestAwaitReaperStartupCapsTheAcknowledgement(t *testing.T) {
	flood := strings.NewReader(strings.Repeat("x", maxStartupAckBytes*4))
	err := awaitReaperStartup(flood, 5*time.Second)
	if err == nil {
		t.Fatal("awaitReaperStartup() = nil error, want a bounded failure")
	}
	if !strings.Contains(err.Error(), "closed before readiness") {
		t.Errorf("error = %q, want the capped read to surface as a closed stream", err)
	}
}
