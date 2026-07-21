package main

import "testing"

func TestParseOutcome(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Outcome
	}{
		{"approved plain", "approved", Approved},
		{"approved sentence", "Approved — no blocking findings.", Approved},
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
		{"resolve down human beats approved", "approved but needs-human-review for the deploy", NeedsHumanReview},
		{"resolve down rejected beats approved", "not approved, rejected", Rejected},
		{"resolve down needs-work beats approved", "mostly approved but needs-work", NeedsWork},
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
