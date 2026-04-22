package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCommitMessage_NonDocsPrefix(t *testing.T) {
	warn, block := CheckCommitMessage("feat: add scheduler", "/tmp/empty")
	assert.Empty(t, warn)
	assert.False(t, block)
}

func TestCheckCommitMessage_DocsWithoutAspirationKeyword(t *testing.T) {
	warn, block := CheckCommitMessage("docs: fix typo in README", "/tmp/empty")
	assert.Empty(t, warn)
	assert.False(t, block)
}

func TestCheckCommitMessage_DocsAddUnbuiltFeature(t *testing.T) {
	// Create a temp repo root with no matching code.
	root := t.TempDir()

	warn, block := CheckCommitMessage("docs: add scheduler specification", root)
	assert.Contains(t, warn, "unbuilt feature")
	assert.Contains(t, warn, "scheduler")
	assert.False(t, block, "should warn, not block")
}

func TestCheckCommitMessage_DocsDesignUnbuiltFeature(t *testing.T) {
	root := t.TempDir()

	warn, block := CheckCommitMessage("docs: design pipeline architecture", root)
	assert.Contains(t, warn, "unbuilt feature")
	assert.Contains(t, warn, "pipeline")
	assert.False(t, block)
}

func TestCheckCommitMessage_DocsPlanUnbuiltFeature(t *testing.T) {
	root := t.TempDir()

	warn, block := CheckCommitMessage("docs: plan migration strategy", root)
	assert.Contains(t, warn, "unbuilt feature")
	assert.Contains(t, warn, "migration")
	assert.False(t, block)
}

func TestCheckCommitMessage_DocsAddBuiltFeature(t *testing.T) {
	root := t.TempDir()

	// Create a Go file that matches the feature name.
	dir := filepath.Join(root, "scheduler")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "scheduler.go"),
		[]byte("package scheduler\n"),
		0o644,
	))

	warn, block := CheckCommitMessage("docs: add scheduler specification", root)
	assert.Empty(t, warn, "should not warn when code exists")
	assert.False(t, block)
}

func TestCheckCommitMessage_CaseInsensitive(t *testing.T) {
	root := t.TempDir()

	warn, block := CheckCommitMessage("Docs: Add Scheduler spec", root)
	assert.Contains(t, warn, "unbuilt feature")
	assert.False(t, block)
}

func TestCheckCommitMessage_NeverBlocks(t *testing.T) {
	root := t.TempDir()

	cases := []string{
		"docs: add new feature",
		"docs: design system",
		"docs: plan rollout",
		"feat: something else",
		"docs: fix typo",
	}
	for _, msg := range cases {
		_, block := CheckCommitMessage(msg, root)
		assert.False(t, block, "should never block for %q", msg)
	}
}

func TestIsAspirationDocCommit(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"docs: add feature", true},
		{"docs: design system", true},
		{"docs: plan migration", true},
		{"docs: fix typo", false},
		{"feat: add feature", false},
		{"docs: update existing", false},
		{"DOCS: ADD feature", true},
		{"  docs: plan something  ", true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isAspirationDocCommit(tt.msg), "msg=%q", tt.msg)
	}
}

func TestExtractFeatureName(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"docs: add scheduler spec", "scheduler"},
		{"docs: design pipeline architecture", "pipeline"},
		{"docs: plan for migration", "migration"},
		{"docs: add the new widget", "widget"},
		{"docs: add", ""},
		{"feat: add scheduler", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, extractFeatureName(tt.msg), "msg=%q", tt.msg)
	}
}

func TestCodeExists_SkipsTestFiles(t *testing.T) {
	root := t.TempDir()

	dir := filepath.Join(root, "scheduler")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	// Only a test file — should not count as "code exists".
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "scheduler_test.go"),
		[]byte("package scheduler\n"),
		0o644,
	))

	assert.False(t, codeExists("scheduler", root))
}

func TestCodeExists_MatchesSubpath(t *testing.T) {
	root := t.TempDir()

	dir := filepath.Join(root, "internal", "scheduler")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "run.go"),
		[]byte("package scheduler\n"),
		0o644,
	))

	assert.True(t, codeExists("scheduler", root))
}

func TestCodeExists_EmptyRoot(t *testing.T) {
	assert.False(t, codeExists("anything", ""))
}
