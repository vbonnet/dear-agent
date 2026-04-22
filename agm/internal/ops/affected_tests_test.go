package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

func TestSplitNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "foo", []string{"foo"}},
		{"multi", "a\nb\nc", []string{"a", "b", "c"}},
		{"trailing_newline", "a\nb\n", []string{"a", "b"}},
		{"blank_lines", "a\n\nb\n\n", []string{"a", "b"}},
		{"whitespace", "  a  \n  b  ", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitNonEmpty(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("splitNonEmpty(%q) = %v; want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitNonEmpty(%q)[%d] = %q; want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestChangedPackages(t *testing.T) {
	// Use a temp dir and create a fake go.mod.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/test\n\ngo 1.21\n")

	files := []string{
		"cmd/main.go",
		"pkg/util/helper.go",
		"README.md",           // non-Go file, should be skipped
		"internal/foo/bar.go", // Go file
	}

	pkgs := changedPackages(dir, files)

	expected := map[string]bool{
		"example.com/test/cmd":          true,
		"example.com/test/pkg/util":     true,
		"example.com/test/internal/foo": true,
	}

	if len(pkgs) != len(expected) {
		t.Fatalf("changedPackages = %v; want %v", pkgs, expected)
	}
	for pkg := range expected {
		if !pkgs[pkg] {
			t.Errorf("missing package %q", pkg)
		}
	}
}

func TestChangedPackages_RootFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/root\n\ngo 1.21\n")

	pkgs := changedPackages(dir, []string{"main.go"})

	if !pkgs["example.com/root"] {
		t.Errorf("root package not detected, got: %v", pkgs)
	}
}

func TestFindTransitivelyAffected(t *testing.T) {
	// Dependency graph: A -> B -> C, D -> B
	// If B changes, A and D should be affected.
	reverseDeps := map[string]map[string]bool{
		"pkg/B": {"pkg/A": true, "pkg/D": true},
		"pkg/C": {"pkg/B": true},
	}

	changed := map[string]bool{"pkg/B": true}
	affected := findTransitivelyAffected(changed, reverseDeps)

	expected := map[string]bool{
		"pkg/B": true, // itself
		"pkg/A": true, // depends on B
		"pkg/D": true, // depends on B
	}

	if len(affected) != len(expected) {
		t.Fatalf("affected = %v; want %v", affected, expected)
	}
	for pkg := range expected {
		if !affected[pkg] {
			t.Errorf("missing affected package %q", pkg)
		}
	}
}

func TestFindTransitivelyAffected_Transitive(t *testing.T) {
	// Chain: C -> B -> A
	// If C changes, B and A should be affected transitively.
	reverseDeps := map[string]map[string]bool{
		"pkg/C": {"pkg/B": true},
		"pkg/B": {"pkg/A": true},
	}

	changed := map[string]bool{"pkg/C": true}
	affected := findTransitivelyAffected(changed, reverseDeps)

	for _, pkg := range []string{"pkg/C", "pkg/B", "pkg/A"} {
		if !affected[pkg] {
			t.Errorf("expected %q to be affected", pkg)
		}
	}
}

func TestFindTransitivelyAffected_Empty(t *testing.T) {
	affected := findTransitivelyAffected(map[string]bool{}, nil)
	if len(affected) != 0 {
		t.Errorf("expected empty, got %v", affected)
	}
}

func TestFindAffectedPackages_EmptyInputs(t *testing.T) {
	_, err := FindAffectedPackages("", "main")
	if err == nil {
		t.Error("expected error for empty repoPath")
	}

	_, err = FindAffectedPackages("/tmp", "")
	if err == nil {
		t.Error("expected error for empty baseBranch")
	}
}

// TestFindAffectedPackages_Integration creates a real git repo with multiple
// packages to verify the full pipeline end-to-end.
func TestFindAffectedPackages_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Verify git and go are available.
	for _, bin := range []string{"git", "go"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found in PATH", bin)
		}
	}

	dir := t.TempDir()

	// Initialize git repo.
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")

	// Create module with three packages:
	//   root (main) -> pkg/a -> pkg/b
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/affected\n\ngo 1.21\n")

	writeFile(t, filepath.Join(dir, "main.go"), `package main

import _ "example.com/affected/pkg/a"

func main() {}
`)

	os.MkdirAll(filepath.Join(dir, "pkg", "a"), 0o755)
	writeFile(t, filepath.Join(dir, "pkg", "a", "a.go"), `package a

import _ "example.com/affected/pkg/b"

func A() string { return "a" }
`)

	os.MkdirAll(filepath.Join(dir, "pkg", "b"), 0o755)
	writeFile(t, filepath.Join(dir, "pkg", "b", "b.go"), `package b

func B() string { return "b" }
`)

	// Create an independent package that doesn't depend on anything.
	os.MkdirAll(filepath.Join(dir, "pkg", "c"), 0o755)
	writeFile(t, filepath.Join(dir, "pkg", "c", "c.go"), `package c

func C() string { return "c" }
`)

	// Commit everything on main.
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-m", "initial")
	run(t, dir, "git", "branch", "-M", "main")

	// Create feature branch and modify pkg/b.
	run(t, dir, "git", "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "pkg", "b", "b.go"), `package b

func B() string { return "b-modified" }
`)
	run(t, dir, "git", "add", "-A")
	run(t, dir, "git", "commit", "-m", "modify pkg/b")

	// Find affected packages.
	affected, err := FindAffectedPackages(dir, "main")
	if err != nil {
		t.Fatalf("FindAffectedPackages failed: %v", err)
	}

	sort.Strings(affected)

	// pkg/b changed directly.
	// pkg/a depends on pkg/b.
	// root (main) depends on pkg/a.
	// pkg/c is independent — should NOT be affected.
	expected := []string{
		"example.com/affected",
		"example.com/affected/pkg/a",
		"example.com/affected/pkg/b",
	}

	if len(affected) != len(expected) {
		t.Fatalf("affected = %v; want %v", affected, expected)
	}
	for i := range expected {
		if affected[i] != expected[i] {
			t.Errorf("affected[%d] = %q; want %q", i, affected[i], expected[i])
		}
	}

	// Verify pkg/c is NOT in the affected list.
	for _, pkg := range affected {
		if pkg == "example.com/affected/pkg/c" {
			t.Error("pkg/c should not be affected — it has no dependency on pkg/b")
		}
	}
}

// writeFile creates a file with the given content, creating parent dirs as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// run executes a command in the given directory.
func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}
