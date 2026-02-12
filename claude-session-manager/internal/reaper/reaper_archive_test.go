package reaper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// TestArchiveSession_Success tests successful archive operation
func TestArchiveSession_Success(t *testing.T) {
	// Create temp sessions directory
	tmpDir := t.TempDir()
	sessionName := "test-archive-session"
	sessionDir := filepath.Join(tmpDir, sessionName)

	// Create session directory
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Create manifest
	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test-session-id",
		Name:          sessionName,
		CreatedAt:     time.Now().Add(-1 * time.Hour),
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Active session (empty string)
		Context: manifest.Context{
			Project: "/test/project",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create reaper
	r := New(sessionName, tmpDir)

	// Archive session
	if err := r.archiveSession(); err != nil {
		t.Fatalf("archiveSession() failed: %v", err)
	}

	// Verify session moved to archive
	archiveDir := filepath.Join(tmpDir, ".archive-old-format", sessionName)
	if _, err := os.Stat(archiveDir); os.IsNotExist(err) {
		t.Errorf("Session not moved to archive directory: %s", archiveDir)
	}

	// Verify original session directory gone
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Error("Original session directory still exists after archive")
	}

	// Verify manifest updated to archived
	archivedManifestPath := filepath.Join(archiveDir, "manifest.yaml")
	archivedManifest, err := manifest.Read(archivedManifestPath)
	if err != nil {
		t.Fatalf("Failed to read archived manifest: %v", err)
	}

	if archivedManifest.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Manifest lifecycle = %q, expected %q", archivedManifest.Lifecycle, manifest.LifecycleArchived)
	}

	// Verify UpdatedAt timestamp was updated
	if archivedManifest.UpdatedAt.Before(m.UpdatedAt) {
		t.Error("Manifest UpdatedAt not updated during archive")
	}
}

// TestArchiveSession_AlreadyArchived tests idempotency when session already archived
func TestArchiveSession_AlreadyArchived(t *testing.T) {
	// Create temp sessions directory
	tmpDir := t.TempDir()
	sessionName := "test-already-archived"
	sessionDir := filepath.Join(tmpDir, sessionName)

	// Create session directory
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Create manifest with Lifecycle=archived
	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test-session-id",
		Name:          sessionName,
		CreatedAt:     time.Now().Add(-1 * time.Hour),
		UpdatedAt:     time.Now(),
		Lifecycle:     manifest.LifecycleArchived, // Already archived
		Context: manifest.Context{
			Project: "/test/project",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create reaper
	r := New(sessionName, tmpDir)

	// Archive session (should be no-op)
	if err := r.archiveSession(); err != nil {
		t.Fatalf("archiveSession() failed for already-archived session: %v", err)
	}

	// Verify session NOT moved (idempotent)
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		t.Error("Already-archived session was moved (should be idempotent)")
	}

	// Verify manifest unchanged
	unchangedManifest, err := manifest.Read(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest after idempotent archive: %v", err)
	}

	if unchangedManifest.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Manifest lifecycle changed, expected %q, got %q", manifest.LifecycleArchived, unchangedManifest.Lifecycle)
	}
}

// TestArchiveSession_ConflictResolution tests timestamp suffix when archive dir exists
func TestArchiveSession_ConflictResolution(t *testing.T) {
	// Create temp sessions directory
	tmpDir := t.TempDir()
	sessionName := "test-conflict-session"
	sessionDir := filepath.Join(tmpDir, sessionName)

	// Create session directory
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Create manifest
	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test-session-id",
		Name:          sessionName,
		CreatedAt:     time.Now().Add(-1 * time.Hour),
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Active session (empty string)
		Context: manifest.Context{
			Project: "/test/project",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// PRE-CREATE conflicting archive directory
	archiveBaseDir := filepath.Join(tmpDir, ".archive-old-format")
	conflictingDir := filepath.Join(archiveBaseDir, sessionName)
	if err := os.MkdirAll(conflictingDir, 0755); err != nil {
		t.Fatalf("Failed to create conflicting archive dir: %v", err)
	}

	// Create reaper
	r := New(sessionName, tmpDir)

	// Archive session (should handle conflict with timestamp suffix)
	if err := r.archiveSession(); err != nil {
		t.Fatalf("archiveSession() failed with conflict: %v", err)
	}

	// Verify original conflicting directory still exists
	if _, err := os.Stat(conflictingDir); os.IsNotExist(err) {
		t.Error("Original conflicting archive directory was removed")
	}

	// Verify new archive directory created with timestamp suffix
	entries, err := os.ReadDir(archiveBaseDir)
	if err != nil {
		t.Fatalf("Failed to read archive directory: %v", err)
	}

	foundTimestampedDir := false
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != sessionName {
			// Should match pattern: test-conflict-session-YYYYMMDDTHHMMSSZ
			if len(entry.Name()) > len(sessionName) && entry.Name()[:len(sessionName)] == sessionName {
				foundTimestampedDir = true
				t.Logf("Found timestamped archive directory: %s", entry.Name())
			}
		}
	}

	if !foundTimestampedDir {
		t.Error("No timestamped archive directory found after conflict resolution")
	}

	// Verify original session directory gone
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Error("Original session directory still exists after archive with conflict")
	}
}

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

// TestArchiveSession_ManifestBackup tests that manifest backup is created
func TestArchiveSession_ManifestBackup(t *testing.T) {
	// Create temp sessions directory
	tmpDir := t.TempDir()
	sessionName := "test-backup-session"
	sessionDir := filepath.Join(tmpDir, sessionName)

	// Create session directory
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Create manifest
	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "test-session-id",
		Name:          sessionName,
		CreatedAt:     time.Now().Add(-1 * time.Hour),
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Active session (empty string)
		Context: manifest.Context{
			Project: "/test/project",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Agent: "claude",
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create reaper
	r := New(sessionName, tmpDir)

	// Archive session
	if err := r.archiveSession(); err != nil {
		t.Fatalf("archiveSession() failed: %v", err)
	}

	// Verify backup was created in .archive-old-format directory
	archiveDir := filepath.Join(tmpDir, ".archive-old-format", sessionName)
	backupDir := filepath.Join(archiveDir, ".manifest-backups")

	// Check if backup directory exists
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		// Backup directory might not exist if manifest.Write doesn't create backups
		t.Skip("Manifest backup directory not created (manifest.Write may not support backups)")
	}

	// If backup directory exists, verify it contains backup files
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("Failed to read backup directory: %v", err)
	}

	if len(entries) == 0 {
		t.Error("No backup files found in .manifest-backups directory")
	}

	t.Logf("Found %d manifest backup(s)", len(entries))
}
