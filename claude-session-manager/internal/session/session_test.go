package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

func TestFindArchived_InvalidPattern(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := FindArchived(tmpDir, "[invalid")
	if err == nil {
		t.Error("expected error for invalid glob pattern")
	}
}

func TestFindArchived_NoArchivedSessions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an active (non-archived) session
	createTestSession(t, tmpDir, "active-session", "")

	results, err := FindArchived(tmpDir, "*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 archived sessions, got %d", len(results))
	}
}

func TestFindArchived_SingleMatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create archived session
	createTestSession(t, tmpDir, "archived-session", manifest.LifecycleArchived)

	results, err := FindArchived(tmpDir, "archived-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "archived-session" {
		t.Errorf("expected name 'archived-session', got '%s'", results[0].Name)
	}
}

func TestFindArchived_WildcardPattern(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple archived sessions
	createTestSession(t, tmpDir, "[REDACTED_EMPLOYER]-session-1", manifest.LifecycleArchived)
	createTestSession(t, tmpDir, "[REDACTED_EMPLOYER]-session-2", manifest.LifecycleArchived)
	createTestSession(t, tmpDir, "other-session", manifest.LifecycleArchived)

	results, err := FindArchived(tmpDir, "*[REDACTED_EMPLOYER]*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Check both [REDACTED_EMPLOYER] sessions are returned
	names := make(map[string]bool)
	for _, r := range results {
		names[r.Name] = true
	}
	if !names["[REDACTED_EMPLOYER]-session-1"] || !names["[REDACTED_EMPLOYER]-session-2"] {
		t.Error("expected both [REDACTED_EMPLOYER] sessions in results")
	}
}

func TestFindArchived_QuestionMarkPattern(t *testing.T) {
	tmpDir := t.TempDir()

	createTestSession(t, tmpDir, "session-2024-01", manifest.LifecycleArchived)
	createTestSession(t, tmpDir, "session-2025-01", manifest.LifecycleArchived)
	createTestSession(t, tmpDir, "session-2026-01", manifest.LifecycleArchived)

	results, err := FindArchived(tmpDir, "session-202?-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should match session-2024-01 and session-2025-01 and session-2026-01
	if len(results) != 3 {
		t.Errorf("expected 3 results with ? pattern, got %d", len(results))
	}
}

func TestFindArchived_NoMatches(t *testing.T) {
	tmpDir := t.TempDir()

	createTestSession(t, tmpDir, "archived-session", manifest.LifecycleArchived)

	results, err := FindArchived(tmpDir, "*nonexistent*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching pattern, got %d", len(results))
	}
}

func TestFindArchived_WithTags(t *testing.T) {
	tmpDir := t.TempDir()

	// Create session with tags
	sessionDir := filepath.Join(tmpDir, "tagged-session")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "tagged-session",
		Name:          "tagged-session",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     manifest.LifecycleArchived,
		Context: manifest.Context{
			Project: "/tmp/test",
			Tags:    []string{"tag1", "tag2"},
		},
		Tmux: manifest.Tmux{
			SessionName: "tagged-session",
		},
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatal(err)
	}

	results, err := FindArchived(tmpDir, "*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(results[0].Tags))
	}
}

func TestFindArchived_SortedByDate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create sessions with different update times (days apart to ensure different date strings)
	createTestSessionWithTime(t, tmpDir, "old-session", manifest.LifecycleArchived, time.Now().Add(-72*time.Hour))
	createTestSessionWithTime(t, tmpDir, "mid-session", manifest.LifecycleArchived, time.Now().Add(-48*time.Hour))
	createTestSessionWithTime(t, tmpDir, "new-session", manifest.LifecycleArchived, time.Now())

	results, err := FindArchived(tmpDir, "*session*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Should be sorted by date descending (most recent first)
	// Compare formatted dates
	if results[0].ArchivedAt < results[1].ArchivedAt {
		t.Error("results not sorted by date descending")
	}
	if results[1].ArchivedAt < results[2].ArchivedAt {
		t.Error("results not sorted by date descending")
	}
}

// Helper functions

func createTestSession(t *testing.T, dir, name, lifecycle string) {
	t.Helper()
	createTestSessionWithTime(t, dir, name, lifecycle, time.Now())
}

func createTestSessionWithTime(t *testing.T, dir, name, lifecycle string, updatedAt time.Time) {
	t.Helper()

	sessionDir := filepath.Join(dir, name)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     name,
		Name:          name,
		CreatedAt:     time.Now(),
		UpdatedAt:     updatedAt,
		Lifecycle:     lifecycle,
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: name,
		},
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatal(err)
	}
}
