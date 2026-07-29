package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestRun_NoSpecFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	outF, errF := tmpFiles(t)
	code := run([]string{tmpDir}, outF, errF)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(readFile(t, errF), "no SPEC.md files found") {
		t.Errorf("stderr missing 'no SPEC.md files found'; got: %q", readFile(t, errF))
	}
}

func TestRun_StdoutMode(t *testing.T) {
	t.Parallel()
	spec := writeTmpSpec(t, "## Requirements\n\n**BTN-01** When a button is clicked, the system shall increment the counter.\n")
	outF, errF := tmpFiles(t)
	code := run([]string{spec}, outF, errF)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, readFile(t, errF))
	}
	if !strings.Contains(readFile(t, outF), "Scenario") {
		t.Errorf("stdout missing Scenario block; got: %q", readFile(t, outF))
	}
}

func TestRun_DryRun(t *testing.T) {
	t.Parallel()
	spec := writeTmpSpec(t, "## Requirements\n\n**AUTH-01** When a user logs in, the system shall start a session.\n")
	outDir := t.TempDir()
	outF, errF := tmpFiles(t)
	code := run([]string{"-out", outDir, "-dry-run", spec}, outF, errF)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, readFile(t, errF))
	}
	if !strings.Contains(readFile(t, outF), "would write:") {
		t.Errorf("dry-run stdout = %q, want 'would write:'", readFile(t, outF))
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("dry-run wrote %d file(s), expected 0", len(entries))
	}
}

func TestRun_WriteToDir(t *testing.T) {
	t.Parallel()
	spec := writeTmpSpec(t, "## Requirements\n\n**SVC-01** When the service starts, the system shall return 200 on the health endpoint.\n")
	outDir := t.TempDir()
	outF, errF := tmpFiles(t)
	code := run([]string{"-out", outDir, spec}, outF, errF)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, readFile(t, errF))
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) == 0 {
		t.Error("expected a .feature file to be written, got none")
	}
}

func TestFeatureOutPath(t *testing.T) {
	t.Parallel()
	got := featureOutPath("/out", "internal/fsguard/SPEC.md")
	want := filepath.Join("/out", "fsguard.feature")
	if got != want {
		t.Errorf("featureOutPath = %q, want %q", got, want)
	}
}

func TestExpandPaths_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := expandPaths([]string{"/nonexistent/path"})
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}

func TestExpandPathsHonorsRepositoryIgnoreAndGeneratedPolicy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cmd := gittest.Command(t, root, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"visible", "ignored", "dist"} {
		path := filepath.Join(root, dir, "SPEC.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("The system shall be deterministic.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files, err := expandPaths([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "visible", "SPEC.md")}
	if !slices.Equal(files, want) {
		t.Fatalf("expanded files = %v, want %v", files, want)
	}
}

// writeTmpSpec writes a SPEC.md in a temp subdirectory and returns its path.
func writeTmpSpec(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "SPEC.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTmpSpec: %v", err)
	}
	return path
}

// tmpFiles creates two throwaway temp files (stdout, stderr) for a run() call.
func tmpFiles(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	makeF := func(name string) *os.File {
		f, err := os.CreateTemp(t.TempDir(), name+"-*")
		if err != nil {
			t.Fatalf("tmpFiles %s: %v", name, err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}
	return makeF("stdout"), makeF("stderr")
}

// readFile seeks to the beginning of f and returns its contents.
func readFile(t *testing.T, f *os.File) string {
	t.Helper()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("readFile seek: %v", err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("readFile read: %v", err)
	}
	return string(b)
}
