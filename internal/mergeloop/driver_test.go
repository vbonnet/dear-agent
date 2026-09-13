package mergeloop

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- fakes ---

type fakeLister struct{ prs []PR }

func (f *fakeLister) ListOpen(_ context.Context, _ string, _ int) ([]PR, error) {
	return f.prs, nil
}

type fakeRebaser struct{ calls []int }

func (f *fakeRebaser) Rebase(_ context.Context, _ string, pr int) error {
	f.calls = append(f.calls, pr)
	return nil
}

type fakeMerger struct {
	calls []int
	err   error
}

func (f *fakeMerger) Merge(_ context.Context, _ string, pr int) error {
	f.calls = append(f.calls, pr)
	return f.err
}

type fakeSpawner struct {
	active   bool
	spawned  []AgentRequest
	spawnErr error
}

func (f *fakeSpawner) ActiveSession(_ context.Context, _ string, _ int) (bool, error) {
	return f.active, nil
}

func (f *fakeSpawner) Spawn(_ context.Context, req AgentRequest) (string, error) {
	if f.spawnErr != nil {
		return "", f.spawnErr
	}
	f.spawned = append(f.spawned, req)
	return req.SessionName, nil
}

type fakeThreadResolver struct {
	calls    []int
	resolved int
	withheld int
	err      error

	gateCalls []int
	blocking  []BlockingFinding
	gateErr   error
}

func (f *fakeThreadResolver) ResolveBotThreads(_ context.Context, _ string, pr int) (ThreadResolution, error) {
	f.calls = append(f.calls, pr)
	return ThreadResolution{Resolved: f.resolved, Withheld: f.withheld}, f.err
}

func (f *fakeThreadResolver) BlockingFindings(_ context.Context, _ string, pr int) ([]BlockingFinding, error) {
	f.gateCalls = append(f.gateCalls, pr)
	return f.blocking, f.gateErr
}

// auditActions flattens an audit log to its action names for assertions.
func auditActions(evs []AuditEvent) []string {
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		out = append(out, e.Action)
	}
	return out
}

func hasAction(evs []AuditEvent, action string) bool {
	for _, e := range evs {
		if e.Action == action {
			return true
		}
	}
	return false
}

func newTestDriver(t *testing.T, prs []PR, deps *Deps) (*Driver, *Tracker) {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.json")
	tr, err := LoadTracker("owner/repo", statePath)
	if err != nil {
		t.Fatalf("LoadTracker: %v", err)
	}
	clock := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	if deps.Clock == nil {
		deps.Clock = func() time.Time { return clock }
	}
	// Silence audit to avoid touching the real state dir.
	if deps.Audit == nil {
		deps.Audit = func(AuditEvent) {}
	}
	if deps.Lister == nil {
		deps.Lister = &fakeLister{prs: prs}
	}
	return &Driver{
		Repo:    "owner/repo",
		Policy:  NewPolicy(),
		Tracker: tr,
		Deps:    *deps,
		Cap:     50,
	}, tr
}

func TestTickRoutesActions(t *testing.T) {
	prs := []PR{
		{Number: 1, MergeStateStatus: "BEHIND", Mergeable: "MERGEABLE"},
		{Number: 2, Mergeable: "CONFLICTING"},
		{Number: 3, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckFail)}},
		{Number: 4, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckPass)}},
		{Number: 5, Labels: []string{"needs-security-review"}},
		{Number: 6, IsDraft: true},
	}
	reb := &fakeRebaser{}
	mer := &fakeMerger{}
	spn := &fakeSpawner{}
	d, _ := newTestDriver(t, prs, &Deps{Rebaser: reb, Merger: mer, Spawner: spn})

	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if res.OpenPRs != 6 {
		t.Errorf("OpenPRs = %d, want 6", res.OpenPRs)
	}
	if len(reb.calls) != 1 || reb.calls[0] != 1 {
		t.Errorf("rebaser calls = %v, want [1]", reb.calls)
	}
	if len(mer.calls) != 1 || mer.calls[0] != 4 {
		t.Errorf("merger calls = %v, want [4]", mer.calls)
	}
	if len(spn.spawned) != 2 {
		t.Fatalf("spawned %d agents, want 2", len(spn.spawned))
	}
	// PR 2 -> resolve conflict, PR 3 -> fix CI.
	kinds := map[int]AgentKind{}
	for _, r := range spn.spawned {
		kinds[r.PRNumber] = r.Kind
	}
	if kinds[2] != AgentResolveConflict {
		t.Errorf("PR2 kind = %s, want %s", kinds[2], AgentResolveConflict)
	}
	if kinds[3] != AgentFixCI {
		t.Errorf("PR3 kind = %s, want %s", kinds[3], AgentFixCI)
	}
	if res.Escalated != 1 { // PR5 policy block
		t.Errorf("Escalated = %d, want 1", res.Escalated)
	}
	if res.Merged != 1 || res.Rebased != 1 || res.AgentsSpawn != 2 {
		t.Errorf("counts merged=%d rebased=%d spawn=%d", res.Merged, res.Rebased, res.AgentsSpawn)
	}
}

func TestAgentInFlightNotRespawned(t *testing.T) {
	prs := []PR{{Number: 3, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckFail)}}}
	spn := &fakeSpawner{active: true}
	d, _ := newTestDriver(t, prs, &Deps{Spawner: spn})
	res, _ := d.Tick(context.Background())
	if len(spn.spawned) != 0 {
		t.Errorf("spawned %d agents, want 0 (already active)", len(spn.spawned))
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", res.Skipped)
	}
}

func TestAttemptCapLeadsToAbandon(t *testing.T) {
	failing := PR{Number: 7, HeadRefName: "f", MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("Build & Test", CheckFail)}}
	spn := &fakeSpawner{}
	d, tr := newTestDriver(t, []PR{failing}, &Deps{Spawner: spn})

	// Two ticks spawn two agents (attempts 1, 2).
	for i := range 2 {
		if _, err := d.Tick(context.Background()); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}
	if len(spn.spawned) != 2 {
		t.Fatalf("after 2 ticks spawned %d, want 2", len(spn.spawned))
	}
	if got := tr.Attempts(7); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	// Third tick: attempts == max → abandon (escalate), no new spawn.
	res, _ := d.Tick(context.Background())
	if len(spn.spawned) != 2 {
		t.Errorf("third tick spawned again: %d, want still 2", len(spn.spawned))
	}
	if res.Escalated != 1 {
		t.Errorf("Escalated = %d, want 1", res.Escalated)
	}
}

func TestProjectionErrorPreservesAttemptBudget(t *testing.T) {
	failing := PR{Number: 7, HeadRefName: "f", MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("Build & Test", CheckFail)}}
	lister := &fakeLister{prs: []PR{failing}}
	spn := &fakeSpawner{}
	d, tr := newTestDriver(t, nil, &Deps{Lister: lister, Spawner: spn})

	// Exhaust the two-attempt budget for one stable failing-check signature.
	for i := range 2 {
		if _, err := d.Tick(context.Background()); err != nil {
			t.Fatalf("failing tick %d: %v", i, err)
		}
	}
	if got := tr.Attempts(7); got != 2 {
		t.Fatalf("attempts before projection error = %d, want 2", got)
	}

	// A transient projection outage must not manufacture an "unknown"
	// signature and erase that history.
	lister.prs = []PR{{
		Number:               7,
		HeadRefName:          "f",
		MergeStateStatus:     "CLEAN",
		Mergeable:            "MERGEABLE",
		CheckProjectionError: "provider timeout",
	}}
	if res, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("projection-error tick: %v", err)
	} else if res.Skipped != 1 {
		t.Fatalf("projection-error skipped = %d, want 1", res.Skipped)
	}
	if got := tr.Attempts(7); got != 2 {
		t.Fatalf("attempts after projection error = %d, want 2", got)
	}

	// A normal CI rerun also passes through pending and has no failure
	// signature yet. It must preserve the same budget.
	lister.prs = []PR{{
		Number:           7,
		HeadRefName:      "f",
		MergeStateStatus: "CLEAN",
		Mergeable:        "MERGEABLE",
		Checks:           []Check{reqCheck("Build & Test", CheckPending)},
	}}
	if res, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("pending tick: %v", err)
	} else if res.Skipped != 1 {
		t.Fatalf("pending skipped = %d, want 1", res.Skipped)
	}
	if got := tr.Attempts(7); got != 2 {
		t.Fatalf("attempts after pending rerun = %d, want 2", got)
	}

	// Once projection recovers with the same failure, the exhausted budget
	// still escalates instead of spawning an unbounded third repair agent.
	lister.prs = []PR{failing}
	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("recovered tick: %v", err)
	}
	if len(spn.spawned) != 2 {
		t.Fatalf("spawn count after recovery = %d, want 2", len(spn.spawned))
	}
	if res.Escalated != 1 {
		t.Fatalf("escalated after recovery = %d, want 1", res.Escalated)
	}

	// A complete projection of a genuinely different failure still opens a
	// fresh bounded budget.
	different := failing
	different.Checks = []Check{reqCheck("Lint", CheckFail)}
	lister.prs = []PR{different}
	res, err = d.Tick(context.Background())
	if err != nil {
		t.Fatalf("different-failure tick: %v", err)
	}
	if res.AgentsSpawn != 1 || len(spn.spawned) != 3 {
		t.Fatalf("different-failure spawns = %d total %d, want 1 and 3", res.AgentsSpawn, len(spn.spawned))
	}
	if got := tr.Attempts(7); got != 1 {
		t.Fatalf("different-failure attempts = %d, want fresh budget at 1", got)
	}
}

func TestSpawnUnavailableDefersNotBlocks(t *testing.T) {
	prs := []PR{
		{Number: 8, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckFail)}},
		{Number: 9, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckPass)}},
	}
	mer := &fakeMerger{}
	spn := &fakeSpawner{spawnErr: fmt.Errorf("no token: %w", ErrSpawnUnavailable)}
	d, tr := newTestDriver(t, prs, &Deps{Merger: mer, Spawner: spn})
	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick should not fail when spawn unavailable: %v", err)
	}
	// PR8's fix is deferred (escalated), PR9 still merges — loop kept going.
	if res.Escalated != 1 {
		t.Errorf("Escalated = %d, want 1 (deferred spawn)", res.Escalated)
	}
	if len(mer.calls) != 1 || mer.calls[0] != 9 {
		t.Errorf("merger calls = %v, want [9] (loop continued past deferred PR)", mer.calls)
	}
	if rec := tr.Get(8, time.Now()); rec.EscalationReason == "" {
		t.Errorf("PR8 should have an escalation reason recorded")
	}
}

func TestMergeNotReadyIsNotFailure(t *testing.T) {
	prs := []PR{{Number: 10, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckPass)}}}
	mer := &fakeMerger{err: fmt.Errorf("soak window: %w", ErrNotReady)}
	d, _ := newTestDriver(t, prs, &Deps{Merger: mer})
	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if res.Merged != 0 {
		t.Errorf("Merged = %d, want 0 (not ready)", res.Merged)
	}
	// No escalation for a soft not-ready; it simply retries next tick.
	if res.Escalated != 0 {
		t.Errorf("Escalated = %d, want 0", res.Escalated)
	}
}

func TestBackpressureSkipsTick(t *testing.T) {
	var many []PR
	for i := range 5 {
		many = append(many, PR{Number: i, Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"})
	}
	mer := &fakeMerger{}
	d, _ := newTestDriver(t, many, &Deps{Merger: mer})
	d.Cap = 3
	res, _ := d.Tick(context.Background())
	if res.Merged != 0 || len(mer.calls) != 0 {
		t.Errorf("backpressure should skip all merges, got merged=%d calls=%v", res.Merged, mer.calls)
	}
}

func TestGreenPRResolvesBotThreadsBeforeMerge(t *testing.T) {
	prs := []PR{{Number: 20, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	mer := &fakeMerger{}
	thr := &fakeThreadResolver{resolved: 2}
	d, _ := newTestDriver(t, prs, &Deps{Merger: mer, Threads: thr})
	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(thr.calls) != 1 || thr.calls[0] != 20 {
		t.Errorf("thread resolver calls = %v, want [20]", thr.calls)
	}
	if len(mer.calls) != 1 || mer.calls[0] != 20 {
		t.Errorf("merger calls = %v, want [20] (merge should still proceed)", mer.calls)
	}
	if res.Merged != 1 {
		t.Errorf("Merged = %d, want 1", res.Merged)
	}
}

func TestNilThreadResolverIsSafe(t *testing.T) {
	prs := []PR{{Number: 21, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	mer := &fakeMerger{}
	d, _ := newTestDriver(t, prs, &Deps{Merger: mer})
	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if res.Merged != 1 {
		t.Errorf("Merged = %d, want 1 (nil thread resolver should not block)", res.Merged)
	}
}

func TestThreadResolverErrorDoesNotBlockMerge(t *testing.T) {
	prs := []PR{{Number: 22, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	mer := &fakeMerger{}
	thr := &fakeThreadResolver{err: fmt.Errorf("GraphQL error")}
	var audited []AuditEvent
	d, _ := newTestDriver(t, prs, &Deps{
		Merger:  mer,
		Threads: thr,
		Audit:   func(ev AuditEvent) { audited = append(audited, ev) },
	})
	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if res.Merged != 1 {
		t.Errorf("Merged = %d, want 1 (thread error should not block merge)", res.Merged)
	}
	foundError := false
	for _, ev := range audited {
		if ev.Action == "thread_resolve_error" {
			foundError = true
		}
	}
	if !foundError {
		t.Error("expected thread_resolve_error audit event")
	}
}

// ---- ce-lr7j: the independent at-merge-time review-thread gate ----
//
// The severity classifier decides what the loop may auto-resolve. This gate
// decides what the loop may merge, and it deliberately does NOT trust the
// classifier's earlier verdict: it re-queries the PR at merge time. A
// classifier that mis-reads a future badge format still cannot land a blocking
// finding, because the merge itself is refused here.

func TestMergeRefusedWhenBlockingFindingPresent(t *testing.T) {
	prs := []PR{{Number: 30, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	mer := &fakeMerger{}
	thr := &fakeThreadResolver{
		blocking: []BlockingFinding{{
			ThreadID: "PRRT_x", Author: "chatgpt-codex-connector",
			Severity: SeverityBlocking, Excerpt: "Require delivery evidence before closing merged work",
		}},
	}
	var audited []AuditEvent
	d, _ := newTestDriver(t, prs, &Deps{Merger: mer, Threads: thr,
		Audit: func(ev AuditEvent) { audited = append(audited, ev) }})

	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(mer.calls) != 0 {
		t.Errorf("merger was called %v, want no merge over a blocking finding", mer.calls)
	}
	if res.Merged != 0 {
		t.Errorf("Merged = %d, want 0", res.Merged)
	}
	if !hasAction(audited, "merge_blocked_findings") {
		t.Errorf("audit actions = %v, want merge_blocked_findings", auditActions(audited))
	}
}

func TestMergeRefusedWhenGateErrors(t *testing.T) {
	// Fail closed. An unreachable gate must never be read as "no findings".
	prs := []PR{{Number: 31, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	mer := &fakeMerger{}
	thr := &fakeThreadResolver{gateErr: fmt.Errorf("GraphQL 502")}
	var audited []AuditEvent
	d, _ := newTestDriver(t, prs, &Deps{Merger: mer, Threads: thr,
		Audit: func(ev AuditEvent) { audited = append(audited, ev) }})

	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(mer.calls) != 0 {
		t.Errorf("merger was called %v, want no merge when the gate cannot be evaluated", mer.calls)
	}
	if res.Merged != 0 {
		t.Errorf("Merged = %d, want 0", res.Merged)
	}
	if !hasAction(audited, "thread_gate_error") {
		t.Errorf("audit actions = %v, want thread_gate_error", auditActions(audited))
	}
}

func TestMergeProceedsWhenGateClean(t *testing.T) {
	prs := []PR{{Number: 32, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	mer := &fakeMerger{}
	thr := &fakeThreadResolver{resolved: 2}
	d, _ := newTestDriver(t, prs, &Deps{Merger: mer, Threads: thr})

	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(thr.gateCalls) != 1 || thr.gateCalls[0] != 32 {
		t.Errorf("gate calls = %v, want [32]: the gate must run on every merge", thr.gateCalls)
	}
	if res.Merged != 1 {
		t.Errorf("Merged = %d, want 1", res.Merged)
	}
}

func TestWithheldThreadsAreAudited(t *testing.T) {
	// DoD item 6: a distinct audit event when a thread is withheld from
	// auto-resolve on severity grounds, so the withholding is observable.
	prs := []PR{{Number: 33, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	thr := &fakeThreadResolver{resolved: 1, withheld: 3}
	var audited []AuditEvent
	d, _ := newTestDriver(t, prs, &Deps{Merger: &fakeMerger{}, Threads: thr,
		Audit: func(ev AuditEvent) { audited = append(audited, ev) }})

	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if !hasAction(audited, "bot_threads_withheld") {
		t.Errorf("audit actions = %v, want bot_threads_withheld", auditActions(audited))
	}
	if !hasAction(audited, "bot_threads_resolved") {
		t.Errorf("audit actions = %v, want bot_threads_resolved too", auditActions(audited))
	}
}

func TestGateRunsEvenWhenResolverErrors(t *testing.T) {
	// A resolver error leaves threads unresolved, which GitHub's own gate
	// blocks, so it does not itself stop the merge attempt. The independent
	// gate must still run: the resolver failing is not licence to skip it.
	prs := []PR{{Number: 34, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	mer := &fakeMerger{}
	thr := &fakeThreadResolver{err: fmt.Errorf("GraphQL error")}
	d, _ := newTestDriver(t, prs, &Deps{Merger: mer, Threads: thr})

	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(thr.gateCalls) != 1 {
		t.Errorf("gate calls = %v, want the gate to run despite the resolver error", thr.gateCalls)
	}
}

// A PR that is BEHIND must not be rebased again until the CI run the previous
// rebase started has had time to finish. Rebasing sooner discards that run and
// restarts the wait, which is how the loop can tick forever and merge nothing.
func TestRebaseCooldownLetsCISettle(t *testing.T) {
	prs := []PR{{Number: 1, MergeStateStatus: "BEHIND", Mergeable: "MERGEABLE"}}
	reb := &fakeRebaser{}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	deps := &Deps{Rebaser: reb, Clock: func() time.Time { return now }}
	d, _ := newTestDriver(t, prs, deps)
	d.RebaseCooldown = 30 * time.Minute

	mustTick := func(label string) TickResult {
		t.Helper()
		res, err := d.Tick(context.Background())
		if err != nil {
			t.Fatalf("Tick (%s): %v", label, err)
		}
		return res
	}

	if res := mustTick("first"); res.Rebased != 1 {
		t.Fatalf("first tick Rebased = %d, want 1", res.Rebased)
	}

	// A tick inside the cooldown window leaves the branch alone.
	now = now.Add(10 * time.Minute)
	res := mustTick("within cooldown")
	if res.Rebased != 0 {
		t.Errorf("tick within cooldown Rebased = %d, want 0", res.Rebased)
	}
	if res.Skipped != 1 {
		t.Errorf("tick within cooldown Skipped = %d, want 1", res.Skipped)
	}
	if len(reb.calls) != 1 {
		t.Errorf("rebaser calls = %v, want a single call from the first tick", reb.calls)
	}

	// Once the window elapses the PR is still BEHIND, so it is rebased again.
	now = now.Add(21 * time.Minute)
	if res := mustTick("after cooldown"); res.Rebased != 1 {
		t.Errorf("tick after cooldown Rebased = %d, want 1", res.Rebased)
	}
	if len(reb.calls) != 2 {
		t.Errorf("rebaser calls = %v, want two calls", reb.calls)
	}
}

// The cooldown is opt-in: a zero value preserves the rebase-every-tick default.
func TestRebaseCooldownZeroRebasesEveryTick(t *testing.T) {
	prs := []PR{{Number: 1, MergeStateStatus: "BEHIND", Mergeable: "MERGEABLE"}}
	reb := &fakeRebaser{}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	d, _ := newTestDriver(t, prs, &Deps{Rebaser: reb, Clock: func() time.Time { return now }})

	for i := range 3 {
		if _, err := d.Tick(context.Background()); err != nil {
			t.Fatalf("Tick %d: %v", i, err)
		}
		now = now.Add(time.Minute)
	}
	if len(reb.calls) != 3 {
		t.Errorf("rebaser calls = %v, want three", reb.calls)
	}
}

// Intermediate actions such as agent spawns update LastActionAt, but must not
// delay or extend the rebase cooldown calculation, which is keyed on LastRebaseAt.
func TestRebaseCooldownUnaffectedByAgentSpawn(t *testing.T) {
	prs := []PR{{Number: 1, MergeStateStatus: "BEHIND", Mergeable: "MERGEABLE"}}
	reb := &fakeRebaser{}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	deps := &Deps{Rebaser: reb, Clock: func() time.Time { return now }}
	d, _ := newTestDriver(t, prs, deps)
	d.RebaseCooldown = 30 * time.Minute

	// First tick rebases the PR and records LastRebaseAt = 12:00.
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if len(reb.calls) != 1 {
		t.Fatalf("rebaser calls = %d, want 1", len(reb.calls))
	}

	// 10 minutes later, an agent spawn occurs and updates LastActionAt.
	agentSpawnTime := now.Add(10 * time.Minute)
	d.Tracker.RecordAgentSpawn(1, "failure-sig", "session-1", agentSpawnTime)

	// 31 minutes after the original rebase (but only 21 minutes after the agent spawn),
	// the rebase cooldown has expired and another rebase is allowed.
	now = now.Add(31 * time.Minute)
	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick after cooldown: %v", err)
	}
	if res.Rebased != 1 {
		t.Errorf("Rebased = %d, want 1", res.Rebased)
	}
	if len(reb.calls) != 2 {
		t.Errorf("rebaser calls = %d, want 2", len(reb.calls))
	}
}

// TestMergeDeferredDoesNotRefreshStallClock pins the ce-lr7j review finding
// that a green PR safe-merge keeps refusing can never escalate.
//
// The shape: an allowlisted bot leaves an UNRESOLVED thread whose severity this
// code does not recognise. The resolver withholds it (correctly — an unreadable
// badge is never auto-resolved), and blockingFindingsGate deliberately lets it
// through, because while the thread is still unresolved GitHub's own
// required_review_thread_resolution is the live gate. safe-merge then refuses
// the merge as not-ready on every tick. doMerge used to call RecordAction on
// that path, refreshing LastActionAt each time, so the stall detector never
// fired: the PR sat green-but-unmergeable forever with no durable escalation
// and no telemetry, which is precisely the silence this bead exists to end.
//
// A deferral is the absence of progress. It must not count as an action.
func TestMergeDeferredDoesNotRefreshStallClock(t *testing.T) {
	prs := []PR{{Number: 7, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	mg := &fakeMerger{err: fmt.Errorf("unresolved review threads: %w", ErrNotReady)}
	thr := &fakeThreadResolver{withheld: 1}
	var evs []AuditEvent
	deps := &Deps{
		Merger: mg, Threads: thr,
		Clock: func() time.Time { return now },
		Audit: func(e AuditEvent) { evs = append(evs, e) },
	}
	d, tr := newTestDriver(t, prs, deps)
	d.StallThreshold = time.Hour

	// First tick: the merge is attempted and deferred.
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if !hasAction(evs, "merge_deferred") {
		t.Fatalf("want a merge_deferred audit event, got %v", auditActions(evs))
	}
	firstAction := tr.Get(7, now).LastActionAt

	// Keep ticking past the stall threshold. Every tick defers again.
	for range 5 {
		now = now.Add(20 * time.Minute)
		if _, err := d.Tick(context.Background()); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	if len(mg.calls) < 2 {
		t.Fatalf("merger calls = %v, want the loop to keep retrying", mg.calls)
	}

	// The stall clock must not have been pushed forward by the deferrals.
	if got := tr.Get(7, now).LastActionAt; got.After(firstAction) {
		t.Errorf("LastActionAt advanced from %s to %s: a deferred merge must not "+
			"refresh the stall clock, or the PR can never be detected as stalled",
			firstAction.Format(time.RFC3339), got.Format(time.RFC3339))
	}
	if !hasAction(evs, "stall_detected") {
		t.Errorf("want stall_detected after %s of deferrals, got %v", d.StallThreshold, auditActions(evs))
	}
}

// TestStallRecordsDurableEscalation pins the ce-lr7j review finding that
// letting the stall clock run did not, on its own, create the durable
// remediation path it was supposed to enable.
//
// TestMergeDeferredDoesNotRefreshStallClock proved the clock advances. But the
// stall branch only emitted an audit line and a metric: it never called
// RecordEscalation, so EscalationReason and EscalatedAt stayed empty and no
// human-escalation metric was emitted. A PR could therefore sit green and
// unmergeable forever with nothing durable recording why.
func TestStallRecordsDurableEscalation(t *testing.T) {
	prs := []PR{{Number: 7, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	mg := &fakeMerger{err: fmt.Errorf("unresolved review threads: %w", ErrNotReady)}
	thr := &fakeThreadResolver{withheld: 1}
	var evs []AuditEvent
	d, tr := newTestDriver(t, prs, &Deps{
		Merger: mg, Threads: thr,
		Clock: func() time.Time { return now },
		Audit: func(e AuditEvent) { evs = append(evs, e) },
	})
	d.StallThreshold = time.Hour

	for range 6 {
		if _, err := d.Tick(context.Background()); err != nil {
			t.Fatalf("tick: %v", err)
		}
		now = now.Add(20 * time.Minute)
	}

	if !hasAction(evs, "stall_detected") {
		t.Fatalf("precondition: want stall_detected, got %v", auditActions(evs))
	}
	rec := tr.Get(7, now)
	if rec.EscalatedAt.IsZero() {
		t.Error("EscalatedAt is zero: a stalled PR must record a durable escalation, " +
			"not just an audit line nobody reads")
	}
	if rec.EscalationReason == "" {
		t.Error("EscalationReason is empty: the durable record must say why the PR is stuck")
	}
}

// TestNoFalseStallAfterLongDraft pins the ce-lr7j review finding that anchoring
// the stall clock on FirstSeenAt punished PRs for time the loop was never
// allowed to act on them.
//
// A PR observed as a draft (or with CI pending) for longer than the threshold
// kept that old FirstSeenAt. The moment it turned green the fallback made
// isStalled fire immediately, even though the loop had only just been handed
// actionable work. The clock must start when the PR becomes actionable.
func TestNoFalseStallAfterLongDraft(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	draft := PR{Number: 9, IsDraft: true, MergeStateStatus: "DRAFT", Mergeable: "MERGEABLE"}
	lister := &fakeLister{prs: []PR{draft}}
	var evs []AuditEvent
	d, _ := newTestDriver(t, nil, &Deps{
		Lister: lister,
		Merger: &fakeMerger{},
		Clock:  func() time.Time { return now },
		Audit:  func(e AuditEvent) { evs = append(evs, e) },
	})
	d.StallThreshold = time.Hour

	// Sit as a draft for well over the stall threshold.
	for range 5 {
		if _, err := d.Tick(context.Background()); err != nil {
			t.Fatalf("draft tick: %v", err)
		}
		now = now.Add(time.Hour)
	}
	if hasAction(evs, "stall_detected") {
		t.Fatalf("a draft must never be reported stalled, got %v", auditActions(evs))
	}

	// It becomes green. The loop has had no chance to act on it yet, so this
	// tick must not report a stall.
	lister.prs = []PR{{Number: 9, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	evs = nil
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("green tick: %v", err)
	}
	if hasAction(evs, "stall_detected") {
		t.Errorf("stall reported on the first actionable tick after a long draft: the clock "+
			"must start when the PR becomes actionable, got %v", auditActions(evs))
	}
}

// TestNoFalseStallAfterDraftFollowingAnAction pins the ce-lr7j review finding
// that clearing ActionableSinceAt alone did not stop false stalls.
//
// stallSince prefers LastActionAt, so a PR the loop HAD acted on, which then
// sat as a draft past the threshold, was reported and durably escalated as
// stalled the instant it became actionable again, even though the new
// actionable clock had only just started. The stale action anchor must not
// outrank the fresh actionable one.
func TestNoFalseStallAfterDraftFollowingAnAction(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	behind := PR{Number: 11, MergeStateStatus: "BEHIND", Mergeable: "MERGEABLE"}
	lister := &fakeLister{prs: []PR{behind}}
	var evs []AuditEvent
	d, tr := newTestDriver(t, nil, &Deps{
		Lister: lister, Rebaser: &fakeRebaser{}, Merger: &fakeMerger{},
		Clock: func() time.Time { return now },
		Audit: func(e AuditEvent) { evs = append(evs, e) },
	})
	d.StallThreshold = time.Hour

	// The loop acts on it once, recording LastActionAt.
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if tr.Get(11, now).LastActionAt.IsZero() {
		t.Fatal("precondition: expected a recorded action from the rebase")
	}

	// It then becomes a draft and sits there well past the threshold.
	lister.prs = []PR{{Number: 11, IsDraft: true, MergeStateStatus: "DRAFT", Mergeable: "MERGEABLE"}}
	for range 4 {
		now = now.Add(time.Hour)
		if _, err := d.Tick(context.Background()); err != nil {
			t.Fatalf("draft tick: %v", err)
		}
	}

	// Back to actionable. The loop has had no chance to act since, so this must
	// not report or escalate a stall.
	lister.prs = []PR{behind}
	evs = nil
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("actionable tick: %v", err)
	}
	if hasAction(evs, "stall_detected") {
		t.Errorf("stall reported on the first actionable tick after a long draft, using the "+
			"stale pre-draft action anchor; got %v", auditActions(evs))
	}
}

// TestStallCountsTowardTickSummary pins the ce-lr7j review finding that the
// durable stall escalation was invisible in the tick summary.
//
// The stall branch persisted an escalation and emitted the escalation metric
// but never incremented res.Escalated, so printSummary reported escalated=0 for
// a tick that had durably escalated a PR. That hides the very remediation path
// the escalation was added to create from the operator reading the summary.
func TestStallCountsTowardTickSummary(t *testing.T) {
	prs := []PR{{Number: 7, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	mg := &fakeMerger{err: fmt.Errorf("unresolved review threads: %w", ErrNotReady)}
	d, _ := newTestDriver(t, prs, &Deps{
		Merger: mg, Threads: &fakeThreadResolver{withheld: 1},
		Clock: func() time.Time { return now },
	})
	d.StallThreshold = time.Hour

	var last TickResult
	for range 6 {
		res, err := d.Tick(context.Background())
		if err != nil {
			t.Fatalf("tick: %v", err)
		}
		last = res
		now = now.Add(20 * time.Minute)
	}
	if last.Stalled == 0 {
		t.Fatalf("precondition: expected a stalled PR, got %+v", last)
	}
	if last.Escalated == 0 {
		t.Error("TickResult.Escalated = 0 on a tick that durably escalated a stalled PR; " +
			"the summary must not hide the escalation it just recorded")
	}
}

// TestDryRunDoesNotPersistEscalations pins the ce-lr7j review finding that
// `mergeloop tick --dry-run` wrote durable state.
//
// This is not hypothetical. A dry-run tick against the live repository on
// 2026-09-13 persisted seven escalations to the tracker, because the refusal
// paths call RecordEscalation and Tick then saves the record. An operator who
// asked for classification only inherited escalations that never happened, and
// the real escalation metric was emitted alongside them.
//
// Dry run means observe. It may audit and it may count, but it must not write
// durable state or emit telemetry that claims a human escalation occurred.
func TestDryRunDoesNotPersistEscalations(t *testing.T) {
	prs := []PR{{Number: 3, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	thr := &fakeThreadResolver{blocking: []BlockingFinding{{
		ThreadID: "t1", Author: "chatgpt-codex-connector",
		Severity: SeverityBlocking, Excerpt: "something blocking",
	}}}
	var evs []AuditEvent
	d, tr := newTestDriver(t, prs, &Deps{
		Merger: &fakeMerger{}, Threads: thr,
		Clock: func() time.Time { return now },
		Audit: func(e AuditEvent) { evs = append(evs, e) },
	})
	d.DryRun = true

	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if got := tr.Get(3, now); !got.EscalatedAt.IsZero() || got.EscalationReason != "" {
		t.Errorf("dry run persisted an escalation: EscalatedAt=%v reason=%q",
			got.EscalatedAt, got.EscalationReason)
	}
	// The refusal must still be observable: that is the point of a dry run.
	if res.Escalated == 0 {
		t.Error("dry run should still COUNT the refusal it would have escalated")
	}
	if !hasAction(evs, "merge_blocked_findings") {
		t.Errorf("dry run should still audit the refusal, got %v", auditActions(evs))
	}
}

// TestDescribeFindingsNamesTheThread pins the ce-lr7j review finding that a
// durable escalation identified neither the finding nor its thread: the
// excerpt for an unknown-severity finding was always "(no excerpt)", and the
// description omitted the thread ID, leaving an operator with a refusal and no
// way to find what caused it.
func TestDescribeFindingsNamesTheThread(t *testing.T) {
	got := describeFindings([]BlockingFinding{{
		ThreadID: "PRRT_abc123", Author: "chatgpt-codex-connector",
		Severity: SeverityUnknown, Excerpt: "(no excerpt)",
	}})
	if !strings.Contains(got, "PRRT_abc123") {
		t.Errorf("describeFindings() = %q, want it to name the thread so the refusal is actionable", got)
	}
}
