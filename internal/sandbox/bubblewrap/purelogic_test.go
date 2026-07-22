package bubblewrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file extends provider_test.go with coverage of the remaining
// cross-platform, pure-logic surface of the Bubblewrap provider: git-repo
// discovery, directory lifecycle, and secret materialisation. None of these
// paths invoke bwrap, so they run on every platform.

// gitInit turns dir into a real git repository. isGitRepo and the discovery
// helpers shell out to `git rev-parse`, so a bare .git directory is not
// enough -- the repo must be genuine.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-q", dir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init failed: %s", string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

// resolveSym returns path with symlinks evaluated. The discovery helpers
// resolve symlinks in the repo paths they return; on macOS t.TempDir() lives
// under /tmp, itself a symlink to /private/tmp, so expected paths must be
// resolved the same way before comparison.
func resolveSym(t *testing.T, path string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return r
}

// --- isGitRepo --------------------------------------------------------------

func TestProvider_isGitRepo(t *testing.T) {
	p := NewProvider()

	repo := t.TempDir()
	gitInit(t, repo)
	assert.True(t, p.isGitRepo(repo), "an initialized repo must be detected")

	plain := t.TempDir()
	assert.False(t, p.isGitRepo(plain), "a plain dir must not be a git repo")

	assert.False(t, p.isGitRepo(filepath.Join(plain, "does-not-exist")),
		"a missing path must not be a git repo")
}

// --- findGitRootFromPath ----------------------------------------------------

func TestProvider_findGitRootFromPath(t *testing.T) {
	p := NewProvider()

	repo := t.TempDir()
	gitInit(t, repo)

	// A file at the repo root resolves to the repo (its parent dir is the
	// repo). This is the case that matters in practice: resolveRepoFromSymlinks
	// feeds it root-level project markers (go.mod, AGENTS.md).
	rootFile := filepath.Join(repo, "go.mod")
	writeFile(t, rootFile, "module r\n")
	assert.Equal(t, repo, p.findGitRootFromPath(rootFile))

	// The repo directory itself resolves to itself.
	assert.Equal(t, repo, p.findGitRootFromPath(repo))

	// isGitRepo shells out to `git rev-parse`, which succeeds from any
	// subdirectory of a worktree -- so a path inside the repo resolves to the
	// first directory walked, i.e. the containing directory, not the worktree
	// root. This documents the real (slightly surprising) behaviour.
	nested := filepath.Join(repo, "a", "b", "c.go")
	writeFile(t, nested, "package c")
	assert.Equal(t, filepath.Join(repo, "a", "b"), p.findGitRootFromPath(nested))

	// A path with no git ancestor returns empty.
	noRepo := t.TempDir()
	assert.Equal(t, "", p.findGitRootFromPath(filepath.Join(noRepo, "x", "y")))
}

// --- resolveRepoFromSymlinks ------------------------------------------------

func TestProvider_resolveRepoFromSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	p := NewProvider()

	t.Run("resolves via highest-priority project marker", func(t *testing.T) {
		// repoA is referenced only by a Makefile symlink; repoB by go.mod.
		// go.mod outranks Makefile, so repoB must win even though repoA's
		// link is encountered first alphabetically.
		repoA := t.TempDir()
		gitInit(t, repoA)
		writeFile(t, filepath.Join(repoA, "Makefile"), "all:\n")

		repoB := t.TempDir()
		gitInit(t, repoB)
		writeFile(t, filepath.Join(repoB, "go.mod"), "module b\n")

		merged := t.TempDir()
		require.NoError(t, os.Symlink(filepath.Join(repoA, "Makefile"), filepath.Join(merged, "Makefile")))
		require.NoError(t, os.Symlink(filepath.Join(repoB, "go.mod"), filepath.Join(merged, "go.mod")))

		entries, err := os.ReadDir(merged)
		require.NoError(t, err)
		assert.Equal(t, resolveSym(t, repoB), p.resolveRepoFromSymlinks(merged, entries))
	})

	t.Run("falls back to first symlinked repo when no markers", func(t *testing.T) {
		repo := t.TempDir()
		gitInit(t, repo)
		writeFile(t, filepath.Join(repo, "data.txt"), "x")

		merged := t.TempDir()
		require.NoError(t, os.Symlink(filepath.Join(repo, "data.txt"), filepath.Join(merged, "data.txt")))

		entries, err := os.ReadDir(merged)
		require.NoError(t, err)
		assert.Equal(t, resolveSym(t, repo), p.resolveRepoFromSymlinks(merged, entries))
	})

	t.Run("ignores non-symlink entries and dead links", func(t *testing.T) {
		merged := t.TempDir()
		// A plain file (not a symlink) must be ignored.
		writeFile(t, filepath.Join(merged, "go.mod"), "module real\n")
		// A symlink to a path with no git ancestor contributes nothing.
		orphan := t.TempDir()
		writeFile(t, filepath.Join(orphan, "loose"), "x")
		require.NoError(t, os.Symlink(filepath.Join(orphan, "loose"), filepath.Join(merged, "loose")))

		entries, err := os.ReadDir(merged)
		require.NoError(t, err)
		assert.Equal(t, "", p.resolveRepoFromSymlinks(merged, entries))
	})
}

// --- findGitRepo ------------------------------------------------------------

func TestProvider_findGitRepo(t *testing.T) {
	p := NewProvider()

	t.Run("direct git repo in lower dirs", func(t *testing.T) {
		repo := t.TempDir()
		gitInit(t, repo)
		assert.Equal(t, resolveSym(t, repo), p.findGitRepo([]string{repo}))
	})

	t.Run("git repo in a subdirectory", func(t *testing.T) {
		parent := t.TempDir()
		sub := filepath.Join(parent, "myrepo")
		require.NoError(t, os.MkdirAll(sub, 0755))
		gitInit(t, sub)
		assert.Equal(t, resolveSym(t, sub), p.findGitRepo([]string{parent}))
	})

	t.Run("no repo anywhere returns empty", func(t *testing.T) {
		// An empty, repo-free dir with HOME redirected so the AGM-config and
		// well-known-location fallbacks also find nothing.
		t.Setenv("HOME", t.TempDir())
		assert.Equal(t, "", p.findGitRepo([]string{t.TempDir()}))
	})
}

// --- createDirectories / cleanupDirectories / cleanup -----------------------

func TestProvider_createAndCleanupDirectories(t *testing.T) {
	p := NewProvider()
	base := t.TempDir()
	upper := filepath.Join(base, "upper")
	work := filepath.Join(base, "work")
	merged := filepath.Join(base, "merged")

	require.NoError(t, p.createDirectories(upper, work, merged))
	for _, d := range []string{upper, work, merged} {
		assert.DirExists(t, d)
	}

	// Re-creating existing directories is a no-op (MkdirAll is idempotent).
	require.NoError(t, p.createDirectories(upper, work, merged))

	require.NoError(t, p.cleanupDirectories(upper, work, merged))
	for _, d := range []string{upper, work, merged} {
		assert.NoDirExists(t, d)
	}

	// Cleaning already-absent directories must not error.
	require.NoError(t, p.cleanupDirectories(upper, work, merged))
}

func TestProvider_cleanup(t *testing.T) {
	p := NewProvider()
	base := t.TempDir()
	upper := filepath.Join(base, "upper")
	work := filepath.Join(base, "work")
	merged := filepath.Join(base, "merged")
	require.NoError(t, p.createDirectories(upper, work, merged))

	require.NoError(t, p.cleanup(upper, work, merged))
	assert.NoDirExists(t, merged)
	assert.NoDirExists(t, work)
	assert.NoDirExists(t, upper)
}

// --- writeSecrets -----------------------------------------------------------

func TestProvider_writeSecrets(t *testing.T) {
	p := NewProvider()

	t.Run("writes .env with header and expands env vars", func(t *testing.T) {
		t.Setenv("BWRAP_TEST_TOKEN", "expanded-value")
		upper := t.TempDir()

		err := p.writeSecrets(upper, map[string]string{
			"API_KEY": "literal-123",
			"TOKEN":   "$BWRAP_TEST_TOKEN",
		})
		require.NoError(t, err)

		envPath := filepath.Join(upper, ".env")
		content, err := os.ReadFile(envPath)
		require.NoError(t, err)
		s := string(content)

		assert.Contains(t, s, "DO NOT COMMIT")
		assert.Contains(t, s, "API_KEY=literal-123")
		assert.Contains(t, s, "TOKEN=expanded-value")

		// Secrets file must be owner-only (0600).
		if runtime.GOOS != "windows" {
			info, statErr := os.Stat(envPath)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
		}
	})

	t.Run("empty secrets still writes a header-only file", func(t *testing.T) {
		upper := t.TempDir()
		require.NoError(t, p.writeSecrets(upper, map[string]string{}))
		content, err := os.ReadFile(filepath.Join(upper, ".env"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "Auto-generated")
	})

	t.Run("missing upper dir is an error", func(t *testing.T) {
		err := p.writeSecrets(filepath.Join(t.TempDir(), "absent"), map[string]string{"K": "v"})
		require.Error(t, err)
	})
}
