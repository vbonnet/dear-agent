package mergeloop

import "regexp"

// ThreadSeverity is the merge-relevance verdict for one review comment, or for
// a thread taken as a whole.
//
// Background (ce-lr7j, P0). The mergeloop used to auto-resolve bot review
// threads by AUTHOR IDENTITY ALONE: if every comment came from an allowlisted
// bot, the thread was resolved. dear-agent's branch-protection ruleset (id
// 18061003, bypass_actors=[]) sets required_review_thread_resolution, so
// resolving a thread releases the merge gate. Between 2026-07-21 and 07-22 that
// released 30 P1 and 25 P2 findings across 21 PRs, 18 of which merged, none of
// them read by a human or an agent.
//
// The premise recorded in the old code was that bot threads are "advisory".
// That premise is false for a P1 correctness finding on a live reconciler. This
// type exists so the loop can tell the difference.
type ThreadSeverity int

const (
	// SeverityUnknown means no severity marker this code recognises was found.
	// It is the zero value on purpose: any parsing path that falls through, and
	// any future badge format neither bot has shipped yet, lands here and is
	// treated as blocking. A parser miss must fail closed.
	SeverityUnknown ThreadSeverity = iota

	// SeverityAdvisory is an explicitly recognised low-priority finding: Codex
	// P2 or below, Gemini medium or below. These may be auto-resolved.
	SeverityAdvisory

	// SeverityBlocking is an explicitly recognised correctness-class finding:
	// Codex P1 or above, Gemini high or above. These are never auto-resolved.
	SeverityBlocking
)

// String renders the severity for audit records and test failures.
func (s ThreadSeverity) String() string {
	switch s {
	case SeverityAdvisory:
		return "advisory"
	case SeverityBlocking:
		return "blocking"
	case SeverityUnknown:
		return "unknown"
	}
	return "unknown"
}

// BlocksResolution reports whether a thread at this severity must be withheld
// from auto-resolution. Only an explicitly recognised advisory marker clears
// it: both SeverityBlocking and SeverityUnknown withhold, so the default for
// anything unrecognised is to leave the thread alone and let the GitHub gate do
// its job.
func (s ThreadSeverity) BlocksResolution() bool {
	return s != SeverityAdvisory
}

// codexBadgePattern matches the Codex severity badge. Real markup, from PR #989:
//
//	**<sub><sub>![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat)</sub></sub>  Title**
//
// The shields.io host is required: an image alt of "P1 Badge" pointing anywhere
// else is not a marker this code claims to understand, and falls through to
// SeverityUnknown rather than being trusted.
var codexBadgePattern = regexp.MustCompile(`!\[P(\d+) Badge\]\(https://img\.shields\.io/badge/P\d+-`)

// geminiBadgePattern matches the Gemini Code Assist severity badge. Real markup,
// from PR #945:
//
//	![high](https://www.gstatic.com/codereviewagent/high-priority.svg)
var geminiBadgePattern = regexp.MustCompile(`!\[(critical|high|medium|low)\]\(https://www\.gstatic\.com/codereviewagent/(?:critical|high|medium|low)-priority\.svg\)`)

// ClassifyCommentSeverity reads one review comment body and returns its
// severity. A body carrying no marker this code recognises returns
// SeverityUnknown, which blocks resolution.
//
// When a body carries several markers the most severe wins, so a bot that
// quotes a P1 finding inside a P2 comment cannot downgrade it.
func ClassifyCommentSeverity(body string) ThreadSeverity {
	worst := SeverityUnknown
	seen := false

	for _, m := range codexBadgePattern.FindAllStringSubmatch(body, -1) {
		seen = true
		// P0 and P1 are correctness-class. P2 and below are advisory.
		if m[1] == "0" || m[1] == "1" {
			return SeverityBlocking
		}
		if worst != SeverityBlocking {
			worst = SeverityAdvisory
		}
	}

	for _, m := range geminiBadgePattern.FindAllStringSubmatch(body, -1) {
		seen = true
		switch m[1] {
		case "critical", "high":
			return SeverityBlocking
		case "medium", "low":
			if worst != SeverityBlocking {
				worst = SeverityAdvisory
			}
		}
	}

	if !seen {
		return SeverityUnknown
	}
	return worst
}

// ThreadSeverityOf reduces every comment body in one thread to a single verdict.
// The most severe comment wins, and blocking outranks unknown so a thread that
// mixes a recognised P1 with unparseable prose still reports as blocking rather
// than merely unknown. A thread with no comments is unknown, which withholds.
func ThreadSeverityOf(bodies []string) ThreadSeverity {
	if len(bodies) == 0 {
		return SeverityUnknown
	}
	worst := SeverityAdvisory
	for _, b := range bodies {
		switch ClassifyCommentSeverity(b) {
		case SeverityBlocking:
			return SeverityBlocking
		case SeverityUnknown:
			worst = SeverityUnknown
		case SeverityAdvisory:
			// keep looking; advisory never downgrades a prior unknown
		}
	}
	return worst
}
