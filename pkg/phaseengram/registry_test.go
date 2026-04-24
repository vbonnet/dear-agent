package phaseengram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveEngramPath_KnownPhases(t *testing.T) {
	// Skip if engram repo is not available (CI environment)
	_, err := findEngramRepoRoot()
	if err != nil {
		t.Skipf("engram repo not found, skipping: %v", err)
	}

	phases := []string{"CHARTER", "W0", "PROBLEM", "D1", "RESEARCH", "D2", "DESIGN", "S6", "PLAN", "S7", "BUILD", "S8", "RETRO", "S11"}

	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			path, err := ResolveEngramPath(phase)
			if err != nil {
				t.Fatalf("ResolveEngramPath(%q) error: %v", phase, err)
			}
			if path == "" {
				t.Fatalf("ResolveEngramPath(%q) returned empty path", phase)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("ResolveEngramPath(%q) returned non-existent path: %s", phase, path)
			}
		})
	}
}

func TestResolveEngramPath_CaseInsensitive(t *testing.T) {
	_, err := findEngramRepoRoot()
	if err != nil {
		t.Skipf("engram repo not found, skipping: %v", err)
	}

	path1, err := ResolveEngramPath("charter")
	if err != nil {
		t.Fatalf("ResolveEngramPath(charter) error: %v", err)
	}
	path2, err := ResolveEngramPath("CHARTER")
	if err != nil {
		t.Fatalf("ResolveEngramPath(CHARTER) error: %v", err)
	}
	if path1 != path2 {
		t.Errorf("case mismatch: %q != %q", path1, path2)
	}
}

func TestResolveEngramPath_UnknownPhase(t *testing.T) {
	_, err := ResolveEngramPath("NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for unknown phase, got nil")
	}
	if !strings.Contains(err.Error(), "unknown phase") {
		t.Errorf("expected 'unknown phase' error, got: %v", err)
	}
}

func TestResolveEngramHash_Deterministic(t *testing.T) {
	_, err := findEngramRepoRoot()
	if err != nil {
		t.Skipf("engram repo not found, skipping: %v", err)
	}

	hash1, err := ResolveEngramHash("CHARTER")
	if err != nil {
		t.Fatalf("ResolveEngramHash(CHARTER) error: %v", err)
	}
	hash2, err := ResolveEngramHash("CHARTER")
	if err != nil {
		t.Fatalf("ResolveEngramHash(CHARTER) second call error: %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("non-deterministic hashes: %q != %q", hash1, hash2)
	}
	if !strings.HasPrefix(hash1, "sha256:") {
		t.Errorf("hash should start with sha256:, got %q", hash1)
	}
}

func TestResolveEngramPathAndHash(t *testing.T) {
	_, err := findEngramRepoRoot()
	if err != nil {
		t.Skipf("engram repo not found, skipping: %v", err)
	}

	path, hashValue, err := ResolveEngramPathAndHash("BUILD")
	if err != nil {
		t.Fatalf("ResolveEngramPathAndHash(BUILD) error: %v", err)
	}
	if path == "" {
		t.Fatal("path is empty")
	}
	if !strings.HasPrefix(hashValue, "sha256:") {
		t.Errorf("hash should start with sha256:, got %q", hashValue)
	}
}

func TestKnownPhases(t *testing.T) {
	phases := KnownPhases()
	if len(phases) == 0 {
		t.Fatal("KnownPhases() returned empty list")
	}
	// Should include at least CHARTER and BUILD
	found := map[string]bool{}
	for _, p := range phases {
		found[p] = true
	}
	for _, expected := range []string{"CHARTER", "BUILD", "RETRO"} {
		if !found[expected] {
			t.Errorf("KnownPhases() missing %q", expected)
		}
	}
}

func TestFindEngramRepoRoot_EnvVar(t *testing.T) {
	// Create a temp directory with the expected structure
	tmpDir := t.TempDir()
	workflowDir := filepath.Join(tmpDir, engramWorkflowDir)
	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		t.Fatalf("failed to create workflow dir: %v", err)
	}

	t.Setenv("ENGRAM_REPO_ROOT", tmpDir)
	root, err := findEngramRepoRoot()
	if err != nil {
		t.Fatalf("findEngramRepoRoot() with env var error: %v", err)
	}
	if root != tmpDir {
		t.Errorf("findEngramRepoRoot() = %q, want %q", root, tmpDir)
	}
}

func TestFindEngramRepoRoot_EnvVarInvalid(t *testing.T) {
	// Set env var to a directory without the workflow structure
	tmpDir := t.TempDir()
	t.Setenv("ENGRAM_REPO_ROOT", tmpDir)
	// Override HOME to avoid finding real repos
	t.Setenv("HOME", tmpDir)

	_, err := findEngramRepoRoot()
	if err == nil {
		t.Fatal("expected error when no engram repo found, got nil")
	}
	if !strings.Contains(err.Error(), "engram repo not found") {
		t.Errorf("expected 'engram repo not found' error, got: %v", err)
	}
}

func TestFindEngramRepoRoot_WorktreeSubdirectory(t *testing.T) {
	// Simulate the worktrees path layout where a subdirectory contains the repo
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	// Unset ENGRAM_REPO_ROOT so it doesn't take priority
	t.Setenv("ENGRAM_REPO_ROOT", "")

	// Create a worktrees directory with a branch subdirectory containing the workflow dir
	worktreeBase := filepath.Join(tmpDir, "src/ws/oss/worktrees/engram")
	branchDir := filepath.Join(worktreeBase, "my-branch")
	workflowDir := filepath.Join(branchDir, engramWorkflowDir)
	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		t.Fatalf("failed to create workflow dir: %v", err)
	}

	root, err := findEngramRepoRoot()
	if err != nil {
		t.Fatalf("findEngramRepoRoot() error: %v", err)
	}
	if root != branchDir {
		t.Errorf("findEngramRepoRoot() = %q, want %q", root, branchDir)
	}
}

func TestResolveEngramPath_FileNotFound(t *testing.T) {
	// Create a temp engram repo with workflow dir but no actual engram files
	tmpDir := t.TempDir()
	workflowDir := filepath.Join(tmpDir, engramWorkflowDir)
	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		t.Fatalf("failed to create workflow dir: %v", err)
	}

	t.Setenv("ENGRAM_REPO_ROOT", tmpDir)

	_, err := ResolveEngramPath("CHARTER")
	if err == nil {
		t.Fatal("expected error when engram file missing, got nil")
	}
	if !strings.Contains(err.Error(), "engram file not found") {
		t.Errorf("expected 'engram file not found' error, got: %v", err)
	}
}

func TestResolveEngramHash_UnknownPhase(t *testing.T) {
	_, err := ResolveEngramHash("NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for unknown phase, got nil")
	}
	if !strings.Contains(err.Error(), "unknown phase") {
		t.Errorf("expected 'unknown phase' error, got: %v", err)
	}
}

func TestResolveEngramPathAndHash_UnknownPhase(t *testing.T) {
	_, _, err := ResolveEngramPathAndHash("NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for unknown phase, got nil")
	}
	if !strings.Contains(err.Error(), "unknown phase") {
		t.Errorf("expected 'unknown phase' error, got: %v", err)
	}
}

func TestKnownPhases_AllMapped(t *testing.T) {
	phases := KnownPhases()
	// Verify count matches the map (each key should appear exactly once)
	if len(phases) != len(phaseToEngram) {
		t.Errorf("KnownPhases() returned %d phases, want %d", len(phases), len(phaseToEngram))
	}
	// Verify no duplicates
	seen := make(map[string]bool)
	for _, p := range phases {
		if seen[p] {
			t.Errorf("duplicate phase: %s", p)
		}
		seen[p] = true
	}
}

func TestKnownPhasesString(t *testing.T) {
	result := knownPhases()
	if result == "" {
		t.Fatal("knownPhases() returned empty string")
	}
	// Should contain comma-separated values
	if !strings.Contains(result, ",") {
		t.Errorf("knownPhases() should be comma-separated, got: %s", result)
	}
}
