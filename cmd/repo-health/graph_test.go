package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLongestChainAcyclic(t *testing.T) {
	// a -> b -> c -> d  (chain of 3 edges); a -> e is shorter.
	g := map[string][]string{
		"a": {"b", "e"},
		"b": {"c"},
		"c": {"d"},
		"d": {},
		"e": {},
	}
	depth, cycles := longestChainAndCycles(g)
	if depth != 3 {
		t.Errorf("depth = %d, want 3", depth)
	}
	if len(cycles) != 0 {
		t.Errorf("unexpected cycles: %v", cycles)
	}
}

func TestDetectCycle(t *testing.T) {
	// a -> b -> c -> a
	g := map[string][]string{
		"a": {"b"},
		"b": {"c"},
		"c": {"a"},
	}
	_, cycles := longestChainAndCycles(g)
	if len(cycles) != 1 {
		t.Fatalf("got %d cycles, want 1: %v", len(cycles), cycles)
	}
}

func TestCycleKeyRotationStable(t *testing.T) {
	k1 := cycleKey([]string{"a", "b", "c"})
	k2 := cycleKey([]string{"b", "c", "a"})
	k3 := cycleKey([]string{"c", "a", "b"})
	if k1 != k2 || k2 != k3 {
		t.Errorf("rotations should share a key: %q %q %q", k1, k2, k3)
	}
}

func TestDocPairingDrift(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		if err := os.WriteFile(filepath.Join(root, rel), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Paired rationale (has .md content) — not drift.
	write("AGENTS.md")
	write("AGENTS.why.md")
	// Orphaned rationale — drift (warn).
	write("ORPHAN.why.md")
	// .ai.md without .why.md — info only.
	write("LONELY.ai.md")

	sc := &scanCtx{root: root, opts: defaultOptions(root, "m")}
	m, unpaired := docPairingDrift(sc)
	if !m.Available {
		t.Fatal("doc pairing should be available")
	}
	var orphan, info int
	for _, u := range unpaired {
		if hasSuffixInfo(u) {
			info++
		} else {
			orphan++
		}
	}
	if orphan != 1 {
		t.Errorf("orphaned = %d, want 1 (ORPHAN.why.md); got %v", orphan, unpaired)
	}
	if info != 1 {
		t.Errorf("info = %d, want 1 (LONELY.ai.md); got %v", info, unpaired)
	}
}

func TestRenderMarkdownContainsVerdict(t *testing.T) {
	r := healthyReport()
	r.Status = StatusHealthy
	md := renderMarkdown(r)
	if len(md) == 0 {
		t.Fatal("empty markdown")
	}
	for _, want := range []string{"# Repo Health", "Code Quality", "Architecture", "Agent Health", "Drift"} {
		if !contains(md, want) {
			t.Errorf("markdown missing section %q", want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
