package skilllint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestCheckFile_Compliant(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "commands/good.md", `---
model: haiku
effort: low
description: test skill
---

# Body
`)
	vs, err := CheckFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 0 {
		t.Errorf("expected no violations, got %v", vs)
	}
}

func TestCheckFile_MissingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "commands/bare.md", `# No frontmatter here
`)
	vs, err := CheckFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Reason, "no YAML frontmatter") {
		t.Errorf("expected 'no YAML frontmatter' reason, got %q", vs[0].Reason)
	}
}

func TestCheckFile_MissingModel(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "commands/partial.md", `---
effort: low
description: no model
---
`)
	vs, err := CheckFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation, got %v", vs)
	}
	if !strings.Contains(vs[0].Reason, "missing `model:`") {
		t.Errorf("expected missing model reason, got %q", vs[0].Reason)
	}
}

func TestCheckFile_MissingEffort(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "commands/partial.md", `---
model: sonnet
description: no effort
---
`)
	vs, err := CheckFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation, got %v", vs)
	}
	if !strings.Contains(vs[0].Reason, "missing `effort:`") {
		t.Errorf("expected missing effort reason, got %q", vs[0].Reason)
	}
}

func TestCheckFile_InvalidValues(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "commands/bad.md", `---
model: banana
effort: extreme
---
`)
	vs, err := CheckFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 2 {
		t.Fatalf("expected 2 violations, got %v", vs)
	}
}

func TestCheckDir_ScansCommandsAndSkillMD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "commands/good.md", "---\nmodel: haiku\neffort: low\n---\n")
	writeFile(t, dir, "commands/bad.md", "---\ndescription: missing tiers\n---\n")
	writeFile(t, dir, "skills/foo/SKILL.md", "---\nmodel: sonnet\neffort: medium\n---\n")
	writeFile(t, dir, "skills/bar/SKILL.md", "---\ndescription: no pin\n---\n")
	// Not a skill file — should be ignored.
	writeFile(t, dir, "commands/helper_test.sh", "echo hi")
	writeFile(t, dir, "commands/notes.txt", "plain text")

	vs, err := CheckDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vs) != 4 {
		// bad.md contributes 2 (missing model + missing effort); SKILL.md for bar contributes 2.
		t.Fatalf("expected 4 violations, got %d: %v", len(vs), vs)
	}
}

// TestRepoSkillsAreCompliant walks the actual AGM plugin commands directory
// and confirms every skill declares both model and effort. This is the live
// CI gate — if someone adds a skill without frontmatter, this test fails
// locally and in CI.
//
// Scope note: only agm-plugin/commands is gated here. Wayfinder commands
// still have pre-existing unpinned skills and are tracked as a follow-up;
// add them to this list once compliant.
//
// The test auto-locates the repo root by walking up from the test file
// until it finds go.mod, so it works regardless of where `go test` is run.
func TestRepoSkillsAreCompliant(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot find repo root: %v", err)
		return
	}
	// Directories known to contain skill files that this lint governs.
	dirs := []string{
		filepath.Join(root, "agm", "agm-plugin", "commands"),
	}
	var all []Violation
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			continue // missing optional dir → skip silently
		}
		vs, err := CheckDir(d)
		if err != nil {
			t.Fatalf("CheckDir(%s): %v", d, err)
		}
		all = append(all, vs...)
	}
	if len(all) > 0 {
		t.Errorf("%d skill(s) missing or wrong model/effort frontmatter:", len(all))
		for _, v := range all {
			t.Errorf("  %s", v)
		}
	}
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
