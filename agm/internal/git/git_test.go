package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitManifest_NotInGitRepo verifies that CommitManifest
// returns nil (no-op) when not in a git repository
func TestCommitManifest_NotInGitRepo(t *testing.T) {
	// Create a temporary directory (not a git repo)
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	// Create a dummy manifest file
	if err := os.WriteFile(manifestPath, []byte("test: data\n"), 0600); err != nil {
		t.Fatalf("Failed to create test manifest: %v", err)
	}

	// Call CommitManifest - should return nil (no error)
	err := CommitManifest(manifestPath, "test", "test-session")
	if err != nil {
		t.Errorf("Expected nil error for non-git directory, got: %v", err)
	}
}

// TestCommitManifest_InGitRepo verifies that CommitManifest
// successfully commits a manifest file in a git repository
func TestCommitManifest_InGitRepo(t *testing.T) {
	// Create a temporary git repository
	tmpDir := t.TempDir()
	gitInit := exec.Command("git", "init", tmpDir)
	if err := gitInit.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user (required for commits)
	configUser := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User")
	if err := configUser.Run(); err != nil {
		t.Fatalf("Failed to config git user: %v", err)
	}
	configEmail := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com")
	if err := configEmail.Run(); err != nil {
		t.Fatalf("Failed to config git email: %v", err)
	}

	// Create a manifest file
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("test: data\n"), 0600); err != nil {
		t.Fatalf("Failed to create test manifest: %v", err)
	}

	// Call CommitManifest
	err := CommitManifest(manifestPath, "create", "test-session")
	if err != nil {
		t.Fatalf("CommitManifest failed: %v", err)
	}

	// Verify commit was created
	logCmd := exec.Command("git", "-C", tmpDir, "log", "--oneline")
	output, err := logCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get git log: %v", err)
	}

	// Check commit message
	logStr := string(output)
	expectedMsg := "agm: create session 'test-session'"
	if !strings.Contains(logStr, expectedMsg) {
		t.Errorf("Expected commit message to contain %q, got: %s", expectedMsg, logStr)
	}

	// Verify only manifest.yaml is in the commit
	showCmd := exec.Command("git", "-C", tmpDir, "show", "--name-only", "--format=")
	showOutput, err := showCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to show commit files: %v", err)
	}

	files := strings.TrimSpace(string(showOutput))
	if files != "manifest.yaml" {
		t.Errorf("Expected only 'manifest.yaml' in commit, got: %s", files)
	}
}

// TestCommitManifest_WithUnstagedFiles verifies that CommitManifest
// only commits the manifest file, not other unstaged files
func TestCommitManifest_WithUnstagedFiles(t *testing.T) {
	// Create a temporary git repository
	tmpDir := t.TempDir()
	gitInit := exec.Command("git", "init", tmpDir)
	if err := gitInit.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	configUser := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User")
	if err := configUser.Run(); err != nil {
		t.Fatalf("Failed to config git user: %v", err)
	}
	configEmail := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com")
	if err := configEmail.Run(); err != nil {
		t.Fatalf("Failed to config git email: %v", err)
	}

	// Create an initial commit (git requires at least one commit)
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0600); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}
	addReadme := exec.Command("git", "-C", tmpDir, "add", "README.md")
	if err := addReadme.Run(); err != nil {
		t.Fatalf("Failed to add README: %v", err)
	}
	commitReadme := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit")
	if err := commitReadme.Run(); err != nil {
		t.Fatalf("Failed to commit README: %v", err)
	}

	// Create an unstaged file (should NOT be committed)
	unstagedPath := filepath.Join(tmpDir, "unstaged.txt")
	if err := os.WriteFile(unstagedPath, []byte("unstaged content\n"), 0600); err != nil {
		t.Fatalf("Failed to create unstaged file: %v", err)
	}

	// Create a manifest file
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("test: data\n"), 0600); err != nil {
		t.Fatalf("Failed to create test manifest: %v", err)
	}

	// Call CommitManifest
	err := CommitManifest(manifestPath, "create", "test-session")
	if err != nil {
		t.Fatalf("CommitManifest failed: %v", err)
	}

	// Verify only manifest.yaml is in the commit
	showCmd := exec.Command("git", "-C", tmpDir, "show", "--name-only", "--format=")
	showOutput, err := showCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to show commit files: %v", err)
	}

	files := strings.TrimSpace(string(showOutput))
	if files != "manifest.yaml" {
		t.Errorf("Expected only 'manifest.yaml' in commit, got: %s", files)
	}

	// Verify unstaged.txt is still unstaged
	statusCmd := exec.Command("git", "-C", tmpDir, "status", "--porcelain")
	statusOutput, err := statusCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get git status: %v", err)
	}

	status := string(statusOutput)
	if !strings.Contains(status, "?? unstaged.txt") {
		t.Errorf("Expected unstaged.txt to remain unstaged, got status: %s", status)
	}
}

// TestCommitManifest_WithStagedFiles verifies that CommitManifest
// only commits the manifest file, preserving other staged files
func TestCommitManifest_WithStagedFiles(t *testing.T) {
	// Create a temporary git repository
	tmpDir := t.TempDir()
	gitInit := exec.Command("git", "init", tmpDir)
	if err := gitInit.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	configUser := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User")
	if err := configUser.Run(); err != nil {
		t.Fatalf("Failed to config git user: %v", err)
	}
	configEmail := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com")
	if err := configEmail.Run(); err != nil {
		t.Fatalf("Failed to config git email: %v", err)
	}

	// Create an initial commit
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0600); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}
	addReadme := exec.Command("git", "-C", tmpDir, "add", "README.md")
	if err := addReadme.Run(); err != nil {
		t.Fatalf("Failed to add README: %v", err)
	}
	commitReadme := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit")
	if err := commitReadme.Run(); err != nil {
		t.Fatalf("Failed to commit README: %v", err)
	}

	// Create and stage a file (should remain staged after manifest commit)
	stagedPath := filepath.Join(tmpDir, "staged.txt")
	if err := os.WriteFile(stagedPath, []byte("staged content\n"), 0600); err != nil {
		t.Fatalf("Failed to create staged file: %v", err)
	}
	addStaged := exec.Command("git", "-C", tmpDir, "add", "staged.txt")
	if err := addStaged.Run(); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}

	// Create a manifest file
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("test: data\n"), 0600); err != nil {
		t.Fatalf("Failed to create test manifest: %v", err)
	}

	// Call CommitManifest
	err := CommitManifest(manifestPath, "create", "test-session")
	if err != nil {
		t.Fatalf("CommitManifest failed: %v", err)
	}

	// Verify only manifest.yaml is in the latest commit
	showCmd := exec.Command("git", "-C", tmpDir, "show", "--name-only", "--format=")
	showOutput, err := showCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to show commit files: %v", err)
	}

	files := strings.TrimSpace(string(showOutput))
	if files != "manifest.yaml" {
		t.Errorf("Expected only 'manifest.yaml' in commit, got: %s", files)
	}

	// Verify staged.txt is still staged (not committed)
	statusCmd := exec.Command("git", "-C", tmpDir, "status", "--porcelain")
	statusOutput, err := statusCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get git status: %v", err)
	}

	status := string(statusOutput)
	if !strings.Contains(status, "A  staged.txt") {
		t.Errorf("Expected staged.txt to remain staged, got status: %s", status)
	}
}

// TestCommitManifest_NoChanges verifies that CommitManifest
// handles the case where there are no changes to commit
func TestCommitManifest_NoChanges(t *testing.T) {
	// Create a temporary git repository
	tmpDir := t.TempDir()
	gitInit := exec.Command("git", "init", tmpDir)
	if err := gitInit.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	configUser := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User")
	if err := configUser.Run(); err != nil {
		t.Fatalf("Failed to config git user: %v", err)
	}
	configEmail := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com")
	if err := configEmail.Run(); err != nil {
		t.Fatalf("Failed to config git email: %v", err)
	}

	// Create and commit a manifest file
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("test: data\n"), 0600); err != nil {
		t.Fatalf("Failed to create test manifest: %v", err)
	}
	addManifest := exec.Command("git", "-C", tmpDir, "add", "manifest.yaml")
	if err := addManifest.Run(); err != nil {
		t.Fatalf("Failed to add manifest: %v", err)
	}
	commitManifest := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial manifest")
	if err := commitManifest.Run(); err != nil {
		t.Fatalf("Failed to commit manifest: %v", err)
	}

	// Call CommitManifest with no changes - should return nil
	err := CommitManifest(manifestPath, "update", "test-session")
	if err != nil {
		t.Errorf("Expected nil error when no changes to commit, got: %v", err)
	}
}

// TestCommitManifest_InSubdirectory verifies that CommitManifest
// works correctly when the manifest is in a subdirectory
func TestCommitManifest_InSubdirectory(t *testing.T) {
	// Create a temporary git repository
	tmpDir := t.TempDir()
	gitInit := exec.Command("git", "init", tmpDir)
	if err := gitInit.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	configUser := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User")
	if err := configUser.Run(); err != nil {
		t.Fatalf("Failed to config git user: %v", err)
	}
	configEmail := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com")
	if err := configEmail.Run(); err != nil {
		t.Fatalf("Failed to config git email: %v", err)
	}

	// Create subdirectory structure
	sessionsDir := filepath.Join(tmpDir, ".agm", "sessions", "session-123")
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		t.Fatalf("Failed to create sessions directory: %v", err)
	}

	// Create a manifest file in subdirectory
	manifestPath := filepath.Join(sessionsDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte("test: data\n"), 0600); err != nil {
		t.Fatalf("Failed to create test manifest: %v", err)
	}

	// Call CommitManifest
	err := CommitManifest(manifestPath, "create", "test-session")
	if err != nil {
		t.Fatalf("CommitManifest failed: %v", err)
	}

	// Verify commit was created
	logCmd := exec.Command("git", "-C", tmpDir, "log", "--oneline")
	output, err := logCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get git log: %v", err)
	}

	logStr := string(output)
	expectedMsg := "agm: create session 'test-session'"
	if !strings.Contains(logStr, expectedMsg) {
		t.Errorf("Expected commit message to contain %q, got: %s", expectedMsg, logStr)
	}

	// Verify correct file path in commit
	showCmd := exec.Command("git", "-C", tmpDir, "show", "--name-only", "--format=")
	showOutput, err := showCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to show commit files: %v", err)
	}

	files := strings.TrimSpace(string(showOutput))
	expectedPath := ".agm/sessions/session-123/manifest.yaml"
	if files != expectedPath {
		t.Errorf("Expected %q in commit, got: %s", expectedPath, files)
	}
}

// TestFindGitRoot verifies findGitRoot functionality
func TestFindGitRoot(t *testing.T) {
	// Test case 1: Not in git repo
	tmpDir := t.TempDir()
	_, err := findGitRoot(tmpDir)
	if err != ErrNotInGitRepo {
		t.Errorf("Expected ErrNotInGitRepo, got: %v", err)
	}

	// Test case 2: In git repo root
	gitInit := exec.Command("git", "init", tmpDir)
	if err := gitInit.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	root, err := findGitRoot(tmpDir)
	if err != nil {
		t.Fatalf("findGitRoot failed: %v", err)
	}
	if root != tmpDir {
		t.Errorf("Expected root %q, got %q", tmpDir, root)
	}

	// Test case 3: In subdirectory of git repo
	subdir := filepath.Join(tmpDir, "subdir", "nested")
	if err := os.MkdirAll(subdir, 0700); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	root, err = findGitRoot(subdir)
	if err != nil {
		t.Fatalf("findGitRoot failed for subdir: %v", err)
	}
	if root != tmpDir {
		t.Errorf("Expected root %q from subdir, got %q", tmpDir, root)
	}
}

// TestIsInGitRepo verifies IsInGitRepo convenience function
func TestIsInGitRepo(t *testing.T) {
	// Test case 1: Not in git repo
	tmpDir := t.TempDir()
	if IsInGitRepo(tmpDir) {
		t.Error("Expected false for non-git directory")
	}

	// Test case 2: In git repo
	gitInit := exec.Command("git", "init", tmpDir)
	if err := gitInit.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	if !IsInGitRepo(tmpDir) {
		t.Error("Expected true for git directory")
	}
}
