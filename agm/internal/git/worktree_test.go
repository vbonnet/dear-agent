package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a git repo in tmpDir with an initial commit.
func initTestRepo(t *testing.T, tmpDir string) {
	t.Helper()

	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run(); err != nil {
		t.Fatalf("Failed to config git user: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Fatalf("Failed to config git email: %v", err)
	}

	// Create initial commit so we have a branch
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0600); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "add", "README.md").Run(); err != nil {
		t.Fatalf("Failed to add README: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}
}

// TestListWorktrees_NoExtraWorktrees verifies that listing worktrees with only
// the main worktree returns a single entry marked as IsMain.
func TestListWorktrees_NoExtraWorktrees(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	worktrees, err := ListWorktrees(tmpDir)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("Expected 1 worktree, got %d", len(worktrees))
	}

	if !worktrees[0].IsMain {
		t.Error("Expected first worktree to be main")
	}
	if worktrees[0].Path != tmpDir {
		t.Errorf("Expected path %q, got %q", tmpDir, worktrees[0].Path)
	}
}

// TestListWorktrees_MultipleWorktrees creates 2 additional worktrees and
// verifies all 3 (main + 2) are listed with correct branches.
func TestListWorktrees_MultipleWorktrees(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	// Create two worktrees
	wt1Path := filepath.Join(t.TempDir(), "wt1")
	wt2Path := filepath.Join(t.TempDir(), "wt2")

	if output, err := exec.Command("git", "-C", tmpDir, "worktree", "add", wt1Path, "-b", "feature-1").CombinedOutput(); err != nil {
		t.Fatalf("Failed to add worktree 1: %v\nOutput: %s", err, output)
	}
	if output, err := exec.Command("git", "-C", tmpDir, "worktree", "add", wt2Path, "-b", "feature-2").CombinedOutput(); err != nil {
		t.Fatalf("Failed to add worktree 2: %v\nOutput: %s", err, output)
	}

	worktrees, err := ListWorktrees(tmpDir)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(worktrees) != 3 {
		t.Fatalf("Expected 3 worktrees, got %d", len(worktrees))
	}

	// Verify main worktree
	if !worktrees[0].IsMain {
		t.Error("Expected first worktree to be main")
	}

	// Collect branch names from non-main worktrees
	branches := make(map[string]bool)
	for _, wt := range worktrees {
		if !wt.IsMain {
			branches[wt.Branch] = true
		}
	}

	if !branches["feature-1"] {
		t.Error("Expected to find branch 'feature-1'")
	}
	if !branches["feature-2"] {
		t.Error("Expected to find branch 'feature-2'")
	}
}

// TestRemoveMergedWorktrees_MergedBranch creates a branch, merges it into main,
// and verifies the worktree is removed.
func TestRemoveMergedWorktrees_MergedBranch(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	// Create a worktree with a new branch
	wtPath := filepath.Join(t.TempDir(), "feature-wt")
	if output, err := exec.Command("git", "-C", tmpDir, "worktree", "add", wtPath, "-b", "feature-merged").CombinedOutput(); err != nil {
		t.Fatalf("Failed to add worktree: %v\nOutput: %s", err, output)
	}

	// The branch is at the same commit as main, so it's effectively "merged"
	// (merge-base --is-ancestor will succeed since they point to the same commit)

	results, err := RemoveMergedWorktrees(tmpDir, "main")
	if err != nil {
		t.Fatalf("RemoveMergedWorktrees failed: %v", err)
	}

	// Find result for our branch
	var found bool
	for _, r := range results {
		if r.Branch == "feature-merged" {
			found = true
			if !r.Removed {
				t.Errorf("Expected branch 'feature-merged' to be removed, err: %v", r.Err)
			}
			if r.Err != nil {
				t.Errorf("Expected no error for branch 'feature-merged', got: %v", r.Err)
			}
		}
	}
	if !found {
		t.Error("Expected to find result for branch 'feature-merged'")
	}

	// Verify worktree directory no longer exists
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("Expected worktree directory to be removed")
	}
}

// TestRemoveMergedWorktrees_UnmergedBranch creates a branch with an unmerged
// commit and verifies it is NOT removed.
func TestRemoveMergedWorktrees_UnmergedBranch(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	// Create a worktree with a new branch
	wtPath := filepath.Join(t.TempDir(), "unmerged-wt")
	if output, err := exec.Command("git", "-C", tmpDir, "worktree", "add", wtPath, "-b", "feature-unmerged").CombinedOutput(); err != nil {
		t.Fatalf("Failed to add worktree: %v\nOutput: %s", err, output)
	}

	// Add an unmerged commit on the feature branch
	unmergedFile := filepath.Join(wtPath, "unmerged.txt")
	if err := os.WriteFile(unmergedFile, []byte("unmerged content\n"), 0600); err != nil {
		t.Fatalf("Failed to create unmerged file: %v", err)
	}
	if err := exec.Command("git", "-C", wtPath, "add", "unmerged.txt").Run(); err != nil {
		t.Fatalf("Failed to add unmerged file: %v", err)
	}
	if err := exec.Command("git", "-C", wtPath, "commit", "-m", "Unmerged commit").Run(); err != nil {
		t.Fatalf("Failed to commit unmerged file: %v", err)
	}

	results, err := RemoveMergedWorktrees(tmpDir, "main")
	if err != nil {
		t.Fatalf("RemoveMergedWorktrees failed: %v", err)
	}

	// Find result for our branch
	var found bool
	for _, r := range results {
		if r.Branch == "feature-unmerged" {
			found = true
			if r.Removed {
				t.Error("Expected branch 'feature-unmerged' to NOT be removed")
			}
			if r.Err != nil {
				t.Errorf("Expected no error for unmerged branch, got: %v", r.Err)
			}
		}
	}
	if !found {
		t.Error("Expected to find result for branch 'feature-unmerged'")
	}

	// Verify worktree directory still exists
	if _, err := os.Stat(wtPath); err != nil {
		t.Error("Expected worktree directory to still exist")
	}
}

// TestRemoveMergedWorktrees_MixedBranches creates a repo with both merged and
// unmerged branches and verifies merged ones get removed while unmerged ones are kept.
func TestRemoveMergedWorktrees_MixedBranches(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	// Create a worktree with a merged branch (same commit as main = merged)
	mergedPath := filepath.Join(t.TempDir(), "merged-wt")
	if output, err := exec.Command("git", "-C", tmpDir, "worktree", "add", mergedPath, "-b", "feature-merged-mix").CombinedOutput(); err != nil {
		t.Fatalf("Failed to add merged worktree: %v\nOutput: %s", err, output)
	}

	// Create a worktree with an unmerged branch (has extra commit)
	unmergedPath := filepath.Join(t.TempDir(), "unmerged-wt")
	if output, err := exec.Command("git", "-C", tmpDir, "worktree", "add", unmergedPath, "-b", "feature-unmerged-mix").CombinedOutput(); err != nil {
		t.Fatalf("Failed to add unmerged worktree: %v\nOutput: %s", err, output)
	}
	unmergedFile := filepath.Join(unmergedPath, "extra.txt")
	if err := os.WriteFile(unmergedFile, []byte("extra\n"), 0600); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := exec.Command("git", "-C", unmergedPath, "add", "extra.txt").Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}
	if err := exec.Command("git", "-C", unmergedPath, "commit", "-m", "Extra commit").Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	results, err := RemoveMergedWorktrees(tmpDir, "main")
	if err != nil {
		t.Fatalf("RemoveMergedWorktrees failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		switch r.Branch {
		case "feature-merged-mix":
			if !r.Removed {
				t.Errorf("Expected merged branch to be removed, err: %v", r.Err)
			}
		case "feature-unmerged-mix":
			if r.Removed {
				t.Error("Expected unmerged branch to NOT be removed")
			}
		default:
			t.Errorf("Unexpected branch in results: %s", r.Branch)
		}
	}

	// Verify merged worktree directory is gone
	if _, err := os.Stat(mergedPath); !os.IsNotExist(err) {
		t.Error("Expected merged worktree directory to be removed")
	}
	// Verify unmerged worktree directory still exists
	if _, err := os.Stat(unmergedPath); err != nil {
		t.Error("Expected unmerged worktree directory to still exist")
	}
}

// TestListWorktrees_DetachedHead creates a worktree in detached HEAD state
// and verifies it's listed with an empty Branch field.
func TestListWorktrees_DetachedHead(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	// Get the HEAD commit hash
	hashOut, err := exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get HEAD hash: %v", err)
	}
	commitHash := strings.TrimSpace(string(hashOut))

	// Create a worktree in detached HEAD state (checkout a commit directly)
	detachedPath := filepath.Join(t.TempDir(), "detached-wt")
	if output, err := exec.Command("git", "-C", tmpDir, "worktree", "add", "--detach", detachedPath, commitHash).CombinedOutput(); err != nil {
		t.Fatalf("Failed to add detached worktree: %v\nOutput: %s", err, output)
	}

	worktrees, err := ListWorktrees(tmpDir)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(worktrees) != 2 {
		t.Fatalf("Expected 2 worktrees, got %d", len(worktrees))
	}

	// Find the detached worktree
	var foundDetached bool
	for _, wt := range worktrees {
		if wt.Path == detachedPath {
			foundDetached = true
			if wt.Branch != "" {
				t.Errorf("Expected empty branch for detached HEAD, got %q", wt.Branch)
			}
			if wt.IsMain {
				t.Error("Detached worktree should not be marked as main")
			}
		}
	}
	if !foundDetached {
		t.Error("Expected to find detached worktree in listing")
	}
}

// TestParseWorktreeOutput_EdgeCases tests parseWorktreeOutput with edge cases.
func TestParseWorktreeOutput_EdgeCases(t *testing.T) {
	// Empty string input
	result := parseWorktreeOutput("")
	if len(result) != 0 {
		t.Errorf("Empty input: expected 0 worktrees, got %d", len(result))
	}

	// Single worktree (main only)
	singleOutput := "worktree /tmp/repo\nHEAD abc123\nbranch refs/heads/main\n\n"
	result = parseWorktreeOutput(singleOutput)
	if len(result) != 1 {
		t.Fatalf("Single worktree: expected 1 worktree, got %d", len(result))
	}
	if !result[0].IsMain {
		t.Error("Single worktree: expected IsMain to be true")
	}
	if result[0].Branch != "main" {
		t.Errorf("Single worktree: expected branch 'main', got %q", result[0].Branch)
	}
	if result[0].Path != "/tmp/repo" {
		t.Errorf("Single worktree: expected path '/tmp/repo', got %q", result[0].Path)
	}

	// Output without trailing newline
	noTrailingNewline := "worktree /tmp/repo\nHEAD abc123\nbranch refs/heads/main"
	result = parseWorktreeOutput(noTrailingNewline)
	if len(result) != 1 {
		t.Fatalf("No trailing newline: expected 1 worktree, got %d", len(result))
	}
	if !result[0].IsMain {
		t.Error("No trailing newline: expected IsMain to be true")
	}
	if result[0].Branch != "main" {
		t.Errorf("No trailing newline: expected branch 'main', got %q", result[0].Branch)
	}
}

// TestRemoveMergedWorktrees_RemoveFailure tests the error path when git worktree
// remove fails (dirty worktree with uncommitted changes). Verifies the error is
// captured in CleanupResult.Err but the operation continues.
func TestRemoveMergedWorktrees_RemoveFailure(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	// Create a merged worktree (same commit as main)
	wtPath := filepath.Join(t.TempDir(), "dirty-wt")
	if output, err := exec.Command("git", "-C", tmpDir, "worktree", "add", wtPath, "-b", "feature-dirty").CombinedOutput(); err != nil {
		t.Fatalf("Failed to add worktree: %v\nOutput: %s", err, output)
	}

	// Make the worktree dirty (uncommitted changes prevent removal)
	dirtyFile := filepath.Join(wtPath, "dirty.txt")
	if err := os.WriteFile(dirtyFile, []byte("dirty\n"), 0600); err != nil {
		t.Fatalf("Failed to write dirty file: %v", err)
	}
	if err := exec.Command("git", "-C", wtPath, "add", "dirty.txt").Run(); err != nil {
		t.Fatalf("Failed to stage dirty file: %v", err)
	}

	results, err := RemoveMergedWorktrees(tmpDir, "main")
	if err != nil {
		t.Fatalf("RemoveMergedWorktrees failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Branch != "feature-dirty" {
		t.Errorf("Expected branch 'feature-dirty', got %q", r.Branch)
	}
	if r.Removed {
		t.Error("Expected dirty worktree to NOT be removed")
	}
	if r.Err == nil {
		t.Error("Expected error for dirty worktree removal, got nil")
	}
}

// TestRemoveMergedWorktrees_NotAGitRepo verifies graceful nil return
// when the path is not a git repository.
func TestRemoveMergedWorktrees_NotAGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	results, err := RemoveMergedWorktrees(tmpDir, "main")
	if err != nil {
		t.Errorf("Expected nil error for non-git directory, got: %v", err)
	}
	if results != nil {
		t.Errorf("Expected nil results for non-git directory, got: %v", results)
	}
}

// TestRemoveWorktree verifies that RemoveWorktree removes a worktree.
func TestRemoveWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	wtPath := filepath.Join(t.TempDir(), "removable-wt")
	if output, err := exec.Command("git", "-C", tmpDir, "worktree", "add", wtPath, "-b", "removable").CombinedOutput(); err != nil {
		t.Fatalf("Failed to add worktree: %v\nOutput: %s", err, output)
	}

	// Verify worktree exists
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("Worktree should exist: %v", err)
	}

	// Remove it
	if err := RemoveWorktree(tmpDir, wtPath, false); err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("Expected worktree directory to be removed")
	}
}

// TestRemoveWorktree_NotAGitRepo verifies error when path is not a git repo.
func TestRemoveWorktree_NotAGitRepo(t *testing.T) {
	err := RemoveWorktree(t.TempDir(), "/nonexistent", false)
	if err == nil {
		t.Error("Expected error for non-git repo")
	}
}

// TestDeleteBranch verifies that DeleteBranch removes a merged branch.
func TestDeleteBranch(t *testing.T) {
	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	// Create a branch at the same commit (effectively merged)
	if err := exec.Command("git", "-C", tmpDir, "branch", "deletable-branch").Run(); err != nil {
		t.Fatalf("Failed to create branch: %v", err)
	}

	// Delete with safe mode (-d)
	if err := DeleteBranch(tmpDir, "deletable-branch", false); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	// Verify branch is gone
	out, _ := exec.Command("git", "-C", tmpDir, "branch").CombinedOutput()
	if strings.Contains(string(out), "deletable-branch") {
		t.Error("Branch should have been deleted")
	}
}

// TestDeleteBranch_NotAGitRepo verifies error when path is not a git repo.
func TestDeleteBranch_NotAGitRepo(t *testing.T) {
	err := DeleteBranch(t.TempDir(), "nonexistent", false)
	if err == nil {
		t.Error("Expected error for non-git repo")
	}
}
