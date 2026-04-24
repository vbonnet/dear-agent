package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupTestRepo creates a test git repository with an initial commit.
// Returns the repository path and cleanup function.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")

	if err := os.Mkdir(repoPath, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.CommandContext(context.Background(), "git", "init", repoPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git init: %v", err)
	}

	// Configure git
	exec.CommandContext(context.Background(), "git", "-C", repoPath, "config", "user.email", "test@example.com").Run()
	exec.CommandContext(context.Background(), "git", "-C", repoPath, "config", "user.name", "Test User").Run()

	// Create initial commit (required for worktree)
	readmeFile := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmeFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write README: %v", err)
	}

	exec.CommandContext(context.Background(), "git", "-C", repoPath, "add", ".").Run()
	cmd = exec.CommandContext(context.Background(), "git", "-C", repoPath, "commit", "-m", "Initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	cleanup := func() {
		// Cleanup is automatic via t.TempDir()
	}

	return repoPath, cleanup
}

func TestProvisioner_Provision_CreatesWorktree(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "test-session-123"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	// First provision should create worktree
	worktreePath, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("Provision() failed: %v", err)
	}

	// Verify worktree was created
	if !provisioner.Exists() {
		t.Error("Worktree directory does not exist after provisioning")
	}

	// Verify worktree path is correct
	expectedPath := filepath.Join(worktreeBase, "session-test-session-123")
	if worktreePath != expectedPath {
		t.Errorf("Expected worktree path %q, got %q", expectedPath, worktreePath)
	}

	// Verify README.md exists in worktree
	readmePath := filepath.Join(worktreePath, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		t.Error("README.md does not exist in worktree")
	}
}

func TestProvisioner_Provision_Idempotent(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "test-session-456"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	// First provision
	path1, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("First Provision() failed: %v", err)
	}

	// Second provision should return same path
	path2, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("Second Provision() failed: %v", err)
	}

	if path1 != path2 {
		t.Errorf("Provision not idempotent: first=%q, second=%q", path1, path2)
	}

	// Verify cache was used (should be instant)
	start := time.Now()
	path3, _ := provisioner.Provision()
	duration := time.Since(start)

	if duration > 10*time.Millisecond {
		t.Errorf("Cached provision took too long: %v (expected <10ms)", duration)
	}

	if path3 != path1 {
		t.Errorf("Cached provision returned different path: %q", path3)
	}
}

func TestProvisioner_Exists_DetectsExisting(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "existing-session"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	// Initially should not exist
	if provisioner.Exists() {
		t.Error("Exists() returned true before worktree created")
	}

	// Create worktree
	_, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("Provision() failed: %v", err)
	}

	// Now should exist
	if !provisioner.Exists() {
		t.Error("Exists() returned false after worktree created")
	}
}

func TestProvisioner_GetPath_Deterministic(t *testing.T) {
	config := &ProvisionerConfig{
		WorktreeBase:   "/tmp/worktrees",
		RepositoryRoot: "/tmp/repo",
		SessionID:      "abc123",
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	// GetPath should always return same value for same session
	path1 := provisioner.GetPath()
	path2 := provisioner.GetPath()

	if path1 != path2 {
		t.Errorf("GetPath not deterministic: %q != %q", path1, path2)
	}

	expectedPath := "/tmp/worktrees/session-abc123"
	if path1 != expectedPath {
		t.Errorf("GetPath() = %q, want %q", path1, expectedPath)
	}
}

func TestProvisioner_CustomBranchName(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "custom-branch-session"
	customBranch := "feature-custom"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
		BranchName:     customBranch,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	worktreePath, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("Provision() with custom branch failed: %v", err)
	}

	// Verify worktree was created
	if !provisioner.Exists() {
		t.Error("Worktree with custom branch does not exist")
	}

	// Verify branch name is correct
	cmd := exec.CommandContext(context.Background(), "git", "-C", worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get branch name: %v", err)
	}

	branchName := strings.TrimSpace(string(output))
	if branchName != customBranch {
		t.Errorf("Expected branch name %q, got %q", customBranch, branchName)
	}
}

func TestProvisioner_GetBranchName(t *testing.T) {
	tests := []struct {
		name           string
		sessionID      string
		customBranch   string
		expectedBranch string
	}{
		{
			name:           "default branch name from session ID",
			sessionID:      "abc123",
			customBranch:   "",
			expectedBranch: "session-abc123",
		},
		{
			name:           "custom branch name",
			sessionID:      "abc123",
			customBranch:   "my-feature",
			expectedBranch: "my-feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ProvisionerConfig{
				WorktreeBase:   "/tmp/worktrees",
				RepositoryRoot: "/tmp/repo",
				SessionID:      tt.sessionID,
				BranchName:     tt.customBranch,
			}

			cache := NewProvisionCache()
			provisioner := NewProvisioner(config, cache)

			branchName := provisioner.GetBranchName()
			if branchName != tt.expectedBranch {
				t.Errorf("GetBranchName() = %q, want %q", branchName, tt.expectedBranch)
			}
		})
	}
}

func TestProvisioner_ErrorHandling_InvalidRepo(t *testing.T) {
	config := &ProvisionerConfig{
		WorktreeBase:   t.TempDir(),
		RepositoryRoot: "/nonexistent/repo",
		SessionID:      "error-session",
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	// Provision should fail for invalid repo
	_, err := provisioner.Provision()
	if err == nil {
		t.Error("Expected error for invalid repository, got nil")
	}

	// Error message should mention git
	if !strings.Contains(err.Error(), "git") {
		t.Errorf("Expected error to mention 'git', got: %v", err)
	}
}

func TestProvisioner_CacheIntegration(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "cache-test"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()

	// Verify cache is empty
	if cache.Size() != 0 {
		t.Errorf("Expected empty cache, got size %d", cache.Size())
	}

	provisioner := NewProvisioner(config, cache)

	// First provision populates cache
	path1, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("Provision() failed: %v", err)
	}

	// Verify cache was populated
	if cache.Size() != 1 {
		t.Errorf("Expected cache size 1 after provision, got %d", cache.Size())
	}

	cachedPath := cache.Get(sessionID)
	if cachedPath != path1 {
		t.Errorf("Cache returned wrong path: %q, want %q", cachedPath, path1)
	}

	// Second provision uses cache
	path2, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("Second Provision() failed: %v", err)
	}

	if path2 != path1 {
		t.Errorf("Second provision returned different path: %q", path2)
	}

	// Cache size should still be 1
	if cache.Size() != 1 {
		t.Errorf("Expected cache size 1 after second provision, got %d", cache.Size())
	}
}

// Benchmark worktree provisioning
func BenchmarkProvisioner_Provision_Cached(b *testing.B) {
	// Setup is outside the benchmark timing
	tmpDir := b.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	os.Mkdir(repoPath, 0755)
	exec.CommandContext(context.Background(), "git", "init", repoPath).Run()
	exec.CommandContext(context.Background(), "git", "-C", repoPath, "config", "user.email", "bench@test.com").Run()
	exec.CommandContext(context.Background(), "git", "-C", repoPath, "config", "user.name", "Bench").Run()

	readme := filepath.Join(repoPath, "README.md")
	os.WriteFile(readme, []byte("bench"), 0644)
	exec.CommandContext(context.Background(), "git", "-C", repoPath, "add", ".").Run()
	exec.CommandContext(context.Background(), "git", "-C", repoPath, "commit", "-m", "Init").Run()

	worktreeBase := filepath.Join(tmpDir, "worktrees")
	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      "bench-session",
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	// Create worktree once before benchmarking
	provisioner.Provision()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provisioner.Provision()
	}
}
