package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestDefaultGitCommitClassifiesRepositoryState(t *testing.T) {
	sandbox := gittest.New(t)

	tests := []struct {
		name   string
		mutate func(t *testing.T, sandbox *gittest.Sandbox, repo string)
		dirty  bool
	}{
		{name: "clean branch"},
		{
			name: "tracked modification",
			mutate: func(t *testing.T, _ *gittest.Sandbox, repo string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			dirty: true,
		},
		{
			name: "untracked Go input",
			mutate: func(t *testing.T, _ *gittest.Sandbox, repo string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repo, "untracked.go"), []byte("package untracked\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			dirty: true,
		},
		{
			name: "ignored Go source input",
			mutate: func(t *testing.T, _ *gittest.Sandbox, repo string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("check-*.go\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(repo, "check-local.go"), []byte("package main\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			dirty: true,
		},
		{
			name: "ignored build output directory",
			mutate: func(t *testing.T, sandbox *gittest.Sandbox, repo string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("bin/\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				sandbox.Run(t, repo, "add", ".gitignore")
				sandbox.Run(t, repo, "commit", "-m", "ignore bin")
				if err := os.MkdirAll(filepath.Join(repo, "bin"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(repo, "bin", "agm"), []byte("binary"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			dirty: false,
		},
		{
			name: "detached HEAD",
			mutate: func(t *testing.T, sandbox *gittest.Sandbox, repo string) {
				t.Helper()
				sandbox.Run(t, repo, "checkout", "--detach", "HEAD")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := sandbox.NewRepo(t)
			if tc.mutate != nil {
				tc.mutate(t, sandbox, repo)
			}
			revision := strings.TrimSpace(sandbox.Run(t, repo, "rev-parse", "--short=12", "HEAD"))
			want := revision
			if tc.dirty {
				want += "-dirty"
			}
			if got := defaultGitCommit(repo, runGit); got != want {
				t.Fatalf("defaultGitCommit() = %q, want %q", got, want)
			}
		})
	}
}

func TestDefaultGitCommitReturnsUnknownForIndeterminateGit(t *testing.T) {
	sandbox := gittest.New(t)
	repo := sandbox.NewRepo(t)

	t.Run("Git unavailable", func(t *testing.T) {
		unavailable := func(string, ...string) ([]byte, error) {
			return nil, exec.ErrNotFound
		}
		if got := defaultGitCommit(repo, unavailable); got != unknownGitCommit {
			t.Fatalf("defaultGitCommit() = %q, want %q", got, unknownGitCommit)
		}
	})

	t.Run("status failure", func(t *testing.T) {
		statusFailure := func(dir string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "status" {
				return nil, errors.New("status unavailable")
			}
			return runGit(dir, args...)
		}
		if got := defaultGitCommit(repo, statusFailure); got != unknownGitCommit {
			t.Fatalf("defaultGitCommit() = %q, want %q", got, unknownGitCommit)
		}
	})

	t.Run("not a repository", func(t *testing.T) {
		if got := defaultGitCommit(t.TempDir(), runGit); got != unknownGitCommit {
			t.Fatalf("defaultGitCommit() = %q, want %q", got, unknownGitCommit)
		}
	})
}

func TestShortGitRevisionValidation(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{value: "0123456789ab", want: true},
		{value: "0123456789abcdef", want: true},
		{value: "0123456789a", want: false},
		{value: "0123456789AB", want: false},
		{value: "0123456789ag", want: false},
		{value: "0123456789ab-dirty", want: false},
	} {
		if got := isShortGitRevision(tc.value); got != tc.want {
			t.Errorf("isShortGitRevision(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}
