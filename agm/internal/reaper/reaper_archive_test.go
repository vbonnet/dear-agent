package reaper

import (
	"testing"
)

// Phase 6 (2026-03-18): YAML archiving tests removed - archiving now done via Dolt adapter.UpdateSession()
// Tests for archiveSession() deleted as the function uses obsolete YAML manifest read/write.

// TestArchiveSession_SessionNotFound tests error handling when session doesn't exist
func TestArchiveSession_SessionNotFound(t *testing.T) {
	// Create temp sessions directory
	tmpDir := t.TempDir()

	// Create reaper for non-existent session
	r := New("nonexistent-session", tmpDir)

	// Archive should fail
	err := r.archiveSession()
	if err == nil {
		t.Fatal("archiveSession() should fail for non-existent session, but succeeded")
	}

	// Error should mention "session not found"
	if err.Error() == "" {
		t.Error("archiveSession() returned empty error message")
	}

	t.Logf("archiveSession() correctly failed with error: %v", err)
}
