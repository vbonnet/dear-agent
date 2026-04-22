package trigger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScopeResolveProjectIDFromWayfinder(t *testing.T) {
	dir := t.TempDir()

	content := `---
project_name: my-cool-project
session_id: abc123
---
# Status
Everything is fine.
`
	err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	sr := NewScopeResolver()
	got := sr.ResolveProjectID(dir)
	if got != "my-cool-project" {
		t.Errorf("ResolveProjectID = %q, want %q", got, "my-cool-project")
	}
}

func TestScopeResolveProjectIDFromWayfinderQuoted(t *testing.T) {
	dir := t.TempDir()

	content := `---
project_name: "quoted-project"
---
`
	err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	sr := NewScopeResolver()
	got := sr.ResolveProjectID(dir)
	if got != "quoted-project" {
		t.Errorf("ResolveProjectID = %q, want %q", got, "quoted-project")
	}
}

func TestScopeResolveProjectIDFromGit(t *testing.T) {
	// Create a directory structure: repoRoot/.git/ and repoRoot/subdir/
	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	subdir := filepath.Join(repoRoot, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	sr := NewScopeResolver()

	// From subdir, should find git root and use its basename
	got := sr.ResolveProjectID(subdir)
	expected := filepath.Base(repoRoot)
	if got != expected {
		t.Errorf("ResolveProjectID = %q, want %q", got, expected)
	}

	// From repo root directly
	got = sr.ResolveProjectID(repoRoot)
	if got != expected {
		t.Errorf("ResolveProjectID = %q, want %q", got, expected)
	}
}

func TestScopeResolveProjectIDFallback(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "my-project")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	sr := NewScopeResolver()
	got := sr.ResolveProjectID(subdir)
	if got != "my-project" {
		t.Errorf("ResolveProjectID = %q, want %q", got, "my-project")
	}
}

func TestScopeResolveProjectIDWayfinderPriority(t *testing.T) {
	// Wayfinder should take priority over .git
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	content := `---
project_name: wayfinder-wins
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	sr := NewScopeResolver()
	got := sr.ResolveProjectID(dir)
	if got != "wayfinder-wins" {
		t.Errorf("ResolveProjectID = %q, want %q (wayfinder should take priority over git)", got, "wayfinder-wins")
	}
}

func TestScopeFindProjectEngramDir(t *testing.T) {
	sr := NewScopeResolver()

	t.Run("exists", func(t *testing.T) {
		dir := t.TempDir()
		engramDir := filepath.Join(dir, ".engram", "engrams")
		if err := os.MkdirAll(engramDir, 0755); err != nil {
			t.Fatal(err)
		}

		got := sr.FindProjectEngramDir(dir)
		if got != engramDir {
			t.Errorf("FindProjectEngramDir = %q, want %q", got, engramDir)
		}
	})

	t.Run("not_exists", func(t *testing.T) {
		dir := t.TempDir()
		got := sr.FindProjectEngramDir(dir)
		if got != "" {
			t.Errorf("FindProjectEngramDir = %q, want empty string", got)
		}
	})

	t.Run("partial_path", func(t *testing.T) {
		dir := t.TempDir()
		// Only .engram exists but not engrams subdir
		if err := os.Mkdir(filepath.Join(dir, ".engram"), 0755); err != nil {
			t.Fatal(err)
		}

		got := sr.FindProjectEngramDir(dir)
		if got != "" {
			t.Errorf("FindProjectEngramDir = %q, want empty string", got)
		}
	})
}

func TestScopeIsInScopeGlobal(t *testing.T) {
	sr := NewScopeResolver()

	if !sr.IsInScope("global", "projA", "projB", "sess1", "sess2") {
		t.Error("global scope should always return true")
	}

	if !sr.IsInScope("Global", "projA", "projB", "sess1", "sess2") {
		t.Error("global scope (capitalized) should always return true")
	}
}

func TestScopeIsInScopeProject(t *testing.T) {
	sr := NewScopeResolver()

	// Matching project IDs
	if !sr.IsInScope("project", "myproj", "myproj", "", "") {
		t.Error("project scope with matching IDs should return true")
	}

	// Non-matching project IDs
	if sr.IsInScope("project", "projA", "projB", "", "") {
		t.Error("project scope with non-matching IDs should return false")
	}

	// Empty trigger project ID means match any
	if !sr.IsInScope("project", "", "projB", "", "") {
		t.Error("project scope with empty triggerProjectID should return true")
	}
}

func TestScopeIsInScopeSession(t *testing.T) {
	sr := NewScopeResolver()

	// Matching session IDs
	if !sr.IsInScope("session", "", "", "sess1", "sess1") {
		t.Error("session scope with matching IDs should return true")
	}

	// Non-matching session IDs
	if sr.IsInScope("session", "", "", "sess1", "sess2") {
		t.Error("session scope with non-matching IDs should return false")
	}
}

func TestScopeIsInScopeEmpty(t *testing.T) {
	sr := NewScopeResolver()

	// Empty scope treated as global
	if !sr.IsInScope("", "projA", "projB", "sess1", "sess2") {
		t.Error("empty scope should be treated as global and return true")
	}
}

func TestScopeIsInScopeUnknown(t *testing.T) {
	sr := NewScopeResolver()

	if sr.IsInScope("unknown", "a", "a", "b", "b") {
		t.Error("unknown scope should return false")
	}
}
