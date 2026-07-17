package reaper

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
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
		return
	}

	// Error should mention "session not found"
	if err.Error() == "" {
		t.Error("archiveSession() returned empty error message")
	}

	t.Logf("archiveSession() correctly failed with error: %v", err)
}

func TestRun_ArchivePreflightBlocksProtectedSupervisorBeforeTmux(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agm.db")
	t.Setenv("AGM_DB_PATH", dbPath)

	adapter, err := dolt.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "protected-supervisor-id",
		Name:          "vroom-orchestrator",
		Harness:       "agy",
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "vroom-orchestrator"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	f := &fakeBoundary{}
	f.install(t)
	err = New(m.Name, t.TempDir()).Run()
	if err == nil {
		t.Fatal("Run() should fail archive preflight for a protected supervisor")
	}
	if !strings.Contains(err.Error(), "protected supervisor") {
		t.Fatalf("Run() error = %q, want protected supervisor failure", err)
	}
	if f.sendPromptCalls != 0 || f.killSessionCalls != 0 {
		t.Fatalf("preflight failure touched tmux: send=%d kill=%d", f.sendPromptCalls, f.killSessionCalls)
	}

	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Lifecycle != "" {
		t.Fatalf("Lifecycle = %q, want unchanged", stored.Lifecycle)
	}
}
