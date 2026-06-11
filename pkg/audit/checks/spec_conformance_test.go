package checks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// writeSpec writes a SPEC.md with the given content into dir under root/pkg,
// creating the directory structure as needed.
func writeSpec(t *testing.T, dir, relPath, content string) {
	t.Helper()
	abs := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
}

func TestSpecConformanceCheck_NoFindings_EmptyRoots(t *testing.T) {
	dir := t.TempDir()
	env := audit.Env{RepoRoot: dir, WorkingDir: dir}
	res, err := SpecConformanceCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, audit.StatusOK, res.Status)
	assert.Empty(t, res.Findings)
}

func TestSpecConformanceCheck_NoFindings_NoSpec(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg", "foo"), 0o755))
	env := audit.Env{RepoRoot: dir, WorkingDir: dir}
	res, err := SpecConformanceCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, res.Findings)
}

func TestSpecConformanceCheck_AllRefsExist(t *testing.T) {
	dir := t.TempDir()
	// Create the referenced file.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg", "foo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg", "foo", "bar.go"), nil, 0o644))
	writeSpec(t, dir, "pkg/foo/SPEC.md", "See `pkg/foo/bar.go` for the implementation.\n")

	env := audit.Env{RepoRoot: dir, WorkingDir: dir}
	res, err := SpecConformanceCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, res.Findings)
}

func TestSpecConformanceCheck_DeadRef(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg", "foo"), 0o755))
	writeSpec(t, dir, "pkg/foo/SPEC.md",
		"Implementation at `pkg/foo/bar.go` (moved to `pkg/foo/baz.go`).\n")
	// bar.go does not exist; baz.go does.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg", "foo", "baz.go"), nil, 0o644))

	env := audit.Env{RepoRoot: dir, WorkingDir: dir}
	res, err := SpecConformanceCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, res.Findings, 1)
	assert.Equal(t, "pkg/foo/SPEC.md", res.Findings[0].Path)
	assert.Equal(t, "pkg/foo/bar.go", res.Findings[0].Evidence["dead_ref"])
}

func TestSpecConformanceCheck_SkipsURLs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg", "foo"), 0o755))
	writeSpec(t, dir, "pkg/foo/SPEC.md",
		"See `https://github.com/vbonnet/dear-agent/pkg/foo` and `github.com/vbonnet/dear-agent/pkg/foo`.\n")
	env := audit.Env{RepoRoot: dir, WorkingDir: dir}
	res, err := SpecConformanceCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, res.Findings)
}

func TestSpecConformanceCheck_SkipsAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg", "foo"), 0o755))
	writeSpec(t, dir, "pkg/foo/SPEC.md", "Lives at `/usr/local/bin/foo`.\n")
	env := audit.Env{RepoRoot: dir, WorkingDir: dir}
	res, err := SpecConformanceCheck{}.Run(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, res.Findings)
}

func TestShouldSkipPathToken(t *testing.T) {
	assert.True(t, shouldSkipPathToken("https://example.com/foo"))
	assert.True(t, shouldSkipPathToken("http://example.com/foo"))
	assert.True(t, shouldSkipPathToken("github.com/vbonnet/dear-agent/pkg/foo"))
	assert.True(t, shouldSkipPathToken("golang.org/x/tools/go/ssa"))
	assert.True(t, shouldSkipPathToken("/usr/local/bin"))
	assert.True(t, shouldSkipPathToken("~/src/dear-agent"))
	assert.True(t, shouldSkipPathToken("pkg/foo/*.go"))
	assert.False(t, shouldSkipPathToken("pkg/foo/bar.go"))
	assert.False(t, shouldSkipPathToken("cmd/agm/main.go"))
}

func TestDeadPathRefs_Deduplicates(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "SPEC.md")
	require.NoError(t, os.WriteFile(specPath, []byte(
		"`pkg/foo/bar.go` is mentioned twice: `pkg/foo/bar.go`.\n",
	), 0o644))
	dead, err := deadPathRefs(specPath, dir)
	require.NoError(t, err)
	assert.Len(t, dead, 1, "same dead path should be deduped")
}
