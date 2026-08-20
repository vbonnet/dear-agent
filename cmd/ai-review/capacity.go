package main

import "strings"

// AIREV-22 requires the reviewer to prove, before it reads a credential, that
// every classifier prompt and every complete verdict for a diff fits its
// accepted wire bounds. When that proof fails, the reviewer records a
// human-review reason and stops.
//
// Such a reason is not a governance finding. The reasons the SPEC contract
// records elsewhere (ownership edge, reviewer dependency, BDD traceability,
// stale base) are verdicts the reviewer reached after reading the diff, and
// each names a remedy the contributor can act on. A bound proof says the
// opposite: no verdict was ever reachable, because the reviewer could not fit
// this diff into its own budgets. Its remedy is ours, and its byte count
// scales with the repository's SPEC tree rather than with the diff.
//
// AIREV-26 therefore treats a keyless run whose reasons are all bound proofs
// as a cannot-run disposition rather than a conclusive verdict. Classification
// is by leading text, the same idiom onlyReviewerDependencyReasons uses,
// because three of these reasons interpolate a measured byte count.
//
// Every reason recorded by a bound proof in spec_contract.go must be prefixed
// by exactly one entry here; capacity_test.go pins the current set. A bound
// proof missing from this list is not a correctness hazard, it simply keeps
// the old fail-closed behaviour.
var boundProofReasonPrefixes = []string{
	"complete changed-SPEC contract context exceeds the review limit",
	"complete active-harness applicability evidence exceeds the bounded review limit",
	"too many deleted requirements for bounded semantic review",
	"semantic owner shard cannot fit the bounded review contract",
	"minimum complete semantic owner verdict exceeds the bounded review limit",
	"maximum-value canonical semantic owner verdict exceeds the bounded review limit",
	"minimum complete SPEC verdict is ",
	"maximum-value canonical SPEC verdict is ",
}

// isBoundProofReason reports whether one human-review reason is an AIREV-22
// bounded-wire proof rather than a governance finding.
func isBoundProofReason(reason string) bool {
	for _, prefix := range boundProofReasonPrefixes {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}

// capacityOnly reports whether every recorded human-review reason is a bound
// proof. One unrecognised reason is enough to make the verdict conclusive
// governance work, so anything not classified here keeps failing closed.
func (p reviewPlan) capacityOnly() bool {
	if len(p.HumanReasons) == 0 {
		return false
	}
	for _, reason := range p.HumanReasons {
		if !isBoundProofReason(reason) {
			return false
		}
	}
	return true
}
