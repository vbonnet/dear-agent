package main

import (
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
// what was written. Reasonable for short banners; not a full pipe so
// it caps at a few KB before blocking, which suits our use here.
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
		buf := make([]byte, 8192)
		n, _ := r.Read(buf)
		done <- string(buf[:n])
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}
