package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestRunRepository(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init", "-q")
	writeCLIFile(t, repo, ".dear-agent.yml", "instruction-policy:\n  surfaces:\n    - match: AGENTS.md\n      owner: root\n")
	writeCLIFile(t, repo, "AGENTS.md", "# Current instructions\n")
	runTestGit(t, repo, "add", ".dear-agent.yml", "AGENTS.md")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 active instruction file") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunViolationAndUsage(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init", "-q")
	writeCLIFile(t, repo, ".dear-agent.yml", "instruction-policy:\n  surfaces:\n    - match: AGENTS.md\n      owner: root\n")
	writeCLIFile(t, repo, "AGENTS.md", "Create W0-charter.md.\n")
	runTestGit(t, repo, "add", ".dear-agent.yml", "AGENTS.md")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "wayfinder-v1") {
		t.Fatalf("violation run = %d, stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("usage run = %d, stderr=%q", code, stderr.String())
	}
}

func writeCLIFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runTestGit(t *testing.T, root string, args ...string) {
	t.Helper()
	gittest.Run(t, root, args...)
}
