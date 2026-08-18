package repoinventory

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestScanUsesGitIgnoreRules(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	writeFile(t, root, ".gitignore", "ignored/\ndist/\n*~\n")
	writeFile(t, root, "tracked/SPEC.md", "tracked\n")
	writeFile(t, root, "ignored/SPEC.md", "ignored\n")
	writeFile(t, root, "dist/SPEC.md", "generated\n")
	writeFile(t, root, "~/nested/SPEC.md", "backup\n")

	files, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := Paths(files)
	if slices.Contains(paths, "ignored/SPEC.md") || slices.Contains(paths, "dist/SPEC.md") || slices.Contains(paths, "~/nested/SPEC.md") {
		t.Fatalf("ignored paths leaked into inventory: %v", paths)
	}
	if !slices.Contains(paths, "tracked/SPEC.md") {
		t.Fatalf("tracked path missing from inventory: %v", paths)
	}
}

func TestScanFallbackSkipsRepositoryOutputsAndDependencies(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "internal/example/example.go", "package example\n")
	for _, dir := range ExcludedDirectoryNames() {
		writeFile(t, root, filepath.Join(dir, "hidden.go"), "package hidden\n")
	}

	files, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := Paths(files)
	if len(paths) != 1 || paths[0] != "internal/example/example.go" {
		t.Fatalf("fallback inventory = %v, want only implementation file", paths)
	}
}

func TestScanDoesNotDiscardGitIgnorePolicyWhenGitFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, ".git", "gitdir: /definitely/missing\n")
	writeFile(t, root, "generated/uncovered.go", "package generated\n")

	if _, err := Scan(root); err == nil {
		t.Fatal("Scan succeeded through filesystem fallback after Git inventory failed")
	} else if !strings.Contains(err.Error(), "fatal:") {
		t.Fatalf("Scan error = %q, want Git stderr diagnostics", err)
	}
}

func TestFallbackSkipsUnreadableEntry(t *testing.T) {
	t.Parallel()
	var paths []string
	collect := collectFilesystemPath(t.TempDir(), &paths)
	if err := collect("unreadable", nil, fs.ErrPermission); err != nil {
		t.Fatalf("unreadable fallback entry returned error: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("unreadable fallback entry was collected: %v", paths)
	}
}

func TestScanReturnsStableRelativePathsAndExecutableMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "z/file.go", "package z\n")
	writeFile(t, root, "a/hook", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(root, "a", "hook"), 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Paths(files), []string{"a/hook", "z/file.go"}; !slices.Equal(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	if files[0].Name() != "hook" || !files[0].Executable() {
		t.Fatalf("executable metadata lost: %+v", files[0])
	}
	if files[1].Executable() {
		t.Fatalf("ordinary source reported executable: %+v", files[1])
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := gittest.Output(t, dir, args...); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
