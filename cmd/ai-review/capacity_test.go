package main

import "testing"

// boundProofReasons pins one concrete example of every AIREV-22 bound proof
// spec_contract.go can record, in the exact wording it emits, including the
// three that interpolate a measured byte count. If a bound proof is added,
// renamed, or reworded without updating boundProofReasonPrefixes, the matching
// entry here fails rather than silently reverting that reason to fail-closed.
var boundProofReasons = []string{
	"complete changed-SPEC contract context exceeds the review limit",
	"complete active-harness applicability evidence exceeds the bounded review limit",
	"too many deleted requirements for bounded semantic review",
	"semantic owner shard cannot fit the bounded review contract",
	"minimum complete semantic owner verdict exceeds the bounded review limit",
	"maximum-value canonical semantic owner verdict exceeds the bounded review limit",
	"minimum complete SPEC verdict is 118549 bytes and exceeds the 65536-byte review limit",
	"maximum-value canonical SPEC verdict is 96756 bytes and exceeds the 65536-byte review limit",
	"maximum-value canonical SPEC verdict is 70000 unescaped bytes and exceeds the 32768-byte visible-output budget",
}

// governanceReasons are conclusive verdicts the reviewer reached. AIREV-26
// keeps every one of them fail-closed, keyless or not.
var governanceReasons = []string{
	"SPEC review enforcement owner change requires maintainer review (cmd/ai-review/SPEC.md)",
	"SPEC reviewer dependency graph change requires maintainer review (go.mod)",
	"SPEC ownership edge addition requires maintainer review (module/SPEC.owner)",
	"reviewed head does not contain the current protected base; update the branch before SPEC governance review",
	"SPEC lacks a BDD feature or explicit deterministic/no-BDD test consequence (internal/x/SPEC.md)",
	"SPEC requirement identifiers are ambiguous (internal/x/SPEC.md)",
	"SPEC deletion requires a maintainer ownership decision (internal/x/SPEC.md)",
	"SPEC content is unavailable or non-textual (internal/x/SPEC.md)",
	"SPEC diff is unavailable or binary",
}

func TestIsBoundProofReason(t *testing.T) {
	for _, reason := range boundProofReasons {
		if !isBoundProofReason(reason) {
			t.Errorf("bound proof not classified: %q", reason)
		}
	}
	for _, reason := range governanceReasons {
		if isBoundProofReason(reason) {
			t.Errorf("governance finding misclassified as a bound proof: %q", reason)
		}
	}
}

func TestCapacityOnly(t *testing.T) {
	cases := []struct {
		name string
		plan reviewPlan
		want bool
	}{
		{"no reasons is not a bound proof", reviewPlan{}, false},
		{"bound proofs only", reviewPlan{HumanReasons: boundProofReasons}, true},
		{"governance findings only", reviewPlan{HumanReasons: governanceReasons}, false},
		{
			"one governance finding among bound proofs",
			reviewPlan{HumanReasons: append(append([]string{}, boundProofReasons...), governanceReasons[0])},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.plan.capacityOnly(); got != tc.want {
				t.Fatalf("capacityOnly() = %v, want %v", got, tc.want)
			}
		})
	}
}
