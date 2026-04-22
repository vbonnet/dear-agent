package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateOnboardingContent_PathShortening(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		t.Skip("no home directory")
	}

	// Use paths under home dir to test ~ shortening
	repos := []string{
		filepath.Join(homeDir, "src", "repo1"),
		"/opt/external/repo2", // not under home
	}

	content, err := GenerateOnboardingContent("test-session", "/tmp/merged", repos)
	require.NoError(t, err)

	assert.Contains(t, content, "~/src/repo1")
	assert.Contains(t, content, "/opt/external/repo2")
}

func TestGenerateOnboardingContentFromFile_InvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "bad.md")
	// Invalid Go template syntax
	require.NoError(t, os.WriteFile(tmplPath, []byte("{{.Invalid}}{{end}}"), 0644))

	_, err := GenerateOnboardingContentFromFile(tmplPath, "session", "/tmp/merged", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestGenerateOnboardingContentFromFile_NonexistentFile(t *testing.T) {
	_, err := GenerateOnboardingContentFromFile("/nonexistent/template.md", "session", "/tmp/merged", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read template file")
}

func TestGenerateOnboardingContentFromFile_PathShortening(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		t.Skip("no home directory")
	}

	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "custom.md")
	tmplContent := "Repos: {{range .Repos}}{{.}} {{end}}"
	require.NoError(t, os.WriteFile(tmplPath, []byte(tmplContent), 0644))

	repos := []string{
		filepath.Join(homeDir, "src", "myrepo"),
		"/absolute/path/repo",
	}

	content, err := GenerateOnboardingContentFromFile(tmplPath, "session", "/tmp/merged", repos)
	require.NoError(t, err)

	assert.Contains(t, content, "~/src/myrepo")
	assert.Contains(t, content, "/absolute/path/repo")
}

func TestWriteOnboardingClaudeMd_Permissions(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	dir := t.TempDir()
	content := "# Test"
	require.NoError(t, WriteOnboardingClaudeMd(dir, content))

	projectDir, err := ClaudeProjectDir(dir)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(projectDir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestWriteOnboardingClaudeMd_InvalidDir(t *testing.T) {
	// With the new approach, the mergedPath doesn't need to be writable
	// since we write to ~/.claude/projects/ instead. The function only
	// fails if ~/.claude/projects/ can't be created.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	// Should succeed even with a nonexistent mergedPath (it's just used for encoding)
	err := WriteOnboardingClaudeMd("/nonexistent/directory", "content")
	assert.NoError(t, err)

	// Verify it wrote to the project dir
	projectDir, err := ClaudeProjectDir("/nonexistent/directory")
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(projectDir, "CLAUDE.md"))
	assert.NoError(t, err)
}

func TestGenerateOnboardingContent_ContainsExpectedSections(t *testing.T) {
	content, err := GenerateOnboardingContent(
		"my-session-id",
		"/tmp/merged",
		[]string{"/src/repo-a", "/src/repo-b"},
	)
	require.NoError(t, err)

	// Check all key sections of the template
	assert.Contains(t, content, "Read-Only Filesystem")
	assert.Contains(t, content, "Git Worktrees")
	assert.Contains(t, content, "Quick Reference")
	assert.Contains(t, content, "Rules")
	assert.Contains(t, content, "my-session-id")

	// Check that repos appear in the table
	assert.Contains(t, content, "/src/repo-a")
	assert.Contains(t, content, "/src/repo-b")
}

func TestGenerateOnboardingContent_SingleRepo(t *testing.T) {
	content, err := GenerateOnboardingContent("session", "/merged", []string{"/single/repo"})
	require.NoError(t, err)

	// Count occurrences of "Repo (READ-ONLY)"
	count := strings.Count(content, "Repo (READ-ONLY)")
	assert.Equal(t, 1, count)
}
