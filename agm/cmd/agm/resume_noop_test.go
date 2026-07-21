package main

import (
	"context"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestPerformResume_RejectsArchivedSession is a regression test for ce-6as.35
// (legacy BL-042): performResume() in the bare `agm` default-command path used
// to be a stub that printed a placeholder and returned nil — silently
// "succeeding" without ever resuming. It now delegates to resumeResolvedSession
// (the same workflow that backs `agm session resume`), so it must actually run
// the resume logic rather than no-op.
//
// We prove real logic runs via the archived-session guard: a genuine resume
// rejects an archived session with an error. The old no-op stub never inspected
// lifecycle and would have returned nil here.
func TestPerformResume_RejectsArchivedSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Dolt integration test in short mode")
	}

	_, sessionsDir, cleanup := setupArchiveTest(t)
	defer cleanup()

	const sessionID = "session-resume-noop"
	createArchiveTestSession(t, sessionsDir, sessionID, "resume-noop", "claude-resume-noop", manifest.LifecycleArchived)

	adapter, err := getStorage()
	if err != nil {
		t.Skipf("Dolt server not available (infrastructure test): %v", err)
	}
	defer adapter.Close()

	m, err := adapter.GetSession(sessionID)
	if err != nil {
		t.Fatalf("failed to read seeded session: %v", err)
	}

	err = performResume(context.Background(), adapter, m)
	if err == nil {
		t.Fatal("performResume returned nil for an archived session — resume logic is a no-op (regression: ce-6as.35)")
	}
	if !strings.Contains(err.Error(), "archived") {
		t.Errorf("expected archived-session error, got: %v", err)
	}
}
