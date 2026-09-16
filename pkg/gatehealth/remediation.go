package gatehealth

import "strings"

// RemediationKind classifies the likely fix for a systemic check failure.
//
// The kind exists so an automated responder can decide how far to go on its
// own. A dependency bump is mechanical and safe to open unattended; an
// unclassified failure needs a human to look before anything is driven.
type RemediationKind string

const (
	// RemediationDependencyBump means a vulnerable or stale dependency must be
	// raised. Mechanical, and safe to open a PR for without a human.
	RemediationDependencyBump RemediationKind = "dependency_bump"
	// RemediationInfrastructure means the gate itself, not the code, is broken:
	// a dead service, an expired credential, a missing runner.
	RemediationInfrastructure RemediationKind = "infrastructure"
	// RemediationBaseline means a generated baseline or contract file has
	// drifted from what the gate expects and needs regenerating.
	RemediationBaseline RemediationKind = "baseline_drift"
	// RemediationInvestigate is the fallback when the check is unmapped. Still
	// actionable: it names the check and points at the evidence.
	RemediationInvestigate RemediationKind = "investigate"
)

// remediationRule maps a check-name substring to its likely fix. Matching is
// on a lowercased substring rather than an exact name because GitHub check
// names carry matrix suffixes ("govulncheck (scan)", "Build & Test
// (ubuntu-latest)") that would otherwise each need their own entry.
type remediationRule struct {
	match string
	kind  RemediationKind
	fix   string
}

// remediationRules is ordered: the first match wins, so put specific rules
// above general ones.
var remediationRules = []remediationRule{
	{
		match: "govulncheck",
		kind:  RemediationDependencyBump,
		fix: "A Go dependency carries an unpatched advisory. Run 'govulncheck -scan package ./...' " +
			"on main to read the advisory IDs, raise the named modules in go.mod, and land that bump " +
			"first: every branch cut from main inherits the failure, and safe-pr runs the same scan " +
			"locally, so this also blocks new PRs from being opened at all.",
	},
	{
		match: "vulnerability scan",
		kind:  RemediationDependencyBump,
		fix: "A lockfile carries an unpatched advisory. Run the Trivy scan against each lockfile to " +
			"find the offending package, then raise its floor in every lockfile at once. Fixing one " +
			"lockfile at a time leaves the gate red.",
	},
	{
		match: "codeql",
		kind:  RemediationInfrastructure,
		fix:   "The CodeQL analysis is failing fleet-wide. Check the workflow run log for a toolchain or quota error before touching any code.",
	},
	{
		match: "baseline",
		kind:  RemediationBaseline,
		fix:   "A generated baseline has drifted from main. Regenerate it on main and land that first, so every branch stops inheriting the mismatch.",
	},
	{
		match: "contract review",
		kind:  RemediationBaseline,
		fix:   "The SPEC contract gate is failing fleet-wide, which usually means a contract file on main no longer matches the specs it audits. Reconcile it on main first.",
	},
}

// remediationFor returns the likely fix for a check name. It never returns an
// empty string: an unmapped check still gets an actionable next step, because
// a report that names a symptom without a direction is the thing this whole
// package exists to replace.
func remediationFor(check string) (RemediationKind, string) {
	lower := strings.ToLower(check)
	for _, rule := range remediationRules {
		if strings.Contains(lower, rule.match) {
			return rule.kind, rule.fix
		}
	}
	return RemediationInvestigate, "Check " + check + " is failing across the open pull-request queue, " +
		"so the cause is on main rather than in any one branch. Open its most recent failing run log, " +
		"identify the shared cause, and fix it on main before rebasing the queue."
}
