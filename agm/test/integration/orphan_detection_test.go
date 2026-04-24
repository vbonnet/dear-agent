package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/orphan"
)

// TestOrphanDetectionIntegration performs end-to-end orphan detection
// using test database instead of YAML files
func TestOrphanDetectionIntegration(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create temporary sessions directory (not used for Dolt tests, but required by API)
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("Failed to create sessions dir: %v", err)
	}

	t.Run("DetectOrphans_NoWorkspaceFilter", func(t *testing.T) {
		// Insert 2 test manifests into database
		insertTestManifests(t, adapter, 2)

		// Run orphan detection with test adapter
		report, err := orphan.DetectOrphansWithAdapter(sessionsDir, "", adapter)
		if err != nil {
			t.Fatalf("Failed to detect orphans: %v", err)
		}

		// Should find 2 manifests
		if report.ManifestsFound != 2 {
			t.Errorf("Expected 2 manifests, found %d", report.ManifestsFound)
		}

		// Check report structure
		if report.ScanStarted.IsZero() {
			t.Error("Expected ScanStarted to be set")
		}
		if report.ScanCompleted.IsZero() {
			t.Error("Expected ScanCompleted to be set")
		}
		if report.ScanCompleted.Before(report.ScanStarted) {
			t.Error("ScanCompleted should be after ScanStarted")
		}
	})

	t.Run("DetectOrphans_WithWorkspaceFilter", func(t *testing.T) {
		// Clean up any existing test sessions from previous runs
		cleanupTestSessions(t, adapter)

		// Insert 2 test manifests into database
		insertTestManifests(t, adapter, 2)

		// Test workspace filtering
		report, err := orphan.DetectOrphansWithAdapter(sessionsDir, "oss", adapter)
		if err != nil {
			t.Fatalf("Failed to detect orphans with workspace filter: %v", err)
		}

		// Should find 2 manifests (both are in 'oss' workspace)
		if report.ManifestsFound != 2 {
			t.Errorf("Expected 2 manifests, found %d", report.ManifestsFound)
		}

		// All orphans should be from oss workspace
		for _, o := range report.Orphans {
			if o.Workspace != "oss" && o.Workspace != "" {
				t.Errorf("Expected orphan workspace to be 'oss' or empty, got %q", o.Workspace)
			}
		}
	})
}

// TestOrphanDetection_EmptySessionsDir validates behavior with no sessions
func TestOrphanDetection_EmptySessionsDir(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("Failed to create sessions dir: %v", err)
	}

	// Run with test adapter (empty database)
	report, err := orphan.DetectOrphansWithAdapter(sessionsDir, "", adapter)
	if err != nil {
		t.Fatalf("Failed to detect orphans in empty dir: %v", err)
	}

	// Should find 0 manifests
	if report.ManifestsFound != 0 {
		t.Errorf("Expected 0 manifests in empty dir, found %d", report.ManifestsFound)
	}
}

// TestOrphanDetection_NonexistentSessionsDir validates error handling
func TestOrphanDetection_NonexistentSessionsDir(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "nonexistent")

	// Run with test adapter (empty database, nonexistent sessions dir)
	report, err := orphan.DetectOrphansWithAdapter(sessionsDir, "", adapter)
	if err != nil {
		t.Fatalf("Should handle nonexistent dir gracefully: %v", err)
	}

	// Should find 0 manifests (directory doesn't exist, database empty)
	if report.ManifestsFound != 0 {
		t.Errorf("Expected 0 manifests for nonexistent dir, found %d", report.ManifestsFound)
	}
}

// insertTestManifests inserts test manifests into the Dolt test database
func cleanupTestSessions(t *testing.T, adapter *dolt.Adapter) {
	t.Helper()

	// Clean up test sessions to prevent database pollution
	for i := 1; i <= 10; i++ { // Clean up to 10 potential test sessions
		_ = adapter.DeleteSession(fmt.Sprintf("test-session-%d", i))
	}
}

func insertTestManifests(t *testing.T, adapter *dolt.Adapter, count int) {
	t.Helper()

	for i := 0; i < count; i++ {
		m := &manifest.Manifest{
			SessionID:     fmt.Sprintf("test-session-%d", i+1),
			Name:          fmt.Sprintf("test-session-%d", i+1),
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Lifecycle:     "", // Active
			Workspace:     "oss",
			Context: manifest.Context{
				Project: "/tmp/test",
				Purpose: "Integration test",
			},
			Claude: manifest.Claude{
				UUID: fmt.Sprintf("test-uuid-%d", i+1),
			},
			Tmux: manifest.Tmux{
				SessionName: fmt.Sprintf("test-tmux-%d", i+1),
			},
		}

		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("Failed to insert test manifest %d: %v", i+1, err)
		}
	}
}
