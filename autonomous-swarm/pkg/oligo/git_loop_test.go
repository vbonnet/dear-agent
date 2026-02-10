package oligo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a test git repository
func setupTestRepo(t *testing.T) string {
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("initial content\n"), 0644)

	exec.Command("git", "-C", tmpDir, "add", ".").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run()

	return tmpDir
}

// createBranch creates and checks out a new branch
func createBranch(t *testing.T, repoPath, branchName string) {
	cmd := exec.Command("git", "-C", repoPath, "checkout", "-b", branchName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create branch %s: %v", branchName, err)
	}
}

// commitChange makes a change and commits it
func commitChange(t *testing.T, repoPath, filename, content, message string) {
	filePath := filepath.Join(repoPath, filename)
	os.WriteFile(filePath, []byte(content), 0644)

	exec.Command("git", "-C", repoPath, "add", filename).Run()

	cmd := exec.Command("git", "-C", repoPath, "commit", "-m", message)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
}

func TestNewOligoLoop_ValidRepo(t *testing.T) {
	repoPath := setupTestRepo(t)

	loop, err := NewOligoLoop(repoPath, "")
	if err != nil {
		t.Fatalf("Failed to create OligoLoop: %v", err)
	}

	if loop.repoPath != repoPath {
		t.Errorf("Expected repoPath %s, got %s", repoPath, loop.repoPath)
	}

	if loop.engramBin != "engram" {
		t.Errorf("Expected default engram binary, got %s", loop.engramBin)
	}
}

func TestNewOligoLoop_InvalidRepo(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := NewOligoLoop(tmpDir, "")
	if err == nil {
		t.Error("Expected error for non-git directory, got nil")
	}

	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestOligoLoop_Checkout(t *testing.T) {
	repoPath := setupTestRepo(t)
	loop, _ := NewOligoLoop(repoPath, "")

	// Create feature branch
	createBranch(t, repoPath, "feature")

	// Checkout main
	err := loop.checkout("main")
	if err != nil {
		t.Fatalf("Failed to checkout main: %v", err)
	}

	// Verify current branch
	current, _ := loop.GetCurrentBranch()
	if current != "main" {
		t.Errorf("Expected main branch, got %s", current)
	}

	// Checkout feature
	err = loop.checkout("feature")
	if err != nil {
		t.Fatalf("Failed to checkout feature: %v", err)
	}

	current, _ = loop.GetCurrentBranch()
	if current != "feature" {
		t.Errorf("Expected feature branch, got %s", current)
	}
}

func TestOligoLoop_Merge_NoConflict(t *testing.T) {
	repoPath := setupTestRepo(t)
	loop, _ := NewOligoLoop(repoPath, "")

	// Create feature branch with new file
	createBranch(t, repoPath, "feature")
	commitChange(t, repoPath, "feature.txt", "feature content\n", "Add feature file")

	// Checkout main
	loop.checkout("main")

	// Merge feature (should succeed without conflicts)
	result := loop.merge("feature")

	if !result.Success {
		t.Errorf("Expected successful merge, got error: %v", result.Error)
	}

	if result.HasConflict {
		t.Error("Expected no conflicts")
	}

	// Verify file exists in main
	featurePath := filepath.Join(repoPath, "feature.txt")
	if _, err := os.Stat(featurePath); os.IsNotExist(err) {
		t.Error("Feature file should exist after merge")
	}
}

func TestOligoLoop_Merge_WithConflict(t *testing.T) {
	repoPath := setupTestRepo(t)
	loop, _ := NewOligoLoop(repoPath, "")

	// Modify same file on both branches
	commitChange(t, repoPath, "test.txt", "main version\n", "Update on main")

	createBranch(t, repoPath, "feature")
	commitChange(t, repoPath, "test.txt", "feature version\n", "Update on feature")

	loop.checkout("main")

	// Merge should create conflict
	result := loop.merge("feature")

	if result.Success {
		t.Error("Expected merge to fail due to conflict")
	}

	if !result.HasConflict {
		t.Error("Expected HasConflict to be true")
	}

	if len(result.ConflictFiles) == 0 {
		t.Error("Expected conflict files to be listed")
	}

	if result.ConflictFiles[0] != "test.txt" {
		t.Errorf("Expected test.txt in conflicts, got %v", result.ConflictFiles)
	}
}

func TestOligoLoop_DetectConflictFiles(t *testing.T) {
	repoPath := setupTestRepo(t)
	loop, _ := NewOligoLoop(repoPath, "")

	// Create conflict scenario
	commitChange(t, repoPath, "file1.txt", "main content\n", "Main change")

	createBranch(t, repoPath, "feature")
	commitChange(t, repoPath, "file1.txt", "feature content\n", "Feature change")

	loop.checkout("main")
	loop.merge("feature") // Creates conflict

	conflicts := loop.detectConflictFiles()

	if len(conflicts) != 1 {
		t.Errorf("Expected 1 conflict file, got %d", len(conflicts))
	}

	if conflicts[0] != "file1.txt" {
		t.Errorf("Expected file1.txt, got %s", conflicts[0])
	}
}

func TestOligoLoop_BuildConflictResolutionPrompt(t *testing.T) {
	repoPath := setupTestRepo(t)
	loop, _ := NewOligoLoop(repoPath, "")

	conflictFiles := []string{"file1.go", "file2.go", "file3.go"}
	prompt := loop.buildConflictResolutionPrompt(conflictFiles)

	// Verify prompt contains all files
	for i, file := range conflictFiles {
		expectedLine := fmt.Sprintf("%d. %s", i+1, file)
		if !strings.Contains(prompt, expectedLine) {
			t.Errorf("Prompt should contain '%s'", expectedLine)
		}
	}

	// Verify prompt contains resolution instructions
	expectedPhrases := []string{
		"Analyze both versions",
		"semantic intent",
		"Remove conflict markers",
		"git add",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("Prompt should contain '%s'", phrase)
		}
	}
}

func TestOligoLoop_AbortMerge(t *testing.T) {
	repoPath := setupTestRepo(t)
	loop, _ := NewOligoLoop(repoPath, "")

	// Create conflict
	commitChange(t, repoPath, "test.txt", "main version\n", "Main update")

	createBranch(t, repoPath, "feature")
	commitChange(t, repoPath, "test.txt", "feature version\n", "Feature update")

	loop.checkout("main")
	loop.merge("feature") // Creates conflict

	// Abort merge
	err := loop.AbortMerge()
	if err != nil {
		t.Fatalf("Failed to abort merge: %v", err)
	}

	// Verify no conflicts remain
	conflicts := loop.detectConflictFiles()
	if len(conflicts) > 0 {
		t.Error("Expected no conflicts after abort")
	}
}

func TestOligoLoop_GetCurrentBranch(t *testing.T) {
	repoPath := setupTestRepo(t)
	loop, _ := NewOligoLoop(repoPath, "")

	branch, err := loop.GetCurrentBranch()
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}

	// Should be on main or master initially
	if branch != "main" && branch != "master" {
		t.Errorf("Expected main or master, got %s", branch)
	}

	// Switch to feature
	createBranch(t, repoPath, "test-branch")

	branch, _ = loop.GetCurrentBranch()
	if branch != "test-branch" {
		t.Errorf("Expected test-branch, got %s", branch)
	}
}

func TestOligoLoop_ExecuteIntegration_Success(t *testing.T) {
	repoPath := setupTestRepo(t)
	loop, _ := NewOligoLoop(repoPath, "")

	// Create clean feature branch
	createBranch(t, repoPath, "feature")
	commitChange(t, repoPath, "new.txt", "new content\n", "Add new file")

	loop.checkout("main")

	// Execute integration (should succeed)
	result, err := loop.ExecuteIntegration("feature", "main")
	if err != nil {
		t.Fatalf("Integration failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected successful integration")
	}

	if result.HasConflict {
		t.Error("Expected no conflicts")
	}
}
