package stophook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// initRepo creates an empty git repo at dir with a known identity and an
// initial commit so branch / log / status commands behave deterministically.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	gittest.Run(t, dir, "init", "-q", "--initial-branch=main")
	gittest.Run(t, dir, "config", "user.email", "test@example.com")
	gittest.Run(t, dir, "config", "user.name", "Test User")
	gittest.Run(t, dir, "config", "commit.gpgsign", "false")
	if err := writeFile(filepath.Join(dir, "README.md"), "init\n"); err != nil {
		t.Fatalf("write README: %v", err)
	}
	gittest.Run(t, dir, "add", "README.md")
	gittest.Run(t, dir, "commit", "-q", "-m", "init")
}

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o600)
}

func TestGitStatus_Clean(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)

	files, err := GitStatus(dir)
	if err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected clean repo, got dirty: %v", files)
	}
}

func TestGitStatus_Dirty(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	if err := writeFile(filepath.Join(dir, "dirty.txt"), "uncommitted"); err != nil {
		t.Fatalf("write: %v", err)
	}

	files, err := GitStatus(dir)
	if err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 dirty file, got %v", files)
	}
	if !strings.Contains(files[0], "dirty.txt") {
		t.Fatalf("expected dirty.txt in output, got %q", files[0])
	}
}

func TestGitStatus_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := GitStatus(dir)
	if err == nil {
		t.Fatal("expected error from non-git dir, got nil")
	}
}

func TestGitUnpushedCommits_NoUpstream_NotAnError(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)

	// No upstream configured; git log @{u}..HEAD exits non-zero. The function
	// must classify this as "nothing to report", not an error.
	commits, err := GitUnpushedCommits(dir)
	if err != nil {
		t.Fatalf("GitUnpushedCommits should swallow no-upstream error, got: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected no commits, got %v", commits)
	}
}

func TestGitExtraBranches(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)
	gittest.Run(t, dir, "branch", "feature-a")
	gittest.Run(t, dir, "branch", "feature-b")

	extras, err := GitExtraBranches(dir)
	if err != nil {
		t.Fatalf("GitExtraBranches: %v", err)
	}
	// Current branch (main) is skipped; feature-a and feature-b remain.
	seen := map[string]bool{}
	for _, b := range extras {
		seen[b] = true
	}
	if !seen["feature-a"] || !seen["feature-b"] {
		t.Fatalf("expected feature-a and feature-b in extras, got %v", extras)
	}
	if seen["main"] {
		t.Fatalf("current branch should be filtered out, got %v", extras)
	}
}

func TestGitWorktrees(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir)

	worktrees, err := GitWorktrees(dir)
	if err != nil {
		t.Fatalf("GitWorktrees: %v", err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("expected 1 worktree (main), got %d: %+v", len(worktrees), worktrees)
	}
	if worktrees[0].Branch != "refs/heads/main" {
		t.Fatalf("expected branch refs/heads/main, got %q", worktrees[0].Branch)
	}
	if worktrees[0].Bare {
		t.Fatalf("expected non-bare worktree, got bare=true")
	}
}

func TestIsGitRepo(t *testing.T) {
	t.Run("yes", func(t *testing.T) {
		dir := t.TempDir()
		initRepo(t, dir)
		if !IsGitRepo(dir) {
			t.Fatalf("expected true for initialized repo")
		}
	})
	t.Run("no", func(t *testing.T) {
		dir := t.TempDir()
		if IsGitRepo(dir) {
			t.Fatalf("expected false for non-repo dir")
		}
	})
}

func TestParseLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace only", "  \n\n  ", nil},
		{"single line", "abc\n", []string{"abc"}},
		{"multi line", "a\nb\nc\n", []string{"a", "b", "c"}},
		{"trailing whitespace trimmed", "a\nb\n\n", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLines([]byte(tt.in))
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("line %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
