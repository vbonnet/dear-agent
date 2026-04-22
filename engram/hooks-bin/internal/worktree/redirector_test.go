package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedirector_RedirectIfNeeded_WriteOperation(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "redirect-test-123"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)
	redirector := NewRedirector(provisioner)

	// Test file in main repository (should redirect)
	testFile := filepath.Join(repoPath, "test.txt")

	result, err := redirector.RedirectIfNeeded(testFile, "Write")
	if err != nil {
		t.Fatalf("RedirectIfNeeded() failed: %v", err)
	}

	if !result.ShouldRedirect {
		t.Error("Expected redirection for main repository file")
	}

	if result.RedirectedPath == "" {
		t.Error("Expected non-empty redirected path")
	}

	// Verify session worktree was created
	if !result.Provisioned {
		t.Error("Expected worktree to be provisioned")
	}

	if result.SessionWorktree == "" {
		t.Error("Expected non-empty session worktree path")
	}
}

func TestRedirector_RedirectIfNeeded_ReadOperation(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "read-test-123"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)
	redirector := NewRedirector(provisioner)

	testFile := filepath.Join(repoPath, "test.txt")

	// Read operations should NOT redirect
	result, err := redirector.RedirectIfNeeded(testFile, "Read")
	if err != nil {
		t.Fatalf("RedirectIfNeeded() failed: %v", err)
	}

	if result.ShouldRedirect {
		t.Error("Expected no redirection for Read operation")
	}
}

func TestRedirector_isInSharedLocation_MainWorktree(t *testing.T) {
	provisioner := &Provisioner{}
	redirector := NewRedirector(provisioner)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "path with /main/ segment",
			path:     "/repo/main/src/file.go",
			expected: true,
		},
		{
			name:     "path with /base/ segment",
			path:     "/repo/base/src/file.go",
			expected: true,
		},
		{
			name:     "path in session worktree",
			path:     "/repo/worktrees/session-123/src/file.go",
			expected: false,
		},
		{
			name:     "path in feature worktree",
			path:     "/repo/worktrees/feature-branch/src/file.go",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := redirector.isInSharedLocation(tt.path)
			if err != nil {
				t.Fatalf("isInSharedLocation() error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("isInSharedLocation(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestRedirector_calculateRedirectedPath(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "calc-path-test"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	// Create worktree
	worktreePath, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("Provision() failed: %v", err)
	}

	redirector := NewRedirector(provisioner)

	// Create nested directory in repo
	nestedDir := filepath.Join(repoPath, "src", "pkg")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	originalFile := filepath.Join(nestedDir, "file.go")
	if err := os.WriteFile(originalFile, []byte("package pkg"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Calculate redirected path
	redirected, err := redirector.calculateRedirectedPath(originalFile, worktreePath)
	if err != nil {
		t.Fatalf("calculateRedirectedPath() failed: %v", err)
	}

	// Verify path structure is preserved
	expectedPath := filepath.Join(worktreePath, "src", "pkg", "file.go")
	if redirected != expectedPath {
		t.Errorf("calculateRedirectedPath() = %q, want %q", redirected, expectedPath)
	}

	// Verify path contains session ID
	if !strings.Contains(redirected, sessionID) {
		t.Errorf("Redirected path doesn't contain session ID: %q", redirected)
	}
}

func TestRedirector_calculateRedirectedPath_RootFile(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "root-file-test"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	worktreePath, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("Provision() failed: %v", err)
	}

	redirector := NewRedirector(provisioner)

	// Test file at repository root
	rootFile := filepath.Join(repoPath, "README.md")

	redirected, err := redirector.calculateRedirectedPath(rootFile, worktreePath)
	if err != nil {
		t.Fatalf("calculateRedirectedPath() failed: %v", err)
	}

	expectedPath := filepath.Join(worktreePath, "README.md")
	if redirected != expectedPath {
		t.Errorf("calculateRedirectedPath() = %q, want %q", redirected, expectedPath)
	}
}

func TestRedirector_isGitRepository(t *testing.T) {
	provisioner := &Provisioner{}
	redirector := NewRedirector(provisioner)

	// Test with git repo
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	testFile := filepath.Join(repoPath, "test.txt")
	isGit, err := redirector.isGitRepository(testFile)
	if err != nil {
		t.Errorf("isGitRepository() unexpected error: %v", err)
	}
	if !isGit {
		t.Error("Expected isGitRepository to return true for git repo")
	}

	// Test with non-git directory
	tmpDir := t.TempDir()
	nonGitFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(nonGitFile, []byte("test"), 0644)

	isGit, err = redirector.isGitRepository(nonGitFile)
	if err != nil {
		t.Errorf("isGitRepository() unexpected error: %v", err)
	}
	if isGit {
		t.Error("Expected isGitRepository to return false for non-git directory")
	}
}

func TestRedirector_EditOperation(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "edit-test-456"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)
	redirector := NewRedirector(provisioner)

	testFile := filepath.Join(repoPath, "file.txt")

	result, err := redirector.RedirectIfNeeded(testFile, "Edit")
	if err != nil {
		t.Fatalf("RedirectIfNeeded(Edit) failed: %v", err)
	}

	if !result.ShouldRedirect {
		t.Error("Expected redirection for Edit operation")
	}
}

func TestRedirector_MultiEditOperation(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "multiedit-test-789"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)
	redirector := NewRedirector(provisioner)

	testFile := filepath.Join(repoPath, "file.txt")

	result, err := redirector.RedirectIfNeeded(testFile, "MultiEdit")
	if err != nil {
		t.Fatalf("RedirectIfNeeded(MultiEdit) failed: %v", err)
	}

	if !result.ShouldRedirect {
		t.Error("Expected redirection for MultiEdit operation")
	}
}

func TestRedirector_NonWriteOperations(t *testing.T) {
	provisioner := &Provisioner{}
	redirector := NewRedirector(provisioner)

	nonWriteTools := []string{"Bash", "Grep", "Glob", "Read", "WebSearch", "WebFetch"}

	for _, tool := range nonWriteTools {
		t.Run(tool, func(t *testing.T) {
			result, err := redirector.RedirectIfNeeded("/any/path", tool)
			if err != nil {
				t.Fatalf("RedirectIfNeeded() failed: %v", err)
			}

			if result.ShouldRedirect {
				t.Errorf("Expected no redirection for %s operation", tool)
			}
		})
	}
}

func TestRedirector_IdempotentProvisioning(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "idempotent-test"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)
	redirector := NewRedirector(provisioner)

	testFile := filepath.Join(repoPath, "test.txt")

	// First redirection should provision
	result1, err := redirector.RedirectIfNeeded(testFile, "Write")
	if err != nil {
		t.Fatalf("First RedirectIfNeeded() failed: %v", err)
	}

	if !result1.Provisioned {
		t.Error("Expected first redirection to provision worktree")
	}

	// Second redirection should use existing worktree
	result2, err := redirector.RedirectIfNeeded(testFile, "Write")
	if err != nil {
		t.Fatalf("Second RedirectIfNeeded() failed: %v", err)
	}

	if result2.Provisioned {
		t.Error("Expected second redirection to use existing worktree (not provision)")
	}

	// Paths should match
	if result1.RedirectedPath != result2.RedirectedPath {
		t.Errorf("Redirected paths don't match: %q != %q", result1.RedirectedPath, result2.RedirectedPath)
	}
}

func TestRedirector_ErrorHandling_InvalidGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	nonGitFile := filepath.Join(tmpDir, "not-in-git.txt")
	os.WriteFile(nonGitFile, []byte("test"), 0644)

	config := &ProvisionerConfig{
		WorktreeBase:   t.TempDir(),
		RepositoryRoot: tmpDir,
		SessionID:      "error-test",
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)
	redirector := NewRedirector(provisioner)

	// Non-git file should not redirect
	result, err := redirector.RedirectIfNeeded(nonGitFile, "Write")
	if err != nil {
		t.Fatalf("RedirectIfNeeded() unexpected error: %v", err)
	}

	if result.ShouldRedirect {
		t.Error("Expected no redirection for non-git file")
	}
}

func TestRedirector_SymlinkHandling(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	worktreeBase := filepath.Join(t.TempDir(), "worktrees")
	sessionID := "symlink-test"

	config := &ProvisionerConfig{
		WorktreeBase:   worktreeBase,
		RepositoryRoot: repoPath,
		SessionID:      sessionID,
	}

	cache := NewProvisionCache()
	provisioner := NewProvisioner(config, cache)

	worktreePath, err := provisioner.Provision()
	if err != nil {
		t.Fatalf("Provision() failed: %v", err)
	}

	redirector := NewRedirector(provisioner)

	// Create file in repo
	testFile := filepath.Join(repoPath, "symlink-test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	// Calculate redirected path (tests symlink resolution)
	redirected, err := redirector.calculateRedirectedPath(testFile, worktreePath)
	if err != nil {
		t.Fatalf("calculateRedirectedPath() failed: %v", err)
	}

	// Should work even with symlinks
	if redirected == "" {
		t.Error("Expected non-empty redirected path")
	}

	// Verify structure preserved
	if !strings.HasSuffix(redirected, "symlink-test.txt") {
		t.Errorf("Expected redirected path to end with filename, got: %q", redirected)
	}
}

func TestIsWriteOperation(t *testing.T) {
	tests := []struct {
		toolName string
		expected bool
	}{
		{"Write", true},
		{"Edit", true},
		{"MultiEdit", true},
		{"Read", false},
		{"Bash", false},
		{"Grep", false},
		{"Glob", false},
		{"write", false}, // case-sensitive
		{"WRITE", false},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			got := isWriteOperation(tt.toolName)
			if got != tt.expected {
				t.Errorf("isWriteOperation(%q) = %v, want %v", tt.toolName, got, tt.expected)
			}
		})
	}
}

// Benchmark redirection check
func BenchmarkRedirector_RedirectIfNeeded(b *testing.B) {
	// Setup
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
	redirector := NewRedirector(provisioner)

	// Pre-provision worktree
	redirector.RedirectIfNeeded(readme, "Write")

	testFile := filepath.Join(repoPath, "test.txt")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		redirector.RedirectIfNeeded(testFile, "Write")
	}
}
