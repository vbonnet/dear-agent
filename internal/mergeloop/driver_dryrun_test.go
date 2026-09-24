package mergeloop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Dry-run invariants live together because they are one rule, learned the hard
// way: an observational tick persists NOTHING. Guarding individual call sites
// kept missing one, so these pin the invariant rather than the call sites.

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

	// The invariant is about DURABLE state, not the in-memory tracker: a dry
	// run computes what it would do by mutating its own copy, then persists
	// none of it. Reload from disk to check what actually survived.
	reloaded, err := LoadTracker("owner/repo", tr.path)
	if err != nil {
		t.Fatalf("reload tracker: %v", err)
	}
	if got := reloaded.Get(3, now); !got.EscalatedAt.IsZero() || got.EscalationReason != "" {
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

// TestDryRunDoesNotMutateActionableClock pins the ce-lr7j review finding that
// guarding recordEscalation did not guard the OTHER tracker mutation added in
// the same work.
//
// NoteActionable was called unconditionally and Tick then persisted the
// tracker, so a dry run against an actionable PR could start its real stall
// timer, and a dry run against a draft could erase an existing one and delay a
// later escalation. A dry run must not move the clock it is only reporting on.
func TestDryRunDoesNotMutateActionableClock(t *testing.T) {
	prs := []PR{{Number: 4, MergeStateStatus: "BEHIND", Mergeable: "MERGEABLE"}}
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	d, tr := newTestDriver(t, prs, &Deps{
		Rebaser: &fakeRebaser{}, Merger: &fakeMerger{},
		Clock: func() time.Time { return now },
	})
	d.DryRun = true

	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	reloaded, err := LoadTracker("owner/repo", tr.path)
	if err != nil {
		t.Fatalf("reload tracker: %v", err)
	}
	if got := reloaded.Get(4, now).ActionableSinceAt; !got.IsZero() {
		t.Errorf("dry run persisted a stall clock: ActionableSinceAt = %v", got)
	}
}

// TestDryRunPersistsNothing is the general form of the two dry-run findings,
// and it exists because guarding individual call sites kept missing one.
//
// Guarding recordEscalation left NoteActionable writing; guarding that left
// the merge path writing. A dry-run tick against the live repository on
// 2026-09-13 still DELETED a tracker record, because the dry-run merger
// reports success and doMerge then calls RecordAction, recordMerge and Forget.
//
// The invariant is simpler than any list of call sites: a dry run persists
// nothing. The tick may mutate its in-memory tracker freely, because that is
// how it computes what it WOULD do, but none of it reaches disk.
func TestDryRunPersistsNothing(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	tr, err := LoadTracker("owner/repo", statePath)
	if err != nil {
		t.Fatalf("LoadTracker: %v", err)
	}
	// Seed a record that a dry run must not destroy: a green PR the dry-run
	// merger will report as merged, which is what triggers Forget.
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	tr.RecordAction(5, StateGreen, now)
	if err := tr.Save(); err != nil {
		t.Fatalf("seed Save: %v", err)
	}
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read seeded state: %v", err)
	}

	d := &Driver{
		Repo: "owner/repo", Policy: NewPolicy(), Tracker: tr, Cap: 50, DryRun: true,
		Deps: Deps{
			Lister: &fakeLister{prs: []PR{{Number: 5, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE",
				Checks: []Check{reqCheck("ci", CheckPass)}}}},
			Merger: &fakeMerger{}, // reports success, exactly like the dry-run merger
			Clock:  func() time.Time { return now },
			Audit:  func(AuditEvent) {},
		},
	}
	if _, err := d.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state after dry run: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("dry run changed persisted state.\nbefore: %s\nafter:  %s", before, after)
	}
}
