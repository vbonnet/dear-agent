package ecphory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/engram"
	"gopkg.in/yaml.v3"
)

// newTestEcphory creates a minimal Ecphory for testing writeFrontmatter.
func newTestEcphory(basePath string) *Ecphory {
	return &Ecphory{
		parser:   engram.NewParser(),
		basePath: basePath,
	}
}

func writeEngram(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write engram: %v", err)
	}
	return path
}

func TestWriteFrontmatter_PersistsRetrievalCount(t *testing.T) {
	dir := t.TempDir()
	path := writeEngram(t, dir, "test.ai.md", `---
type: pattern
title: Test
tags: ["go"]
retrieval_count: 0
---
# Content
Some body text.
`)

	e := newTestEcphory(dir)
	fm := &engram.Frontmatter{
		Type:           "pattern",
		Title:          "Test",
		Tags:           []string{"go"},
		RetrievalCount: 5,
		LastAccessed:   time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
	}

	if err := e.writeFrontmatter(path, fm, "# Content\nSome body text.\n"); err != nil {
		t.Fatalf("writeFrontmatter failed: %v", err)
	}

	// Re-parse and verify
	eg, err := e.parser.Parse(path)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if eg.Frontmatter.RetrievalCount != 5 {
		t.Errorf("retrieval_count = %d, want 5", eg.Frontmatter.RetrievalCount)
	}
}

func TestWriteFrontmatter_PersistsLastAccessed(t *testing.T) {
	dir := t.TempDir()
	path := writeEngram(t, dir, "test.ai.md", `---
type: pattern
title: Accessed
tags: ["go"]
---
# Body
`)

	e := newTestEcphory(dir)
	accessed := time.Date(2026, 3, 30, 15, 30, 0, 0, time.UTC)
	fm := &engram.Frontmatter{
		Type:         "pattern",
		Title:        "Accessed",
		Tags:         []string{"go"},
		LastAccessed: accessed,
	}

	if err := e.writeFrontmatter(path, fm, "# Body\n"); err != nil {
		t.Fatalf("writeFrontmatter failed: %v", err)
	}

	// Read raw file and verify last_accessed is present in YAML
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(data), "last_accessed:") {
		t.Error("file should contain last_accessed field")
	}

	// Re-parse and verify timestamp
	eg, err := e.parser.Parse(path)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if eg.Frontmatter.LastAccessed.IsZero() {
		t.Error("last_accessed should not be zero after write")
	}
}

func TestWriteFrontmatter_PreservesContent(t *testing.T) {
	dir := t.TempDir()
	bodyContent := "# Important Notes\n\nDo not lose this content.\n\n- Item 1\n- Item 2\n"
	path := writeEngram(t, dir, "test.ai.md", "---\ntype: pattern\ntitle: Preserve\ntags: [\"go\"]\n---\n"+bodyContent)

	e := newTestEcphory(dir)
	fm := &engram.Frontmatter{
		Type:           "pattern",
		Title:          "Preserve",
		Tags:           []string{"go"},
		RetrievalCount: 1,
	}

	if err := e.writeFrontmatter(path, fm, bodyContent); err != nil {
		t.Fatalf("writeFrontmatter failed: %v", err)
	}

	// Re-parse and check content is intact
	eg, err := e.parser.Parse(path)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if eg.Content != bodyContent {
		t.Errorf("content mismatch:\ngot:  %q\nwant: %q", eg.Content, bodyContent)
	}
}

func TestWriteFrontmatter_AtomicNoCorruption(t *testing.T) {
	dir := t.TempDir()
	path := writeEngram(t, dir, "test.ai.md", "---\ntype: pattern\ntitle: Atomic\ntags: [\"go\"]\n---\n# Body\n")

	e := newTestEcphory(dir)
	fm := &engram.Frontmatter{
		Type:           "pattern",
		Title:          "Atomic",
		Tags:           []string{"go"},
		RetrievalCount: 42,
	}

	if err := e.writeFrontmatter(path, fm, "# Body\n"); err != nil {
		t.Fatalf("writeFrontmatter failed: %v", err)
	}

	// Verify no temp file left behind
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("temp file should not exist after successful write")
	}

	// Verify file is valid YAML frontmatter
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.HasPrefix(string(data), "---\n") {
		t.Error("file should start with ---")
	}

	// Verify roundtrip: file is parseable and has correct values
	eg, err := e.parser.Parse(path)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if eg.Frontmatter.RetrievalCount != 42 {
		t.Errorf("retrieval_count = %d, want 42", eg.Frontmatter.RetrievalCount)
	}
}

func TestWriteFrontmatter_PreservesFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := writeEngram(t, dir, "test.ai.md", "---\ntype: pattern\ntitle: Perms\ntags: []\n---\n# Body\n")

	// Set specific permissions
	os.Chmod(path, 0600)

	e := newTestEcphory(dir)
	fm := &engram.Frontmatter{
		Type:  "pattern",
		Title: "Perms",
	}

	if err := e.writeFrontmatter(path, fm, "# Body\n"); err != nil {
		t.Fatalf("writeFrontmatter failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestWriteFrontmatter_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeEngram(t, dir, "test.ai.md", "---\ntype: pattern\ntitle: Valid\ntags: [\"go\"]\n---\n# Body\n")

	e := newTestEcphory(dir)
	fm := &engram.Frontmatter{
		Type:           "pattern",
		Title:          "Valid",
		Tags:           []string{"go", "testing"},
		RetrievalCount: 3,
		LastAccessed:   time.Now(),
	}

	if err := e.writeFrontmatter(path, fm, "# Body\n"); err != nil {
		t.Fatalf("writeFrontmatter failed: %v", err)
	}

	// Read raw data and verify YAML is valid
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Extract frontmatter section
	content := string(data)
	parts := strings.SplitN(content[4:], "\n---\n", 2) // skip opening ---\n
	if len(parts) != 2 {
		t.Fatal("could not split frontmatter from content")
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(parts[0]), &parsed); err != nil {
		t.Fatalf("frontmatter is not valid YAML: %v", err)
	}
	if parsed["title"] != "Valid" {
		t.Errorf("title = %v, want Valid", parsed["title"])
	}
}
