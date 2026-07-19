package gracefulexit

// This test guards the link between the behavioural anti-stall contract and
// the place agents actually read. The anti-stall policy
// (docs/policies/anti-stall.ai.md) only works if AGENTS.md keeps pointing at it
// and the spec keeps stating its directives — a dangling reference or a
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
// AGENTS.md. It fails the test rather than guessing if neither is
// found.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		_, goModErr := os.Stat(filepath.Join(dir, "go.mod"))
		_, agentsErr := os.Stat(filepath.Join(dir, "AGENTS.md"))
		if goModErr == nil && agentsErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found walking up from package dir")
		}
		dir = parent
	}
}

// TestAntiStallSpecExists asserts the consolidated anti-stall policy is present
// and still carries its five behavioral directives plus the boundary
// section that reconciles it with Agent Delegation Enforcement. If any
// directive heading is removed the spec has been gutted and the test fails
// loudly.
func TestAntiStallSpecExists(t *testing.T) {
	root := repoRoot(t)
	specPath := filepath.Join(root, "docs", "policies", "anti-stall.ai.md")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("anti-stall spec missing at %s: %v", specPath, err)
	}
	spec := string(raw)

	wantMarkers := []string{
		"Continue through known work",              // directive 1
		"Treat an empty result as valid",           // directive 2
		"Make reversible implementation decisions", // directive 3
		"Minimize human blocking",                  // directive 4
		"Track a local blocker and move on",        // directive 5
		"Stop boundaries",                          // stop-side reconciliation
		"pkg/gracefulexit/SPEC.md",                 // executable no-overfit owner
	}
	for _, m := range wantMarkers {
		if !strings.Contains(spec, m) {
			t.Errorf("anti-stall spec is missing required marker %q — the directive may have been removed", m)
		}
	}
}

// TestAgentsMdReferencesAntiStallSpec asserts the root router still points
// agents at the spec and preserves its keep-working default. The detailed
// directives stay in the spec so AGENTS.md does not become a second copy.
func TestAgentsMdReferencesAntiStallSpec(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	agents := string(raw)
	normalized := strings.Join(strings.Fields(agents), " ")

	if !strings.Contains(agents, "docs/policies/anti-stall.ai.md") {
		t.Error("AGENTS.md no longer references docs/policies/anti-stall.ai.md — agents will not inherit the anti-stall contract")
	}
	if !strings.Contains(normalized, "continue through known work") {
		t.Error("AGENTS.md no longer states the anti-stall keep-working default")
	}
}
