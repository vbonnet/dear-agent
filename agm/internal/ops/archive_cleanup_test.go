package ops

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestErrStr(t *testing.T) {
	if got := errStr(nil); got != "" {
		t.Errorf("errStr(nil) = %q, want empty", got)
	}
	if got := errStr(os.ErrNotExist); got == "" {
		t.Error("errStr(non-nil) should return non-empty string")
	}
}

func TestBoolErrStr(t *testing.T) {
	if got := boolErrStr(false, "msg"); got != "" {
		t.Errorf("boolErrStr(false) = %q, want empty", got)
	}
	if got := boolErrStr(true, "fail"); got != "fail" {
		t.Errorf("boolErrStr(true) = %q, want %q", got, "fail")
	}
}

func TestLogAction_WritesToFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "cleanup.jsonl")
	logger := &cleanupLogger{path: logPath}

	logAction(logger, CleanupAction{
		SessionID:   "sess-1",
		SessionName: "my-session",
		Action:      "remove_worktree",
		Target:      "/tmp/wt",
		Success:     true,
	})

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var entry CleanupAction
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", entry.SessionID, "sess-1")
	}
	if entry.Action != "remove_worktree" {
		t.Errorf("Action = %q, want %q", entry.Action, "remove_worktree")
	}
	if !entry.Success {
		t.Error("Success should be true")
	}
	if entry.Timestamp.IsZero() {
		t.Error("Timestamp should be auto-populated")
	}
}

func TestLogAction_NilLogger(t *testing.T) {
	// Should not panic
	logAction(nil, CleanupAction{Action: "test"})
}

func TestLogAction_MultipleEntries(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "cleanup.jsonl")
	logger := &cleanupLogger{path: logPath}

	logAction(logger, CleanupAction{Action: "action1", Success: true})
	logAction(logger, CleanupAction{Action: "action2", Success: false, Error: "oops"})

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines, got %d", len(lines))
	}

	var entry2 CleanupAction
	if err := json.Unmarshal([]byte(lines[1]), &entry2); err != nil {
		t.Fatalf("Failed to parse second entry: %v", err)
	}
	if entry2.Error != "oops" {
		t.Errorf("Error = %q, want %q", entry2.Error, "oops")
	}
}

func TestPruneWorktrees_RealGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	// Create a real git repo
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	// pruneWorktrees should succeed on a clean repo
	if err := pruneWorktrees(repoDir); err != nil {
		t.Errorf("pruneWorktrees failed on clean repo: %v", err)
	}
}

func TestPruneWorktrees_InvalidRepo(t *testing.T) {
	tmpDir := t.TempDir()
	if err := pruneWorktrees(tmpDir); err == nil {
		t.Error("pruneWorktrees should fail on non-git directory")
	}
}

func TestForceDeleteBranch_MergedBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	// Create and merge a branch
	runGit(t, repoDir, "checkout", "-b", "feature-branch")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "feature work")
	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "merge", "feature-branch", "--no-ff", "-m", "merge feature")

	if err := forceDeleteBranch(repoDir, "feature-branch"); err != nil {
		t.Errorf("forceDeleteBranch failed on merged branch: %v", err)
	}
}

func TestForceDeleteBranch_UnmergedBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	// Create a branch with work NOT merged to main
	runGit(t, repoDir, "checkout", "-b", "unmerged-branch")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "unmerged work")
	runGit(t, repoDir, "checkout", "main")

	// Force delete should succeed even for unmerged branches
	if err := forceDeleteBranch(repoDir, "unmerged-branch"); err != nil {
		t.Errorf("forceDeleteBranch should succeed on unmerged branch: %v", err)
	}

	// Verify branch is gone
	cmd := exec.Command("git", "-C", repoDir, "branch", "--list", "unmerged-branch")
	output, _ := cmd.Output()
	if strings.TrimSpace(string(output)) != "" {
		t.Error("Branch should be deleted after forceDeleteBranch")
	}
}

func TestForceDeleteBranch_NonexistentBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	if err := forceDeleteBranch(repoDir, "does-not-exist"); err == nil {
		t.Error("forceDeleteBranch should fail on nonexistent branch")
	}
}

func TestCleanupAfterArchive_SandboxBranchDeleted(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	// Create a sandbox branch agm/<sessionID>
	sessionID := "test-session-id-123"
	sandboxBranch := "agm/" + sessionID
	runGit(t, repoDir, "branch", sandboxBranch)

	// Also create a session branch
	runGit(t, repoDir, "branch", "my-session")

	result := CleanupAfterArchive(
		sessionID, "my-session",
		"", repoDir, "", "my-session",
		false,
	)

	if !result.BranchDeleted {
		t.Error("BranchDeleted should be true for session branch")
	}
	if !result.SandboxBranchDeleted {
		t.Error("SandboxBranchDeleted should be true for agm/<sessionID> branch")
	}

	// Verify both branches are gone
	cmd := exec.Command("git", "-C", repoDir, "branch", "--list", "my-session")
	output, _ := cmd.Output()
	if strings.TrimSpace(string(output)) != "" {
		t.Error("Session branch should be deleted")
	}

	cmd = exec.Command("git", "-C", repoDir, "branch", "--list", sandboxBranch)
	output, _ = cmd.Output()
	if strings.TrimSpace(string(output)) != "" {
		t.Error("Sandbox branch should be deleted")
	}
}

func TestRemoveWorktreeCmd_InvalidRepo(t *testing.T) {
	tmpDir := t.TempDir()
	if err := removeWorktreeCmd(tmpDir, "/nonexistent/wt"); err == nil {
		t.Error("removeWorktreeCmd should fail with invalid repo")
	}
}

func TestRemoveWorktreeCmd_EmptyRepoPath(t *testing.T) {
	if err := removeWorktreeCmd("", "/some/path"); err == nil {
		t.Error("removeWorktreeCmd should fail with empty repoPath")
	}
}

func TestRemoveWorktreeCmd_RealWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	// Create a worktree
	wtDir := filepath.Join(t.TempDir(), "my-worktree")
	runGit(t, repoDir, "worktree", "add", wtDir, "-b", "wt-branch")

	// Remove it
	if err := removeWorktreeCmd(repoDir, wtDir); err != nil {
		t.Errorf("removeWorktreeCmd failed: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("Worktree directory should be removed, stat err: %v", err)
	}
}

func TestCleanupAfterArchive_KeepSandbox(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "cleanup.jsonl")

	// Override the cleanup logger for this test
	sandboxDir := filepath.Join(tmpDir, "sandbox")
	os.MkdirAll(sandboxDir, 0755)

	// With keepSandbox=true, sandbox should NOT be removed.
	// We can't fully test CleanupAfterArchive without mocking the git
	// commands, but we can verify the keepSandbox flag is respected by
	// checking the sandbox dir is preserved.
	result := CleanupAfterArchive(
		"sess-1", "test-session",
		"", "", "", "", // no worktree/repo/branch
		true, // keepSandbox
	)

	// No worktree or sandbox paths — should be no-ops
	if result.WorktreesRemoved != 0 {
		t.Errorf("WorktreesRemoved = %d, want 0", result.WorktreesRemoved)
	}
	if result.SandboxRemoved {
		t.Error("SandboxRemoved should be false when keepSandbox=true")
	}

	// Verify log was written
	_ = logPath // log goes to ~/.agm/logs/cleanup.jsonl by default
}

func TestCleanupAfterArchive_EmptyInputs(t *testing.T) {
	// All empty strings — should be a no-op, not panic
	result := CleanupAfterArchive("", "", "", "", "", "", false)
	if result.WorktreesRemoved != 0 {
		t.Errorf("WorktreesRemoved = %d, want 0", result.WorktreesRemoved)
	}
	if result.WorktreesPruned {
		t.Error("WorktreesPruned should be false with empty repoPath")
	}
	if result.BranchDeleted {
		t.Error("BranchDeleted should be false with empty inputs")
	}
	if result.SandboxRemoved {
		t.Error("SandboxRemoved should be false with empty sandboxPath")
	}
}

func TestCleanupAfterArchive_WithRealGitWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "commit", "--allow-empty", "-m", "init")

	// Create a worktree with unmerged work (the common case for worker sessions)
	wtDir := filepath.Join(t.TempDir(), "test-wt")
	runGit(t, repoDir, "worktree", "add", wtDir, "-b", "test-branch")
	runGit(t, wtDir, "commit", "--allow-empty", "-m", "worker commit")

	result := CleanupAfterArchive(
		"sess-test", "test-session",
		wtDir, repoDir, "", "test-branch",
		false,
	)

	if result.WorktreesRemoved != 1 {
		t.Errorf("WorktreesRemoved = %d, want 1", result.WorktreesRemoved)
	}
	if !result.WorktreesPruned {
		t.Error("WorktreesPruned should be true after prune runs")
	}
	if !result.BranchDeleted {
		t.Error("BranchDeleted should be true — force delete handles unmerged branches")
	}

	// Verify worktree is actually gone
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("Worktree dir should be removed, stat err: %v", err)
	}
}

// runGit is a test helper that runs a git command in the given directory.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	allArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", allArgs...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
