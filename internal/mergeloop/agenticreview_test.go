package mergeloop

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/agenticreview"
)

var (
	reviewReadyAt  = time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	reviewObserved = reviewReadyAt.Add(20 * time.Minute)
	reviewLate     = reviewReadyAt.Add(6 * time.Hour)
)

func reviewPolicy() Policy {
	p := NewPolicy()
	cfg := agenticreview.Config{
		Families:        agenticreview.DefaultFamilies,
		Quorum:          2,
		VerdictTimeout:  45 * time.Minute,
		DispatchTimeout: 30 * time.Minute,
	}
	p.AgenticReview = &cfg
	return p
}

// greenPR is a pull request that clears every pre-existing gate, so any
// classification below is attributable to the agentic review gate alone.
func greenPR(observedAt time.Time, labels ...string) PR {
	applied := make(map[string]time.Time, len(labels))
	for _, l := range labels {
		applied[l] = reviewReadyAt.Add(time.Minute)
	}
	return PR{
		Number:           7,
		Mergeable:        "MERGEABLE",
		MergeStateStatus: "CLEAN",
		Labels:           labels,
		Checks:           []Check{{Name: "CI Gateway", Verdict: CheckPass, Required: true}},
		LabelAppliedAt:   applied,
		ReadyAt:          reviewReadyAt,
		ObservedAt:       observedAt,
	}
}

func familyLabels(f agenticreview.Family, phases ...agenticreview.Phase) []string {
	out := make([]string, 0, len(phases))
	for _, p := range phases {
		out = append(out, agenticreview.Label(f, p))
	}
	return out
}

func allApproved() []string {
	var out []string
	for _, f := range agenticreview.DefaultFamilies {
		out = append(out, familyLabels(f, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)...)
	}
	return out
}

func TestClassifyMergesWhenEveryFamilyApproved(t *testing.T) {
	got := reviewPolicy().Classify(greenPR(reviewObserved, allApproved()...), 0, false)
	if got.State != StateGreen {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateGreen)
	}
}

// The merge loop must refuse a merge in the window between a pull request
// going ready and its reviews resolving, even with every other required check
// green.
func TestClassifyWaitsWhenNoFamilyHasStarted(t *testing.T) {
	got := reviewPolicy().Classify(greenPR(reviewObserved), 0, false)
	if got.State != StateCIPending {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateCIPending)
	}
	if !strings.Contains(strings.ToLower(got.Reason), "agentic review") {
		t.Fatalf("reason %q should name the agentic review gate", got.Reason)
	}
}

func TestClassifyWaitsWhileAFamilyIsStillReviewing(t *testing.T) {
	labels := append(allApprovedExcept(agenticreview.FamilyCodex),
		agenticreview.Label(agenticreview.FamilyCodex, agenticreview.PhaseStarted))

	got := reviewPolicy().Classify(greenPR(reviewObserved, labels...), 0, false)
	if got.State != StateCIPending {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateCIPending)
	}
}

// A family that requested changes is a repairable finding, so the loop hands
// it to a fix agent rather than escalating it to a human.
func TestClassifySpawnsFixAgentWhenAFamilyRequestedChanges(t *testing.T) {
	labels := append(allApprovedExcept(agenticreview.FamilyCodex),
		familyLabels(agenticreview.FamilyCodex, agenticreview.PhaseStarted, agenticreview.PhaseChangesRequested)...)

	got := reviewPolicy().Classify(greenPR(reviewObserved, labels...), 0, false)
	if got.State != StateCIFailing {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateCIFailing)
	}
	if !strings.Contains(got.Reason, "codex") {
		t.Fatalf("reason %q should name the family that requested changes", got.Reason)
	}
}

// The same finding after the repair budget is spent is abandoned rather than
// respawned forever, matching how a failing required check behaves.
func TestClassifyAbandonsRequestedChangesAfterRepairBudget(t *testing.T) {
	labels := append(allApprovedExcept(agenticreview.FamilyCodex),
		familyLabels(agenticreview.FamilyCodex, agenticreview.PhaseStarted, agenticreview.PhaseChangesRequested)...)

	p := reviewPolicy()
	got := p.Classify(greenPR(reviewObserved, labels...), p.maxAttempts(), false)
	if got.State != StateAbandoned {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateAbandoned)
	}
}

// One reviewer down and two approved reaches the quorum, so the merge proceeds.
func TestClassifyMergesOnQuorumWithOneFamilyDown(t *testing.T) {
	labels := append(allApprovedExcept(agenticreview.FamilyCodex),
		familyLabels(agenticreview.FamilyCodex, agenticreview.PhaseStarted, agenticreview.PhaseError)...)

	got := reviewPolicy().Classify(greenPR(reviewObserved, labels...), 0, false)
	if got.State != StateGreen {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateGreen)
	}
}

// Too few reviewers alive to reach a quorum is an infrastructure problem, not
// something a repair agent can fix, so it escalates to a human.
func TestClassifyEscalatesWhenQuorumCannotBeReached(t *testing.T) {
	labels := familyLabels(agenticreview.FamilyClaude,
		agenticreview.PhaseStarted, agenticreview.PhaseApproved)

	got := reviewPolicy().Classify(greenPR(reviewLate, labels...), 0, false)
	if got.State != StateBlockedPolicy {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateBlockedPolicy)
	}
	if !strings.Contains(got.Reason, "quorum") {
		t.Fatalf("reason %q should explain the quorum shortfall", got.Reason)
	}
}

// Without an observation clock the gate cannot age a reviewer out, so the loop
// waits instead of merging on incomplete evidence.
func TestClassifyWaitsWithoutAnObservationClock(t *testing.T) {
	pr := greenPR(time.Time{}, allApproved()...)

	got := reviewPolicy().Classify(pr, 0, false)
	if got.State != StateCIPending {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateCIPending)
	}
}

// A policy misconfiguration blocks rather than silently disabling the gate.
func TestClassifyEscalatesUnsatisfiableReviewPolicy(t *testing.T) {
	p := reviewPolicy()
	p.AgenticReview.Quorum = 99

	got := p.Classify(greenPR(reviewObserved, allApproved()...), 0, false)
	if got.State != StateBlockedPolicy {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateBlockedPolicy)
	}
}

// An unconfigured merge loop keeps its previous behavior exactly, so adopting
// the gate is an explicit choice rather than a silent change to every caller.
func TestClassifyIgnoresReviewLabelsWhenGateIsUnconfigured(t *testing.T) {
	p := NewPolicy()
	if p.AgenticReview != nil {
		t.Fatal("NewPolicy enabled the agentic review gate by default")
	}

	got := p.Classify(greenPR(reviewObserved), 0, false)
	if got.State != StateGreen {
		t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, StateGreen)
	}
}

// The gate is evaluated after the existing checks, so a genuine human-only
// block still outranks it and reports its own reason.
func TestClassifyKeepsPolicyBlockAheadOfTheReviewGate(t *testing.T) {
	pr := greenPR(reviewObserved)
	pr.Labels = append(pr.Labels, "do-not-merge")

	got := reviewPolicy().Classify(pr, 0, false)
	if got.State != StateBlockedPolicy || !strings.Contains(got.Reason, "do-not-merge") {
		t.Fatalf("state = %s (%s), want the do-not-merge policy block", got.State, got.Reason)
	}
}

// A failing required check still outranks the review gate: there is no point
// asking a reviewer to bless a red build.
func TestClassifyKeepsFailingCIAheadOfTheReviewGate(t *testing.T) {
	pr := greenPR(reviewObserved)
	pr.Checks = []Check{{Name: "CI Gateway", Verdict: CheckFail, Required: true}}

	got := reviewPolicy().Classify(pr, 0, false)
	if got.State != StateCIFailing || !strings.Contains(got.Reason, "required CI") {
		t.Fatalf("state = %s (%s), want the required-CI failure", got.State, got.Reason)
	}
}

func allApprovedExcept(skip agenticreview.Family) []string {
	var out []string
	for _, f := range agenticreview.DefaultFamilies {
		if f == skip {
			continue
		}
		out = append(out, familyLabels(f, agenticreview.PhaseStarted, agenticreview.PhasePosted, agenticreview.PhaseApproved)...)
	}
	return out
}
