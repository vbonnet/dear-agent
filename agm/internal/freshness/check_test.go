package freshness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheck_MatchingCommits(t *testing.T) {
	// Use the actual ai-tools repo if available
	home, _ := os.UserHomeDir()
	repoPath := filepath.Join(home, "src", "ws", "oss", "repos", "ai-tools", "agm")
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err != nil {
		t.Skip("ai-tools repo not available")
	}

	// Get actual HEAD to test matching
	result := Check(repoPath, "nonexistent-hash-12345")
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if !result.Stale {
		t.Error("expected stale=true for non-matching commit")
	}

	// Now test with matching commit
	result2 := Check(repoPath, result.RepoHEAD)
	if result2.Error != nil {
		t.Fatalf("unexpected error: %v", result2.Error)
	}
	if result2.Stale {
		t.Error("expected stale=false for matching commit")
	}
}

func TestCheck_UnknownCommit(t *testing.T) {
	result := Check("/tmp", "unknown")
	if !result.Stale {
		t.Error("expected stale=true for unknown commit")
	}
}

func TestCheck_EmptyCommit(t *testing.T) {
	result := Check("/tmp", "")
	if !result.Stale {
		t.Error("expected stale=true for empty commit")
	}
}

func TestCheck_DirtySuffix(t *testing.T) {
	home, _ := os.UserHomeDir()
	repoPath := filepath.Join(home, "src", "ws", "oss", "repos", "ai-tools", "agm")
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err != nil {
		t.Skip("ai-tools repo not available")
	}

	// Get HEAD and append -dirty
	result := Check(repoPath, "nonexistent")
	if result.Error != nil {
		t.Skip("could not get repo HEAD")
	}
	dirtyCommit := result.RepoHEAD + "-dirty"
	result2 := Check(repoPath, dirtyCommit)
	if result2.Stale {
		t.Error("expected stale=false for dirty suffix of matching commit")
	}
}

func TestCheck_NoGitRepo(t *testing.T) {
	result := Check("/tmp/nonexistent-path-xyz", "abc1234")
	if result.Stale {
		t.Error("expected stale=false when repo not found (fail open)")
	}
	if result.Error == nil {
		t.Error("expected error when repo path is invalid")
	}
}

func TestFindRepoPath_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a fake go.mod
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	t.Setenv("AGM_SOURCE_DIR", tmpDir)
	path, err := FindRepoPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != tmpDir {
		t.Errorf("expected %s, got %s", tmpDir, path)
	}
}

func TestFindRepoPath_InvalidEnv(t *testing.T) {
	t.Setenv("AGM_SOURCE_DIR", "/tmp/nonexistent-xyz-123")
	_, err := FindRepoPath()
	if err == nil {
		t.Error("expected error for invalid AGM_SOURCE_DIR")
	}
}
