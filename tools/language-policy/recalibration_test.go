package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", rel, err)
	}
}

func TestBuildTestIndex(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "tests/bats/a.bats", `
@test "deploy works" {
  run "$BATS_TEST_DIRNAME/../../scripts/deploy.sh" --dry-run
}
`)
	writeFile(t, root, "tests/bats/b.bats", `run bash scripts/nested/tool.sh`)
	// Not a .bats file: must be ignored, or any mention anywhere would count.
	writeFile(t, root, "tests/bats/notes.md", `scripts/uncovered.sh is not tested`)

	idx, err := BuildTestIndex(root)
	if err != nil {
		t.Fatalf("BuildTestIndex: %v", err)
	}
	for _, p := range []string{"scripts/deploy.sh", "./scripts/deploy.sh", "scripts/nested/tool.sh"} {
		if !idx.Covered(p) {
			t.Errorf("Covered(%q) = false, want true", p)
		}
	}
	if idx.Covered("scripts/uncovered.sh") {
		t.Error("a mention in a non-bats file was counted as coverage")
	}
}

// A repository with no bats suite must not error; it simply has no test
// evidence to credit.
func TestBuildTestIndexNoSuite(t *testing.T) {
	idx, err := BuildTestIndex(t.TempDir())
	if err != nil {
		t.Fatalf("BuildTestIndex with no suite = %v, want nil", err)
	}
	if idx.Covered("anything.sh") {
		t.Error("empty index reported coverage")
	}
}

// The substantive recalibration: a long script passes on a test, with no
// waiver. This is what makes the escape hatch productive.
func TestTestedScriptNeedsNoWaiver(t *testing.T) {
	root := newFixtureRepo(t, goodStore, nil)
	long := "#!/bin/bash\n" + strings.Repeat("echo x\n", 40)
	writeFile(t, root, "scripts/covered.sh", long)
	writeFile(t, root, "scripts/naked.sh", long)
	writeFile(t, root, "tests/bats/cover.bats", `run scripts/covered.sh`)

	if err := runCheck([]string{"-repo", root, "--all", "scripts/covered.sh"}); err != nil {
		t.Errorf("tested over-limit script reported a violation: %v", err)
	}
	err := runCheck([]string{"-repo", root, "--all", "scripts/naked.sh"})
	if err == nil {
		t.Fatal("untested, unwaived 40-line script passed")
	}
	if !strings.Contains(err.Error(), "untested") {
		t.Errorf("error = %q, want it to name the untested condition", err)
	}
}

func TestRatchet(t *testing.T) {
	baseline := `{"rules":{"bash-20-line-limit":{"max_waivers":2,"goal":"drive to 0"}}}`
	b, err := LoadBaseline(strings.NewReader(baseline))
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if err := b.CheckRatchet("bash-20-line-limit", 2); err != nil {
		t.Errorf("count at the ceiling = %v, want nil", err)
	}
	if err := b.CheckRatchet("bash-20-line-limit", 1); err != nil {
		t.Errorf("count below the ceiling = %v, want nil", err)
	}
	err = b.CheckRatchet("bash-20-line-limit", 3)
	if err == nil {
		t.Fatal("count above the ceiling passed the ratchet")
	}
	if !strings.Contains(err.Error(), "ratchet exceeded") {
		t.Errorf("error = %q, want it to name the ratchet", err)
	}
	// An undeclared rule must fail loudly, not pass by default.
	if err := b.CheckRatchet("some-other-rule", 0); err == nil {
		t.Error("undeclared rule passed the ratchet")
	}
}

// The committed baseline must match the committed store, so the ratchet is
// never quietly slack.
func TestCommittedBaselineMatchesStore(t *testing.T) {
	root := repoRootForTest(t)
	bf, err := os.Open(filepath.Join(root, baselinePath))
	if err != nil {
		t.Fatalf("opening baseline: %v", err)
	}
	defer func() { _ = bf.Close() }()
	b, err := LoadBaseline(bf)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	sf, err := os.Open(filepath.Join(root, storePath))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer func() { _ = sf.Close() }()
	store, err := LoadStore(sf)
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	if err := b.CheckRatchet(ruleName, len(store.All)); err != nil {
		t.Fatalf("committed store violates the committed ratchet: %v", err)
	}
	// The ceiling should track the store. A ceiling well above the actual
	// count is slack that lets the backlog refill silently.
	declared := b.Rules[ruleName].MaxWaivers
	if declared != len(store.All) {
		t.Errorf("baseline max_waivers = %d but the store has %d waivers; lower the ceiling to match",
			declared, len(store.All))
	}
}
