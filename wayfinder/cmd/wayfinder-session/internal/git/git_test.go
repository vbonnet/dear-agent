package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/archive"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()

	// Initialize git repo (hermetically: no host hooks, no host config)
	gittest.Run(t, tmpDir, "init")
	// The package under test starts its own Git processes, so persist the
	// sandbox hook path in this repository as well as applying it to setup.
	gittest.HardenRepo(t, tmpDir)

	// Configure git user (required for commits). The gittest sandbox already
	// supplies an identity to the Git commands this file runs, but the package
	// under test shells out to `git commit` with the ambient environment, so
	// the identity also has to live in the repository's own config.
	gittest.Run(t, tmpDir, "config", "user.name", "Test User")
	gittest.Run(t, tmpDir, "config", "user.email", "test@example.com")

	return tmpDir
}

func TestNew(t *testing.T) {
	g := New("/tmp/test")
	if g.projectDir != "/tmp/test" {
		t.Errorf("New() projectDir = %q, want %q", g.projectDir, "/tmp/test")
	}
}

func TestIsGitRepo(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() string
		expected bool
	}{
		{
			name: "valid git repo",
			setup: func() string {
				return setupGitRepo(t)
			},
			expected: true,
		},
		{
			name: "non-git directory",
			setup: func() string {
				return t.TempDir()
			},
			expected: false,
		},
		{
			name: "subdirectory within git repo",
			setup: func() string {
				// Create git repo
				repoDir := setupGitRepo(t)
				// Create subdirectory (wayfinder project location)
				subDir := filepath.Join(repoDir, "wf", "my-project")
				if err := os.MkdirAll(subDir, 0755); err != nil {
					t.Fatalf("failed to create subdir: %v", err)
				}
				return subDir
			},
			expected: true, // Should detect git repo from subdirectory!
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup()
			g := New(dir)

			if got := g.IsGitRepo(); got != tt.expected {
				t.Errorf("IsGitRepo() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCheckGitRepo(t *testing.T) {
	repoDir := setupGitRepo(t)
	gRepo := New(repoDir)
	isRepo, err := gRepo.CheckGitRepo()
	if err != nil || !isRepo {
		t.Errorf("CheckGitRepo(repoDir) = (%v, %v), want (true, nil)", isRepo, err)
	}

	nonRepoDir := t.TempDir()
	gNonRepo := New(nonRepoDir)
	isRepo, err = gNonRepo.CheckGitRepo()
	if err != nil || isRepo {
		t.Errorf("CheckGitRepo(nonRepoDir) = (%v, %v), want (false, nil)", isRepo, err)
	}
}

func TestIsGitWorktree(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() string
		expected bool
	}{
		{
			name: "worktree root",
			setup: func() string {
				return setupGitRepo(t)
			},
			expected: true,
		},
		{
			name: "subdirectory within worktree",
			setup: func() string {
				repoDir := setupGitRepo(t)
				subDir := filepath.Join(repoDir, "wf", "my-project")
				if err := os.MkdirAll(subDir, 0o755); err != nil {
					t.Fatalf("create project directory: %v", err)
				}
				return subDir
			},
			expected: true,
		},
		{
			name: "non-git directory",
			setup: func() string {
				return t.TempDir()
			},
			expected: false,
		},
		{
			name: "bare repository",
			setup: func() string {
				dir := t.TempDir()
				cmd := gittest.Command(t, "", "init", "--bare", dir)
				if err := cmd.Run(); err != nil {
					t.Fatalf("git init --bare: %v", err)
				}
				return dir
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.setup()).IsGitWorktree(); got != tt.expected {
				t.Errorf("IsGitWorktree() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCommitPhaseCompletion(t *testing.T) {
	// Setup git repo
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Create initial commit (git requires at least one commit)
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Project\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	addCmd := gittest.Command(t, repoDir, "add", "README.md")
	if err := addCmd.Run(); err != nil {
		t.Fatalf("git add README failed: %v", err)
	}
	commitCmd := gittest.Command(t, repoDir, "commit", "-m", "Initial commit")
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Create wayfinder files
	statusPath := filepath.Join(repoDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte("# Status\n"), 0644); err != nil {
		t.Fatalf("failed to write STATUS: %v", err)
	}

	historyPath := filepath.Join(repoDir, "WAYFINDER-HISTORY.jsonl")
	if err := os.WriteFile(historyPath, []byte("{}\n"), 0644); err != nil {
		t.Fatalf("failed to write HISTORY: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "PROBLEM-evidence.md"), []byte("# Evidence\n"), 0644); err != nil {
		t.Fatalf("failed to write phase artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "user-notes.md"), []byte("private notes\n"), 0644); err != nil {
		t.Fatalf("failed to write unrelated file: %v", err)
	}
	if err := gittest.Command(t, repoDir, "add", "user-notes.md").Run(); err != nil {
		t.Fatalf("stage unrelated file: %v", err)
	}

	// Commit phase completion
	err := g.CommitPhaseCompletion("PROBLEM", "success", "Completed discovery phase")
	if err != nil {
		t.Fatalf("CommitPhaseCompletion() error = %v", err)
	}

	// Verify commit was created
	logCmd := gittest.Command(t, repoDir, "log", "--format=%s", "-n", "1")
	output, err := logCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}

	subject := strings.TrimSpace(string(output))
	expectedSubject := "wayfinder: complete PROBLEM (success)"
	if subject != expectedSubject {
		t.Errorf("commit subject = %q, want %q", subject, expectedSubject)
	}

	// Verify commit message body
	msgCmd := gittest.Command(t, repoDir, "log", "--format=%B", "-n", "1")
	msgOutput, err := msgCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}

	commitMsg := string(msgOutput)
	if !strings.Contains(commitMsg, "Completed discovery phase") {
		t.Errorf("commit message missing context: %q", commitMsg)
	}
	if !strings.Contains(commitMsg, "Wayfinder-Phase: PROBLEM") {
		t.Errorf("commit message missing phase metadata: %q", commitMsg)
	}
	if !strings.Contains(commitMsg, "Wayfinder-Outcome: success") {
		t.Errorf("commit message missing outcome metadata: %q", commitMsg)
	}
	showCmd := gittest.Command(t, repoDir, "show", "--name-only", "--format=")
	showOutput, err := showCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git show: %v", err)
	}
	committed := string(showOutput)
	if !strings.Contains(committed, "PROBLEM-evidence.md") {
		t.Errorf("phase artifact was not committed:\n%s", committed)
	}
	if strings.Contains(committed, "user-notes.md") {
		t.Errorf("unrelated staged file was swept into phase commit:\n%s", committed)
	}
	stagedOutput, err := gittest.Command(t, repoDir, "diff", "--cached", "--name-only").CombinedOutput()
	if err != nil {
		t.Fatalf("git diff --cached: %v", err)
	}
	if !strings.Contains(string(stagedOutput), "user-notes.md") {
		t.Errorf("unrelated file no longer staged after scoped commit: %s", stagedOutput)
	}
}

func TestCommitPhaseCompletionIncludesDesignADRs(t *testing.T) {
	repoDir := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gittest.Command(t, repoDir, "add", "README.md").Run(); err != nil {
		t.Fatal(err)
	}
	if err := gittest.Command(t, repoDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatal(err)
	}

	for name, content := range map[string]string{
		"WAYFINDER-STATUS.md":     "# Status\n",
		"WAYFINDER-HISTORY.jsonl": "{}\n",
		"DESIGN-overview.md":      "# Design\n",
		"ARCHITECTURE.md":         "# Architecture\n",
		"ADR-001-storage.md":      "# ADR-001 Storage\n",
		"user-notes.md":           "private notes\n",
	} {
		if err := os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := gittest.Command(t, repoDir, "add", "user-notes.md").Run(); err != nil {
		t.Fatal(err)
	}

	if err := New(repoDir).CommitPhaseCompletion("DESIGN", "success", "Reviewed design documents"); err != nil {
		t.Fatalf("CommitPhaseCompletion(DESIGN): %v", err)
	}
	showOutput, err := gittest.Command(t, repoDir, "show", "--name-only", "--format=").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	committed := string(showOutput)
	for _, name := range []string{"WAYFINDER-STATUS.md", "WAYFINDER-HISTORY.jsonl", "DESIGN-overview.md", "ARCHITECTURE.md", "ADR-001-storage.md"} {
		if !strings.Contains(committed, name) {
			t.Errorf("DESIGN commit missing %s:\n%s", name, committed)
		}
	}
	if strings.Contains(committed, "user-notes.md") {
		t.Errorf("DESIGN commit swept unrelated staged file:\n%s", committed)
	}
}

func TestCommitRewindCommitsCanonicalMarkersAndExactArchive(t *testing.T) {
	repoDir := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gittest.Command(t, repoDir, "add", "README.md").Run(); err != nil {
		t.Fatal(err)
	}
	if err := gittest.Command(t, repoDir, "commit", "-m", "Initial").Run(); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		".gitignore":              "*.jsonl\n.wayfinder/archives/\nRETRO-retrospective.md\n",
		"WAYFINDER-STATUS.md":     "status: in-progress\n",
		"WAYFINDER-HISTORY.jsonl": "{}\n",
		"RETRO-retrospective.md":  "# Retro\n",
		"user-notes.md":           "private\n",
	} {
		if err := os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := gittest.Command(t, repoDir, "add", ".gitignore").Run(); err != nil {
		t.Fatal(err)
	}
	if err := gittest.Command(t, repoDir, "add", "user-notes.md").Run(); err != nil {
		t.Fatal(err)
	}
	archiveRef, err := archive.New(repoDir).ArchivePhase("BUILD")
	if err != nil {
		t.Fatalf("create rewind archive: %v", err)
	}
	sibling := filepath.Join(repoDir, ".wayfinder", "archives", "BUILD-sibling")
	if err := os.MkdirAll(sibling, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "unrelated.txt"), []byte("private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := New(repoDir).CommitRewind("BUILD", "DESIGN", archiveRef); err != nil {
		t.Fatalf("CommitRewind: %v", err)
	}
	showOutput, err := gittest.Command(t, repoDir, "show", "--name-only", "--format=").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	committed := string(showOutput)
	for _, name := range []string{"WAYFINDER-STATUS.md", "WAYFINDER-HISTORY.jsonl", "RETRO-retrospective.md", archiveRef.RelativePath() + "/WAYFINDER-STATUS.md"} {
		if !strings.Contains(committed, name) {
			t.Errorf("rewind commit missing %s:\n%s", name, committed)
		}
	}
	if strings.Contains(committed, "user-notes.md") {
		t.Errorf("rewind swept unrelated staged file:\n%s", committed)
	}
	if strings.Contains(committed, "BUILD-sibling") {
		t.Errorf("rewind committed sibling archive:\n%s", committed)
	}
}

// TestCommitSessionInit_NonGitRepo verifies CommitSessionInit is a no-op
// (returns nil, no error) outside a git repository so non-git workflows work.
func TestCommitSessionInit_NonGitRepo(t *testing.T) {
	g := New(t.TempDir())
	if err := g.CommitSessionInit("my-project"); err != nil {
		t.Errorf("CommitSessionInit() on non-git repo should return nil, got: %v", err)
	}
}

// TestCommitSessionInit_CommitsStatusFile verifies that CommitSessionInit
// creates a "wayfinder: init session" commit that includes WAYFINDER-STATUS.md,
// leaving the worktree clean so the first start-phase does not refuse
// with "uncommitted files detected" (ce-11fi bootstrap fix).
func TestCommitSessionInit_CommitsStatusFile(t *testing.T) {
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Seed the repo with an initial commit (git requires a HEAD before committing).
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\n"), 0644)
	gittest.Command(t, repoDir, "add", "README.md").Run()
	gittest.Command(t, repoDir, "commit", "-m", "Initial commit").Run()

	// Write the STATUS file (mirrors what `wayfinder session start` does).
	os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("schema_version: \"2.0\"\n"), 0644)

	if err := g.CommitSessionInit("my-project"); err != nil {
		t.Fatalf("CommitSessionInit() error = %v", err)
	}

	// Commit subject must contain the project name.
	out, err := gittest.Command(t, repoDir, "log", "--format=%s", "-n", "1").Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "wayfinder: init session my-project" {
		t.Errorf("commit subject = %q, want %q", got, "wayfinder: init session my-project")
	}

	// Worktree must be clean after CommitSessionInit so start-phase CHARTER succeeds.
	statusOut, err := gittest.Command(t, repoDir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status failed: %v", err)
	}
	if len(strings.TrimSpace(string(statusOut))) != 0 {
		t.Errorf("worktree should be clean after CommitSessionInit, got:\n%s", string(statusOut))
	}
}

// TestCommitSessionInit_NothingToCommit verifies CommitSessionInit is a no-op
// when WAYFINDER-STATUS.md is already committed.
func TestCommitSessionInit_NothingToCommit(t *testing.T) {
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("schema_version: \"2.0\"\n"), 0644)
	gittest.Command(t, repoDir, "add", ".").Run()
	gittest.Command(t, repoDir, "commit", "-m", "Add status").Run()

	if err := g.CommitSessionInit("my-project"); err != nil {
		t.Errorf("CommitSessionInit() with nothing to commit should not error, got: %v", err)
	}
}

// TestCommitSessionInit_MissingStatusFile verifies CommitSessionInit is a no-op
// when WAYFINDER-STATUS.md does not yet exist.
func TestCommitSessionInit_MissingStatusFile(t *testing.T) {
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Bootstrap git (git requires a HEAD for non-initial commits, but this test
	// never reaches the commit path so no HEAD is needed).
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\n"), 0644)
	gittest.Command(t, repoDir, "add", "README.md").Run()
	gittest.Command(t, repoDir, "commit", "-m", "Initial commit").Run()

	// No STATUS file written — CommitSessionInit should be a silent no-op.
	if err := g.CommitSessionInit("my-project"); err != nil {
		t.Errorf("CommitSessionInit() with no STATUS file should not error, got: %v", err)
	}
}

// TestCommitPhaseStart verifies that CommitPhaseStart creates a commit
// containing WAYFINDER-STATUS.md and WAYFINDER-HISTORY.jsonl with the correct
// "wayfinder: start <PHASE>" subject line.
func TestCommitPhaseStart(t *testing.T) {
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Create initial commit (git requires at least one commit to commit against)
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Project\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	gittest.Command(t, repoDir, "add", "README.md").Run()
	if err := gittest.Command(t, repoDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("initial commit failed: %v", err)
	}

	// Write marker files (mirrors what runStartPhase does before calling CommitPhaseStart)
	if err := os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("# Status\n"), 0644); err != nil {
		t.Fatalf("failed to write STATUS: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "WAYFINDER-HISTORY.jsonl"), []byte("{}\n"), 0644); err != nil {
		t.Fatalf("failed to write HISTORY: %v", err)
	}

	if err := g.CommitPhaseStart("CHARTER"); err != nil {
		t.Fatalf("CommitPhaseStart() error = %v", err)
	}

	// Verify commit subject
	out, err := gittest.Command(t, repoDir, "log", "--format=%s", "-n", "1").Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "wayfinder: start CHARTER" {
		t.Errorf("commit subject = %q, want %q", got, "wayfinder: start CHARTER")
	}

	// Verify both marker files are tracked (not untracked)
	statusOut, err := gittest.Command(t, repoDir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status failed: %v", err)
	}
	if len(strings.TrimSpace(string(statusOut))) != 0 {
		t.Errorf("worktree should be clean after CommitPhaseStart, got:\n%s", string(statusOut))
	}
}

func TestCommitPhaseStartForceAddsIgnoredCanonicalHistory(t *testing.T) {
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	for path, content := range map[string]string{
		".gitignore":              "*.jsonl\n",
		"README.md":               "# Test Project\n",
		"WAYFINDER-STATUS.md":     "# Status\n",
		"WAYFINDER-HISTORY.jsonl": "{}\n",
	} {
		if err := os.WriteFile(filepath.Join(repoDir, path), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := gittest.Command(t, repoDir, "add", ".gitignore", "README.md").Run(); err != nil {
		t.Fatalf("stage initial files: %v", err)
	}
	if err := gittest.Command(t, repoDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	if err := g.CommitPhaseStart("CHARTER"); err != nil {
		t.Fatalf("CommitPhaseStart() error = %v", err)
	}

	names, err := gittest.Command(t, repoDir, "show", "--name-only", "--format=", "HEAD").Output()
	if err != nil {
		t.Fatalf("git show: %v", err)
	}
	for _, marker := range []string{"WAYFINDER-STATUS.md", "WAYFINDER-HISTORY.jsonl"} {
		if !strings.Contains(string(names), marker) {
			t.Errorf("lifecycle commit missing ignored canonical marker %s:\n%s", marker, names)
		}
	}
}

func TestLifecycleCommitsStageLegacyHistoryDeletion(t *testing.T) {
	tests := map[string]func(*GitIntegrator) error{
		"start": func(g *GitIntegrator) error {
			return g.CommitPhaseStart("CHARTER")
		},
		"completion": func(g *GitIntegrator) error {
			return g.CommitPhaseCompletion("PROBLEM", "success", "")
		},
		"rewind": func(g *GitIntegrator) error {
			if err := os.WriteFile(filepath.Join(g.projectDir, "RETRO-retrospective.md"), []byte("# Retro\n"), 0o644); err != nil {
				return err
			}
			archiveRef, err := archive.New(g.projectDir).ArchivePhase("BUILD")
			if err != nil {
				return err
			}
			return g.CommitRewind("BUILD", "DESIGN", archiveRef)
		},
	}

	for name, commitLifecycle := range tests {
		t.Run(name, func(t *testing.T) {
			repoDir := setupGitRepo(t)
			for path, content := range map[string]string{
				"WAYFINDER-STATUS.md":  "# Status\n",
				"WAYFINDER-HISTORY.md": "{}\n",
			} {
				if err := os.WriteFile(filepath.Join(repoDir, path), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := gittest.Command(t, repoDir, "add", ".").Run(); err != nil {
				t.Fatal(err)
			}
			if err := gittest.Command(t, repoDir, "commit", "-m", "legacy history").Run(); err != nil {
				t.Fatal(err)
			}

			if err := os.Rename(
				filepath.Join(repoDir, "WAYFINDER-HISTORY.md"),
				filepath.Join(repoDir, "WAYFINDER-HISTORY.jsonl"),
			); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("# Updated status\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			if err := commitLifecycle(New(repoDir)); err != nil {
				t.Fatalf("lifecycle commit: %v", err)
			}
			status, err := gittest.Command(t, repoDir, "status", "--porcelain").Output()
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(status)) != "" {
				t.Fatalf("legacy migration left dirty worktree:\n%s", status)
			}
			names, err := gittest.Command(t, repoDir, "show", "--no-renames", "--name-only", "--format=", "HEAD").Output()
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"WAYFINDER-HISTORY.md", "WAYFINDER-HISTORY.jsonl"} {
				if !strings.Contains(string(names), want) {
					t.Errorf("migration commit missing %s:\n%s", want, names)
				}
			}
		})
	}
}

// TestCommitPhaseStart_NonGitRepo verifies CommitPhaseStart returns an error
// when the directory is not inside a git repository.
func TestCommitPhaseStart_NonGitRepo(t *testing.T) {
	g := New(t.TempDir())
	if err := g.CommitPhaseStart("CHARTER"); err == nil {
		t.Error("CommitPhaseStart() on non-git repo should return error")
	}
}

// TestCommitPhaseStart_NothingToCommit verifies CommitPhaseStart is a no-op
// when both marker files are already committed and unchanged.
func TestCommitPhaseStart_NothingToCommit(t *testing.T) {
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Create initial commit with the marker files already committed.
	os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("# Status\n"), 0644)
	os.WriteFile(filepath.Join(repoDir, "WAYFINDER-HISTORY.jsonl"), []byte("{}\n"), 0644)
	gittest.Command(t, repoDir, "add", ".").Run()
	gittest.Command(t, repoDir, "commit", "-m", "Add wayfinder files").Run()

	// CommitPhaseStart when nothing has changed should succeed silently.
	if err := g.CommitPhaseStart("PROBLEM"); err != nil {
		t.Errorf("CommitPhaseStart() with nothing to commit should not error, got: %v", err)
	}
}

// TestCommitPhaseStart_ScopedToMarkerFiles verifies CommitPhaseStart only
// commits WAYFINDER-STATUS.md and WAYFINDER-HISTORY.jsonl, leaving any other
// staged files untouched in the index (ce-fvkz regression guard).
func TestCommitPhaseStart_ScopedToMarkerFiles(t *testing.T) {
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Bootstrap the repo with an initial commit.
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\n"), 0644)
	gittest.Command(t, repoDir, "add", "README.md").Run()
	gittest.Command(t, repoDir, "commit", "-m", "Initial commit").Run()

	// Stage a user file that should NOT be swept up by CommitPhaseStart.
	userFile := filepath.Join(repoDir, "my-deliverable.md")
	os.WriteFile(userFile, []byte("# Work in progress\n"), 0644)
	gittest.Command(t, repoDir, "add", "my-deliverable.md").Run()

	// Write and let CommitPhaseStart commit only the marker files.
	os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("# Status\n"), 0644)
	os.WriteFile(filepath.Join(repoDir, "WAYFINDER-HISTORY.jsonl"), []byte("{}\n"), 0644)

	if err := g.CommitPhaseStart("DESIGN"); err != nil {
		t.Fatalf("CommitPhaseStart() error = %v", err)
	}

	// The user's staged file should still be in the index (not committed).
	statusOut, err := gittest.Command(t, repoDir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status failed: %v", err)
	}
	if !strings.Contains(string(statusOut), "my-deliverable.md") {
		t.Errorf("user's staged file should still be staged; got:\n%s", string(statusOut))
	}

	// Verify only the marker files appear in the last commit.
	showOut, err := gittest.Command(t, repoDir, "show", "--stat", "--format=", "HEAD").Output()
	if err != nil {
		t.Fatalf("git show failed: %v", err)
	}
	if strings.Contains(string(showOut), "my-deliverable.md") {
		t.Errorf("user file should not have been included in the commit:\n%s", string(showOut))
	}
}

// TestCommitPhaseStart_LeavesWorktreeCleanForNextTransition is the regression
// test for the ce-fvkz recurrence (2026-06-13): after start-phase writes marker
// files and calls CommitPhaseStart, GetUncommittedFilesInProjectDir must return
// an empty list so that the *next* start-phase does not refuse.
func TestCommitPhaseStart_LeavesWorktreeCleanForNextTransition(t *testing.T) {
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Bootstrap: initial commit so git has a HEAD.
	os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Repo\n"), 0644)
	gittest.Command(t, repoDir, "add", "README.md").Run()
	gittest.Command(t, repoDir, "commit", "-m", "Initial commit").Run()

	// Simulate what start-phase does: write marker files then auto-commit.
	os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("status: in_progress\n"), 0644)
	os.WriteFile(filepath.Join(repoDir, "WAYFINDER-HISTORY.jsonl"), []byte("[]\n"), 0644)

	if err := g.CommitPhaseStart("CHARTER"); err != nil {
		t.Fatalf("CommitPhaseStart() error = %v", err)
	}

	// The next start-phase uses GetUncommittedFilesInProjectDir to decide
	// whether to refuse. It must see an empty list after the auto-commit.
	uncommitted, err := g.GetUncommittedFilesInProjectDir()
	if err != nil {
		t.Fatalf("GetUncommittedFilesInProjectDir() error = %v", err)
	}
	if len(uncommitted) != 0 {
		t.Errorf("worktree has uncommitted files after CommitPhaseStart — next start-phase would refuse: %v", uncommitted)
	}
}

func TestCommitPhaseCompletion_NonGitRepo(t *testing.T) {
	// Non-git directory
	tmpDir := t.TempDir()
	g := New(tmpDir)

	err := g.CommitPhaseCompletion("PROBLEM", "success", "")
	if err == nil {
		t.Error("CommitPhaseCompletion() on non-git repo should return error")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("error should mention git repository, got: %v", err)
	}
}

func TestCommitPhaseCompletion_NothingToCommit(t *testing.T) {
	// Setup git repo
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Create initial commit
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	addCmd := gittest.Command(t, repoDir, "add", "README.md")
	addCmd.Run()
	commitCmd := gittest.Command(t, repoDir, "commit", "-m", "Initial commit")
	commitCmd.Run()

	// Create and commit wayfinder files
	statusPath := filepath.Join(repoDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte("# Status\n"), 0644); err != nil {
		t.Fatalf("failed to write STATUS: %v", err)
	}
	addCmd2 := gittest.Command(t, repoDir, "add", "WAYFINDER-STATUS.md")
	addCmd2.Run()
	commitCmd2 := gittest.Command(t, repoDir, "commit", "-m", "Add wayfinder files")
	commitCmd2.Run()

	// Try to commit again without changes (should not error)
	err := g.CommitPhaseCompletion("PROBLEM", "success", "")
	if err != nil {
		t.Errorf("CommitPhaseCompletion() with nothing to commit should not error, got: %v", err)
	}
}

func TestFormatCommitMessage(t *testing.T) {
	g := New("/tmp/test")

	tests := []struct {
		name     string
		phase    string
		outcome  string
		context  string
		contains []string
	}{
		{
			name:    "with context",
			phase:   "PROBLEM",
			outcome: "success",
			context: "Completed user interviews",
			contains: []string{
				"wayfinder: complete PROBLEM (success)",
				"Completed user interviews",
				"Wayfinder-Phase: PROBLEM",
				"Wayfinder-Outcome: success",
			},
		},
		{
			name:    "without context",
			phase:   "PLAN",
			outcome: "partial",
			context: "",
			contains: []string{
				"wayfinder: complete PLAN (partial)",
				"Wayfinder-Phase: PLAN",
				"Wayfinder-Outcome: partial",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := g.formatCommitMessage(tt.phase, tt.outcome, tt.context)

			for _, expected := range tt.contains {
				if !strings.Contains(msg, expected) {
					t.Errorf("formatCommitMessage() missing %q in:\n%s", expected, msg)
				}
			}
		})
	}
}

func TestGetCommitHash(t *testing.T) {
	// Setup git repo with a commit
	repoDir := setupGitRepo(t)
	g := New(repoDir)

	// Create initial commit
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}
	addCmd := gittest.Command(t, repoDir, "add", "README.md")
	addCmd.Run()
	commitCmd := gittest.Command(t, repoDir, "commit", "-m", "Initial commit")
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Get commit hash
	hash, err := g.GetCommitHash()
	if err != nil {
		t.Fatalf("GetCommitHash() error = %v", err)
	}

	// Verify hash format (40 hex characters)
	if len(hash) != 40 {
		t.Errorf("GetCommitHash() returned hash length = %d, want 40", len(hash))
	}

	// Verify hash is valid hex
	for _, c := range hash {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("GetCommitHash() returned invalid hex character: %c", c)
		}
	}
}

func TestIsSourceCodeFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		// Source code files (should return true)
		{"Go file", "main.go", true},
		{"Python file", "script.py", true},
		{"JavaScript file", "app.js", true},
		{"TypeScript file", "component.ts", true},
		{"JSX file", "App.jsx", true},
		{"TSX file", "Component.tsx", true},
		{"C file", "main.c", true},
		{"C++ file", "main.cpp", true},
		{"Java file", "Main.java", true},
		{"Ruby file", "script.rb", true},
		{"Rust file", "main.rs", true},
		{"PHP file", "index.php", true},

		// Non-code files (should return false)
		{"Markdown file", "README.md", false},
		{"YAML file", "config.yaml", false},
		{"YAML file (.yml)", "config.yml", false},
		{"JSON file", "package.json", false},
		{"Text file", "notes.txt", false},
		{"Shell script", "setup.sh", false},
		{"Bash script", "install.bash", false},
		{"No extension", "Makefile", false},
		{"Hidden file", ".gitignore", false},

		// Path with directory (should check extension only)
		{"Go file in subdirectory", "internal/validator/validator.go", true},
		{"MD file in subdirectory", "docs/guide.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSourceCodeFile(tt.filePath)
			if got != tt.want {
				t.Errorf("isSourceCodeFile(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestIsInProjectDir(t *testing.T) {
	tests := []struct {
		name       string
		filePath   string
		projectDir string
		want       bool
	}{
		{
			name:       "File directly in project dir",
			filePath:   "/tmp/test/src/wf/phase-boundary-self-check/research.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       true,
		},
		{
			name:       "File in subdirectory of project dir",
			filePath:   "/tmp/test/src/wf/phase-boundary-self-check/subdir/test.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       true,
		},
		{
			name:       "File outside project dir (sibling)",
			filePath:   "/tmp/test/src/wf/other-project/file.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       false,
		},
		{
			name:       "File outside project dir (parent)",
			filePath:   "/tmp/test/src/engram/main.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       false,
		},
		{
			name:       "File in similarly named dir (false match test)",
			filePath:   "/tmp/test/src/wf/phase-boundary-self-check-2/file.go",
			projectDir: "/tmp/test/src/wf/phase-boundary-self-check",
			want:       false,
		},
		{
			name:       "Relative path in project dir",
			filePath:   "research.go",
			projectDir: ".",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInProjectDir(tt.filePath, tt.projectDir)
			if got != tt.want {
				t.Errorf("isInProjectDir(%q, %q) = %v, want %v",
					tt.filePath, tt.projectDir, got, tt.want)
			}
		})
	}
}

func TestGetUncommittedFilesInProjectDir(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantFiles []string
		wantErr   bool
	}{
		{
			name: "No git repo (graceful handling)",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Clean project directory",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Create initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()
				return repoDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Uncommitted deliverable files in project directory",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Create initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()

				// Create uncommitted deliverable files
				os.WriteFile(filepath.Join(repoDir, "CHARTER-charter.md"), []byte("# Charter\n"), 0644)
				os.WriteFile(filepath.Join(repoDir, "PROBLEM-problem.md"), []byte("# Problem\n"), 0644)
				os.WriteFile(filepath.Join(repoDir, "WAYFINDER-STATUS.md"), []byte("# Status\n"), 0644)

				return repoDir
			},
			wantFiles: []string{"PROBLEM-problem.md", "CHARTER-charter.md", "WAYFINDER-STATUS.md"},
			wantErr:   false,
		},
		{
			name: "Uncommitted files with .wayfinder/ directory (should ignore .wayfinder/)",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()

				// Create .wayfinder directory with files (should be ignored)
				wayfinderDir := filepath.Join(repoDir, ".wayfinder")
				os.MkdirAll(wayfinderDir, 0755)
				os.WriteFile(filepath.Join(wayfinderDir, "archive.json"), []byte("{}"), 0644)

				// Create uncommitted deliverable
				os.WriteFile(filepath.Join(repoDir, "BUILD-implementation.md"), []byte("# Implementation\n"), 0644)

				return repoDir
			},
			wantFiles: []string{"BUILD-implementation.md"},
			wantErr:   false,
		},
		{
			name: "Only .wayfinder/ files uncommitted (should return empty)",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()

				// Create .wayfinder directory with files (should all be ignored)
				wayfinderDir := filepath.Join(repoDir, ".wayfinder")
				os.MkdirAll(wayfinderDir, 0755)
				os.WriteFile(filepath.Join(wayfinderDir, "metadata.json"), []byte("{}"), 0644)

				return repoDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Modified and untracked files",
			setup: func(t *testing.T) string {
				repoDir := setupGitRepo(t)
				// Create and commit initial files
				os.WriteFile(filepath.Join(repoDir, "CHARTER-charter.md"), []byte("# Charter revision one\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()

				// Modify committed file
				os.WriteFile(filepath.Join(repoDir, "CHARTER-charter.md"), []byte("# Charter v2\n"), 0644)

				// Add untracked file
				os.WriteFile(filepath.Join(repoDir, "PROBLEM-problem.md"), []byte("# Problem\n"), 0644)

				return repoDir
			},
			wantFiles: []string{"PROBLEM-problem.md", "CHARTER-charter.md"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := tt.setup(t)
			g := New(projectDir)

			gotFiles, err := g.GetUncommittedFilesInProjectDir()

			if (err != nil) != tt.wantErr {
				t.Errorf("GetUncommittedFilesInProjectDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(gotFiles) != len(tt.wantFiles) {
					t.Errorf("GetUncommittedFilesInProjectDir() returned %d files, want %d\nGot: %v\nWant: %v",
						len(gotFiles), len(tt.wantFiles), gotFiles, tt.wantFiles)
				}

				// Check each expected file is present
				for _, want := range tt.wantFiles {
					found := false
					for _, got := range gotFiles {
						if got == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("GetUncommittedFilesInProjectDir() missing expected file %q\nGot: %v\nWant: %v",
							want, gotFiles, tt.wantFiles)
					}
				}
			}
		})
	}
}

func TestGetModifiedSourceFiles(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) (repoDir, projectDir string)
		wantFiles  []string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "No git repo (graceful handling)",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				return tmpDir, tmpDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "No modified files",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Create initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()
				return repoDir, repoDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Modified source file outside project dir",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Create initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()

				// Create project dir subdirectory
				projectDir := filepath.Join(repoDir, "wf", "my-project")
				os.MkdirAll(projectDir, 0755)

				// Modify .go file outside project dir
				goFile := filepath.Join(repoDir, "main.go")
				os.WriteFile(goFile, []byte("package main\n"), 0644)

				return repoDir, projectDir
			},
			wantFiles: []string{"main.go"},
			wantErr:   false,
		},
		{
			name: "Modified source file inside project dir (should be ignored)",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()

				// Create project dir
				projectDir := filepath.Join(repoDir, "wf", "my-project")
				os.MkdirAll(projectDir, 0755)

				// Modify .go file inside project dir (should be ignored)
				goFile := filepath.Join(projectDir, "research.go")
				os.WriteFile(goFile, []byte("package main\n"), 0644)

				return repoDir, projectDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Modified non-code file (should be ignored)",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()

				// Modify markdown file (not source code)
				mdFile := filepath.Join(repoDir, "PLAN.md")
				os.WriteFile(mdFile, []byte("# Plan\n"), 0644)

				return repoDir, repoDir
			},
			wantFiles: []string{},
			wantErr:   false,
		},
		{
			name: "Multiple source files modified",
			setup: func(t *testing.T) (string, string) {
				repoDir := setupGitRepo(t)
				// Initial commit
				readmePath := filepath.Join(repoDir, "README.md")
				os.WriteFile(readmePath, []byte("# Test\n"), 0644)
				gittest.Command(t, repoDir, "add", ".").Run()
				gittest.Command(t, repoDir, "commit", "-m", "Initial").Run()

				// Modify multiple source files
				os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0644)
				os.WriteFile(filepath.Join(repoDir, "script.py"), []byte("print('hello')\n"), 0644)
				os.WriteFile(filepath.Join(repoDir, "app.js"), []byte("console.log('hi')\n"), 0644)

				return repoDir, repoDir
			},
			wantFiles: []string{"app.js", "main.go", "script.py"}, // Alphabetical order from git status
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoDir, projectDir := tt.setup(t)
			g := New(projectDir)

			gotFiles, err := g.GetModifiedSourceFiles(projectDir)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetModifiedSourceFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("GetModifiedSourceFiles() error = %v, want error containing %q", err, tt.wantErrMsg)
			}

			if !tt.wantErr {
				// Sort both slices for comparison (git status order may vary)
				gotStr := strings.Join(gotFiles, ",")
				wantStr := strings.Join(tt.wantFiles, ",")

				if len(gotFiles) != len(tt.wantFiles) {
					t.Errorf("GetModifiedSourceFiles() returned %d files, want %d\nGot: %v\nWant: %v",
						len(gotFiles), len(tt.wantFiles), gotFiles, tt.wantFiles)
				}

				// Check each expected file is present
				for _, want := range tt.wantFiles {
					found := false
					for _, got := range gotFiles {
						if got == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("GetModifiedSourceFiles() missing expected file %q\nGot: %v\nWant: %v",
							want, gotStr, wantStr)
					}
				}
			}

			// Cleanup
			_ = repoDir
		})
	}
}
