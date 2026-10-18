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
	// P3, Gemini medium or below. These may be auto-resolved.
	SeverityAdvisory

	// SeverityBlocking is an explicitly recognised correctness-class finding:
	// Codex P2 or above, Gemini high or above. These are never auto-resolved.
	//
	// P2 belongs here, not in advisory. The incident above counts 25 P2
	// findings among those released unread, and Gemini's "high" maps to the
	// same tier in the parent implementation.
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
// Only the priorities Codex actually ships (P0-P3) are recognised. An
// unsupported value such as P6 is deliberately NOT matched here: it falls
// through to SeverityUnknown, which withholds. Matching `\d+` and then
// treating everything except P0/P1 as advisory would let an unrecognised
// priority auto-resolve and clear the independent gate at the same time.
// Both priorities are captured so they can be required to AGREE. Matching the
// label and the URL independently and then classifying from the label alone let
// a malformed or transitional badge such as
// `![P2 Badge](https://img.shields.io/badge/P1-orange?style=flat)` read as
// advisory while carrying a blocking marker. The resolver and the independent
// merge gate share this classifier, so a single wrong verdict cleared both.
// The trailing `[^)\s]*\)` is load-bearing: the pattern must match a COMPLETE
// Markdown badge, not merely its URL prefix. Stopping at the prefix let a
// truncated `![P2 Badge](.../badge/P2-yellow` count as parsed while the shape
// pattern, which does span to ")", swallowed it and a following complete P6
// badge as one shape. unread then equalled parsed and an unsupported marker
// rode through on a malformed one. Excluding whitespace stops the suffix from
// spanning ACROSS a following marker: a URL contains no spaces, so a match that
// would have to cross one is not a single badge.
var codexBadgePattern = regexp.MustCompile(`!\[P([0-3]) Badge\]\(https://img\.shields\.io/badge/P([0-3])-[^)\s]*\)`)

// geminiBadgePattern matches the Gemini Code Assist severity badge. Real markup,
// from PR #945:
//
//	![high](https://www.gstatic.com/codereviewagent/high-priority.svg)
//
// The label and the SVG path priority are both captured, and required to agree,
// for the same reason as the Codex badge above.
var geminiBadgePattern = regexp.MustCompile(`!\[(critical|high|medium|low)\]\(https://www\.gstatic\.com/codereviewagent/(critical|high|medium|low)-priority\.svg\)`)

// codexBadgeShape and geminiBadgeShape match the SHAPE of each bot's severity
// badge without constraining the priority to a value this code understands.
//
// They deliberately match the SYNTAX and say nothing about the destination.
// Validating the host first made an off-host badge such as
// `![P1 Badge](https://example.invalid/p1.svg)` invisible to the unread
// counter, so it could hide behind a valid badge in the same comment. A marker
// shaped like a severity badge is one this code must be able to vouch for; if
// it cannot, that is a reason to withhold, not to ignore it.
//
// They also exist because narrowing the real patterns to the supported
// priorities made the parser blind to an unsupported priority rather than
// suspicious of it.
// A comment carrying a valid P2 badge AND an unsupported P6 badge matched only
// the P2, so the comment classified as advisory and both the resolver and the
// independent gate cleared it. Counting badge-shaped markers separately lets
// the classifier notice that it failed to read one.
var (
	// Ordered alternation, so a well-formed badge matches the FIRST branch and
	// is counted exactly once. Branch one recognises the alt-text spelling
	// whatever its destination (an off-host badge is still a badge this code
	// cannot vouch for); branch two recognises a shields.io priority badge
	// whatever its alt text is spelled, which is the part that can actually be
	// validated. Keying only on the alt text let `![Priority P1](...)` go
	// uncounted and hide behind a valid P2 in the same comment.
	// The destination classes exclude "[" so a match can never run past the
	// start of a LATER marker. With a plain [^)]* an unterminated badge
	// swallowed everything up to the next badge's closing paren and counted the
	// pair as one, leaving counted == parsed so nothing looked unread. The
	// final branch then catches an unterminated badge on its own, which is what
	// makes the malformed marker countable at all. Ordered: a well-formed badge
	// still matches an earlier branch and is counted exactly once.
	codexBadgeShape = regexp.MustCompile(
		`!\[P\d+ Badge\]\([^)\[]*\)|!\[[^\]]*\]\(https://img\.shields\.io/badge/P\d+[^)\[]*\)|!\[P\d+ Badge\]\(`)
	// The suffix is deliberately tolerant: a query string or fragment after
	// ".svg" is still a priority badge, and anchoring on ")" meant
	// `high-priority.svg?v=2` was counted by neither the strict parser nor this
	// shape, so it could hide behind a badge the parser CAN read. The strict
	// parser stays narrow on purpose; the shape counter is what must be liberal.
	geminiBadgeShape = regexp.MustCompile(`!\[[^\]]*\]\([^)\[]*-priority\.svg[^)\[]*\)`)
)

// HasSeverityMarkerShape reports whether a body carries anything shaped like a
// severity badge, readable or not.
//
// It separates two cases that both classify as SeverityUnknown but mean
// opposite things. A comment with an unreadable or unsupported marker IS a
// finding this parser failed to read, and must fail closed. Routine bot prose
// with no marker at all, such as an acknowledgement after a human replies, is
// not a finding, and treating it as one refused every merge on that thread
// forever with nothing for a human to actually fix.
func HasSeverityMarkerShape(body string) bool {
	return codexBadgeShape.MatchString(body) || geminiBadgeShape.MatchString(body) ||
		codexBadgePattern.MatchString(body) || geminiBadgePattern.MatchString(body)
}

// ClassifyCommentSeverity reads one review comment body and returns its
// severity. A body carrying no marker this code recognises returns
// SeverityUnknown, which blocks resolution.
//
// When a body carries several markers the most severe wins, so a bot that
// quotes a P1 finding inside a P2 comment cannot downgrade it.
func ClassifyCommentSeverity(body string) ThreadSeverity {
	worst := SeverityUnknown
	seen := false
	// mismatched records a badge whose label and URL disagree on the priority.
	// That is not a marker this code understands, so it forces the verdict back
	// to unknown rather than being classified from either half.
	mismatched := false

	for _, m := range codexBadgePattern.FindAllStringSubmatch(body, -1) {
		if m[1] != m[2] {
			mismatched = true
			continue
		}
		seen = true
		// P0 and P1 are correctness-class. P2 and below are advisory.
		// P0, P1 and P2 are all correctness-class. P2 is NOT advisory: the
		// incident above counts 25 P2 findings released unread, so treating
		// them as auto-resolvable would leave the very failure this change
		// exists to fix intact. The parent implementation agrees, blocking on
		// `s >= severityP2` and reserving advisory for the P3-P5 badge form.
		if m[1] == "0" || m[1] == "1" || m[1] == "2" {
			return SeverityBlocking
		}
		if worst != SeverityBlocking {
			worst = SeverityAdvisory
		}
	}

	for _, m := range geminiBadgePattern.FindAllStringSubmatch(body, -1) {
		if m[1] != m[2] {
			mismatched = true
			continue
		}
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

	// Every badge-shaped marker must have been READ, not merely skipped. A
	// priority outside the supported range, or a label this code does not know,
	// still looks like a severity badge; if more of them are present than were
	// parsed, the comment carries a marker this parser cannot interpret.
	unread := len(codexBadgeShape.FindAllString(body, -1)) + len(geminiBadgeShape.FindAllString(body, -1))
	parsed := len(codexBadgePattern.FindAllString(body, -1)) + len(geminiBadgePattern.FindAllString(body, -1))

	// A recognised blocking marker has already returned above, so reaching here
	// with a mismatch or an unread badge means the strongest thing seen was
	// advisory or nothing. Withhold: an unreadable badge must never clear the
	// gate, and it must not be rescued by a readable badge sitting beside it.
	if !seen || mismatched || unread > parsed {
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
