package main

import (
	"slices"
	"strings"
	"testing"
)

// supervisorRoleTokens mirrors the role substrings that AGM's two
// IsSupervisorSession matchers key off of:
//   - agm/internal/tmux:       "orchestrator", "meta-orchestrator", "overseer"
//   - agm/internal/ops:        "orchestrator", "overseer", "meta-"
//
// vroom-dispatch lives outside the agm/ module subtree, so it cannot import
// those internal packages directly. This list is the intersection that every
// supervisor session Name must satisfy in BOTH matchers. If a name fails to
// contain at least one token recognized by each matcher, the supervisor is
// miscounted as a worker — eating a worker slot and losing auto-respawn
// (regression ce-cg0o, where "vroom-orch"/"vroom-meta-o" matched neither or
// only one matcher).
var (
	tmuxTokens = []string{"orchestrator", "meta-orchestrator", "overseer"}
	opsTokens  = []string{"orchestrator", "overseer", "meta-"}
)

func matchesAny(name string, tokens []string) bool {
	lower := strings.ToLower(name)
	for _, t := range tokens {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// TestSupervisorNamesAreRecognized guards the dispatch-vs-matcher naming
// contract: each persistent supervisor session must be detected as a
// supervisor by BOTH AGM IsSupervisorSession implementations, otherwise it is
// treated as a worker (ce-cg0o).
func TestSupervisorNamesAreRecognized(t *testing.T) {
	if len(supervisors) == 0 {
		t.Fatal("no supervisors defined")
	}
	for _, s := range supervisors {
		t.Run(s.Name, func(t *testing.T) {
			if !matchesAny(s.Name, tmuxTokens) {
				t.Errorf("supervisor Name %q not recognized by tmux matcher (tokens %v); it would be miscounted as a worker", s.Name, tmuxTokens)
			}
			if !matchesAny(s.Name, opsTokens) {
				t.Errorf("supervisor Name %q not recognized by ops matcher (tokens %v); it would be miscounted as a worker", s.Name, opsTokens)
			}
		})
	}
}

// TestSessionNewArgsPinModelAndMode guards the ce-84l2 fix: supervisors must be
// spawned with an explicit 200k-context model and auto permission mode. Relying
// on agm's claude-code defaults (sonnet at 1M context, plan mode) gives a
// session that credit-gate-fails every tick and, even when it doesn't, can only
// plan — never execute — because a detached session can't clear approval
// prompts. The model is now caller-supplied (default defaultSupervisorModel,
// overridable via -model); this test pins the wiring and the default.
func TestSessionNewArgsPinModelAndMode(t *testing.T) {
	args := sessionNewArgs("vroom-orchestrator", defaultSupervisorModel, "orchestrator")

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"session new vroom-orchestrator",
		"--detached",
		"--workspace=oss",
		"--harness=claude-code",
		"--model=sonnet-200k",
		"--mode=auto",
		"--role=orchestrator",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("sessionNewArgs missing %q; got %v", want, args)
		}
	}

	// An empty role must not emit a --role flag (omitted, not "--role=").
	if strings.Contains(strings.Join(sessionNewArgs("x", "sonnet-200k", ""), " "), "--role") {
		t.Errorf("empty role should omit --role flag")
	}

	// Default is the 200k-context Sonnet variant: conserve Opus quota until the
	// cost/benefit of Opus supervisors is proven (supersedes the PR #507 Opus
	// default; Opus reachable via -model=opus-200k).
	if defaultSupervisorModel != "sonnet-200k" {
		t.Errorf("defaultSupervisorModel = %q, want sonnet-200k (conserve Opus; 200k dodges the 1M credit gate)", defaultSupervisorModel)
	}
	// Defend against silent reintroduction of the credit-gated 1M aliases for
	// the default. The bare `opus`/`sonnet` aliases both resolve to [1m] models.
	if defaultSupervisorModel == "opus" || defaultSupervisorModel == "sonnet" {
		t.Errorf("default model %q is a 1M-context alias (credit-gated); use the -200k variant", defaultSupervisorModel)
	}
	if supervisorMode != "auto" {
		t.Errorf("supervisorMode = %q, want auto (plan mode can't execute when detached)", supervisorMode)
	}
}

// TestSupervisorPeerRefsResolve ensures every PrimaryFor/TertiaryFor reference
// points at a real supervisor ID, so heartbeat verification wiring stays
// internally consistent after a rename.
func TestSupervisorPeerRefsResolve(t *testing.T) {
	ids := make(map[string]bool, len(supervisors))
	for _, s := range supervisors {
		ids[s.ID] = true
	}
	for _, s := range supervisors {
		if s.PrimaryFor != "" && !ids[s.PrimaryFor] {
			t.Errorf("supervisor %q PrimaryFor %q does not match any supervisor ID", s.ID, s.PrimaryFor)
		}
		if s.TertiaryFor != "" && !ids[s.TertiaryFor] {
			t.Errorf("supervisor %q TertiaryFor %q does not match any supervisor ID", s.ID, s.TertiaryFor)
		}
	}
}

// TestWorkerSpawnPinsOpusAndWayfinder guards the worker-side counterpart of the
// ce-84l2 supervisor fix. Workers are spawned by the Orchestrator from its skill
// instructions (not by Go code here), so the contract lives in the embedded
// orchestrator.md. This test pins it: workers must spawn with opus-200k + auto
// mode and be told to drive their bead through wayfinder, never raw sonnet
// execution. The same credit-gate / plan-mode reasoning as the supervisors
// applies — workers run on the same Max-plan OAuth.
func TestWorkerSpawnPinsOpusAndWayfinder(t *testing.T) {
	b, err := skills.ReadFile("skills/orchestrator.md")
	if err != nil {
		t.Fatalf("read embedded orchestrator.md: %v", err)
	}
	doc := string(b)

	for _, want := range []string{
		"--model=opus-200k", // Opus, 200k variant (dodges the 1M credit gate)
		"--mode=auto",       // detached workers can't clear plan-exit prompts
		"/wayfinder",        // workers drive the bead through the SDLC workflow
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("orchestrator.md worker dispatch missing %q", want)
		}
	}

	if slices.Contains(strings.Fields(doc), "--model=opus") {
		t.Errorf("orchestrator.md must not spawn workers with the 1M-context opus alias (credit-gated)")
	}
	if strings.Contains(doc, `"model":"default"`) {
		t.Errorf(`orchestrator.md still records "model":"default"; dispatch record must say "opus-200k"`)
	}
}
