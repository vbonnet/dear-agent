package main

import "testing"

func TestParseOutcome(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Outcome
	}{
		{"approved plain", "approved", Approved},
		{"approved sentence is not verdict-only", "Approved — no blocking findings.", NeedsHumanReview},
		{"needs-work hyphen", "needs-work", NeedsWork},
		{"needs work space", "Needs work: missing tests", NeedsWork},
		{"rejected", "rejected: fundamental design problem", Rejected},
		{"needs-human hyphen", "needs-human-review", NeedsHumanReview},
		{"human review words", "This needs human review for the security boundary", NeedsHumanReview},
		{"empty fails closed", "", NeedsHumanReview},
		{"garbage fails closed", "synthesis unavailable", NeedsHumanReview},
		{"whitespace", "   approved  ", Approved},
		// Resolve-down: a line mentioning both approved and a blocking state
		// must resolve to the more-severe outcome (REVIEW.md §1).
		{"resolve down rejected beats approved", "not approved, rejected", Rejected},
		{"resolve down needs-work beats approved", "mostly approved but needs-work", NeedsWork},

		// A first line that is not a BARE verdict is a contract violation and
		// fails closed. This is deliberately stricter than "leading token
		// wins": position alone cannot separate "approved — no needs-work
		// findings" from "approved is not warranted; needs-work", so the
		// prompt demands the verdict alone on line 1 and anything else blocks.
		{"approved with same-line summary blocks", "approved — no rejected or needs-work findings", Rejected},
		{"approved with human-review mention blocks", "approved; no needs-human-review triggers fired", NeedsHumanReview},
		{"leading needs-work wins", "needs-work — not approved yet", NeedsWork},
		// Regression: a negated leading approval must never approve.
		{"negated leading approval", "approved is not warranted; needs-work", NeedsWork},
		{"negated leading approval bare", "approved is not warranted", NeedsHumanReview},

		// Regression guards for the fail-open substring hole: these lines all
		// CONTAIN "approve" but must never be classified as Approved.
		{"negated not approved", "not approved due to blocking findings", NeedsHumanReview},
		{"negated cannot approve", "approval cannot be given", NeedsHumanReview},
		{"negated no approval", "no approval — see findings", NeedsHumanReview},
		{"negated unable to approve", "unable to be approved", NeedsHumanReview},
		{"approve stem alone is not a token", "I would approve this", NeedsHumanReview},
		// Regression: negation outside a fixed lookback window must still fail
		// closed — approval is positional, not merely "not obviously negated".
		{"far negation prose", "This cannot currently be considered safe or approved", NeedsHumanReview},
		{"trailing approved in prose", "after review the change looks approved", NeedsHumanReview},
		{"hedged prose mentioning approval", "I am inclined to say this is approved", NeedsHumanReview},
		{"unknown token fails closed", "looks-fine", NeedsHumanReview},
		{"markdown bold approved", "**approved**", Approved},
		{"backticked approved", "`approved`", Approved},
		{"outcome prefix", "Outcome: approved", Approved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseOutcome(tt.in); got != tt.want {
				t.Errorf("ParseOutcome(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestExitFor(t *testing.T) {
	tests := []struct {
		name     string
		outcome  Outcome
		override bool
		want     int
	}{
		{"approved no override", Approved, false, 0},
		{"approved with override", Approved, true, 0},
		{"needs-work no override blocks", NeedsWork, false, 1},
		{"needs-work override passes", NeedsWork, true, 0},
		{"rejected no override blocks", Rejected, false, 1},
		{"rejected override passes", Rejected, true, 0},
		{"needs-human no override blocks", NeedsHumanReview, false, 1},
		{"needs-human override passes", NeedsHumanReview, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitFor(tt.outcome, tt.override); got != tt.want {
				t.Errorf("ExitFor(%v, override=%v) = %d, want %d", tt.outcome, tt.override, got, tt.want)
			}
		})
	}
}

// TestZeroValueFailsClosed guards the invariant that the Outcome zero value
// blocks the merge — a forgotten assignment must never approve.
func TestZeroValueFailsClosed(t *testing.T) {
	var o Outcome // zero value
	if o != NeedsHumanReview {
		t.Fatalf("zero-value Outcome = %v, want NeedsHumanReview (fail closed)", o)
	}
	if ExitFor(o, false) != 1 {
		t.Fatalf("zero-value Outcome must block merge without override")
	}
}
