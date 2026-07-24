// Package gittest builds hermetic Git subprocesses for tests.
//
// Tests that create temporary repositories used to inherit the developer's
// real Git configuration. On 2026-07-10 a plain `go test -count=1 ./...` run
// picked up the host's global `core.hooksPath`, executed the real post-merge
// hook from two temporary repositories, and launched two repository-wide
// `agm worktree sweep --execute` processes that deleted two live worktrees
// (audit finding F-01, bead ce-3knl.1).
//
// Every helper here runs Git with a sandboxed HOME, no system/global
// configuration, an empty template directory, and an empty hooks directory
// forced on the command line. Command-line configuration wins over every
// other source and Git forwards it to the processes it spawns, so a
// sandboxed invocation cannot reach a host hook even indirectly.
package gittest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// dropEnvPrefixes name the environment that must not leak into a hermetic Git
// subprocess. Anything Git-scoped is rebuilt from scratch; HOME and
// XDG_CONFIG_HOME are redirected so `~/.gitconfig` and `~/.config/git/*`
// resolve inside the sandbox.
var dropEnvPrefixes = []string{
	"GIT_",
	"HOME=",
	"XDG_CONFIG_HOME=",
}

// Sandbox is a throwaway Git environment rooted in a test-owned directory.
// The zero value is not usable; obtain one from New.
type Sandbox struct {
	// Home is the sandboxed HOME. Git resolves ~/.gitconfig under it.
	Home string
	// HooksDir is an existing but empty directory forced as core.hooksPath.
	HooksDir string
	// TemplateDir is an existing but empty directory used for `git init`.
	TemplateDir string
	// ConfigFile is the sandbox's writable global config file. It starts
	// empty, so `git config --global` in a test stays inside the sandbox.
	ConfigFile string

	env  []string
	args []string
}

// New returns a Sandbox rooted in a directory owned by t. The directory and
// everything in it are removed when t finishes.
func New(t testing.TB) *Sandbox {
	t.Helper()

	root := t.TempDir()
	s := &Sandbox{
		Home:        filepath.Join(root, "home"),
		HooksDir:    filepath.Join(root, "hooks"),
		TemplateDir: filepath.Join(root, "template"),
		ConfigFile:  filepath.Join(root, "home", ".gitconfig"),
	}
	for _, dir := range []string{s.Home, s.HooksDir, s.TemplateDir, filepath.Join(s.Home, ".config", "git")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("gittest: create %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(s.ConfigFile, nil, 0o600); err != nil {
		t.Fatalf("gittest: create %s: %v", s.ConfigFile, err)
	}

	s.env = s.buildEnv()
	s.args = s.buildArgs()
	return s
}

func (s *Sandbox) buildEnv() []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+12)
	for _, kv := range base {
		if hasAnyPrefix(kv, dropEnvPrefixes) {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"HOME="+s.Home,
		"XDG_CONFIG_HOME="+filepath.Join(s.Home, ".config"),
		"GIT_CONFIG_GLOBAL="+s.ConfigFile,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_TEMPLATE_DIR="+s.TemplateDir,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=dear-agent test",
		"GIT_AUTHOR_EMAIL=test@dear-agent.invalid",
		"GIT_COMMITTER_NAME=dear-agent test",
		"GIT_COMMITTER_EMAIL=test@dear-agent.invalid",
	)
}

// buildArgs returns the `-c key=value` prefix applied to every invocation.
// Command-line configuration outranks every file-based source and Git
// re-exports it through GIT_CONFIG_PARAMETERS, so a hook cannot be
// reintroduced by a repository-local config or by a Git-spawned child.
func (s *Sandbox) buildArgs() []string {
	return []string{
		"-c", "core.hooksPath=" + s.HooksDir,
		"-c", "init.templateDir=" + s.TemplateDir,
		"-c", "init.defaultBranch=main",
		"-c", "commit.gpgsign=false",
		"-c", "tag.gpgsign=false",
		"-c", "gc.auto=0",
		"-c", "advice.detachedHead=false",
		"-c", "protocol.file.allow=always",
		"-c", "user.name=dear-agent test",
		"-c", "user.email=test@dear-agent.invalid",
	}
}

// Env returns the environment for a hermetic Git subprocess. Callers that
// build their own *exec.Cmd must assign it to Cmd.Env; an unset Cmd.Env
// inherits the host and defeats the sandbox.
func (s *Sandbox) Env() []string {
	out := make([]string, len(s.env))
	copy(out, s.env)
	return out
}

// Command returns a Git command that runs in dir with the sandbox applied.
func (s *Sandbox) Command(dir string, args ...string) *exec.Cmd {
	full := make([]string, 0, len(s.args)+len(args))
	full = append(full, s.args...)
	full = append(full, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = s.Env()
	return cmd
}

// Run executes a Git command in dir and returns its combined output, failing
// t if Git exits non-zero.
func (s *Sandbox) Run(t testing.TB, dir string, args ...string) string {
	t.Helper()
	out, err := s.Output(dir, args...)
	if err != nil {
		t.Fatalf("gittest: git %s failed in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return out
}

// Output executes a Git command in dir and returns its combined output
// without failing t, for callers asserting on failure.
func (s *Sandbox) Output(dir string, args ...string) (string, error) {
	raw, err := s.Command(dir, args...).CombinedOutput()
	return string(raw), err
}

// HardenRepo writes the sandbox's empty hooks path into an existing
// repository's own config.
//
// Env() and Command() only protect commands this package builds. Production
// Git wrappers build their own *exec.Cmd and leave Cmd.Env unset, so when a
// test points one at a sandboxed repository it still reads the host's global
// configuration — the same hook hazard, relocated from the test file into the
// code under test. Repository configuration outranks global configuration, so
// planting the empty hooks path in the repository closes that path for every
// process that touches it, whoever spawned it.
//
// Command-line configuration still outranks the repository, so a test that
// needs its own hooks to fire can pass `-c core.hooksPath=...` for that one
// invocation.
func (s *Sandbox) HardenRepo(t testing.TB, dir string) {
	t.Helper()
	s.Run(t, dir, "config", "core.hooksPath", s.HooksDir)
}

// InitRepo initializes a sandboxed repository in dir, creating dir if needed,
// and returns dir. The repository has one commit so it has a resolvable HEAD.
func (s *Sandbox) InitRepo(t testing.TB, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("gittest: create %s: %v", dir, err)
	}
	s.Run(t, dir, "init", "-b", "main")
	s.HardenRepo(t, dir)
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# test repo\n"), 0o600); err != nil {
		t.Fatalf("gittest: seed %s: %v", readme, err)
	}
	s.Run(t, dir, "add", "README.md")
	s.Run(t, dir, "commit", "-m", "initial commit")
	return dir
}

// NewRepo initializes a sandboxed repository in a fresh temporary directory.
func (s *Sandbox) NewRepo(t testing.TB) string {
	t.Helper()
	return s.InitRepo(t, filepath.Join(t.TempDir(), "repo"))
}

var (
	defaultMu        sync.Mutex
	defaultSandboxes = map[testing.TB]*Sandbox{}
)

// Default returns the Sandbox shared by every package-level helper call made
// from t, creating it on first use. Sub-tests get their own sandbox because
// each *testing.T is a distinct key.
func Default(t testing.TB) *Sandbox {
	t.Helper()
	defaultMu.Lock()
	if s, ok := defaultSandboxes[t]; ok {
		defaultMu.Unlock()
		return s
	}
	defaultMu.Unlock()

	s := New(t)
	defaultMu.Lock()
	// Another goroutine may have raced us here; keep the first winner so a
	// test never observes two sandboxes.
	if existing, ok := defaultSandboxes[t]; ok {
		defaultMu.Unlock()
		return existing
	}
	defaultSandboxes[t] = s
	defaultMu.Unlock()

	t.Cleanup(func() {
		defaultMu.Lock()
		delete(defaultSandboxes, t)
		defaultMu.Unlock()
	})
	return s
}

// Env returns the hermetic environment of t's default sandbox.
func Env(t testing.TB) []string { t.Helper(); return Default(t).Env() }

// Command returns a hermetic Git command from t's default sandbox.
func Command(t testing.TB, dir string, args ...string) *exec.Cmd {
	t.Helper()
	return Default(t).Command(dir, args...)
}

// Run executes a hermetic Git command from t's default sandbox, failing t on
// a non-zero exit.
func Run(t testing.TB, dir string, args ...string) string {
	t.Helper()
	return Default(t).Run(t, dir, args...)
}

// Output executes a hermetic Git command from t's default sandbox and returns
// its combined output and error without failing t.
func Output(t testing.TB, dir string, args ...string) (string, error) {
	t.Helper()
	return Default(t).Output(dir, args...)
}

// InitRepo initializes a sandboxed repository in dir using t's default
// sandbox and returns dir.
func InitRepo(t testing.TB, dir string) string { t.Helper(); return Default(t).InitRepo(t, dir) }

// HardenRepo writes t's default sandbox's empty hooks path into an existing
// repository's own config. Use it for repositories a test did not create
// through InitRepo — a clone, or one the code under test produced.
func HardenRepo(t testing.TB, dir string) { t.Helper(); Default(t).HardenRepo(t, dir) }

// NewRepo initializes a sandboxed repository in a fresh temporary directory
// using t's default sandbox.
func NewRepo(t testing.TB) string { t.Helper(); return Default(t).NewRepo(t) }

// CommandContext returns a hermetic Git command for test helpers that cannot
// receive a testing.TB (for example, BDD step implementations).  The command
// cannot inherit the developer's Git configuration or hooks.  Callers retain
// ownership of the context and of any command-specific process handling.
func CommandContext(ctx context.Context, dir string, args ...string) *exec.Cmd {
	full := make([]string, 0, len(args)+2)
	full = append(full, "-c", "core.hooksPath="+os.DevNull)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	env := make([]string, 0, len(os.Environ())+3)
	for _, kv := range os.Environ() {
		if hasAnyPrefix(kv, dropEnvPrefixes) {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)
	cmd.Env = env
	return cmd
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
