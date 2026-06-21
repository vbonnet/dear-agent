package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

// TestWorkerDispatchUsesFileHandoff pins the file-handoff discipline (ce-4tqd):
// the Orchestrator must hand a worker its task brief as a FILE artifact, not as
// inline pasted context. Inline pasting wastes tokens, leaves no auditable
// handoff record, and is lost when the worker's context compacts. The contract,
// like the spawn contract above, lives in the embedded orchestrator.md skill:
//   - write the brief to ~/.agm/vroom/prompts/<bead-id>.md BEFORE dispatch, and
//   - send the worker only a short pointer that reads that file back.
func TestWorkerDispatchUsesFileHandoff(t *testing.T) {
	b, err := skills.ReadFile("skills/orchestrator.md")
	if err != nil {
		t.Fatalf("read embedded orchestrator.md: %v", err)
	}
	doc := string(b)

	for _, want := range []string{
		"cat > ~/.agm/vroom/prompts/<bead-id>.md", // brief is written to the file before dispatch
		"cat ~/.agm/vroom/prompts/<bead-id>.md",   // the pointer message reads the brief back
		`"prompt_file"`,                           // dispatch record references the handoff artifact
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("orchestrator.md file-handoff dispatch missing %q", want)
		}
	}

	// The worker-facing send must be a POINTER, not the full brief pasted inline.
	// The old inline form opened the message with the brief's first line; assert
	// that exact paste no longer rides in an `agm send` prompt.
	if strings.Contains(doc, `--prompt "You are a worker session assigned to bead`) {
		t.Errorf("orchestrator.md still pastes the full worker brief inline; it must hand off via ~/.agm/vroom/prompts/<bead-id>.md and send only a pointer")
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
		{"vroom-orchestrator", "orch"},
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
