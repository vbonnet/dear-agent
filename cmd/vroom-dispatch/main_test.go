package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	vroomsupervisor "github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
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

// TestSessionNewArgsPinModelAndMode guards the ce-84l2/ce-o5nj fixes:
// supervisors must be spawned with an explicit model and startup auto mode
// whenever the canonical harness supports it. Detached sessions cannot clear
// approval prompts, so default/plan mode turns every tick into an operator
// intervention. The Claude model is caller-supplied (default
// defaultSupervisorModel, overridable via -model); this test pins the wiring
// and the default.
func TestSessionNewArgsPinModelAndMode(t *testing.T) {
	meta := supervisor{
		Name:    "vroom-meta-orchestrator",
		Role:    "meta-orchestrator",
		Harness: "claude-code",
		Model:   defaultSupervisorModel,
	}
	args := sessionNewArgs(meta, defaultSupervisorModel)

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"session new vroom-meta-orchestrator",
		"--detached",
		"--workspace=oss",
		"--harness=claude-code",
		"--model=sonnet-200k",
		"--mode=auto",
		"--role=meta-orchestrator",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("sessionNewArgs missing %q; got %v", want, args)
		}
	}

	// An empty role must not emit a --role flag (omitted, not "--role=").
	if strings.Contains(strings.Join(sessionNewArgs(supervisor{Name: "x", Harness: "claude-code", Model: "sonnet-200k"}, "sonnet-200k"), " "), "--role") {
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

func TestSupervisorCanonicalHarnesses(t *testing.T) {
	for _, sup := range supervisors {
		args := sessionNewArgs(sup, "opus-200k")
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--harness="+sup.Harness) {
			t.Errorf("%s args missing canonical harness %q: %v", sup.Name, sup.Harness, args)
		}
		if sup.Harness == "claude-code" {
			if !strings.Contains(joined, "--model=opus-200k") {
				t.Errorf("%s should honor Claude model override, got %v", sup.Name, args)
			}
		} else if !strings.Contains(joined, "--model="+sup.Model) {
			t.Errorf("%s should keep canonical model %q despite Claude override, got %v", sup.Name, sup.Model, args)
		}
		if supportsStartupAutoMode(sup.Harness) {
			if !strings.Contains(joined, "--mode=auto") {
				t.Errorf("%s should include startup auto mode, got %v", sup.Name, args)
			}
		} else if strings.Contains(joined, "--mode=") {
			t.Errorf("%s should not receive unsupported permission mode flag, got %v", sup.Name, args)
		}
	}
}

// TestSpawnSessionWithRetry guards the ce-mu36 fix: createAndBootSession spawns
// the 3 supervisors only ~40s apart, but agm's circuit breaker enforces a
// 2-minute MinSpawnInterval and refuses the 2nd and 3rd spawns with "spawn too
// soon". spawnSessionWithRetry must wait out the window and retry on that
// refusal — but must NOT retry other failures, and must give up after the cap.
func TestSpawnSessionWithRetry(t *testing.T) {
	sup := supervisor{Name: "vroom-orchestrator", Role: "orchestrator"}

	// Save and restore the injectable spawn/sleep hooks.
	origRun, origSleep := runSpawn, sleepFor
	t.Cleanup(func() { runSpawn, sleepFor = origRun, origSleep })

	refusal := []byte("circuit breaker: spawn refused — spawn too soon")
	cbErr := errors.New("exit status 1")

	t.Run("retries past circuit-breaker refusals then succeeds", func(t *testing.T) {
		calls, sleeps := 0, 0
		sleepFor = func(time.Duration) { sleeps++ }
		runSpawn = func(supervisor, string) ([]byte, error) {
			calls++
			if calls < 3 {
				return refusal, cbErr
			}
			return []byte("created"), nil
		}

		if err := spawnSessionWithRetry(sup, "sonnet-200k"); err != nil {
			t.Fatalf("expected success after retries, got %v", err)
		}
		if calls != 3 {
			t.Errorf("expected 3 spawn attempts, got %d", calls)
		}
		// Sleeps the window only between attempts, never after the final one.
		if sleeps != 2 {
			t.Errorf("expected 2 backoff sleeps, got %d", sleeps)
		}
	})

	t.Run("gives up after maxSpawnAttempts of persistent refusal", func(t *testing.T) {
		calls, sleeps := 0, 0
		sleepFor = func(time.Duration) { sleeps++ }
		runSpawn = func(supervisor, string) ([]byte, error) {
			calls++
			return refusal, cbErr
		}

		err := spawnSessionWithRetry(sup, "sonnet-200k")
		if err == nil {
			t.Fatal("expected failure after exhausting retries, got nil")
		}
		if calls != maxSpawnAttempts {
			t.Errorf("expected %d spawn attempts, got %d", maxSpawnAttempts, calls)
		}
		// No sleep after the last attempt: one fewer than the attempt count.
		if sleeps != maxSpawnAttempts-1 {
			t.Errorf("expected %d backoff sleeps, got %d", maxSpawnAttempts-1, sleeps)
		}
	})

	t.Run("does not retry non-circuit-breaker failures", func(t *testing.T) {
		calls, sleeps := 0, 0
		sleepFor = func(time.Duration) { sleeps++ }
		runSpawn = func(supervisor, string) ([]byte, error) {
			calls++
			return []byte("error: workspace not found"), errors.New("exit status 2")
		}

		if err := spawnSessionWithRetry(sup, "sonnet-200k"); err == nil {
			t.Fatal("expected failure to propagate, got nil")
		}
		if calls != 1 {
			t.Errorf("non-breaker failure must not retry; got %d attempts", calls)
		}
		if sleeps != 0 {
			t.Errorf("non-breaker failure must not sleep; got %d sleeps", sleeps)
		}
	})

	t.Run("succeeds on first attempt without sleeping", func(t *testing.T) {
		calls, sleeps := 0, 0
		sleepFor = func(time.Duration) { sleeps++ }
		runSpawn = func(supervisor, string) ([]byte, error) {
			calls++
			return []byte("created"), nil
		}

		if err := spawnSessionWithRetry(sup, "sonnet-200k"); err != nil {
			t.Fatalf("expected immediate success, got %v", err)
		}
		if calls != 1 || sleeps != 0 {
			t.Errorf("happy path: want 1 call/0 sleeps, got %d calls/%d sleeps", calls, sleeps)
		}
	})
}

// TestMinSpawnIntervalMatchesAgm pins the assumption ce-mu36 relies on: the
// backoff window must be at least agm's circuitbreaker.MinSpawnInterval (2
// minutes). If this drifts below agm's value, retries would fire before the
// window clears and be refused again.
func TestMinSpawnIntervalMatchesAgm(t *testing.T) {
	if minSpawnInterval < 2*time.Minute {
		t.Errorf("minSpawnInterval = %s, must be >= agm's 2m MinSpawnInterval", minSpawnInterval)
	}
}

// TestSupervisorPoliciesMaterializeCanonicalTopology proves the dispatch
// adapter owns only launch policy: every identity and peer edge is copied from
// pkg/vroom/supervisor's canonical topology rather than repeated here.
func TestSupervisorPoliciesMaterializeCanonicalTopology(t *testing.T) {
	if len(supervisors) != len(vroomsupervisor.AllMembers()) {
		t.Fatalf("dispatch policies = %d, topology members = %d", len(supervisors), len(vroomsupervisor.AllMembers()))
	}
	seen := make(map[vroomsupervisor.Role]bool, len(supervisors))
	for _, s := range supervisors {
		member, ok := vroomsupervisor.Lookup(s.ID)
		if !ok {
			t.Errorf("dispatch supervisor %q is not in the canonical topology", s.ID)
			continue
		}
		seen[member.Role] = true
		if s.Name != member.ID || s.Role != string(member.Role) ||
			s.PrimaryFor != member.PrimaryFor || s.TertiaryFor != member.TertiaryFor {
			t.Errorf("dispatch topology for %q = %+v, want %+v", s.ID, s, member)
		}
	}
	for _, member := range vroomsupervisor.AllMembers() {
		if !seen[member.Role] {
			t.Errorf("canonical member %q has no dispatch launch policy", member.ID)
		}
	}
}

// TestLoopCommandIsErrorTolerant guards the ce-ihok fix: the /loop command sent to
// every supervisor must carry the tick-resilience guard, so a single failing tick
// (an Anthropic API/credit-gate error, a tool failure, or a transient fault) can
// never halt the loop. Before this fix the happy-path tick prompt let any error
// abort the turn that arms/re-arms the loop schedule, killing the loop permanently
// and leaving the supervisor silently idle. The guard, the role's tick steps, and
// the interval must all survive into the emitted command for every role.
func TestLoopCommandIsErrorTolerant(t *testing.T) {
	if len(supervisors) == 0 {
		t.Fatal("no supervisors defined")
	}

	// Tokens that encode the "a failed tick must not kill the loop" contract.
	// Keep these loose enough to survive wording tweaks but specific enough to
	// fail if the guard is dropped or neutered.
	guardTokens := []string{
		"never end the loop",
		"do NOT stop, exit, or abort the loop",
		"next interval still fires",
		"skipped tick",
	}

	for _, s := range supervisors {
		t.Run(s.Name, func(t *testing.T) {
			cmd := buildLoopCommand(s)

			if !strings.HasPrefix(cmd, "/loop "+tickIntervalArg(s)+" ") {
				t.Errorf("loop command must start with /loop and the interval; got %q", cmd)
			}
			for _, want := range guardTokens {
				if !strings.Contains(cmd, want) {
					t.Errorf("loop command for %q missing resilience guard token %q\ngot: %s", s.Name, want, cmd)
				}
			}
			// The role's tick steps must still be present — the guard frames the
			// tick, it does not replace it.
			if s.TickPrompt == "" {
				t.Fatalf("supervisor %q has empty TickPrompt", s.Name)
			}
			if !strings.Contains(cmd, s.TickPrompt) {
				t.Errorf("loop command for %q dropped the role TickPrompt", s.Name)
			}
			// The guard must be present verbatim AND precede the role steps so it
			// frames the whole tick. Check presence explicitly first: strings.Index
			// returns -1 when absent, which would make a bare "guardIdx > promptIdx"
			// ordering check silently pass on a missing guard (gemini review, PR #523).
			guardIdx := strings.Index(cmd, tickResilienceGuard)
			promptIdx := strings.Index(cmd, s.TickPrompt)
			if guardIdx < 0 {
				t.Errorf("loop command for %q does not contain the resilience guard verbatim", s.Name)
			} else if promptIdx < 0 || guardIdx > promptIdx {
				t.Errorf("resilience guard for %q must come before the role tick steps", s.Name)
			}
		})
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

// TestDeployTaskTypeIsDeterministicAndWorkerless pins the deploy task type
// (ce-33sy, deploy-worker-vroom Phase 4): a roadmap item tagged
// `task_type: "deploy"` is executed by the Orchestrator itself via
// `dear-deploy install`, NOT by spawning a Claude worker — it consumes no worker
// slot, no Opus quota, and opens no PR. The contract spans three embedded skill
// docs: protocol.md defines the schema (task_type/deploy_target), Meta-O tags the
// bead, and the Orchestrator runs the deterministic install. This test asserts
// all three surfaces so the deterministic path cannot silently regress to a
// worker spawn.
func TestDeployTaskTypeIsDeterministicAndWorkerless(t *testing.T) {
	protocol, err := skills.ReadFile("skills/protocol.md")
	if err != nil {
		t.Fatalf("read embedded protocol.md: %v", err)
	}
	orch, err := skills.ReadFile("skills/orchestrator.md")
	if err != nil {
		t.Fatalf("read embedded orchestrator.md: %v", err)
	}
	metao, err := skills.ReadFile("skills/meta-orchestrator.md")
	if err != nil {
		t.Fatalf("read embedded meta-orchestrator.md: %v", err)
	}
	protocolDoc, orchDoc, metaoDoc := string(protocol), string(orch), string(metao)

	for _, want := range []string{"task_type", "deploy_target", `"deploy"`} {
		if !strings.Contains(protocolDoc, want) {
			t.Errorf("protocol.md roadmap schema missing deploy task-type marker %q", want)
		}
	}

	for _, want := range []string{"task_type", "deploy_target"} {
		if !strings.Contains(metaoDoc, want) {
			t.Errorf("meta-orchestrator.md missing deploy task-type tagging marker %q", want)
		}
	}

	for _, want := range []string{
		"dear-deploy install",
		"dear-deploy status",
		"task_type",
	} {
		if !strings.Contains(orchDoc, want) {
			t.Errorf("orchestrator.md missing deploy dispatch marker %q", want)
		}
	}
}

// TestDeployWorkerDispatch pins the deploy-as-worker contract (ce-x9s5): the
// Orchestrator dispatches an episodic deploy worker to land a finished bead's
// PR, and that worker's skill drives the merge through the vetted safe-* path.
// Both halves live in embedded markdown, so guard them here.
func TestDeployWorkerDispatch(t *testing.T) {
	orch, err := skills.ReadFile("skills/orchestrator.md")
	if err != nil {
		t.Fatalf("read embedded orchestrator.md: %v", err)
	}
	od := string(orch)
	for _, want := range []string{
		"worker-deploy-",                    // distinct deploy-worker session name
		"deploy-worker.md",                  // dispatch points at the installed skill
		"deploy-dispatched.jsonl",           // de-dupe ledger so we don't double-spawn
		"--model=opus-200k",                 // same credit-gate guardrail as impl workers
		"--mode=auto",                       // detached deploy worker can't clear prompts
		"supervisor.orch.deploy_dispatched", // trail event for the dispatch
	} {
		if !strings.Contains(od, want) {
			t.Errorf("orchestrator.md deploy dispatch missing %q", want)
		}
	}

	skill, err := skills.ReadFile("skills/deploy-worker.md")
	if err != nil {
		t.Fatalf("read embedded deploy-worker.md: %v", err)
	}
	sd := string(skill)
	for _, want := range []string{
		"safe-rebase",              // rebase onto main (vetted wrapper, never --force)
		"resolve-review-threads",   // resolve bot threads before merge gate
		"safe-merge",               // CI-watch + TOCTOU squash-merge via vetted path
		"--watch",                  // safe-merge --watch is the CI watch
		"WORKER, not a supervisor", // episodic, finite — not a persistent loop
	} {
		if !strings.Contains(sd, want) {
			t.Errorf("deploy-worker.md missing %q", want)
		}
	}
	// A deploy worker must never use the raw, hook-denied merge path.
	if strings.Contains(sd, "gh pr merge") {
		t.Errorf("deploy-worker.md must merge via safe-merge, not raw 'gh pr merge'")
	}
}

// TestWorkerPromptRequiresVerificationGate pins the verification-before-completion
// hard gate (ce-fvsv): the worker dispatch prompt must force ≥1 verification step
// (go test / make preflight / equivalent) before a worker may declare done. Code
// written but never run is not done — this guards against ghost completions.
func TestWorkerPromptRequiresVerificationGate(t *testing.T) {
	b, err := skills.ReadFile("skills/orchestrator.md")
	if err != nil {
		t.Fatalf("read embedded orchestrator.md: %v", err)
	}
	doc := string(b)

	for _, want := range []string{
		"VERIFICATION GATE",
		"go test",
		"make preflight",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("orchestrator.md worker dispatch missing verification-gate marker %q", want)
		}
	}
}

// TestSupervisorSkillsPreauthorizeUnattended guards the ce-4bc1 fix. The
// Orchestrator (and, by the retro's plural principle, ALL supervisors) ran
// over-cautious: spawned unattended in a detached session, they defaulted to
// human-in-the-loop caution and stalled asking "should I proceed? / stand down?"
// instead of acting. The fix is an explicit "no human is watching" pre-authorization
// in each supervisor's boot SKILL. This test pins that contract: every supervisor
// skill must carry the pre-authorization preamble, and the shared protocol must
// document it once for all roles. Without these tokens the mesh silently regresses
// to the stall the bead describes.
func TestSupervisorSkillsPreauthorizeUnattended(t *testing.T) {
	// Tokens common to all three per-role preambles. Loose enough to survive
	// wording tweaks, specific enough to fail if the pre-authorization is dropped.
	preauthTokens := []string{
		"PRE-AUTHORIZED",            // the core grant
		"pause to ask",              // ... the anti-pattern it forbids
		"guardrails, not by asking", // ... and why it is safe to not ask
		"unattended",                // the operating condition
	}
	// Derive the skill-file list from the supervisors slice in main.go rather
	// than hardcoding it, so the test cannot drift from production as
	// supervisors are added, removed, or renamed.
	if len(supervisors) == 0 {
		t.Fatal("no supervisors defined")
	}
	for _, s := range supervisors {
		b, err := skills.ReadFile("skills/" + s.SkillFile)
		if err != nil {
			t.Fatalf("read embedded %s: %v", s.SkillFile, err)
		}
		doc := string(b)
		for _, want := range preauthTokens {
			if !strings.Contains(doc, want) {
				t.Errorf("%s missing unattended pre-authorization token %q", s.SkillFile, want)
			}
		}
	}

	// The shared protocol carries the canonical "(ALL supervisors)" statement so
	// the principle is documented once and the per-role preambles can point to it.
	b, err := skills.ReadFile("skills/protocol.md")
	if err != nil {
		t.Fatalf("read embedded protocol.md: %v", err)
	}
	if !strings.Contains(string(b), "Unattended Operation (ALL supervisors)") {
		t.Errorf("protocol.md missing the shared \"Unattended Operation (ALL supervisors)\" section")
	}
}

// TestOrchestratorCrossChecksPeers pins ce-20p9 Fix 3: the Orchestrator tick
// must run `agm scan --cross-check` to read the ground-truth (tmux) state of peer
// supervisors and clear any that are still stuck with `agm send approve`. Without
// this, a peer blocked on a permission prompt looks `active`/fresh and is never
// unblocked — the ADR-002 gap this bead closes.
func TestOrchestratorCrossChecksPeers(t *testing.T) {
	b, err := skills.ReadFile("skills/orchestrator.md")
	if err != nil {
		t.Fatalf("read embedded orchestrator.md: %v", err)
	}
	doc := string(b)
	for _, want := range []string{
		"agm scan --cross-check",           // the ground-truth peer sweep
		"agm send approve",                 // clear anything cross-check couldn't auto-approve
		"supervisor.orch.peer_cross_check", // trail event for the sweep
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("orchestrator.md missing cross-check marker %q", want)
		}
	}
}

// TestOrchestratorApprovesBlockedWorker pins ce-20p9 Fix 4: a worker stuck in
// PERMISSION_PROMPT cannot receive `agm send msg` (it is frozen on its prompt),
// so the Level 2 response must be `agm send approve worker-<id>` — not a defer
// message that never arrives. Guard both the new behavior and the removal of the
// old (broken) nudge-by-message so it cannot silently regress.
func TestOrchestratorApprovesBlockedWorker(t *testing.T) {
	b, err := skills.ReadFile("skills/orchestrator.md")
	if err != nil {
		t.Fatalf("read embedded orchestrator.md: %v", err)
	}
	doc := string(b)
	for _, want := range []string{
		`agm send approve "worker-<bead-id>"`,        // approve the blocked worker
		"supervisor.orch.worker_permission_approved", // trail event for the approve
		"cannot receive", // the reason messaging fails
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("orchestrator.md missing blocked-worker-approve marker %q", want)
		}
	}
	// The old broken behavior — messaging a permission-blocked worker to defer —
	// must be gone; that message can never reach a frozen session.
	if strings.Contains(doc, "supervisor.orch.worker_permission_nudge") {
		t.Errorf("orchestrator.md still records worker_permission_nudge; a blocked worker can't receive agm send msg (ce-20p9 Fix 4)")
	}
}

// TestProtocolDocumentsPermissionModel pins ce-20p9 Fix 5 & Fix 6: the shared
// protocol must document how supervisors stay unblocked — the pre-approved RBAC
// profile + auto mode (so prompts are rare), mutual `agm send approve` (so the
// rare ones clear), and the supervisor-independent watchdog that survives a
// total-mesh stall (the gap watch-stalled's alert-only recovery cannot cover).
func TestProtocolDocumentsPermissionModel(t *testing.T) {
	b, err := skills.ReadFile("skills/protocol.md")
	if err != nil {
		t.Fatalf("read embedded protocol.md: %v", err)
	}
	doc := string(b)
	for _, want := range []string{
		"Permission Model & Mutual Unblock (ALL supervisors)", // the section
		"--mode=auto",                         // auto mode: no interactive waits
		"agm send approve",                    // mutual unblock channel
		"install-supervisor-unblock-schedule", // the external watchdog (Fix 6)
		"watch-stalled",                       // ... and why it is not enough alone
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("protocol.md missing permission-model marker %q", want)
		}
	}
}

// --- Dispatch Advisor tests (ce-hn8n) ---

func TestRestartTracker_BackoffProgression(t *testing.T) {
	rt := newRestartTracker()
	name := "test-supervisor"

	// First attempt: should be allowed immediately.
	ok, count := rt.shouldRestart(name)
	if !ok || count != 0 {
		t.Fatalf("first attempt: shouldRestart = (%v, %d), want (true, 0)", ok, count)
	}

	rt.recordAttempt(name)

	// Immediately after: backoff should block.
	ok, count = rt.shouldRestart(name)
	if ok {
		t.Fatalf("should be blocked by backoff after first attempt, count=%d", count)
	}
	if count != 1 {
		t.Fatalf("count after first attempt = %d, want 1", count)
	}

	// Verify backoff doubles: initial 30s → 60s → 120s.
	rt.mu.Lock()
	bo := rt.backoff[name]
	rt.mu.Unlock()
	if bo != initialBackoff {
		t.Fatalf("backoff after first attempt = %v, want %v", bo, initialBackoff)
	}

	// Simulate time passing and record second attempt.
	rt.mu.Lock()
	rt.lastTry[name] = time.Now().Add(-initialBackoff - time.Second)
	rt.mu.Unlock()

	ok, count = rt.shouldRestart(name)
	if !ok || count != 1 {
		t.Fatalf("second attempt: shouldRestart = (%v, %d), want (true, 1)", ok, count)
	}

	rt.recordAttempt(name)
	rt.mu.Lock()
	bo = rt.backoff[name]
	rt.mu.Unlock()
	if bo != 2*initialBackoff {
		t.Fatalf("backoff after second attempt = %v, want %v", bo, 2*initialBackoff)
	}
}

func TestRestartTracker_MaxRestarts(t *testing.T) {
	rt := newRestartTracker()
	name := "test-supervisor"

	for i := range maxRestarts {
		rt.mu.Lock()
		rt.lastTry[name] = time.Time{} // clear backoff window
		rt.mu.Unlock()

		ok, count := rt.shouldRestart(name)
		if !ok {
			t.Fatalf("attempt %d: shouldRestart = false (count=%d), want true", i+1, count)
		}
		rt.recordAttempt(name)
	}

	// After maxRestarts, should be blocked regardless of time.
	rt.mu.Lock()
	rt.lastTry[name] = time.Time{}
	rt.mu.Unlock()

	ok, count := rt.shouldRestart(name)
	if ok {
		t.Fatalf("after %d restarts: shouldRestart = true, want false", maxRestarts)
	}
	if count != maxRestarts {
		t.Fatalf("count = %d, want %d", count, maxRestarts)
	}
}

func TestRestartTracker_RecoveryResets(t *testing.T) {
	rt := newRestartTracker()
	name := "test-supervisor"

	rt.recordAttempt(name)
	rt.recordAttempt(name)
	if rt.consecutiveRestarts(name) != 2 {
		t.Fatalf("restarts = %d, want 2", rt.consecutiveRestarts(name))
	}

	rt.recordRecovery(name)
	if rt.consecutiveRestarts(name) != 0 {
		t.Fatalf("restarts after recovery = %d, want 0", rt.consecutiveRestarts(name))
	}

	// Should be able to restart again after recovery.
	ok, _ := rt.shouldRestart(name)
	if !ok {
		t.Fatal("shouldRestart after recovery = false, want true")
	}
}

func TestRestartTracker_ShouldEscalate(t *testing.T) {
	rt := newRestartTracker()
	name := "test-supervisor"

	// Below maxRestarts: shouldEscalate must return false.
	for range maxRestarts - 1 {
		rt.recordAttempt(name)
	}
	if rt.shouldEscalate(name) {
		t.Fatal("shouldEscalate returned true before maxRestarts reached")
	}

	// Reach maxRestarts: first call must return true.
	rt.recordAttempt(name)
	if !rt.shouldEscalate(name) {
		t.Fatal("shouldEscalate returned false at maxRestarts, want true")
	}

	// Subsequent calls must return false (no spam).
	if rt.shouldEscalate(name) {
		t.Fatal("shouldEscalate returned true on second call, want false (no spam)")
	}

	// After recovery, escalate flag resets.
	rt.recordRecovery(name)
	rt.mu.Lock()
	rt.restarts[name] = maxRestarts
	rt.mu.Unlock()
	if !rt.shouldEscalate(name) {
		t.Fatal("shouldEscalate returned false after recovery reset, want true")
	}
}

func TestReadHeartbeatTime(t *testing.T) {
	dir := t.TempDir()
	hbDir := filepath.Join(dir, ".agm", "vroom", "heartbeat")
	os.MkdirAll(hbDir, 0o755)

	// Test bare timestamp string (what the skill files write via `date -u`).
	ts := "2026-06-17T21:32:03Z"
	os.WriteFile(filepath.Join(hbDir, "orch.json"), []byte(ts+"\n"), 0o600)
	got := readHeartbeatTime(dir, "orch")
	want, _ := time.Parse(time.RFC3339, ts)
	if !got.Equal(want) {
		t.Errorf("bare timestamp: got %v, want %v", got, want)
	}

	// Test RFC3339 with timezone offset.
	ts2 := "2026-06-17T14:32:03-07:00"
	os.WriteFile(filepath.Join(hbDir, "meta-o.json"), []byte(ts2+"\n"), 0o600)
	got2 := readHeartbeatTime(dir, "meta-o")
	want2, _ := time.Parse(time.RFC3339, ts2)
	if !got2.Equal(want2) {
		t.Errorf("RFC3339 offset: got %v, want %v", got2, want2)
	}

	// Test missing file returns zero.
	got3 := readHeartbeatTime(dir, "nonexistent")
	if !got3.IsZero() {
		t.Errorf("missing file: got %v, want zero", got3)
	}

	// Test JSON object format.
	jsonHB := `{"timestamp":"2026-06-17T21:00:00Z"}`
	os.WriteFile(filepath.Join(hbDir, "overseer.json"), []byte(jsonHB), 0o600)
	got4 := readHeartbeatTime(dir, "overseer")
	want4, _ := time.Parse(time.RFC3339, "2026-06-17T21:00:00Z")
	if !got4.Equal(want4) {
		t.Errorf("JSON object: got %v, want %v", got4, want4)
	}
}

func TestHeartbeatFileName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"vroom-meta-orchestrator", "meta-o"},
		{"meta-orchestrator", "meta-o"},
		{"meta-o", "meta-o"},
		{"vroom-orchestrator", "orch"},
		{"orchestrator", "orch"},
		{"orch", "orch"},
		{"vroom-overseer", "overseer"},
		{"unknown", "unknown"},
	}
	for _, tc := range cases {
		got := heartbeatFileName(tc.name)
		if got != tc.want {
			t.Errorf("heartbeatFileName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestWriteTrail(t *testing.T) {
	dir := t.TempDir()
	trailDir := filepath.Join(dir, ".agm", "vroom")
	os.MkdirAll(trailDir, 0o755)

	writeTrail(dir, "dispatch.test", map[string]any{"key": "value"})
	writeTrail(dir, "dispatch.test2", nil)

	path := filepath.Join(trailDir, "dispatch-trail.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trail: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("trail lines = %d, want 2", len(lines))
	}

	if !strings.Contains(lines[0], `"dispatch.test"`) {
		t.Errorf("line 0 missing kind: %s", lines[0])
	}
	if !strings.Contains(lines[0], `"dispatch-advisor"`) {
		t.Errorf("line 0 missing role: %s", lines[0])
	}
	if !strings.Contains(lines[0], `"key":"value"`) {
		t.Errorf("line 0 missing payload: %s", lines[0])
	}
}

func TestWriteSelfHeartbeat(t *testing.T) {
	dir := t.TempDir()
	hbDir := filepath.Join(dir, ".agm", "vroom", "heartbeat")
	os.MkdirAll(hbDir, 0o755)

	writeSelfHeartbeat(dir)

	path := filepath.Join(hbDir, "dispatch.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read self heartbeat: %v", err)
	}

	ts := strings.TrimSpace(string(data))
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("self heartbeat not valid RFC3339: %q — %v", ts, err)
	}
}

func TestHealthCheckInterval(t *testing.T) {
	if healthCheckInterval != 30*time.Second {
		t.Errorf("healthCheckInterval = %v, want 30s", healthCheckInterval)
	}
}

// --- Human escalation channel tests (ce-mcw2) ---

// withEscalationStubs swaps the desktopNotify and mcpPush seams for the
// duration of a test and restores them afterwards, so the escalation path can
// be exercised without spawning osascript or an agm session.
func withEscalationStubs(t *testing.T, desktop func(string) error, push func(string, string) (bool, error)) {
	t.Helper()
	origDesktop, origPush := desktopNotify, mcpPush
	desktopNotify, mcpPush = desktop, push
	t.Cleanup(func() { desktopNotify, mcpPush = origDesktop, origPush })
}

func readTrail(t *testing.T, home string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".agm", "vroom", "dispatch-trail.jsonl"))
	if err != nil {
		t.Fatalf("read trail: %v", err)
	}
	return string(data)
}

// TestEscalateToHuman_FiresBothChannels covers the AC-5.5 escalation trigger
// path: a restart-exhausted escalation must reach both the desktop (AC-5.1) and
// MCP (AC-5.2) channels, with the human-readable message, and must append a
// structured escalation record carrying kind, trigger, message, and the extra
// fields (AC-5.4).
func TestEscalateToHuman_FiresBothChannels(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agm", "vroom"), 0o755)

	var gotDesktop, gotPush string
	withEscalationStubs(t,
		func(msg string) error { gotDesktop = msg; return nil },
		func(_ /*home*/, msg string) (bool, error) { gotPush = msg; return true, nil },
	)

	const msg = "vroom-orchestrator: 3 consecutive restart failures — needs human intervention"
	escalateToHuman(dir, "restart_exhausted", msg, map[string]any{
		"supervisor": "vroom-orchestrator",
		"restarts":   3,
	})

	if gotDesktop != msg {
		t.Errorf("desktop channel got %q, want %q", gotDesktop, msg)
	}
	if gotPush != msg {
		t.Errorf("mcp channel got %q, want %q", gotPush, msg)
	}

	trail := readTrail(t, dir)
	for _, want := range []string{
		`"kind":"dispatch.escalation"`,
		`"trigger":"restart_exhausted"`,
		`"message":"` + msg + `"`,
		`"supervisor":"vroom-orchestrator"`,
		`"restarts":3`,
		`"ts":`,
	} {
		if !strings.Contains(trail, want) {
			t.Errorf("escalation trail missing %q\ngot: %s", want, trail)
		}
	}
}

// TestEscalateToHuman_MCPUnavailableIsNotAnError covers AC-5.6: when no session
// is available to relay the MCP push (mcpPush returns sent=false, nil), the
// escalation still succeeds via the desktop channel and the trail records the
// unavailability as a benign event — not an mcp_failed error.
func TestEscalateToHuman_MCPUnavailableIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agm", "vroom"), 0o755)

	desktopCalled := false
	withEscalationStubs(t,
		func(string) error { desktopCalled = true; return nil },
		func(string, string) (bool, error) { return false, nil }, // no active session
	)

	escalateToHuman(dir, "restart_exhausted", "boom", nil)

	if !desktopCalled {
		t.Error("desktop channel must still fire when MCP is unavailable (AC-5.6)")
	}
	trail := readTrail(t, dir)
	if !strings.Contains(trail, `"dispatch.escalation.mcp_unavailable"`) {
		t.Errorf("trail missing mcp_unavailable record\ngot: %s", trail)
	}
	if strings.Contains(trail, `"dispatch.escalation.mcp_failed"`) {
		t.Errorf("unavailable MCP must not be logged as a failure (AC-5.6)\ngot: %s", trail)
	}
}

// TestEscalateToHuman_ChannelFailuresAreLoggedNotPropagated covers AC-5.3: a
// failing notification channel must be recorded but must never stop the other
// channel or the caller.
func TestEscalateToHuman_ChannelFailuresAreLoggedNotPropagated(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agm", "vroom"), 0o755)

	pushCalled := false
	withEscalationStubs(t,
		func(string) error { return os.ErrPermission }, // desktop fails
		func(string, string) (bool, error) { pushCalled = true; return true, nil },
	)

	escalateToHuman(dir, "restart_exhausted", "boom", nil)

	if !pushCalled {
		t.Error("a desktop failure must not prevent the MCP channel from firing (AC-5.3)")
	}
	trail := readTrail(t, dir)
	if !strings.Contains(trail, `"dispatch.escalation.desktop_failed"`) {
		t.Errorf("desktop failure not recorded in trail\ngot: %s", trail)
	}
}

// TestOsascriptArgs pins the AC-5.1 notification shape: the script must use
// `display notification ... with title ...` with the VROOM escalation prefix
// and the Dispatch Advisor title.
func TestOsascriptArgs(t *testing.T) {
	args := osascriptArgs("orchestrator down")
	if len(args) != 2 || args[0] != "-e" {
		t.Fatalf("osascriptArgs = %v, want [-e <script>]", args)
	}
	script := args[1]
	for _, want := range []string{
		`display notification "VROOM escalation: orchestrator down"`,
		`with title "VROOM Dispatch Advisor"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("osascript script missing %q\ngot: %s", want, script)
		}
	}
}

// TestAppleScriptString guards the escaping that keeps a crafted escalation
// message from breaking out of the single-line `-e` AppleScript literal.
func TestAppleScriptString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`plain`, `"plain"`},
		{`say "hi"`, `"say \"hi\""`},
		{`back\slash`, `"back\\slash"`},
		{"new\nline", `"new line"`},
		{"tab\tsep", `"tab sep"`},
	}
	for _, tc := range cases {
		if got := appleScriptString(tc.in); got != tc.want {
			t.Errorf("appleScriptString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// A NUL/control char must be dropped, not emitted into the script.
	if got := appleScriptString("a\x00b"); got != `"ab"` {
		t.Errorf("control char not stripped: got %q", got)
	}
}

func TestSupervisorHealthString(t *testing.T) {
	if healthAlive.String() != "alive" {
		t.Errorf("alive.String() = %q", healthAlive.String())
	}
	if healthStale.String() != "stale" {
		t.Errorf("stale.String() = %q", healthStale.String())
	}
	if healthDead.String() != "dead" {
		t.Errorf("dead.String() = %q", healthDead.String())
	}
}

func TestCheckFlowLiveness(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".agm", "vroom"), 0o755)

	// Stub countReadyBeadsFunc to return 5 ready beads
	origCountReadyBeads := countReadyBeadsFunc
	countReadyBeadsFunc = func(context.Context, string) (int, error) {
		return 5, nil
	}
	defer func() { countReadyBeadsFunc = origCountReadyBeads }()

	var gotEscalationMsg string
	withEscalationStubs(t,
		func(msg string) error { gotEscalationMsg = msg; return nil },
		func(_ /*home*/, msg string) (bool, error) { return true, nil },
	)

	var stallStartTime time.Time
	var escalated bool

	// 1. Initial check with active workers > 0: should not set stall start time
	_, err := checkFlowLiveness(context.Background(), dir, 2, &stallStartTime, &escalated, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stallStartTime.IsZero() {
		t.Error("stallStartTime should be zero when activeWorkers > 0")
	}

	// 2. First check with active workers == 0: should set stall start time but not escalate
	_, err = checkFlowLiveness(context.Background(), dir, 0, &stallStartTime, &escalated, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stallStartTime.IsZero() {
		t.Error("stallStartTime should be set when ready_beads > 0 && activeWorkers == 0")
	}
	if escalated {
		t.Error("should not escalate immediately")
	}
	firstStallTime := stallStartTime

	// 3. Check again shortly after (within threshold): should not escalate
	_, err = checkFlowLiveness(context.Background(), dir, 0, &stallStartTime, &escalated, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stallStartTime != firstStallTime {
		t.Error("stallStartTime should not change on consecutive checks")
	}
	if escalated {
		t.Error("should not escalate before threshold duration")
	}

	// 4. Simulate time passing past threshold: should escalate
	stallStartTime = time.Now().Add(-6 * time.Minute)
	triggered, err := checkFlowLiveness(context.Background(), dir, 0, &stallStartTime, &escalated, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !triggered {
		t.Error("expected checkFlowLiveness to return triggered=true")
	}
	if !escalated {
		t.Error("expected escalated to be true after escalation")
	}
	if gotEscalationMsg == "" {
		t.Error("expected escalation notification to fire")
	}

	// 5. Next check (still stalled): should not escalate again (no spam)
	gotEscalationMsg = ""
	triggered, err = checkFlowLiveness(context.Background(), dir, 0, &stallStartTime, &escalated, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if triggered {
		t.Error("expected checkFlowLiveness to return triggered=false (no spam)")
	}
	if gotEscalationMsg != "" {
		t.Error("should not trigger another escalation notification (no spam)")
	}

	// 6. Active workers return: should reset state
	_, err = checkFlowLiveness(context.Background(), dir, 1, &stallStartTime, &escalated, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stallStartTime.IsZero() {
		t.Error("stallStartTime should be reset to zero")
	}
	if escalated {
		t.Error("escalated flag should be reset to false")
	}
}

func TestCheckFlowLivenessPropagatesCancellation(t *testing.T) {
	origCountReadyBeads := countReadyBeadsFunc
	countReadyBeadsFunc = func(ctx context.Context, _ string) (int, error) {
		return 0, ctx.Err()
	}
	defer func() { countReadyBeadsFunc = origCountReadyBeads }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stallStartTime time.Time
	var escalated bool

	_, err := checkFlowLiveness(ctx, t.TempDir(), 0, &stallStartTime, &escalated, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("checkFlowLiveness() error = %v, want context.Canceled", err)
	}
}

func TestFlowProbesBoundContextAndWrapErrors(t *testing.T) {
	origFlowProbeOutput := flowProbeOutput
	defer func() { flowProbeOutput = origFlowProbeOutput }()

	t.Run("worker health cancellation", func(t *testing.T) {
		flowProbeOutput = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("worker health probe context has no deadline")
			}
			<-ctx.Done()
			return nil, errors.New("command stopped")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := monitorWorkers(ctx, t.TempDir(), newWorkerTracker())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("monitorWorkers() error = %v, want context.Canceled", err)
		}
	})

	t.Run("ready beads decode", func(t *testing.T) {
		flowProbeOutput = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("ready beads probe context has no deadline")
			}
			return []byte("not-json"), nil
		}

		_, err := defaultCountReadyBeads(context.Background(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "decode bd ready") {
			t.Fatalf("defaultCountReadyBeads() error = %v, want wrapped decode error", err)
		}
	})
}
