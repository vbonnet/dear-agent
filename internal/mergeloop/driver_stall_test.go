package mergeloop

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestNoFalseStallAfterSuccessfulRebase pins the review finding that stall
// escalation was gated on res.Merged alone.
//
// A merge is not the only way a tick makes progress. A behind PR whose rebase
// cooldown has expired is rebased by this very tick, which is real forward
// motion, but the stall branch compared only the merge counter and therefore
// escalated the PR to a human anyway. The same false page fires when the tick's
// progress was spawning a repair agent. Escalation must be judged on whether
// this tick acted, not on whether it merged.
func TestNoFalseStallAfterSuccessfulRebase(t *testing.T) {
	prs := []PR{{Number: 11, MergeStateStatus: "BEHIND", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	reb := &fakeRebaser{}
	var evs []AuditEvent
	d, _ := newTestDriver(t, prs, &Deps{
		Rebaser: reb,
		Clock:   func() time.Time { return now },
		Audit:   func(e AuditEvent) { evs = append(evs, e) },
	})
	d.StallThreshold = time.Hour
	d.RebaseCooldown = 30 * time.Minute

	// Tick one rebases and anchors the clock.
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if len(reb.calls) != 1 {
		t.Fatalf("precondition: want 1 rebase on the first tick, got %d", len(reb.calls))
	}

	// Past both the stall threshold and the rebase cooldown, so this tick is
	// stalled on entry yet rebases successfully before the stall is reported.
	evs = nil
	now = now.Add(70 * time.Minute)
	res, err := d.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if res.Rebased != 1 {
		t.Fatalf("precondition: want the second tick to rebase, got Rebased=%d", res.Rebased)
	}
	if hasAction(evs, "stall_detected") {
		t.Errorf("a tick that successfully rebased must not also escalate the PR as stalled, got %v",
			auditActions(evs))
	}
	if res.Stalled != 0 {
		t.Errorf("Stalled = %d, want 0: the tick made progress", res.Stalled)
	}
}

// TestNoSecondEscalationAfterGateRefusal pins the review finding that a green
// PR refused by the blocking-findings gate was escalated twice on one tick.
//
// doMerge records the refusal and increments Escalated with the useful detail
// (the thread ID and excerpt, or the provider error). The stall branch then saw
// no merge/rebase/spawn progress and recorded a SECOND escalation, emitting two
// human-escalation metrics for one PR and overwriting that specific reason with
// a generic stall string. An action that already escalated has said why.
func TestNoSecondEscalationAfterGateRefusal(t *testing.T) {
	prs := []PR{{Number: 21, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	var evs []AuditEvent
	d, tr := newTestDriver(t, prs, &Deps{
		Merger: &fakeMerger{},
		// Refuse every merge at the independent gate, the way a real blocking
		// finding does, using the seam the gate already reads.
		Threads: &fakeThreadResolver{blocking: []BlockingFinding{{
			Author: "chatgpt-codex-connector", Severity: SeverityBlocking,
			ThreadID: "PRRT_example", Excerpt: "a blocking finding",
		}}},
		Clock: func() time.Time { return now },
		Audit: func(e AuditEvent) { evs = append(evs, e) },
	})
	d.StallThreshold = time.Hour

	var res TickResult
	for range 6 {
		r, err := d.Tick(context.Background())
		if err != nil {
			t.Fatalf("tick: %v", err)
		}
		res = r
		now = now.Add(20 * time.Minute)
	}

	if res.Escalated > 1 {
		t.Errorf("Escalated = %d on one tick, want at most 1: the gate refusal already escalated",
			res.Escalated)
	}
	if res.Stalled != 0 {
		t.Errorf("Stalled = %d, want 0: the tick already escalated with a specific reason", res.Stalled)
	}
	if rec := tr.Get(21, now); strings.HasPrefix(rec.EscalationReason, "stalled in") {
		t.Errorf("EscalationReason = %q: a generic stall reason overwrote the gate refusal detail",
			rec.EscalationReason)
	}
}

// TestPersistentGateRefusalEscalatesOnce pins the review finding that an
// unchanged blocking finding re-escalated on every daemon tick.
//
// Each tick overwrote EscalatedAt, incremented Escalated and emitted another
// human_escalation metric, so a finding that had sat untouched for hours looked
// like a brand new escalation every interval and lost the original time that
// said how long it had actually been stuck.
func TestPersistentGateRefusalEscalatesOnce(t *testing.T) {
	prs := []PR{{Number: 31, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
		Checks: []Check{reqCheck("ci", CheckPass)}}}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	d, tr := newTestDriver(t, prs, &Deps{
		Merger: &fakeMerger{},
		Threads: &fakeThreadResolver{blocking: []BlockingFinding{{
			Author: "chatgpt-codex-connector", Severity: SeverityBlocking,
			ThreadID: "PRRT_same", Excerpt: "the same unchanged finding",
		}}},
		Clock: func() time.Time { return now },
		Audit: func(AuditEvent) {},
	})

	firstTick := now
	total := 0
	for range 4 {
		r, err := d.Tick(context.Background())
		if err != nil {
			t.Fatalf("tick: %v", err)
		}
		total += r.Escalated
		now = now.Add(10 * time.Minute)
	}

	if total != 1 {
		t.Errorf("escalations across 4 ticks = %d, want 1: the finding never changed", total)
	}
	if at := tr.Get(31, now).EscalatedAt; !at.Equal(firstTick) {
		t.Errorf("EscalatedAt = %v, want the first escalation %v: the original time is the useful one",
			at, firstTick)
	}
}
