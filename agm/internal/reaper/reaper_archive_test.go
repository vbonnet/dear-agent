package reaper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

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

func TestArchiveSession_SharedOperationPreservesOutcomeAndLegacyMove(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agm.db")
	sessionsDir := t.TempDir()
	t.Setenv("AGM_DB_PATH", dbPath)
	t.Setenv("HOME", t.TempDir())

	adapter, err := dolt.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "legacy-shared-op-id",
		Name:          "legacy-shared-op",
		Harness:       "agy",
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "legacy-shared-op"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	legacyDir := filepath.Join(sessionsDir, m.SessionID)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(legacy) error: %v", err)
	}
	if err := manifest.Write(filepath.Join(legacyDir, "manifest.yaml"), m); err != nil {
		t.Fatalf("manifest.Write() error: %v", err)
	}

	r := NewWithOptions(m.Name, sessionsDir, ArchiveOptions{Force: true, Outcome: manifest.OutcomeKilled})
	if err := r.archiveSession(); err != nil {
		t.Fatalf("archiveSession() error: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Lifecycle != manifest.LifecycleArchived || stored.Outcome != manifest.OutcomeKilled {
		t.Fatalf("stored lifecycle/outcome = (%q, %q), want (%q, %q)", stored.Lifecycle, stored.Outcome, manifest.LifecycleArchived, manifest.OutcomeKilled)
	}
	archiveDir := filepath.Join(sessionsDir, ".archive-old-format", m.SessionID)
	if _, err := os.Stat(archiveDir); err != nil {
		t.Fatalf("legacy archive path missing: %v", err)
	}
}

func TestArchiveSession_AlreadyArchivedIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agm.db")
	t.Setenv("AGM_DB_PATH", dbPath)
	t.Setenv("HOME", t.TempDir())
	adapter, err := dolt.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "already-archived-reaper-id",
		Name:          "already-archived-reaper",
		Harness:       "agy",
		Lifecycle:     manifest.LifecycleArchived,
		Outcome:       manifest.OutcomeCompleted,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "already-archived-reaper"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	if err := New(m.Name, t.TempDir()).archiveSession(); err != nil {
		t.Fatalf("archiveSession() idempotent error: %v", err)
	}
}

func TestRun_UsesStableSessionIDAndResolvedTmuxIdentity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agm.db")
	t.Setenv("AGM_DB_PATH", dbPath)
	t.Setenv("HOME", t.TempDir())

	adapter, err := dolt.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "stable-reaper-session-id",
		Name:          "renamed-after-reaper-spawn",
		Harness:       "codex-cli",
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Hour),
		Context:       manifest.Context{Project: t.TempDir()},
		Tmux:          manifest.Tmux{SessionName: "renamed-tmux-after-reaper-spawn"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	const resolvedTmuxAtSpawn = "resolved-tmux-at-spawn"
	f := &fakeBoundary{}
	f.install(t)
	r := NewWithOptions(resolvedTmuxAtSpawn, t.TempDir(), ArchiveOptions{
		SessionID: m.SessionID,
		Force:     true,
	})
	if got := r.archiveRequest().Identifier; got != m.SessionID {
		t.Fatalf("archive request identifier = %q, want stable ID %q", got, m.SessionID)
	}
	if err := r.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Lifecycle != manifest.LifecycleArchived {
		t.Fatalf("Lifecycle = %q, want archived", stored.Lifecycle)
	}
	if r.SessionName != resolvedTmuxAtSpawn {
		t.Fatalf("tmux identity = %q, want %q", r.SessionName, resolvedTmuxAtSpawn)
	}
}
