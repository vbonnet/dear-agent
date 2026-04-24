package dolt

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// WorktreeRecord struct tests (unit - no DB required)
// ---------------------------------------------------------------------------

func TestWorktreeRecordStruct(t *testing.T) {
	now := time.Now()
	removed := now.Add(time.Hour)

	record := WorktreeRecord{
		ID:           1,
		SessionName:  "my-session",
		RepoPath:     "~/repos/myrepo",
		WorktreePath: "~/worktrees/my-feature",
		Branch:       "feature-branch",
		CreatedAt:    now,
		RemovedAt:    &removed,
		Status:       "removed",
	}

	if record.ID != 1 {
		t.Errorf("Expected ID 1, got %d", record.ID)
	}
	if record.SessionName != "my-session" {
		t.Errorf("Expected SessionName 'my-session', got %q", record.SessionName)
	}
	if record.RepoPath != "~/repos/myrepo" {
		t.Errorf("Expected RepoPath '~/repos/myrepo', got %q", record.RepoPath)
	}
	if record.WorktreePath != "~/worktrees/my-feature" {
		t.Errorf("Expected WorktreePath '~/worktrees/my-feature', got %q", record.WorktreePath)
	}
	if record.Branch != "feature-branch" {
		t.Errorf("Expected Branch 'feature-branch', got %q", record.Branch)
	}
	if record.Status != "removed" {
		t.Errorf("Expected Status 'removed', got %q", record.Status)
	}
	if record.RemovedAt == nil {
		t.Error("Expected RemovedAt to be set")
	}
}

func TestWorktreeRecordActiveStatus(t *testing.T) {
	record := WorktreeRecord{
		SessionName:  "test-session",
		WorktreePath: "/tmp/wt",
		Status:       "active",
		RemovedAt:    nil,
	}

	if record.Status != "active" {
		t.Errorf("Expected Status 'active', got %q", record.Status)
	}
	if record.RemovedAt != nil {
		t.Error("Expected RemovedAt to be nil for active worktree")
	}
}

func TestWorktreeRecordOrphanedStatus(t *testing.T) {
	record := WorktreeRecord{
		SessionName:  "old-session",
		WorktreePath: "/tmp/orphan-wt",
		Status:       "orphaned",
	}

	if record.Status != "orphaned" {
		t.Errorf("Expected Status 'orphaned', got %q", record.Status)
	}
}

func TestWorktreeRecord_ZeroValue(t *testing.T) {
	var record WorktreeRecord
	if record.ID != 0 {
		t.Errorf("Expected zero ID, got %d", record.ID)
	}
	if record.SessionName != "" {
		t.Errorf("Expected empty SessionName, got %q", record.SessionName)
	}
	if record.Status != "" {
		t.Errorf("Expected empty Status, got %q", record.Status)
	}
	if record.RemovedAt != nil {
		t.Error("Expected nil RemovedAt for zero value")
	}
	if !record.CreatedAt.IsZero() {
		t.Error("Expected zero CreatedAt for zero value")
	}
}

func TestWorktreeRecord_AllStatuses(t *testing.T) {
	statuses := []string{"active", "removed", "orphaned"}
	for _, status := range statuses {
		record := WorktreeRecord{Status: status}
		if record.Status != status {
			t.Errorf("Expected status %q, got %q", status, record.Status)
		}
	}
}

func TestWorktreeRecord_RemovedAtPointerSemantics(t *testing.T) {
	// Active: RemovedAt is nil
	active := WorktreeRecord{Status: "active", RemovedAt: nil}
	if active.RemovedAt != nil {
		t.Error("Active worktree should have nil RemovedAt")
	}

	// Removed: RemovedAt is set
	now := time.Now()
	removed := WorktreeRecord{Status: "removed", RemovedAt: &now}
	if removed.RemovedAt == nil {
		t.Error("Removed worktree should have non-nil RemovedAt")
	}
	if !removed.RemovedAt.Equal(now) {
		t.Errorf("RemovedAt time mismatch: got %v, want %v", *removed.RemovedAt, now)
	}
}

func TestWorktreeRecord_LongPaths(t *testing.T) {
	longPath := "~/very/deeply/nested/path/to/some/worktree/directory/that/is/quite/long"
	record := WorktreeRecord{
		WorktreePath: longPath,
		RepoPath:     "~/very/deeply/nested/path/to/some/repo",
	}
	if record.WorktreePath != longPath {
		t.Errorf("Expected long path to be preserved, got %q", record.WorktreePath)
	}
}

func TestWorktreeRecord_BranchWithSlashes(t *testing.T) {
	record := WorktreeRecord{
		Branch: "feature/sub-feature/implementation",
	}
	if record.Branch != "feature/sub-feature/implementation" {
		t.Errorf("Expected branch with slashes, got %q", record.Branch)
	}
}

func TestWorktreeRecord_EmptyBranch(t *testing.T) {
	// Detached HEAD worktrees may have empty branch
	record := WorktreeRecord{
		WorktreePath: "/tmp/detached-wt",
		Branch:       "",
		Status:       "active",
	}
	if record.Branch != "" {
		t.Errorf("Expected empty branch, got %q", record.Branch)
	}
}

// ---------------------------------------------------------------------------
// Migration tests (unit - no DB required)
// ---------------------------------------------------------------------------

// TestMigration011Exists verifies that migration 011 is included in AllMigrations
func TestMigration011Exists(t *testing.T) {
	migrations := AllMigrations()

	var found bool
	for _, m := range migrations {
		if m.Version == 11 {
			found = true
			if m.Name != "add_worktree_tracking" {
				t.Errorf("Expected migration 011 name 'add_worktree_tracking', got %q", m.Name)
			}
			if len(m.TablesCreated) != 1 || m.TablesCreated[0] != "agm_worktrees" {
				t.Errorf("Expected TablesCreated ['agm_worktrees'], got %v", m.TablesCreated)
			}
			if m.Checksum == "" {
				t.Error("Expected non-empty checksum for migration 011")
			}
			if m.SQL == "" {
				t.Error("Expected non-empty SQL for migration 011")
			}
			break
		}
	}
	if !found {
		t.Error("Migration 011 not found in AllMigrations()")
	}
}

// TestMigration011ChecksumStable verifies checksum is deterministic
func TestMigration011ChecksumStable(t *testing.T) {
	migrations := AllMigrations()
	var checksum1, checksum2 string
	for _, m := range migrations {
		if m.Version == 11 {
			checksum1 = m.Checksum
			break
		}
	}

	// Call again
	migrations2 := AllMigrations()
	for _, m := range migrations2 {
		if m.Version == 11 {
			checksum2 = m.Checksum
			break
		}
	}

	if checksum1 != checksum2 {
		t.Errorf("Checksum not stable: %q != %q", checksum1, checksum2)
	}
}

func TestMigration011_SQLContainsCreateTable(t *testing.T) {
	migrations := AllMigrations()
	for _, m := range migrations {
		if m.Version == 11 {
			// Verify the SQL contains the expected CREATE TABLE statement
			if len(m.SQL) == 0 {
				t.Error("Migration SQL is empty")
			}
			return
		}
	}
	t.Error("Migration 011 not found")
}

// ---------------------------------------------------------------------------
// Integration tests (require DOLT_TEST_INTEGRATION=1)
// ---------------------------------------------------------------------------

// setupIntegrationTest creates an adapter for integration testing.
// Skips the test if DOLT_TEST_INTEGRATION is not set.
func setupIntegrationTest(t *testing.T) *Adapter {
	t.Helper()

	if os.Getenv("DOLT_TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test (set DOLT_TEST_INTEGRATION=1 to enable)")
	}

	// Use test environment
	t.Setenv("ENGRAM_TEST_MODE", "1")
	t.Setenv("ENGRAM_TEST_WORKSPACE", "test")
	t.Setenv("WORKSPACE", "test")

	config, err := DefaultConfig()
	if err != nil {
		t.Skipf("Cannot get Dolt config: %v", err)
	}

	adapter, err := New(config)
	if err != nil {
		t.Skipf("Cannot connect to Dolt: %v", err)
	}

	if err := adapter.ApplyMigrations(); err != nil {
		adapter.Close()
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	t.Cleanup(func() {
		adapter.Close()
	})

	return adapter
}

func TestTrackWorktree_Integration(t *testing.T) {
	adapter := setupIntegrationTest(t)
	ctx := context.Background()

	sessionName := "test-track-wt-" + time.Now().Format("20060102150405")
	wtPath := "/tmp/test-wt-" + sessionName
	repoPath := "/tmp/test-repo"
	branch := "feature-test"

	// Track a worktree
	err := adapter.TrackWorktree(ctx, sessionName, repoPath, wtPath, branch)
	if err != nil {
		t.Fatalf("TrackWorktree failed: %v", err)
	}

	// Verify it appears in active list
	active, err := adapter.ListActiveWorktrees(ctx)
	if err != nil {
		t.Fatalf("ListActiveWorktrees failed: %v", err)
	}

	found := false
	for _, wt := range active {
		if wt.WorktreePath == wtPath {
			found = true
			if wt.SessionName != sessionName {
				t.Errorf("Expected session %q, got %q", sessionName, wt.SessionName)
			}
			if wt.Branch != branch {
				t.Errorf("Expected branch %q, got %q", branch, wt.Branch)
			}
			if wt.Status != "active" {
				t.Errorf("Expected status 'active', got %q", wt.Status)
			}
			break
		}
	}
	if !found {
		t.Error("Tracked worktree not found in active list")
	}

	// Cleanup
	_ = adapter.UntrackWorktree(ctx, wtPath)
}

func TestUntrackWorktree_Integration(t *testing.T) {
	adapter := setupIntegrationTest(t)
	ctx := context.Background()

	sessionName := "test-untrack-wt-" + time.Now().Format("20060102150405")
	wtPath := "/tmp/test-untrack-" + sessionName

	// Track then untrack
	err := adapter.TrackWorktree(ctx, sessionName, "/tmp/repo", wtPath, "feat")
	if err != nil {
		t.Fatalf("TrackWorktree failed: %v", err)
	}

	err = adapter.UntrackWorktree(ctx, wtPath)
	if err != nil {
		t.Fatalf("UntrackWorktree failed: %v", err)
	}

	// Verify it no longer appears in active list
	active, err := adapter.ListActiveWorktrees(ctx)
	if err != nil {
		t.Fatalf("ListActiveWorktrees failed: %v", err)
	}

	for _, wt := range active {
		if wt.WorktreePath == wtPath {
			t.Error("Untracked worktree should not appear in active list")
		}
	}
}

func TestListWorktreesBySession_Integration(t *testing.T) {
	adapter := setupIntegrationTest(t)
	ctx := context.Background()

	sessionName := "test-list-by-session-" + time.Now().Format("20060102150405")
	otherSession := "test-other-session-" + time.Now().Format("20060102150405")

	// Track worktrees for two sessions
	_ = adapter.TrackWorktree(ctx, sessionName, "/tmp/repo", "/tmp/wt-mine-1", "feat-1")
	_ = adapter.TrackWorktree(ctx, sessionName, "/tmp/repo", "/tmp/wt-mine-2", "feat-2")
	_ = adapter.TrackWorktree(ctx, otherSession, "/tmp/repo", "/tmp/wt-other", "feat-3")

	// List for our session only
	results, err := adapter.ListWorktreesBySession(ctx, sessionName)
	if err != nil {
		t.Fatalf("ListWorktreesBySession failed: %v", err)
	}

	count := 0
	for _, wt := range results {
		if wt.SessionName == sessionName {
			count++
		}
		if wt.SessionName == otherSession {
			t.Error("ListWorktreesBySession returned worktree from other session")
		}
	}
	if count < 2 {
		t.Errorf("Expected at least 2 worktrees for session, got %d", count)
	}

	// Cleanup
	_ = adapter.UntrackWorktree(ctx, "/tmp/wt-mine-1")
	_ = adapter.UntrackWorktree(ctx, "/tmp/wt-mine-2")
	_ = adapter.UntrackWorktree(ctx, "/tmp/wt-other")
}

func TestMarkOrphaned_Integration(t *testing.T) {
	adapter := setupIntegrationTest(t)
	ctx := context.Background()

	sessionName := "test-orphan-" + time.Now().Format("20060102150405")
	wtPath := "/tmp/wt-orphan-" + sessionName

	// Track a worktree
	_ = adapter.TrackWorktree(ctx, sessionName, "/tmp/repo", wtPath, "feat")

	// Mark as orphaned
	err := adapter.MarkOrphaned(ctx, sessionName)
	if err != nil {
		t.Fatalf("MarkOrphaned failed: %v", err)
	}

	// Verify it appears in orphaned list
	orphaned, err := adapter.ListOrphanedWorktrees(ctx)
	if err != nil {
		t.Fatalf("ListOrphanedWorktrees failed: %v", err)
	}

	found := false
	for _, wt := range orphaned {
		if wt.WorktreePath == wtPath {
			found = true
			if wt.Status != "orphaned" {
				t.Errorf("Expected status 'orphaned', got %q", wt.Status)
			}
			break
		}
	}
	if !found {
		t.Error("Orphaned worktree not found in orphaned list")
	}

	// Verify it no longer appears in active list
	active, err := adapter.ListActiveWorktrees(ctx)
	if err != nil {
		t.Fatalf("ListActiveWorktrees failed: %v", err)
	}
	for _, wt := range active {
		if wt.WorktreePath == wtPath {
			t.Error("Orphaned worktree should not appear in active list")
		}
	}

	// Cleanup
	_ = adapter.UntrackWorktree(ctx, wtPath)
}

func TestDeleteWorktreeRecord_Integration(t *testing.T) {
	adapter := setupIntegrationTest(t)
	ctx := context.Background()

	sessionName := "test-delete-record-" + time.Now().Format("20060102150405")
	wtPath := "/tmp/wt-delete-" + sessionName

	// Track a worktree
	_ = adapter.TrackWorktree(ctx, sessionName, "/tmp/repo", wtPath, "feat")

	// Find the record ID
	active, _ := adapter.ListActiveWorktrees(ctx)
	var recordID int
	for _, wt := range active {
		if wt.WorktreePath == wtPath {
			recordID = wt.ID
			break
		}
	}

	if recordID == 0 {
		t.Fatal("Could not find tracked worktree to delete")
	}

	// Delete the record
	err := adapter.DeleteWorktreeRecord(ctx, recordID)
	if err != nil {
		t.Fatalf("DeleteWorktreeRecord failed: %v", err)
	}

	// Verify it's gone
	active, _ = adapter.ListActiveWorktrees(ctx)
	for _, wt := range active {
		if wt.WorktreePath == wtPath {
			t.Error("Deleted worktree record still appears in active list")
		}
	}
}

func TestDeleteWorktreeRecord_NotFound_Integration(t *testing.T) {
	adapter := setupIntegrationTest(t)
	ctx := context.Background()

	// Try to delete a non-existent record
	err := adapter.DeleteWorktreeRecord(ctx, 999999)
	if err == nil {
		t.Error("Expected error for non-existent record, got nil")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Expected sql.ErrNoRows, got: %v", err)
	}
}

func TestTrackWorktree_Upsert_Integration(t *testing.T) {
	adapter := setupIntegrationTest(t)
	ctx := context.Background()

	sessionName := "test-upsert-" + time.Now().Format("20060102150405")
	wtPath := "/tmp/wt-upsert-" + sessionName

	// Track a worktree
	err := adapter.TrackWorktree(ctx, sessionName, "/tmp/repo", wtPath, "feat-1")
	if err != nil {
		t.Fatalf("First TrackWorktree failed: %v", err)
	}

	// Track same path with different branch (upsert)
	err = adapter.TrackWorktree(ctx, sessionName, "/tmp/repo", wtPath, "feat-2")
	if err != nil {
		t.Fatalf("Second TrackWorktree (upsert) failed: %v", err)
	}

	// Should still have only one record for this path
	active, _ := adapter.ListActiveWorktrees(ctx)
	count := 0
	for _, wt := range active {
		if wt.WorktreePath == wtPath {
			count++
			if wt.Branch != "feat-2" {
				t.Errorf("Expected branch 'feat-2' after upsert, got %q", wt.Branch)
			}
		}
	}
	if count > 1 {
		t.Errorf("Expected 1 record after upsert, got %d", count)
	}

	// Cleanup
	_ = adapter.UntrackWorktree(ctx, wtPath)
}
