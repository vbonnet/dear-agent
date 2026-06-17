package main

import (
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
