package regression_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestArchiveStoppedSessionStorageContract is a regression test for the bug where
// STOPPED sessions visible in `agm session list` could not be archived.
//
// Bug Description:
// - agm session list used Dolt storage (via getStorage())
// - agm session archive used filesystem storage (via session.ResolveIdentifier())
// - Sessions existed in Dolt but archive command searched filesystem
// - Result: "session not found" error when trying to archive STOPPED sessions
//
// Fix:
// - Added ResolveIdentifier() to Dolt adapter
// - Migrated archive command to use Dolt storage
// - Both commands now use same storage backend
//
// This test exercises the backend-neutral adapter contract that fixed the
// historical Dolt/filesystem mismatch. It ensures:
// 1. Sessions stored through the adapter can be found by ResolveIdentifier
// 2. Archive behavior persists through the same adapter
// 3. Archived sessions are excluded from subsequent identifier resolution
func TestArchiveStoppedSessionStorageContract(t *testing.T) {
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	// Create a test session through the shared storage contract.
	sessionID := "regression-test-" + time.Now().Format("20060102-150405")
	session := &manifest.Manifest{
		SessionID:     sessionID,
		Name:          "stopped-session-test",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude",
		Lifecycle:     "", // Active, not archived
		Context: manifest.Context{
			Project: "/tmp/test",
			Purpose: "Regression test for archive bug",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid-regression",
		},
		Tmux: manifest.Tmux{
			SessionName: "stopped-tmux-test",
		},
	}

	// Insert the session through the adapter.
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Test Case 1: ResolveIdentifier should find the session by session ID
	t.Run("ResolveBySessionID", func(t *testing.T) {
		resolved, err := adapter.ResolveIdentifier(sessionID)
		if err != nil {
			t.Fatalf("ResolveIdentifier failed for session ID: %v", err)
		}
		if resolved.SessionID != sessionID {
			t.Errorf("Expected session ID %s, got %s", sessionID, resolved.SessionID)
		}
	})

	// Test Case 2: ResolveIdentifier should find the session by tmux name
	t.Run("ResolveByTmuxName", func(t *testing.T) {
		resolved, err := adapter.ResolveIdentifier("stopped-tmux-test")
		if err != nil {
			t.Fatalf("ResolveIdentifier failed for tmux name: %v", err)
		}
		if resolved.SessionID != sessionID {
			t.Errorf("Expected session ID %s, got %s", sessionID, resolved.SessionID)
		}
	})

	// Test Case 3: ResolveIdentifier should find the session by manifest name
	t.Run("ResolveByManifestName", func(t *testing.T) {
		resolved, err := adapter.ResolveIdentifier("stopped-session-test")
		if err != nil {
			t.Fatalf("ResolveIdentifier failed for manifest name: %v", err)
		}
		if resolved.SessionID != sessionID {
			t.Errorf("Expected session ID %s, got %s", sessionID, resolved.SessionID)
		}
	})

	// Test Case 4: Archive the session (simulating the original bug scenario)
	t.Run("ArchiveStoppedSession", func(t *testing.T) {
		// Resolve the session (this is what archive command does)
		resolved, err := adapter.ResolveIdentifier("stopped-session-test")
		if err != nil {
			t.Fatalf("Failed to resolve session before archive: %v", err)
		}

		// Archive it (update lifecycle)
		resolved.Lifecycle = manifest.LifecycleArchived
		if err := adapter.UpdateSession(resolved); err != nil {
			t.Fatalf("Failed to archive session: %v", err)
		}

		// Verify it's archived
		archived, err := adapter.GetSession(sessionID)
		if err != nil {
			t.Fatalf("Failed to get archived session: %v", err)
		}
		if archived.Lifecycle != manifest.LifecycleArchived {
			t.Errorf("Expected lifecycle 'archived', got '%s'", archived.Lifecycle)
		}
	})

	// Test Case 5: ResolveIdentifier should NOT find archived sessions
	// This is the key behavioral test - prevents re-archiving
	t.Run("ArchivedSessionNotResolvable", func(t *testing.T) {
		// Try to resolve the now-archived session
		_, err := adapter.ResolveIdentifier("stopped-session-test")
		if err == nil {
			t.Fatal("ResolveIdentifier should not find archived sessions")
			return
		}
		expectedError := "session not found: stopped-session-test"
		if err.Error() != expectedError {
			t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
		}
	})
}

// TestResolveIdentifierNotFoundContract verifies the shared adapter result
// that archive operations receive for an unknown session identifier.
func TestResolveIdentifierNotFoundContract(t *testing.T) {
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	// Verify adapter has ResolveIdentifier method
	// If this compiles, the method exists
	_, err = adapter.ResolveIdentifier("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent session")
		return
	}

	// The shared adapter error contract should remain stable.
	if err.Error() != "session not found: nonexistent" {
		t.Errorf("Unexpected error format: %v", err)
	}
}
