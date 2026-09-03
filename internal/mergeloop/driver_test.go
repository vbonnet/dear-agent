package mergeloop

import (
	"context"
	"fmt"
	"path/filepath"
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
	err      error
}

func (f *fakeThreadResolver) ResolveBotThreads(_ context.Context, _ string, pr int) (int, error) {
	f.calls = append(f.calls, pr)
	return f.resolved, f.err
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
