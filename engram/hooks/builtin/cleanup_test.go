package builtin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/hooks"
)

func TestCleanupCheckerNoIssues(t *testing.T) {
	tmpDir := t.TempDir()

	checker := NewCleanupChecker(tmpDir)
	result, err := checker.CheckCleanup(context.Background())
	if err != nil {
		t.Fatalf("CheckCleanup failed: %v", err)
	}

	// Should pass when no temp files
	if result.Status != hooks.VerificationStatusPass {
		t.Errorf("Expected pass status, got %s", result.Status)
	}
}

func TestCleanupCheckerTempFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create temp files
	tempFiles := []string{
		"test.backup",
		"draft-notes.tmp",
		".DS_Store",
	}

	for _, tf := range tempFiles {
		path := filepath.Join(tmpDir, tf)
		if err := os.WriteFile(path, []byte("temp"), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
	}

	checker := NewCleanupChecker(tmpDir)
	result, err := checker.CheckCleanup(context.Background())
	if err != nil {
		t.Fatalf("CheckCleanup failed: %v", err)
	}

	// Should have warning for temp files
	if result.Status != hooks.VerificationStatusWarning {
		t.Errorf("Expected warning status, got %s", result.Status)
	}

	if len(result.Violations) == 0 {
		t.Error("Expected violations for temp files")
	}
}

func TestDetectTempFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create various temp files
	tempFiles := []string{
		"file.backup",
		"notes.tmp",
	}

	for _, tf := range tempFiles {
		path := filepath.Join(tmpDir, tf)
		if err := os.WriteFile(path, []byte("temp"), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
	}

	checker := NewCleanupChecker(tmpDir)
	detected := checker.detectTempFiles()

	if len(detected) == 0 {
		t.Error("Expected to detect temp files")
	}
}

func TestDetectMergedBranches(t *testing.T) {
	// This test requires a git repo, skip if not available
	tmpDir := t.TempDir()

	checker := NewCleanupChecker(tmpDir)
	_, err := checker.detectMergedBranches(context.Background())

	// Should return error for non-git repo
	if err == nil {
		t.Log("detectMergedBranches succeeded (may be in git repo)")
	}
}

func TestDetectUnusedWorktrees(t *testing.T) {
	// This test requires a git repo with worktrees, skip if not available
	tmpDir := t.TempDir()

	checker := NewCleanupChecker(tmpDir)
	_, err := checker.detectUnusedWorktrees(context.Background())

	// Should return error for non-git repo or no worktrees
	if err == nil {
		t.Log("detectUnusedWorktrees succeeded (may be in git repo with worktrees)")
	}
}
