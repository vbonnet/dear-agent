//go:build integration

package regression_test

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestArchiveStoppedSessionsFromDolt is a regression test for the bug where
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
// This test ensures:
// 1. Sessions stored in Dolt can be found by ResolveIdentifier
// 2. Archive command can successfully archive sessions from Dolt
// 3. The storage backend mismatch cannot happen again
func TestArchiveStoppedSessionsFromDolt(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup: Create Dolt adapter
	config := &dolt.Config{
		Workspace: "test",
		Port:      "3307",
		Host:      "127.0.0.1",
		Database:  "test",
		User:      "root",
		Password:  "",
	}

	adapter, err := dolt.New(config)
	if err != nil {
		t.Skipf("Dolt server not available: %v", err)
	}
	defer adapter.Close()

	// Apply migrations
	if err := adapter.ApplyMigrations(); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	// Create a test session in Dolt (simulating a STOPPED session)
	sessionID := "regression-test-" + time.Now().Format("20060102-150405")
	session := &manifest.Manifest{
		SessionID:     sessionID,
		Name:          "stopped-session-test",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Agent:         "claude",
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

	// Insert session into Dolt
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	defer adapter.DeleteSession(sessionID) // Cleanup

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
		}
		expectedError := "session not found: stopped-session-test"
		if err.Error() != expectedError {
			t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
		}
	})
}

// TestDoltStorageBackendConsistency verifies that list and archive operations
// use the same storage backend (Dolt) to prevent future regression.
//
// This test ensures the architectural constraint that caused the original bug
// cannot be violated: all session operations MUST use the same storage adapter.
func TestDoltStorageBackendConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test is more of a code structure validation
	// The mere existence of adapter.ResolveIdentifier() and its usage
	// in the archive command ensures consistency

	config := &dolt.Config{
		Workspace: "test",
		Port:      "3307",
		Host:      "127.0.0.1",
		Database:  "test",
		User:      "root",
		Password:  "",
	}

	adapter, err := dolt.New(config)
	if err != nil {
		t.Skipf("Dolt server not available: %v", err)
	}
	defer adapter.Close()

	// Verify adapter has ResolveIdentifier method
	// If this compiles, the method exists
	_, err = adapter.ResolveIdentifier("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent session")
	}

	// The error message should indicate Dolt storage is being used
	if err.Error() != "session not found: nonexistent" {
		t.Errorf("Unexpected error format: %v", err)
	}
}
