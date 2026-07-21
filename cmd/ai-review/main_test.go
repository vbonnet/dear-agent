package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initRepo creates a throwaway git repo with two commits and returns the repo
// dir plus the base and head SHAs, so run() can compute a real diff without
// touching the network.
func initRepo(t *testing.T, secondFileContent string) (dir, base, head string) {
	t.Helper()
	dir = t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return string(out)
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	base = trim(run("rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(secondFileContent), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "head")
	head = trim(run("rev-parse", "HEAD"))
	return dir, base, head
}

func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}

// baseConfig returns a config with no PR/repo (so comments are skipped) and no
// API key, then applies overrides.
func baseConfig() config {
	return config{
		model:   "claude-opus-4-8",
		effort:  "high",
		maxDiff: defaultMaxDiffBytes,
	}
}

func TestRun_MissingKeyFailsClosed(t *testing.T) {
	c := baseConfig()
	if got := run(c); got != 1 {
		t.Fatalf("missing key: run() = %d, want 1 (fail closed)", got)
	}
}

func TestRun_MissingKeyWithOverridePasses(t *testing.T) {
	c := baseConfig()
	c.override = true
	if got := run(c); got != 0 {
		t.Fatalf("missing key + override: run() = %d, want 0", got)
	}
}

func TestRun_ForkFailsClosed(t *testing.T) {
	c := baseConfig()
	c.isFork = true
	c.apiKey = "sk-does-not-matter" // fork check precedes key check
	if got := run(c); got != 1 {
		t.Fatalf("fork: run() = %d, want 1 (fail closed)", got)
	}
}

func TestRun_ForkWithOverridePassesKeyCheck(t *testing.T) {
	// A fork with the override label bypasses the fork gate; with a real key
	// it would proceed to the (network) review, so we only assert it does NOT
	// short-circuit to the fork failure. Give it no key so it stops at the
	// key gate, which the override also passes — net result 0 without network.
	c := baseConfig()
	c.isFork = true
	c.override = true
	if got := run(c); got != 0 {
		t.Fatalf("fork + override (no key): run() = %d, want 0", got)
	}
}

func TestRun_EmptyDiffApproves(t *testing.T) {
	dir, _, head := initRepo(t, "changed\n")
	chdir(t, dir)
	c := baseConfig()
	c.apiKey = "sk-test"
	c.baseSHA = head
	c.headSHA = head // diff head..head is empty
	if got := run(c); got != 0 {
		t.Fatalf("empty diff: run() = %d, want 0", got)
	}
}

func TestRun_OversizeDiffFailsClosed(t *testing.T) {
	big := make([]byte, 2000)
	for i := range big {
		big[i] = 'x'
	}
	dir, base, head := initRepo(t, string(big)+"\n")
	chdir(t, dir)
	c := baseConfig()
	c.apiKey = "sk-test"
	c.baseSHA = base
	c.headSHA = head
	c.maxDiff = 500 // force oversize
	if got := run(c); got != 1 {
		t.Fatalf("oversize diff: run() = %d, want 1 (fail closed, no truncation)", got)
	}
}

func TestRun_OversizeDiffWithOverridePasses(t *testing.T) {
	big := make([]byte, 2000)
	for i := range big {
		big[i] = 'x'
	}
	dir, base, head := initRepo(t, string(big)+"\n")
	chdir(t, dir)
	c := baseConfig()
	c.apiKey = "sk-test"
	c.baseSHA = base
	c.headSHA = head
	c.maxDiff = 500
	c.override = true
	if got := run(c); got != 0 {
		t.Fatalf("oversize diff + override: run() = %d, want 0", got)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}
