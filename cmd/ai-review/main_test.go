package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// initRepo creates a throwaway git repo with two commits and returns the repo
// dir plus the base and head SHAs, so run() can compute a real diff without
// touching the network.
func initRepo(t *testing.T, secondFileContent string) (dir, base, head string) {
	t.Helper()
	dir = t.TempDir()
	sandbox := gittest.Default(t)
	run := func(args ...string) string {
		return sandbox.Run(t, dir, args...)
	}
	run("init", "-q")
	sandbox.HardenRepo(t, dir)
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

func TestGitMergeBase_ExcludesBaseOnlyChanges(t *testing.T) {
	dir, base, _ := initRepo(t, "feature change\n")
	chdir(t, dir)
	sandbox := gittest.Default(t)
	git := func(args ...string) string {
		return sandbox.Run(t, dir, args...)
	}
	feature := trim(git("rev-parse", "HEAD"))
	git("checkout", "-q", "-b", "base-advanced", base)
	if err := os.WriteFile(filepath.Join(dir, "base-only.txt"), []byte("base advance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "base-only.txt")
	git("commit", "-q", "-m", "base advance")
	advancedBase := trim(git("rev-parse", "HEAD"))

	mergeBase, err := gitMergeBase(advancedBase, feature)
	if err != nil {
		t.Fatal(err)
	}
	if mergeBase != base {
		t.Fatalf("merge base = %s, want %s", mergeBase, base)
	}
	paths, err := gitChangedPaths(mergeBase, feature)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "a.txt" {
		t.Fatalf("PR paths = %v, want only a.txt", paths)
	}
}

// TestGitChangedPaths_IncludesRenameSource is the regression guard for the
// rename bypass: moving a protected file to an ordinary path must still expose
// the protected SOURCE path to escalation scanning.
func TestGitChangedPaths_IncludesRenameSource(t *testing.T) {
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string {
		return sandbox.Run(t, dir, args...)
	}
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Content long enough that git would otherwise detect a 100% rename.
	body := "{\n  \"permissions\": { \"allow\": [\"a\",\"b\",\"c\"], \"deny\": [\"d\"] }\n}\n"
	if err := os.WriteFile(filepath.Join(dir, ".claude/settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))
	git("mv", ".claude/settings.json", "safe-config.json")
	git("commit", "-q", "-m", "move")
	head := trim(git("rev-parse", "HEAD"))

	chdir(t, dir)
	paths, err := gitChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	var sawSource bool
	for _, p := range paths {
		if p == ".claude/settings.json" {
			sawSource = true
		}
	}
	if !sawSource {
		t.Fatalf("rename source .claude/settings.json missing from changed paths %v", paths)
	}
	if got := EscalationTriggers(paths, "", ""); len(got) == 0 {
		t.Fatal("renaming a protected settings file away must still escalate")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
}
