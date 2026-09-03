package dolt

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
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

func TestReleasedMigrationChecksumsImmutable(t *testing.T) {
	want := []struct {
		version  int
		name     string
		checksum string
	}{
		{1, "initial_schema", "5750a28848c941504990952cbcd290aa2d50fca8ba8e022b4992f1b5e91cd81f"},
		{2, "messages_table", "8184630b6383da6be728aaaf3a979bd4c83b1cf7c7f8cd4b92af768dc5198058"},
		{3, "add_tool_calls", "3be8628fddeb855d333ecc65f3302b9875926d663f6ace0731884d9da724a80b"},
		{4, "add_session_tags", "82fc45509de72763dc7212b9ef158d20091dbe8ed738187b06ca03b7952c3bd9"},
		{5, "add_message_embeddings", "3268230d1d134e65f0691b2ba5522a109dc5d630bc4688c760138894f5e99e53"},
		{6, "add_performance_indexes", "a0ef519d4a84ee099bf4ef8d8942b7b7b1576be1235220587ea665d1903d63ca"},
		{7, "add_session_hierarchy", "b828589f062c10c90ccce278b43330723f91e886408ca8998f88ffebc37ed33e"},
		{8, "add_permission_mode", "35b79c502ab10d4a05b2d26d1e30711666969c0b94aebce3869b5e0dff46319c"},
		{9, "rename_agent_to_harness", "8b690fa80c3bd3c4207ca5402b7378c5d11f75017208c77b48f579363777dcf4"},
		{10, "add_is_test", "20087277dc4f363bd688637f7b88e34cdf94ccafebe5b4f299f015771c542d48"},
		{11, "add_worktree_tracking", "1b492fa5c9e73f943f53ef86eeecbcb57a8051da2d065003c9b536a1ebd32646"},
		{12, "add_frecency", "7eaa558df9953349a50d33a3fa3d98bb32adb3c695c7bc7923e29d2e27805a0a"},
		{13, "add_context_usage", "4e2cec27977d2c5f1f003349b22a1e0ed55ab8d6af682be1bd0b8211665c4ced"},
		{14, "add_monitors", "2c723837e4dd2ce591cac7328d92a6abfb2d74df733c1c2438489587a1aee63c"},
		{15, "session_logs", "4de5ae38f1c20898a51972c22a714ad7ab1922b0c0db5656b59ce067574f3c37"},
		{16, "artifacts", "16b4548e0cdfabaaa9d9d593b66d4075f69a3ff63be0aad4c104e957801cf6cc"},
		{17, "harness_history", "44800fcebd4819c44741a4917871d74ec7b684cdf4983e4503323f282d6e30a0"},
		{18, "add_tmux_session_revision", "7e5a34ac2de460064d386f10dbf56c6ffd60364e27b90598e265e597596eb29e"},
		{19, "session_name_reservations", "4ba36ac5d7a644dfbaf00d325de724ffd8657bb8f0dd16e6e4b3403ff7764890"},
	}

	migrations := AllMigrations()
	if len(migrations) != len(want) {
		t.Fatalf("AllMigrations() returned %d entries, want %d; append new migrations without editing released entries", len(migrations), len(want))
	}
	for index, expected := range want {
		migration := migrations[index]
		if migration.Version != expected.version {
			t.Errorf("migration at index %d has version %d, want %d; append new migrations without reordering released entries", index, migration.Version, expected.version)
		}
		if migration.Name != expected.name {
			t.Errorf("migration %03d name = %q, want %q; append a new migration instead of renaming a released entry", expected.version, migration.Name, expected.name)
		}
		if migration.Checksum != expected.checksum {
			t.Errorf("migration %03d stored checksum = %q, want %q; append a new migration instead of editing released SQL", expected.version, migration.Checksum, expected.checksum)
		}
		if checksum := computeChecksum(migration.SQL); checksum != expected.checksum {
			t.Errorf("migration %03d SQL checksum = %q, want %q; append a new migration instead of editing released SQL", expected.version, checksum, expected.checksum)
		}
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

func TestMigration018AddsTmuxSessionRevision(t *testing.T) {
	for _, migration := range AllMigrations() {
		if migration.Version != 18 {
			continue
		}
		if migration.Name != "add_tmux_session_revision" {
			t.Fatalf("migration 018 name = %q, want add_tmux_session_revision", migration.Name)
		}
		if !strings.Contains(migration.SQL, "ADD COLUMN tmux_session_revision VARCHAR(64) NULL") {
			t.Fatalf("migration 018 SQL does not add the nullable ownership revision: %q", migration.SQL)
		}
		if !strings.Contains(migration.PreConditionSQL, "COLUMN_NAME = 'tmux_session_revision'") {
			t.Fatalf("migration 018 precondition does not guard the ownership revision: %q", migration.PreConditionSQL)
		}
		if migration.Checksum == "" {
			t.Fatal("migration 018 checksum is empty")
		}
		return
	}
	t.Fatal("migration 018 not found")
}

func TestMigration019AddsSessionNameReservations(t *testing.T) {
	var found Migration
	for _, migration := range AllMigrations() {
		if migration.Version == 19 {
			found = migration
			break
		}
	}
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS agm_session_name_reservations",
		"PRIMARY KEY (workspace, name)",
		"UNIQUE KEY uq_agm_session_name_reservation_owner",
		"expires_at",
	} {
		if !strings.Contains(found.SQL, required) {
			t.Fatalf("migration 019 lacks %q", required)
		}
	}
	if !strings.Contains(found.PreConditionSQL, "TABLE_NAME = 'agm_session_name_reservations'") {
		t.Fatalf("migration 019 precondition does not guard the reservation table: %q", found.PreConditionSQL)
	}
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
