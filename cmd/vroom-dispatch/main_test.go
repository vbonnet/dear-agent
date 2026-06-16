package main

import (
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
// spawned with an explicit 200k-context Opus model and auto permission mode.
// Relying on agm's claude-code defaults (sonnet at 1M context, plan mode) gives
// a session that credit-gate-fails every tick and, even when it doesn't, can
// only plan — never execute — because a detached session can't clear approval
// prompts.
func TestSessionNewArgsPinModelAndMode(t *testing.T) {
	args := sessionNewArgs("vroom-orchestrator")

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"session new vroom-orchestrator",
		"--detached",
		"--workspace=oss",
		"--harness=claude-code",
		"--model=opus-200k",
		"--mode=auto",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("sessionNewArgs missing %q; got %v", want, args)
		}
	}

	// Defend against silent reintroduction of the credit-gated 1M default and
	// the non-executable plan default.
	if strings.Contains(joined, "--model=opus ") || strings.HasSuffix(joined, "--model=opus") {
		t.Errorf("must not spawn with the 1M-context opus alias (credit-gated); got %v", args)
	}
	if supervisorModel != "opus-200k" {
		t.Errorf("supervisorModel = %q, want opus-200k (200k dodges the 1M credit gate)", supervisorModel)
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
