// Package gatehealth detects systemic merge-gate failure: the state where one
// required check fails across a large share of the open pull-request queue, so
// the blockage is a single repo-wide cause rather than ordinary per-PR churn.
//
// The distinction matters because the two states demand opposite responses. A
// handful of PRs each failing a different check is a healthy queue and paging
// on it trains responders to ignore the alarm. One check failing on 19 of 44
// PRs is an outage with a single fix, and every hour it goes unnamed is an
// hour of fleet-wide merge deadlock.
//
// The 2026-09-03 x/crypto deadlock is the motivating case. Two CVEs in
// golang.org/x/crypto failed the required govulncheck gate on every branch cut
// from main, blocking merges and, because safe-pr runs the same scan locally,
// blocking PR creation too. Main itself stayed green throughout, so the
// scheduled CI Health Monitor - which audits the last five CI runs on main -
// reported success three times across the blackout. Nothing looked at the open
// PR queue in aggregate. Nine hours passed with zero merges before a human
// noticed.
//
// This package is deliberately pure: it takes an already-collected view of the
// queue and returns a verdict. Fetching that view is the caller's job, so the
// detection rule can be tested against the shape of a real outage without a
// network.
package gatehealth

import (
	"fmt"
	"sort"
)

// Status is the verdict Detect reaches about the pull-request queue.
type Status string

const (
	// StatusHealthy means no single check dominates the failures. Individual
	// PRs may still be red; that is normal churn, not a systemic gate failure.
	StatusHealthy Status = "healthy"
	// StatusSystemic means at least one check fails across enough of the queue
	// to indicate one repo-wide cause.
	StatusSystemic Status = "systemic"
	// StatusNoQueue means nothing was evaluable. This is neither health nor an
	// outage: with no open non-draft PRs carrying check results there is no
	// evidence either way, and reporting it as healthy would let a completely
	// dead pipeline read green.
	StatusNoQueue Status = "no_queue"
)

// PullRequest is the collected view of one open pull request. FailingChecks
// carries the names of checks concluded failure or error on the head commit's
// status rollup; duplicates are tolerated and deduplicated during detection.
type PullRequest struct {
	Number int
	// FailingChecks names every check reporting failure or error.
	FailingChecks []string
	// Draft marks a pull request not currently seeking merge.
	Draft bool
	// ChecksUnknown marks a pull request whose rollup could not be read or has
	// not reported yet. Such a PR is excluded from the denominator rather than
	// counted as passing, because absence of a result is not evidence of health.
	ChecksUnknown bool
}

// CheckFailure records one check and the open pull requests it fails on.
type CheckFailure struct {
	Check string `json:"check"`
	// PRCount is the number of distinct evaluated pull requests failing Check.
	PRCount int `json:"pr_count"`
	// Fraction is PRCount divided by the evaluated pull-request count.
	Fraction float64 `json:"fraction"`
	// PRs lists the failing pull-request numbers in ascending order so a
	// responder can go straight to an example.
	PRs []int `json:"prs"`
}

// Config sets the thresholds separating a systemic gate failure from churn.
//
// Both thresholds must be met. The fraction alone would alarm on a nearly
// empty queue where two of three PRs happen to share a flake; the absolute
// count alone would alarm on a large queue where a handful of PRs share a
// legitimately failing check.
type Config struct {
	// MinFraction is the share of evaluated pull requests a check must fail on.
	MinFraction float64
	// MinPRs is the absolute number of pull requests a check must fail on.
	MinPRs int
	// ExcludeDrafts drops draft pull requests from the denominator. Off by
	// default, so drafts are counted.
	//
	// Counting drafts is deliberate and was corrected after this detector was
	// first run against live data. The instinct to exclude them is imported
	// from human review workflows, where a draft signals "not ready" and its
	// red gate is the author's own business. It is wrong for gate health. Draft
	// status describes review intent; it says nothing about whether a branch
	// cut from main inherits a broken required check. In the 2026-09-03 outage
	// 33 of the 42 open pull requests were drafts and the govulncheck failure
	// lived in them: excluding drafts dropped the sample to 9 PRs and read
	// healthy through an active fleet-wide deadlock.
	//
	// Work-in-progress noise is handled by the fraction threshold instead. One
	// check failing across a third of the queue is not what an unfinished
	// branch looks like.
	ExcludeDrafts bool
}

// DefaultConfig returns the shipped thresholds.
//
// The values are calibrated against the 2026-09-03 outage, measured live:
// govulncheck failed on 19 of 42 evaluated open pull requests (45.2%). A 0.30
// fraction leaves headroom below that measurement so a smaller version of the
// same failure still trips, while sitting well above the background rate of
// unrelated per-PR failures. MinPRs of 5 keeps a thin queue from alarming.
// TestDefaultConfigWouldHaveCaughtTheGovulncheckDeadlock pins this to the
// measurement, including the draft-counting decision, which the first
// calibration got wrong.
func DefaultConfig() Config {
	return Config{MinFraction: 0.30, MinPRs: 5}
}

// Validate rejects thresholds that would silently disable detection. A zero
// MinFraction would mark every check systemic and a fraction above one could
// never be met, and both failure modes are invisible at run time: the alarm
// simply never says anything useful. Failing loudly on load is the point.
func (c Config) Validate() error {
	if c.MinFraction <= 0 || c.MinFraction > 1 {
		return fmt.Errorf("gatehealth: MinFraction must be in (0,1], got %v", c.MinFraction)
	}
	if c.MinPRs < 1 {
		return fmt.Errorf("gatehealth: MinPRs must be at least 1, got %d", c.MinPRs)
	}
	return nil
}

// Report is the verdict, shaped for both JSON emission and a notification body.
type Report struct {
	Status Status `json:"status"`
	// EvaluatedPRs counts the pull requests that contributed to the verdict.
	EvaluatedPRs int `json:"evaluated_prs"`
	// SkippedPRs counts pull requests excluded as drafts or unknown rollups.
	SkippedPRs int `json:"skipped_prs"`
	// PRsWithFailures counts evaluated pull requests failing at least one check.
	PRsWithFailures int `json:"prs_with_failures"`
	// Systemic lists every check over threshold, ranked by PRCount descending
	// then check name ascending so repeated runs are byte-identical.
	Systemic []CheckFailure `json:"systemic,omitempty"`
	// Dominant is the highest-ranked systemic check, or nil when healthy.
	Dominant *CheckFailure `json:"dominant,omitempty"`
	// Remediation names the likely single fix for Dominant.
	Remediation string `json:"remediation,omitempty"`
	// RemediationKind classifies that fix so an automated responder can decide
	// whether it is safe to drive without a human.
	RemediationKind RemediationKind `json:"remediation_kind,omitempty"`
}

// Detect applies cfg to the queue and returns the verdict. It performs no I/O.
//
// Callers should Validate cfg first; Detect treats an invalid config as the
// default rather than panicking, so a misconfiguration degrades to the shipped
// thresholds instead of silently disabling the alarm.
func Detect(prs []PullRequest, cfg Config) Report {
	if err := cfg.Validate(); err != nil {
		cfg = DefaultConfig()
	}

	var report Report
	// failingPRs maps a check name to the set of PR numbers failing it. A set
	// rather than a counter because GitHub leaves duplicate contexts on a
	// rollup after re-runs and matrix legs, and double-counting one PR can push
	// a fraction above 1.
	failingPRs := make(map[string]map[int]struct{})

	for _, p := range prs {
		if p.ChecksUnknown || (p.Draft && cfg.ExcludeDrafts) {
			report.SkippedPRs++
			continue
		}
		report.EvaluatedPRs++
		if len(p.FailingChecks) > 0 {
			report.PRsWithFailures++
		}
		for _, check := range p.FailingChecks {
			if failingPRs[check] == nil {
				failingPRs[check] = make(map[int]struct{})
			}
			failingPRs[check][p.Number] = struct{}{}
		}
	}

	if report.EvaluatedPRs == 0 {
		report.Status = StatusNoQueue
		return report
	}

	for check, numbers := range failingPRs {
		count := len(numbers)
		fraction := float64(count) / float64(report.EvaluatedPRs)
		if count < cfg.MinPRs || fraction < cfg.MinFraction {
			continue
		}
		report.Systemic = append(report.Systemic, CheckFailure{
			Check:    check,
			PRCount:  count,
			Fraction: fraction,
			PRs:      sortedNumbers(numbers),
		})
	}

	if len(report.Systemic) == 0 {
		report.Status = StatusHealthy
		return report
	}

	// Rank by breadth, then by name so the output is stable across runs. An
	// unstable ranking would make every tick look like a new alarm.
	sort.Slice(report.Systemic, func(i, j int) bool {
		if report.Systemic[i].PRCount != report.Systemic[j].PRCount {
			return report.Systemic[i].PRCount > report.Systemic[j].PRCount
		}
		return report.Systemic[i].Check < report.Systemic[j].Check
	})

	report.Status = StatusSystemic
	report.Dominant = &report.Systemic[0]
	report.RemediationKind, report.Remediation = remediationFor(report.Dominant.Check)
	return report
}

func sortedNumbers(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}
