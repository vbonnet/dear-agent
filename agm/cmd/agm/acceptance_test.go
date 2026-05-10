package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindDearAgentRootFromWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".dear-agent.yml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := findDearAgentRootFrom(deep)
	if got != root {
		t.Errorf("findDearAgentRootFrom(%q) = %q, want %q", deep, got, root)
	}
}

func TestFindDearAgentRootFromMissing(t *testing.T) {
	// Walk up from /tmp/<random>/x; nothing should be found unless a
	// real .dear-agent.yml lives at /tmp or above.
	dir := t.TempDir()
	got := findDearAgentRootFrom(dir)
	if got != "" {
		// We can't fully control /tmp, but we don't expect a hit. If
		// the test environment has one, skip rather than fail.
		t.Skipf("ambient .dear-agent.yml at %q makes this assertion non-portable", got)
	}
}

func TestAnnounceAcceptanceCriteriaPrintsBanner(t *testing.T) {
	root := t.TempDir()
	yml := `version: 1
acceptance-criteria:
  - type: tests-pass
    command: "go test ./..."
`
	if err := os.WriteFile(filepath.Join(root, ".dear-agent.yml"), []byte(yml), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout := captureStdout(t, func() { announceAcceptanceCriteria(root) })
	if !strings.Contains(stdout, "Acceptance criteria") {
		t.Errorf("banner missing header: %q", stdout)
	}
	if !strings.Contains(stdout, "tests-pass") {
		t.Errorf("banner missing criterion: %q", stdout)
	}
}

func TestAnnounceAcceptanceCriteriaSilentWhenAbsent(t *testing.T) {
	stdout := captureStdout(t, func() { announceAcceptanceCriteria(t.TempDir()) })
	if stdout != "" {
		t.Errorf("expected silent, got %q", stdout)
	}
}

func TestAnnounceAcceptanceCriteriaSilentWhenEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".dear-agent.yml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	stdout := captureStdout(t, func() { announceAcceptanceCriteria(root) })
	if stdout != "" {
		t.Errorf("expected silent for empty section, got %q", stdout)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// what was written. The reader drains until EOF so multi-write banners
// don't get truncated when the pipe wakes the reader between writes
// (a single Read on a pipe is allowed to return short — Linux/CI hits
// this routinely while macOS often coalesces).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}
