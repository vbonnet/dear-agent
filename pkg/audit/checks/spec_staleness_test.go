package checks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/pkg/audit"
)

// newGitRepo initialises a bare-minimum git repo in t.TempDir() and returns
// its root. The gittest sandbox supplies a throwaway user so every host can
// commit without a pre-existing config.
func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		out, err := gittest.Output(t, dir, args...)
		require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	}
	run("init", "-b", "main")
	return dir
}

// writeAndCommit writes content to path (relative to repo) and commits it.
func writeAndCommit(t *testing.T, repo, relPath, content, msg string) {
	t.Helper()
	abs := filepath.Join(repo, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))

	run := func(args ...string) {
		t.Helper()
		out, err := gittest.Output(t, repo, args...)
		require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	}
	run("add", relPath)
	run("commit", "-m", msg)
}

func TestSpecStalenessCheck_NoGit(t *testing.T) {
	dir := t.TempDir()
	// A directory without a .git — check must not error.
	env := audit.Env{RepoRoot: dir, WorkingDir: dir}
	res, err := SpecStalenessCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, audit.StatusOK, res.Status)
	assert.Empty(t, res.Findings)
}

func TestSpecStalenessCheck_NoSpec(t *testing.T) {
	repo := newGitRepo(t)
	writeAndCommit(t, repo, "pkg/foo/foo.go", "package foo", "add foo")
	env := audit.Env{RepoRoot: repo, WorkingDir: repo}
	res, err := SpecStalenessCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, res.Findings) // no SPEC.md → spec.coverage concern, not staleness
}

func TestSpecStalenessCheck_FreshSpec(t *testing.T) {
	repo := newGitRepo(t)
	writeAndCommit(t, repo, "pkg/foo/foo.go", "package foo", "add foo.go")
	writeAndCommit(t, repo, "pkg/foo/SPEC.md", "# Foo\n", "add SPEC.md")
	// One commit AFTER the SPEC (below threshold of 1).
	writeAndCommit(t, repo, "pkg/foo/bar.go", "package foo", "add bar.go")

	env := audit.Env{
		RepoRoot:   repo,
		WorkingDir: repo,
		Config:     map[string]any{"threshold": 2},
	}
	res, err := SpecStalenessCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, res.Findings, "1 commit behind threshold=2 should not fire")
}

func TestSpecStalenessCheck_StaleSpec(t *testing.T) {
	repo := newGitRepo(t)
	writeAndCommit(t, repo, "pkg/foo/foo.go", "package foo", "add foo.go")
	writeAndCommit(t, repo, "pkg/foo/SPEC.md", "# Foo\n", "add SPEC.md")
	// Three commits after the SPEC — above a threshold of 2.
	writeAndCommit(t, repo, "pkg/foo/a.go", "package foo", "add a.go")
	writeAndCommit(t, repo, "pkg/foo/b.go", "package foo", "add b.go")
	writeAndCommit(t, repo, "pkg/foo/c.go", "package foo", "add c.go")

	env := audit.Env{
		RepoRoot:   repo,
		WorkingDir: repo,
		Config:     map[string]any{"threshold": 2},
	}
	res, err := SpecStalenessCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, res.Findings, 1)
	f := res.Findings[0]
	assert.Equal(t, "pkg/foo/SPEC.md", f.Path)
	assert.Equal(t, audit.SeverityP2, f.Severity)
	assert.Equal(t, 3, f.Evidence["commits_behind"])
}

func TestSpecStalenessCheck_SpecUpdatedAfterCommits(t *testing.T) {
	repo := newGitRepo(t)
	writeAndCommit(t, repo, "pkg/foo/SPEC.md", "# Foo\n", "add SPEC.md v1")
	writeAndCommit(t, repo, "pkg/foo/a.go", "package foo", "add a.go")
	writeAndCommit(t, repo, "pkg/foo/b.go", "package foo", "add b.go")
	// SPEC.md is updated — should reset staleness.
	writeAndCommit(t, repo, "pkg/foo/SPEC.md", "# Foo v2\n", "update SPEC.md")

	env := audit.Env{
		RepoRoot:   repo,
		WorkingDir: repo,
		Config:     map[string]any{"threshold": 1},
	}
	res, err := SpecStalenessCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, res.Findings, "SPEC updated after commits should not fire")
}

func TestCommitsBehind_NoSpec(t *testing.T) {
	repo := newGitRepo(t)
	writeAndCommit(t, repo, "pkg/foo/foo.go", "package foo", "add foo.go")
	n, err := commitsBehind(context.Background(), repo, "pkg/foo", "pkg/foo/SPEC.md")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestIntConfig(t *testing.T) {
	assert.Equal(t, 10, intConfig(nil, "threshold", 10))
	assert.Equal(t, 5, intConfig(map[string]any{"threshold": 5}, "threshold", 10))
	assert.Equal(t, 10, intConfig(map[string]any{"threshold": -1}, "threshold", 10))
	assert.Equal(t, 7, intConfig(map[string]any{"threshold": float64(7)}, "threshold", 10))
}
