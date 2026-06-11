package gracefulexit

// This test guards the link between the behavioural anti-stall contract and
// the place agents actually read. The anti-stall spec
// (docs/design/anti-stall.md) only works if .claude/CLAUDE.md keeps pointing
// at it and the spec keeps stating its directives — a dangling reference or a
// gutted spec is a silent regression of an instruction-tier rule. The repo's
// recurring failure mode is exactly this: a doc lands but the wiring that
// makes agents see it rots untested. gracefulexit is the natural home: the
// spec's directive 2 ("nothing found is valid") *is* this package's guardrail,
// so the two are already coupled.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot walks up from the test's working directory (the package dir) until
// it finds the repository root, identified by the co-presence of go.mod and
// .claude/CLAUDE.md. It fails the test rather than guessing if neither is
// found.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		_, goModErr := os.Stat(filepath.Join(dir, "go.mod"))
		_, claudeErr := os.Stat(filepath.Join(dir, ".claude", "CLAUDE.md"))
		if goModErr == nil && claudeErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found walking up from package dir")
		}
		dir = parent
	}
}

// TestAntiStallSpecExists asserts the consolidated anti-stall specification is
// present and still carries its five behavioural directives plus the boundary
// section that reconciles it with Agent Delegation Enforcement. If any
// directive heading is removed the spec has been gutted and the test fails
// loudly.
func TestAntiStallSpecExists(t *testing.T) {
	root := repoRoot(t)
	specPath := filepath.Join(root, "docs", "design", "anti-stall.md")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("anti-stall spec missing at %s: %v", specPath, err)
	}
	spec := string(raw)

	wantMarkers := []string{
		"Continue through backlogs without asking",         // directive 1
		`"Nothing found" is always a valid outcome`,        // directive 2
		"Present decisions, not questions",                 // directive 3
		"Minimize blocking on human input",                 // directive 4
		"If genuinely blocked, file it and move on",        // directive 5
		"The boundary",                                     // stop-side reconciliation
		"graceful-exit.md",                                 // reuses, not duplicates, this pkg's guardrail
	}
	for _, m := range wantMarkers {
		if !strings.Contains(spec, m) {
			t.Errorf("anti-stall spec is missing required marker %q — the directive may have been removed", m)
		}
	}
}

// TestClaudeMdReferencesAntiStallSpec asserts CLAUDE.md still points agents at
// the spec. A dangling or deleted reference means agents stop inheriting the
// contract even though the spec file still exists — the precise rot this test
// exists to catch.
func TestClaudeMdReferencesAntiStallSpec(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	claude := string(raw)

	if !strings.Contains(claude, "docs/design/anti-stall.md") {
		t.Error("CLAUDE.md no longer references docs/design/anti-stall.md — agents will not inherit the anti-stall contract")
	}
	if !strings.Contains(claude, "Anti-Stall — Continuous Execution") {
		t.Error("CLAUDE.md is missing the Anti-Stall — Continuous Execution section heading")
	}
}
