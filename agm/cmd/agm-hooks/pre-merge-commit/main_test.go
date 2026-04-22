package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsMergeCommit(t *testing.T) {
	tests := []struct {
		name           string
		setupMergeHead bool
		want           bool
	}{
		{
			name:           "merge in progress",
			setupMergeHead: true,
			want:           true,
		},
		{
			name:           "no merge in progress",
			setupMergeHead: false,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with .git structure
			tmpDir := t.TempDir()
			gitDir := filepath.Join(tmpDir, ".git")
			require.NoError(t, os.Mkdir(gitDir, 0755))

			// Change to temp directory
			t.Chdir(tmpDir)

			// Setup MERGE_HEAD if needed
			if tt.setupMergeHead {
				mergeHeadFile := filepath.Join(gitDir, "MERGE_HEAD")
				require.NoError(t, os.WriteFile(mergeHeadFile, []byte("abc123\n"), 0644))
			}

			// Test
			got := isMergeCommit()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetMergeTarget(t *testing.T) {
	tests := []struct {
		name       string
		setupRepo  func(t *testing.T, repoDir string)
		wantBranch string
	}{
		{
			name: "on main branch",
			setupRepo: func(t *testing.T, repoDir string) {
				initGitRepo(t, repoDir, "main")
			},
			wantBranch: "main",
		},
		{
			name: "on master branch",
			setupRepo: func(t *testing.T, repoDir string) {
				initGitRepo(t, repoDir, "master")
			},
			wantBranch: "master",
		},
		{
			name: "on feature branch",
			setupRepo: func(t *testing.T, repoDir string) {
				initGitRepo(t, repoDir, "main")
				checkoutBranch(t, repoDir, "feature/test")
			},
			wantBranch: "feature/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setupRepo(t, tmpDir)

			// Change to temp directory
			t.Chdir(tmpDir)

			got := getMergeTarget()
			assert.Equal(t, tt.wantBranch, got)
		})
	}
}

func TestRollback(t *testing.T) {
	// Create a real git repo to test rollback
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir, "main")

	// Create a feature branch and a merge conflict scenario
	checkoutBranch(t, tmpDir, "feature")
	createFile(t, tmpDir, "feature.txt", "feature content")
	gitAdd(t, tmpDir, "feature.txt")
	gitCommit(t, tmpDir, "Add feature file")

	// Switch back to main and create conflicting file
	gitCheckout(t, tmpDir, "main")
	createFile(t, tmpDir, "feature.txt", "main content")
	gitAdd(t, tmpDir, "feature.txt")
	gitCommit(t, tmpDir, "Add conflicting file")

	// Try to merge feature (will create MERGE_HEAD)
	cmd := exec.Command("git", "-C", tmpDir, "merge", "feature", "--no-commit")
	_ = cmd.Run() // Expect this to fail or create conflict

	// Verify MERGE_HEAD exists
	mergeHeadPath := filepath.Join(tmpDir, ".git", "MERGE_HEAD")
	_, err := os.Stat(mergeHeadPath)
	hasMergeHead := err == nil

	// If we have MERGE_HEAD, test rollback
	if hasMergeHead {
		// Change to repo directory for rollback to work
		t.Chdir(tmpDir)

		// Call rollback
		rollback()

		// Verify MERGE_HEAD is gone
		_, err = os.Stat(mergeHeadPath)
		assert.True(t, os.IsNotExist(err), "MERGE_HEAD should be removed after rollback")
	}
}

func TestIntegrationMergeToMain(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name         string
		targetBranch string
		shouldBlock  bool
	}{
		{
			name:         "merge to main",
			targetBranch: "main",
			shouldBlock:  true,
		},
		{
			name:         "merge to master",
			targetBranch: "master",
			shouldBlock:  true,
		},
		{
			name:         "merge to feature branch",
			targetBranch: "feature",
			shouldBlock:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Initialize repo with target branch
			initGitRepo(t, tmpDir, tt.targetBranch)

			// Create a divergent commit on target branch to prevent fast-forward
			createFile(t, tmpDir, "base.txt", "base content")
			gitAdd(t, tmpDir, "base.txt")
			gitCommit(t, tmpDir, "Add base file")

			// Create feature branch from before the divergent commit
			cmd := exec.Command("git", "-C", tmpDir, "checkout", "-b", "feature-branch", "HEAD~1")
			require.NoError(t, cmd.Run(), "git checkout -b feature-branch failed")

			createFile(t, tmpDir, "feature.txt", "content")
			gitAdd(t, tmpDir, "feature.txt")
			gitCommit(t, tmpDir, "Add feature")

			// Switch back to target branch
			gitCheckout(t, tmpDir, tt.targetBranch)

			// Start merge (no commit, no fast-forward to ensure MERGE_HEAD is created)
			cmd = exec.Command("git", "-C", tmpDir, "merge", "feature-branch", "--no-commit", "--no-ff")
			err := cmd.Run()
			// Merge might succeed or fail, we just need MERGE_HEAD to be created

			// Verify MERGE_HEAD exists
			mergeHeadPath := filepath.Join(tmpDir, ".git", "MERGE_HEAD")
			_, statErr := os.Stat(mergeHeadPath)
			if statErr != nil {
				// Skip this test if we couldn't create a merge state
				// This can happen in some git configurations
				t.Skipf("Could not create MERGE_HEAD state: %v (merge command returned: %v)", statErr, err)
			}

			// Change to repo directory
			t.Chdir(tmpDir)

			// Test detection
			assert.True(t, isMergeCommit(), "should detect merge in progress")
			assert.Equal(t, tt.targetBranch, getMergeTarget(), "should detect correct target branch")
		})
	}
}

// Helper functions

func initGitRepo(t *testing.T, dir, branch string) {
	t.Helper()

	// Initialize repo
	cmd := exec.Command("git", "-C", dir, "init", "--initial-branch="+branch)
	require.NoError(t, cmd.Run(), "git init failed")

	// Configure user for commits
	cmd = exec.Command("git", "-C", dir, "config", "user.name", "Test User")
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "-C", dir, "config", "user.email", "test@example.com")
	require.NoError(t, cmd.Run())

	// Create initial commit
	createFile(t, dir, "README.md", "# Test Repo")
	gitAdd(t, dir, "README.md")
	gitCommit(t, dir, "Initial commit")
}

func checkoutBranch(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "checkout", "-b", branch)
	require.NoError(t, cmd.Run(), "git checkout -b failed")
}

func gitCheckout(t *testing.T, dir, branch string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "checkout", branch)
	require.NoError(t, cmd.Run(), "git checkout failed")
}

func createFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func gitAdd(t *testing.T, dir, file string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "add", file)
	require.NoError(t, cmd.Run(), "git add failed")
}

func gitCommit(t *testing.T, dir, message string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "commit", "-m", message)
	require.NoError(t, cmd.Run(), "git commit failed")
}
