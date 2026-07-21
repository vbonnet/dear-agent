package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func chdirOutsideGitRepository(t *testing.T) {
	t.Helper()
	git := NewGitWorktreeManager()
	candidateParents := []string{os.TempDir()}
	if cwd, err := os.Getwd(); err == nil && git.IsGitRepository(cwd) {
		repoRoot, rootErr := git.GetRepositoryRoot(cwd)
		if rootErr != nil {
			t.Fatalf("find invoking repository root: %v", rootErr)
		}
		for parent := filepath.Dir(repoRoot); ; parent = filepath.Dir(parent) {
			candidateParents = append(candidateParents, parent)
			if filepath.Dir(parent) == parent {
				break
			}
		}
	}

	var failures []string
	for _, parent := range candidateParents {
		// t.TempDir inherits TMPDIR and may remain inside the invoking repository.
		//nolint:usetesting // The explicit parent is the isolation mechanism under test.
		dir, err := os.MkdirTemp(parent, "wayfinder-sandbox-test-")
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if git.IsGitRepository(dir) {
			failures = append(failures, dir+" is inside a Git repository")
			if err := os.RemoveAll(dir); err != nil {
				failures = append(failures, "remove rejected directory: "+err.Error())
			}
			continue
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(dir); err != nil {
				t.Errorf("remove isolated working directory %q: %v", dir, err)
			}
		})
		t.Chdir(dir)
		return
	}

	t.Fatalf("create working directory outside Git: %s", strings.Join(failures, "; "))
}

func TestCreateSandbox(t *testing.T) {
	// Setup
	chdirOutsideGitRepository(t)
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Test
	sandbox, err := manager.CreateSandbox("test-project")
	if err != nil {
		t.Fatalf("CreateSandbox failed: %v", err)
	}

	// Verify
	if sandbox.Name != "test-project" {
		t.Errorf("Expected name 'test-project', got '%s'", sandbox.Name)
	}

	if sandbox.ID == "" {
		t.Error("Sandbox ID should not be empty")
	}

	// Check directory exists
	sandboxPath := filepath.Join(tempDir, sandbox.ID)
	if _, err := os.Stat(sandboxPath); os.IsNotExist(err) {
		t.Error("Sandbox directory was not created")
	}

	// Check metadata file exists
	metadataPath := filepath.Join(sandboxPath, ".wayfinder-project")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Error("Metadata file was not created")
	}
}

func TestListSandboxes(t *testing.T) {
	// Setup
	chdirOutsideGitRepository(t)
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create multiple sandboxes
	_, err := manager.CreateSandbox("project-1")
	if err != nil {
		t.Fatalf("Failed to create project-1: %v", err)
	}

	_, err = manager.CreateSandbox("project-2")
	if err != nil {
		t.Fatalf("Failed to create project-2: %v", err)
	}

	// List sandboxes
	sandboxes, err := manager.ListSandboxes()
	if err != nil {
		t.Fatalf("ListSandboxes failed: %v", err)
	}

	// Verify
	if len(sandboxes) != 2 {
		t.Errorf("Expected 2 sandboxes, got %d", len(sandboxes))
	}
}

func TestCleanupSandbox(t *testing.T) {
	// Setup
	chdirOutsideGitRepository(t)
	tempDir := t.TempDir()
	manager := NewManager(tempDir)

	// Create sandbox
	sandbox, err := manager.CreateSandbox("test-cleanup")
	if err != nil {
		t.Fatalf("CreateSandbox failed: %v", err)
	}

	// Verify sandbox exists
	sandboxPath := filepath.Join(tempDir, sandbox.ID)
	if _, err := os.Stat(sandboxPath); os.IsNotExist(err) {
		t.Fatal("Sandbox directory should exist before cleanup")
	}

	// Cleanup
	err = manager.CleanupSandbox(sandbox.Name)
	if err != nil {
		t.Fatalf("CleanupSandbox failed: %v", err)
	}

	// Verify sandbox removed
	if _, err := os.Stat(sandboxPath); !os.IsNotExist(err) {
		t.Error("Sandbox directory should be removed after cleanup")
	}
}

func TestCleanupSandboxPreservesLockedWorktreeAndMetadata(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	sandboxBase := filepath.Join(base, "sandboxes")
	manager := NewManager(sandboxBase)
	sandbox := &Sandbox{
		ID:            "locked-sandbox",
		Name:          "locked-sandbox",
		CreatedAt:     time.Now(),
		LastUsedAt:    time.Now(),
		GitRepository: repo,
	}

	gitSandboxTestRun(t, "init", "-q", "-b", "main", repo)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("sandbox test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitSandboxTestRun(t, "-C", repo, "add", "README.md")
	gitSandboxTestRun(t, "-C", repo, "commit", "-q", "-m", "initial")
	worktree, err := manager.git.CreateWorktree(sandbox.ID, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	sandbox.WorktreePath = worktree
	sandboxPath := filepath.Join(sandboxBase, sandbox.ID)
	if err := os.MkdirAll(sandboxPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := manager.writeMetadata(filepath.Join(sandboxPath, ".wayfinder-project"), sandbox); err != nil {
		t.Fatal(err)
	}
	gitSandboxTestRun(t, "-C", repo, "worktree", "lock", "--reason", "active-safe-pr", worktree)

	err = manager.CleanupSandbox(sandbox.ID)
	if err == nil || !strings.Contains(err.Error(), "preserving") {
		t.Fatalf("CleanupSandbox(locked) = %v, want preserving error", err)
	}
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("protected worktree was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandboxPath, ".wayfinder-project")); err != nil {
		t.Fatalf("sandbox metadata was removed: %v", err)
	}
	porcelain := gitSandboxTestRun(t, "-C", repo, "worktree", "list", "--porcelain")
	if !strings.Contains(porcelain, "locked active-safe-pr") {
		t.Fatalf("protected worktree lock changed:\n%s", porcelain)
	}

	gitSandboxTestRun(t, "-C", repo, "worktree", "unlock", worktree)
	if err := manager.CleanupSandbox(sandbox.ID); err != nil {
		t.Fatalf("CleanupSandbox(after unlock) = %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree after retry stat = %v, want not exist", err)
	}
	if _, err := os.Stat(sandboxPath); !os.IsNotExist(err) {
		t.Fatalf("sandbox metadata after retry stat = %v, want not exist", err)
	}
}

// gitSandboxTestRun runs a hermetic Git command. The arguments carry their own
// repository selector (`-C <repo>` or a path operand), so the command keeps
// running in the test process working directory.
func gitSandboxTestRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := gittest.Command(t, "", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func TestBasicSandboxOperationsDoNotMutateHostRepository(t *testing.T) {
	hostDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	git := NewGitWorktreeManager()
	hostIsRepository := git.IsGitRepository(hostDir)
	var hostRepoRoot string
	var before []string
	if hostIsRepository {
		hostRepoRoot, err = git.GetRepositoryRoot(hostDir)
		if err != nil {
			t.Fatal(err)
		}
		before, err = git.ListWorktrees(hostDir)
		if err != nil {
			t.Fatal(err)
		}
		before = worktreesWithinRepository(hostRepoRoot, before)
		t.Setenv("TMPDIR", hostDir)
	}

	chdirOutsideGitRepository(t)
	isolatedDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if git.IsGitRepository(isolatedDir) {
		t.Fatalf("isolated working directory %q is inside a Git repository", isolatedDir)
	}
	manager := NewManager(t.TempDir())
	for _, name := range []string{"first", "second"} {
		if _, err := manager.CreateSandbox(name); err != nil {
			t.Fatalf("CreateSandbox(%q): %v", name, err)
		}
	}
	sandboxes, err := manager.ListSandboxes()
	if err != nil {
		t.Fatal(err)
	}
	if len(sandboxes) != 2 {
		t.Fatalf("ListSandboxes() returned %d sandboxes, want 2", len(sandboxes))
	}

	if hostIsRepository {
		after, err := git.ListWorktrees(hostDir)
		if err != nil {
			t.Fatal(err)
		}
		after = worktreesWithinRepository(hostRepoRoot, after)
		if !slices.Equal(before, after) {
			t.Fatalf("host worktree inventory changed: before=%q after=%q", before, after)
		}
	}
}

func worktreesWithinRepository(root string, worktrees []string) []string {
	var scoped []string
	for _, worktree := range worktrees {
		rel, err := filepath.Rel(root, worktree)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			scoped = append(scoped, worktree)
		}
	}
	return scoped
}

func TestSandboxGitWorktreeLifecycleIsIsolated(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	runSandboxTestGit(t, "init", "-q", "-b", "main", repo)
	repo = canonicalSandboxTestPath(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("sandbox test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runSandboxTestGit(t, "-C", repo, "add", "README.md")
	runSandboxTestGit(t, "-C", repo, "commit", "-q", "-m", "initial")
	t.Chdir(repo)

	manager := NewManager(filepath.Join(base, "sandboxes"))
	sandbox, err := manager.CreateSandbox("git-lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	cleanupNeeded := true
	t.Cleanup(func() {
		if !cleanupNeeded {
			return
		}
		if err := manager.CleanupSandbox(sandbox.ID); err != nil {
			t.Errorf("cleanup sandbox %q: %v", sandbox.ID, err)
		}
	})
	wantWorktree := filepath.Join(repo, ".worktrees", sandbox.ID)
	if sandbox.WorktreePath != wantWorktree || sandbox.GitRepository != repo {
		t.Fatalf("sandbox git ownership = worktree:%q repo:%q, want %q and %q", sandbox.WorktreePath, sandbox.GitRepository, wantWorktree, repo)
	}
	worktrees, err := manager.git.ListWorktrees(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(worktrees, wantWorktree) {
		t.Fatalf("created worktree %q not registered in %q", wantWorktree, worktrees)
	}

	if err := manager.CleanupSandbox(sandbox.ID); err != nil {
		t.Fatal(err)
	}
	cleanupNeeded = false
	if _, err := os.Stat(wantWorktree); !os.IsNotExist(err) {
		t.Fatalf("worktree path survived cleanup: %v", err)
	}
	worktrees, err = manager.git.ListWorktrees(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 1 || worktrees[0] != repo {
		t.Fatalf("worktree registry after cleanup = %q, want only %q", worktrees, repo)
	}
}

func canonicalSandboxTestPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("canonicalize %q: %v", path, err)
	}
	return canonical
}

// runSandboxTestGit runs a hermetic Git command under a timeout in its own
// process group. Like gitSandboxTestRun, the arguments carry their own
// repository selector.
func runSandboxTestGit(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	base := gittest.Command(t, "", args...)
	cmd := exec.CommandContext(ctx, base.Path, base.Args[1:]...)
	cmd.Dir, cmd.Env = base.Dir, base.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("git %s timed out: %v: %s", strings.Join(args, " "), ctx.Err(), out)
	}
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func TestPathResolver(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	manager := NewManager(tempDir)
	resolver := NewPathResolver(manager)

	// Test fallback (no active sandbox)
	path := resolver.GetSandboxPath("sessions")
	if !filepath.IsAbs(path) {
		t.Error("Path should be absolute")
	}

	// Should contain .wayfinder for fallback mode
	if !contains(path, ".wayfinder") {
		t.Error("Fallback path should contain .wayfinder")
	}
}

func contains(s, substr string) bool {
	return filepath.Base(filepath.Dir(s)) == substr || filepath.Base(filepath.Dir(filepath.Dir(s))) == substr
}
