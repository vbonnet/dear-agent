package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/stophook"
)

func newResult() *stophook.Result { return &stophook.Result{HookName: "test"} }

func makeGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustCmd(t, dir, "git", "init")
	mustCmd(t, dir, "git", "config", "user.email", "test@test.com")
	mustCmd(t, dir, "git", "config", "user.name", "Test")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func mustCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func commitAll(t *testing.T, dir string) {
	t.Helper()
	mustCmd(t, dir, "git", "add", ".")
	mustCmd(t, dir, "git", "commit", "-m", "init")
}

// --- checkCodeMarkers ---

func TestCheckCodeMarkers_NoMarkers(t *testing.T) {
	dir := makeGitRepo(t)
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	commitAll(t, dir)

	r := newResult()
	checkCodeMarkers(r, dir)

	for _, f := range r.Findings {
		if f.Check == "code-markers" && f.Severity == stophook.SeverityPass {
			return
		}
	}
	t.Errorf("expected Pass when no markers, got %+v", r.Findings)
}

func TestCheckCodeMarkers_TODO(t *testing.T) {
	dir := makeGitRepo(t)
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\n// TODO: fix this\nfunc main() {}\n")
	commitAll(t, dir)

	r := newResult()
	checkCodeMarkers(r, dir)

	for _, f := range r.Findings {
		if f.Check == "todos" && f.Severity == stophook.SeverityWarn {
			return
		}
	}
	t.Errorf("expected Warn for TODO markers, got %+v", r.Findings)
}

func TestCheckCodeMarkers_FIXME(t *testing.T) {
	dir := makeGitRepo(t)
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\n// FIXME: broken\nfunc main() {}\n")
	commitAll(t, dir)

	r := newResult()
	checkCodeMarkers(r, dir)

	for _, f := range r.Findings {
		if f.Check == "fixmes" && f.Severity == stophook.SeverityWarn {
			return
		}
	}
	t.Errorf("expected Warn for FIXME markers, got %+v", r.Findings)
}

func TestCheckCodeMarkers_HACK(t *testing.T) {
	dir := makeGitRepo(t)
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\n// HACK: workaround for bug\nfunc main() {}\n")
	commitAll(t, dir)

	r := newResult()
	checkCodeMarkers(r, dir)

	for _, f := range r.Findings {
		if f.Check == "hacks" && f.Severity == stophook.SeverityWarn {
			return
		}
	}
	t.Errorf("expected Warn for HACK markers, got %+v", r.Findings)
}

// --- checkDocs ---

func TestCheckDocs_HasReadme(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "README.md"), "# Project\n\nThis is a well-documented project.\n")

	r := newResult()
	checkDocs(r, dir)

	// With a README, the docs check should not Block.
	for _, f := range r.Findings {
		if f.Severity == stophook.SeverityBlock {
			t.Errorf("unexpected Block when README exists: %+v", f)
		}
	}
}

func TestCheckDocs_NoReadme(t *testing.T) {
	dir := t.TempDir()
	// No README.md; no SPEC.md; no source files → docs warn.

	r := newResult()
	checkDocs(r, dir)

	for _, f := range r.Findings {
		if f.Check == "docs" && f.Severity == stophook.SeverityWarn {
			return
		}
	}
	t.Errorf("expected Warn when no README found, got %+v", r.Findings)
}
