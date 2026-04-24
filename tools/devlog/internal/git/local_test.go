package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLocalRepository(t *testing.T) {
	path := "/path/to/repo"
	repo := NewLocalRepository(path)

	if repo.Path != path {
		t.Errorf("NewLocalRepository() Path = %s, want %s", repo.Path, path)
	}
}

func TestLocalRepository_Exists_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewLocalRepository(filepath.Join(tmpDir, "nonexistent"))

	if repo.Exists() {
		t.Error("Exists() should return false for nonexistent repository")
	}
}

func TestLocalRepository_Exists_NotBareRepo(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewLocalRepository(tmpDir)

	// Empty directory is not a bare repo
	if repo.Exists() {
		t.Error("Exists() should return false for non-bare repository")
	}
}

func TestLocalRepository_Exists_PartialStructure(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewLocalRepository(tmpDir)

	// Create only HEAD file, missing objects/ and refs/
	headPath := filepath.Join(tmpDir, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should still return false (need all three: HEAD, objects/, refs/)
	if repo.Exists() {
		t.Error("Exists() should return false for partial bare repo structure")
	}
}

func TestLocalRepository_Exists_ValidBareRepo(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewLocalRepository(tmpDir)

	// Create bare repo structure: HEAD, objects/, refs/
	headPath := filepath.Join(tmpDir, "HEAD")
	objectsPath := filepath.Join(tmpDir, "objects")
	refsPath := filepath.Join(tmpDir, "refs")

	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(objectsPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(refsPath, 0755); err != nil {
		t.Fatal(err)
	}

	if !repo.Exists() {
		t.Error("Exists() should return true for valid bare repo structure")
	}
}

func TestParseWorktreeList_Empty(t *testing.T) {
	worktrees, err := parseWorktreeList("")
	if err != nil {
		t.Errorf("parseWorktreeList() error = %v, want nil", err)
	}
	if len(worktrees) != 0 {
		t.Errorf("parseWorktreeList() returned %d worktrees, want 0", len(worktrees))
	}
}

func TestParseWorktreeList_Single(t *testing.T) {
	output := `worktree ~/repo/main
HEAD abc123def456
branch refs/heads/main
`

	worktrees, err := parseWorktreeList(output)
	if err != nil {
		t.Errorf("parseWorktreeList() error = %v, want nil", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("parseWorktreeList() returned %d worktrees, want 1", len(worktrees))
	}

	wt := worktrees[0]
	if wt.Path != "~/repo/main" {
		t.Errorf("worktree.Path = %s, want ~/repo/main", wt.Path)
	}
	if wt.Name != "main" {
		t.Errorf("worktree.Name = %s, want main", wt.Name)
	}
	if wt.Commit != "abc123def456" {
		t.Errorf("worktree.Commit = %s, want abc123def456", wt.Commit)
	}
	if wt.Branch != "main" {
		t.Errorf("worktree.Branch = %s, want main", wt.Branch)
	}
}

func TestParseWorktreeList_Multiple(t *testing.T) {
	output := `worktree ~/repo/main
HEAD abc123
branch refs/heads/main

worktree ~/repo/feature
HEAD def456
branch refs/heads/feature-branch

worktree ~/repo/detached
HEAD 789abc
detached
`

	worktrees, err := parseWorktreeList(output)
	if err != nil {
		t.Errorf("parseWorktreeList() error = %v, want nil", err)
	}

	if len(worktrees) != 3 {
		t.Fatalf("parseWorktreeList() returned %d worktrees, want 3", len(worktrees))
	}

	// Check first worktree
	if worktrees[0].Name != "main" {
		t.Errorf("worktrees[0].Name = %s, want main", worktrees[0].Name)
	}
	if worktrees[0].Branch != "main" {
		t.Errorf("worktrees[0].Branch = %s, want main", worktrees[0].Branch)
	}

	// Check second worktree
	if worktrees[1].Name != "feature" {
		t.Errorf("worktrees[1].Name = %s, want feature", worktrees[1].Name)
	}
	if worktrees[1].Branch != "feature-branch" {
		t.Errorf("worktrees[1].Branch = %s, want feature-branch", worktrees[1].Branch)
	}

	// Check third worktree (detached)
	if worktrees[2].Name != "detached" {
		t.Errorf("worktrees[2].Name = %s, want detached", worktrees[2].Name)
	}
	if worktrees[2].Branch != "" {
		t.Errorf("worktrees[2].Branch = %s, want empty (detached HEAD)", worktrees[2].Branch)
	}
}

func TestParseWorktreeList_DetachedHead(t *testing.T) {
	output := `worktree ~/repo/detached
HEAD 1234567890abcdef
detached
`

	worktrees, err := parseWorktreeList(output)
	if err != nil {
		t.Errorf("parseWorktreeList() error = %v, want nil", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("parseWorktreeList() returned %d worktrees, want 1", len(worktrees))
	}

	wt := worktrees[0]
	if wt.Branch != "" {
		t.Errorf("worktree.Branch = %s, want empty string for detached HEAD", wt.Branch)
	}
	if wt.Commit != "1234567890abcdef" {
		t.Errorf("worktree.Commit = %s, want 1234567890abcdef", wt.Commit)
	}
}

func TestLocalRepository_CreateWorktree_RepoNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewLocalRepository(filepath.Join(tmpDir, "nonexistent"))

	err := repo.CreateWorktree("test", "main")
	if err == nil {
		t.Error("CreateWorktree() should error when repository doesn't exist")
	}
}

func TestLocalRepository_GetCurrentBranch_WorktreeNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	repo := NewLocalRepository(tmpDir)

	branch, err := repo.GetCurrentBranch("nonexistent")
	if err != nil {
		t.Errorf("GetCurrentBranch() error = %v, want nil", err)
	}
	if branch != "" {
		t.Errorf("GetCurrentBranch() = %s, want empty string for nonexistent worktree", branch)
	}
}

// Note: Clone, CreateWorktree, ListWorktrees, and GetCurrentBranch with real git
// would require integration tests with actual git repositories. Those are tested
// separately to avoid slow unit tests that depend on external git command.
