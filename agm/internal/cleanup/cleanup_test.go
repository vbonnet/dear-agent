package cleanup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mockWorktreeStore implements WorktreeStore for testing.
type mockWorktreeStore struct {
	worktrees  []WorktreeRecord
	listErr    error
	untracked  []string
	untrackErr error
}

func (m *mockWorktreeStore) ListWorktreesBySession(_ context.Context, _ string) ([]WorktreeRecord, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.worktrees, nil
}

func (m *mockWorktreeStore) UntrackWorktree(_ context.Context, path string) error {
	if m.untrackErr != nil {
		return m.untrackErr
	}
	m.untracked = append(m.untracked, path)
	return nil
}

// mockGitOps implements GitOps for testing.
type mockGitOps struct {
	removedWorktrees []string
	deletedBranches  []string
	removeErr        error
	deleteErr        error
}

func (m *mockGitOps) RemoveWorktree(_, worktreePath string, _ bool) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.removedWorktrees = append(m.removedWorktrees, worktreePath)
	return nil
}

func (m *mockGitOps) DeleteBranch(_, branchName string, _ bool) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedBranches = append(m.deletedBranches, branchName)
	return nil
}

func TestSessionResources_WorktreeCleanup(t *testing.T) {
	// Create fake worktree directories so os.Stat succeeds
	tmpDir := t.TempDir()
	wt1Path := filepath.Join(tmpDir, "wt1")
	wt2Path := filepath.Join(tmpDir, "wt2")
	os.MkdirAll(wt1Path, 0755)
	os.MkdirAll(wt2Path, 0755)

	store := &mockWorktreeStore{
		worktrees: []WorktreeRecord{
			{WorktreePath: wt1Path, RepoPath: "/repo/a", Branch: "my-session", SessionName: "my-session"},
			{WorktreePath: wt2Path, RepoPath: "/repo/b", Branch: "my-session", SessionName: "my-session"},
		},
	}
	git := &mockGitOps{}

	result := SessionResources(context.Background(), "my-session", store, git, nil)

	if result.WorktreesRemoved != 2 {
		t.Errorf("Expected 2 worktrees removed, got %d", result.WorktreesRemoved)
	}
	if len(git.removedWorktrees) != 2 {
		t.Errorf("Expected 2 git worktree removals, got %d", len(git.removedWorktrees))
	}
	if len(store.untracked) != 2 {
		t.Errorf("Expected 2 untracked worktrees, got %d", len(store.untracked))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %v", result.Errors)
	}
}

func TestSessionResources_WorktreeAlreadyGone(t *testing.T) {
	store := &mockWorktreeStore{
		worktrees: []WorktreeRecord{
			{WorktreePath: "/nonexistent/path/wt1", RepoPath: "/repo/a", Branch: "s", SessionName: "s"},
		},
	}
	git := &mockGitOps{}

	result := SessionResources(context.Background(), "s", store, git, nil)

	// Worktree doesn't exist on disk — should still count as removed and untracked
	if result.WorktreesRemoved != 1 {
		t.Errorf("Expected 1 worktree removed (already gone), got %d", result.WorktreesRemoved)
	}
	if len(git.removedWorktrees) != 0 {
		t.Errorf("Expected 0 git removals (path doesn't exist), got %d", len(git.removedWorktrees))
	}
	if len(store.untracked) != 1 {
		t.Errorf("Expected 1 untracked, got %d", len(store.untracked))
	}
}

func TestSessionResources_BranchCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	wtPath := filepath.Join(tmpDir, "wt")
	os.MkdirAll(wtPath, 0755)

	store := &mockWorktreeStore{
		worktrees: []WorktreeRecord{
			{WorktreePath: wtPath, RepoPath: "/repo/a", Branch: "my-session", SessionName: "my-session"},
		},
	}
	git := &mockGitOps{}

	result := SessionResources(context.Background(), "my-session", store, git, nil)

	if result.BranchesDeleted != 1 {
		t.Errorf("Expected 1 branch deleted, got %d", result.BranchesDeleted)
	}
	if len(git.deletedBranches) != 1 || git.deletedBranches[0] != "my-session" {
		t.Errorf("Expected branch 'my-session' deleted, got %v", git.deletedBranches)
	}
}

func TestSessionResources_BranchCleanupDeduplicatesRepos(t *testing.T) {
	tmpDir := t.TempDir()
	wt1 := filepath.Join(tmpDir, "wt1")
	wt2 := filepath.Join(tmpDir, "wt2")
	os.MkdirAll(wt1, 0755)
	os.MkdirAll(wt2, 0755)

	// Two worktrees in the same repo — should only delete branch once
	store := &mockWorktreeStore{
		worktrees: []WorktreeRecord{
			{WorktreePath: wt1, RepoPath: "/repo/same", Branch: "s", SessionName: "s"},
			{WorktreePath: wt2, RepoPath: "/repo/same", Branch: "s", SessionName: "s"},
		},
	}
	git := &mockGitOps{}

	result := SessionResources(context.Background(), "s", store, git, nil)

	if result.BranchesDeleted != 1 {
		t.Errorf("Expected 1 branch deleted (deduplicated), got %d", result.BranchesDeleted)
	}
}

func TestSessionResources_TmpFileCleanup(t *testing.T) {
	// Create temp files matching the pattern
	tmpDir := os.TempDir()
	sessionName := fmt.Sprintf("test-cleanup-%d", os.Getpid())
	f1 := filepath.Join(tmpDir, "build-"+sessionName+".sh")
	f2 := filepath.Join(tmpDir, "build-"+sessionName+"-extra.log")
	unrelated := filepath.Join(tmpDir, "build-other-session.sh")

	os.WriteFile(f1, []byte("#!/bin/bash"), 0755)
	os.WriteFile(f2, []byte("log"), 0644)
	os.WriteFile(unrelated, []byte("#!/bin/bash"), 0755)
	defer os.Remove(unrelated)

	store := &mockWorktreeStore{} // no worktrees
	git := &mockGitOps{}

	result := SessionResources(context.Background(), sessionName, store, git, nil)

	if result.TmpFilesRemoved != 2 {
		t.Errorf("Expected 2 tmp files removed, got %d", result.TmpFilesRemoved)
	}

	// Verify files are actually gone
	if _, err := os.Stat(f1); !os.IsNotExist(err) {
		t.Errorf("Expected %s to be removed", f1)
	}
	if _, err := os.Stat(f2); !os.IsNotExist(err) {
		t.Errorf("Expected %s to be removed", f2)
	}
	// Unrelated file should still exist
	if _, err := os.Stat(unrelated); os.IsNotExist(err) {
		t.Error("Unrelated tmp file should not be removed")
	}
}

func TestSessionResources_WorktreeRemoveError(t *testing.T) {
	tmpDir := t.TempDir()
	wtPath := filepath.Join(tmpDir, "wt")
	os.MkdirAll(wtPath, 0755)

	store := &mockWorktreeStore{
		worktrees: []WorktreeRecord{
			{WorktreePath: wtPath, RepoPath: "/repo/a", Branch: "s", SessionName: "s"},
		},
	}
	git := &mockGitOps{removeErr: fmt.Errorf("worktree has uncommitted changes")}

	result := SessionResources(context.Background(), "s", store, git, nil)

	if result.WorktreesRemoved != 0 {
		t.Errorf("Expected 0 worktrees removed on error, got %d", result.WorktreesRemoved)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
	// Untrack should still be called even on removal error
	if len(store.untracked) != 1 {
		t.Errorf("Expected worktree to be untracked even on removal error, got %d", len(store.untracked))
	}
}

func TestSessionResources_NilStore(t *testing.T) {
	git := &mockGitOps{}

	// Should not panic with nil store
	result := SessionResources(context.Background(), "s", nil, git, nil)

	if result.WorktreesRemoved != 0 {
		t.Errorf("Expected 0 worktrees removed with nil store, got %d", result.WorktreesRemoved)
	}
	if result.BranchesDeleted != 0 {
		t.Errorf("Expected 0 branches deleted with nil store, got %d", result.BranchesDeleted)
	}
}

func TestSessionResources_ListError(t *testing.T) {
	store := &mockWorktreeStore{listErr: fmt.Errorf("database unavailable")}
	git := &mockGitOps{}

	result := SessionResources(context.Background(), "s", store, git, nil)

	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error from list failure, got %d", len(result.Errors))
	}
}
