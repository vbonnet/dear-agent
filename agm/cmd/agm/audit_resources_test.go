package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/internal/gittest"
)

type fakeActiveSessionStore struct {
	names []string
	err   error
}

func (s *fakeActiveSessionStore) ListActiveSessions(context.Context) ([]string, error) {
	return s.names, s.err
}

func (s *fakeActiveSessionStore) Close() error { return nil }

func TestGetActiveSessionsFromDoltAggregatesConfiguredWorkspaces(t *testing.T) {
	previousConfigs := activeSessionStoreConfigs
	previousOpen := openActiveSessionStore
	t.Cleanup(func() {
		activeSessionStoreConfigs = previousConfigs
		openActiveSessionStore = previousOpen
	})

	activeSessionStoreConfigs = func() ([]*dolt.Config, error) {
		return []*dolt.Config{
			{Workspace: "personal"},
			{Workspace: "oss"},
		}, nil
	}
	openActiveSessionStore = func(config *dolt.Config) (activeSessionStore, error) {
		names := map[string][]string{
			"personal": {"api-only-personal"},
			"oss":      {"api-only-oss"},
		}
		return &fakeActiveSessionStore{names: names[config.Workspace]}, nil
	}

	active, err := getActiveSessionsFromDolt(context.Background())
	if err != nil {
		t.Fatalf("getActiveSessionsFromDolt: %v", err)
	}
	for _, name := range []string{"api-only-personal", "api-only-oss"} {
		if !active[name] {
			t.Errorf("aggregated active set omitted %q: %#v", name, active)
		}
	}
}

func TestGetActiveSessionsFromDoltFailsWhenAnyWorkspaceIsUnavailable(t *testing.T) {
	previousConfigs := activeSessionStoreConfigs
	previousOpen := openActiveSessionStore
	t.Cleanup(func() {
		activeSessionStoreConfigs = previousConfigs
		openActiveSessionStore = previousOpen
	})

	activeSessionStoreConfigs = func() ([]*dolt.Config, error) {
		return []*dolt.Config{{Workspace: "personal"}, {Workspace: "oss"}}, nil
	}
	openActiveSessionStore = func(config *dolt.Config) (activeSessionStore, error) {
		if config.Workspace == "oss" {
			return nil, errors.New("store offline")
		}
		return &fakeActiveSessionStore{names: []string{"personal-live"}}, nil
	}

	_, err := getActiveSessionsFromDolt(context.Background())
	if err == nil || !strings.Contains(err.Error(), `workspace "oss"`) {
		t.Fatalf("getActiveSessionsFromDolt error = %v, want oss workspace failure", err)
	}
}

// ---------------------------------------------------------------------------
// Command registration
// ---------------------------------------------------------------------------

func TestAuditResourcesCmd_Registration(t *testing.T) {
	found := false
	for _, cmd := range auditTrailCmd.Commands() {
		if cmd.Use == "resources" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'resources' to be registered under audit-trail command")
	}
}

func TestAuditResourcesCmd_Short(t *testing.T) {
	if auditResourcesCmd.Short == "" {
		t.Error("Expected non-empty Short description")
	}
}

func TestAuditResourcesCmd_Long(t *testing.T) {
	if len(auditResourcesCmd.Long) < 50 {
		t.Error("Expected detailed Long description")
	}
}

func TestAuditResourcesCmd_RunE(t *testing.T) {
	if auditResourcesCmd.RunE == nil {
		t.Error("Expected RunE to be set")
	}
}

// ---------------------------------------------------------------------------
// Flag registration
// ---------------------------------------------------------------------------

func TestAuditResourcesCmd_FixFlag(t *testing.T) {
	flag := auditResourcesCmd.Flags().Lookup("fix")
	if flag == nil {
		t.Fatal("Expected --fix flag")
		return
	}
	if flag.DefValue != "false" {
		t.Errorf("Expected default false, got %q", flag.DefValue)
	}
}

func TestAuditResourcesCmd_WorktreesDirFlag(t *testing.T) {
	flag := auditResourcesCmd.Flags().Lookup("worktrees-dir")
	if flag == nil {
		t.Fatal("Expected --worktrees-dir flag")
		return
	}
	if flag.DefValue != "" {
		t.Errorf("Expected empty default, got %q", flag.DefValue)
	}
}

func TestAuditResourcesCmd_ReposFlag(t *testing.T) {
	flag := auditResourcesCmd.Flags().Lookup("repos")
	if flag == nil {
		t.Fatal("Expected --repos flag")
	}
}

// ---------------------------------------------------------------------------
// isSessionName
// ---------------------------------------------------------------------------

func TestIsSessionName_Valid(t *testing.T) {
	cases := []string{
		"ecstatic-sinoussi-9a19da",
		"great-jackson-294857",
		"amazing-easley-b5d9f9",
		"fervent-cori-34653d",
	}
	for _, name := range cases {
		if !isSessionName(name) {
			t.Errorf("isSessionName(%q) = false, want true", name)
		}
	}
}

func TestIsSessionName_Invalid(t *testing.T) {
	cases := []string{
		"ai-tools-agm-bus-launchd", // branch name, not session
		"main",
		"feat/some-feature",
		"",
		"single",
		"two-parts",
	}
	for _, name := range cases {
		if isSessionName(name) {
			t.Errorf("isSessionName(%q) = true, want false", name)
		}
	}
}

// ---------------------------------------------------------------------------
// isGitWorktree
// ---------------------------------------------------------------------------

func TestIsGitWorktree_LinkedWorktree(t *testing.T) {
	dir := t.TempDir()
	// Linked worktrees have a .git FILE (not directory)
	gitFile := filepath.Join(dir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /some/repo/.git/worktrees/test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !isGitWorktree(dir) {
		t.Error("Expected isGitWorktree to return true for directory with .git file")
	}
}

func TestIsGitWorktree_MainWorktree(t *testing.T) {
	dir := t.TempDir()
	// Main worktrees have a .git DIRECTORY
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if isGitWorktree(dir) {
		t.Error("Expected isGitWorktree to return false for main worktree (directory .git)")
	}
}

func TestIsGitWorktree_NoGit(t *testing.T) {
	dir := t.TempDir()
	if isGitWorktree(dir) {
		t.Error("Expected isGitWorktree to return false for non-git directory")
	}
}

func TestIsGitWorktree_NonexistentDir(t *testing.T) {
	if isGitWorktree("/nonexistent/path/xyz") {
		t.Error("Expected isGitWorktree to return false for nonexistent directory")
	}
}

// ---------------------------------------------------------------------------
// walkWorktreesDir
// ---------------------------------------------------------------------------

func TestWalkWorktreesDir_TwoLevelStructure(t *testing.T) {
	base := t.TempDir()

	// Create: base/ai-tools/session-abc123/
	sessionDir := filepath.Join(base, "ai-tools", "session-abc123")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create: base/ai-tools/other-session/
	other := filepath.Join(base, "ai-tools", "other-session")
	if err := os.MkdirAll(other, 0755); err != nil {
		t.Fatal(err)
	}
	// Create nested deeper (should NOT be returned)
	nested := filepath.Join(base, "ai-tools", "other-session", "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	paths, err := walkWorktreesDir(base)
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]bool{}
	for _, p := range paths {
		found[p] = true
	}

	if !found[sessionDir] {
		t.Errorf("Expected %s in results", sessionDir)
	}
	if !found[other] {
		t.Errorf("Expected %s in results", other)
	}
	if found[nested] {
		t.Errorf("Did not expect deeply nested %s in results", nested)
	}
}

func TestWalkWorktreesDir_Empty(t *testing.T) {
	base := t.TempDir()
	paths, err := walkWorktreesDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Errorf("Expected empty result for empty dir, got %v", paths)
	}
}

func TestWalkWorktreesDir_Nonexistent(t *testing.T) {
	paths, err := walkWorktreesDir("/nonexistent/path")
	// Should not return a hard error — just empty
	if err == nil && len(paths) != 0 {
		t.Errorf("Expected empty result for nonexistent dir, got %v", paths)
	}
}

// ---------------------------------------------------------------------------
// expandHomePath helper
// ---------------------------------------------------------------------------

func TestExpandHomePath_TildePrefix(t *testing.T) {
	home := "/home/testuser"
	got := expandHomePath("~/foo", home)
	want := "/home/testuser/foo"
	if got != want {
		t.Errorf("expandHomePath(~/foo) = %q, want %q", got, want)
	}
}

func TestExpandHomePath_AbsolutePath(t *testing.T) {
	got := expandHomePath("/absolute/path", "/home/user")
	if got != "/absolute/path" {
		t.Errorf("Expected unchanged absolute path, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// scanWorktreesDir
// ---------------------------------------------------------------------------

func TestScanWorktreesDir_FlagsOrphans(t *testing.T) {
	base := t.TempDir()

	// Create a fake linked worktree directory (active session)
	activeDir := filepath.Join(base, "repo", "active-session-abc123")
	if err := os.MkdirAll(activeDir, 0755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(activeDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /fake/repo/.git/worktrees/active\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a fake linked worktree directory (orphaned session)
	orphanDir := filepath.Join(base, "repo", "orphan-session-def456")
	if err := os.MkdirAll(orphanDir, 0755); err != nil {
		t.Fatal(err)
	}
	gitFile2 := filepath.Join(orphanDir, ".git")
	if err := os.WriteFile(gitFile2, []byte("gitdir: /fake/repo/.git/worktrees/orphan\n"), 0644); err != nil {
		t.Fatal(err)
	}

	activeSessions := map[string]bool{
		"active-session-abc123": true,
	}

	orphans := scanWorktreesDir(base, activeSessions)
	if len(orphans) != 1 {
		t.Fatalf("Expected 1 orphan, got %d: %v", len(orphans), orphans)
	}
	if orphans[0].Path != orphanDir {
		t.Errorf("Expected orphan path %q, got %q", orphanDir, orphans[0].Path)
	}
	if orphans[0].Reason != "no-active-session" {
		t.Errorf("Expected reason 'no-active-session', got %q", orphans[0].Reason)
	}
}

func TestScanWorktreesDir_SkipsNonWorktrees(t *testing.T) {
	base := t.TempDir()

	// Create a plain directory (no .git file) — should NOT be flagged
	plainDir := filepath.Join(base, "repo", "plain-dir")
	if err := os.MkdirAll(plainDir, 0755); err != nil {
		t.Fatal(err)
	}

	orphans := scanWorktreesDir(base, map[string]bool{})
	if len(orphans) != 0 {
		t.Errorf("Expected no orphans for non-worktree dir, got %v", orphans)
	}
}

func TestScanWorktreesDir_SkipsActiveSessions(t *testing.T) {
	base := t.TempDir()

	wtDir := filepath.Join(base, "repo", "my-active-session-aabbcc")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(wtDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /repo/.git/worktrees/x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	activeSessions := map[string]bool{
		"my-active-session-aabbcc": true,
	}

	orphans := scanWorktreesDir(base, activeSessions)
	if len(orphans) != 0 {
		t.Errorf("Expected no orphans for active session, got %v", orphans)
	}
}

func TestRemoveOrphanWorktreePreservesGitLockedCheckout(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	worktree := filepath.Join(base, "worktree")
	auditResourcesGitRun(t, "init", "-q", "-b", "main", repo)
	auditResourcesGitRun(t, "-C", repo, "config", "user.name", "AGM Audit Test")
	auditResourcesGitRun(t, "-C", repo, "config", "user.email", "agm-audit@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("audit test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	auditResourcesGitRun(t, "-C", repo, "add", "README.md")
	auditResourcesGitRun(t, "-C", repo, "commit", "-q", "-m", "initial")
	auditResourcesGitRun(t, "-C", repo, "worktree", "add", "-q", "-b", "audit-test", worktree)
	auditResourcesGitRun(t, "-C", repo, "worktree", "lock", "--reason", "active-safe-pr", worktree)

	orphan := orphanedWorktree{Path: worktree, Repo: repo, Reason: "no-active-session"}
	err := removeOrphanWorktree(orphan)
	if err == nil || !strings.Contains(err.Error(), "refusing direct removal") {
		t.Fatalf("removeOrphanWorktree(locked) = %v, want conservative rejection", err)
	}
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("locked orphan checkout was removed: %v", err)
	}
	porcelain := auditResourcesGitRun(t, "-C", repo, "worktree", "list", "--porcelain")
	if !strings.Contains(porcelain, "locked active-safe-pr") {
		t.Fatalf("locked orphan metadata changed:\n%s", porcelain)
	}

	auditResourcesGitRun(t, "-C", repo, "worktree", "unlock", worktree)
	if err := removeOrphanWorktree(orphan); err != nil {
		t.Fatalf("removeOrphanWorktree(after unlock) = %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("orphan checkout after retry stat = %v, want not exist", err)
	}
}

func auditResourcesGitRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := gittest.Command(t, "", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// repoFromGitDir
// ---------------------------------------------------------------------------

func TestRepoFromGitDir_Valid(t *testing.T) {
	gitDir := "/home/user/src/myrepo/.git/worktrees/feature-branch"
	got := repoFromGitDir(gitDir)
	want := "/home/user/src/myrepo"
	if got != want {
		t.Errorf("repoFromGitDir(%q) = %q, want %q", gitDir, got, want)
	}
}

func TestRepoFromGitDir_EmptyInput(t *testing.T) {
	got := repoFromGitDir("")
	if got != "" {
		t.Errorf("repoFromGitDir('') = %q, want ''", got)
	}
}
