package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func TestRestoreArchivedSessionSerializesWithArchiveCleanup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	previousForce := unarchiveForce
	previousConfig := cfg
	unarchiveForce = true
	cfg = &config.Config{SessionsDir: t.TempDir()}
	t.Cleanup(func() {
		unarchiveForce = previousForce
		cfg = previousConfig
	})

	storage := dolt.NewMockAdapter()
	sessionID := "unarchive-archive-lock-id"
	name := "unarchive-archive-lock"
	if err := storage.CreateSession(&manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          name,
		Lifecycle:     manifest.LifecycleArchived,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	archived := &session.ArchivedSession{
		SessionID:    sessionID,
		Name:         name,
		ArchivedAt:   time.Now().UTC().Format(time.RFC3339),
		ManifestPath: filepath.Join(t.TempDir(), "manifest.yaml"),
	}

	unarchiveDone := make(chan error, 1)
	err := ops.WithSessionLock(sessionID, func() error {
		go func() {
			unarchiveDone <- restoreArchivedSession(storage, archived)
		}()
		select {
		case earlyErr := <-unarchiveDone:
			return fmt.Errorf("unarchive crossed archive cleanup lock: %w", earlyErr)
		case <-time.After(100 * time.Millisecond):
		}
		current, getErr := storage.GetSession(sessionID)
		if getErr != nil {
			return getErr
		}
		if current.Lifecycle != manifest.LifecycleArchived {
			return fmt.Errorf("lifecycle changed while archive cleanup lock was held: %q", current.Lifecycle)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("hold archive cleanup lock: %v", err)
	}

	select {
	case unarchiveErr := <-unarchiveDone:
		if unarchiveErr != nil {
			t.Fatalf("restoreArchivedSession() error: %v", unarchiveErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("unarchive did not continue after archive cleanup lock released")
	}
	current, err := storage.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if current.Lifecycle != "" {
		t.Fatalf("restored lifecycle = %q, want active", current.Lifecycle)
	}
}
