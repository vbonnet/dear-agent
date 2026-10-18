package mergeloop

import "testing"

// Real Codex badge markup, copied verbatim from PR #989 thread bodies (the PR
// whose four P1 findings were auto-resolved into main on 2026-07-21).
const (
	codexP1Body = "**<sub><sub>![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat)</sub></sub>  " +
		"Require delivery evidence before closing merged work**\n\nWhen a matching PR has merged but " +
		"deployment or real-system verification is still pending or has failed, this branch immediately " +
		"closes the bead using only the merge timestamp."
	// codexP3Body is the ADVISORY example. Advisory begins at P3: P2 is a
	// correctness finding and must never auto-resolve.
	codexP3Body = "**<sub><sub>![P3 Badge](https://img.shields.io/badge/P3-blue?style=flat)</sub></sub>  " +
		"Prefer a table here**\n\nA nit about presentation, not correctness."
	codexP2Body = "**<sub><sub>![P2 Badge](https://img.shields.io/badge/P2-yellow?style=flat)</sub></sub>  " +
		"Fail closed when merged-PR reconciliation is unavailable**\n\nIf this new `gh pr list --state " +
		"merged` call transiently fails after the open-PR query succeeded, the code skips reconciliation."
)

// Real Gemini badge markup, copied verbatim from PR #945 / #1013 thread bodies.
const (
	geminiHighBody = "![high](https://www.gstatic.com/codereviewagent/high-priority.svg)\n\nThis file imports " +
		"`golang.org/x/sys/unix` and uses `unix.Statfs`, which is not available on Windows."
	geminiMediumBody = "![medium](https://www.gstatic.com/codereviewagent/medium-priority.svg)\n\nTo prevent " +
		"runtime failures during `tofu apply`, we should add a lifecycle precondition."
)

func TestClassifyCommentSeverity(t *testing.T) {
	tests := []struct {
		name string
		body string
		want ThreadSeverity
	}{
		// Codex. The P1 case is the exact finding class that bypassed
		// required_review_thread_resolution on #989.
		{"codex P1 is blocking", codexP1Body, SeverityBlocking},
		{"codex P2 is blocking", codexP2Body, SeverityBlocking},
		{"codex P0 is blocking", "![P0 Badge](https://img.shields.io/badge/P0-red?style=flat) Data loss", SeverityBlocking},
		{"codex P3 is advisory", "![P3 Badge](https://img.shields.io/badge/P3-blue?style=flat) Nit", SeverityAdvisory},

		// Gemini.
		{"gemini high is blocking", geminiHighBody, SeverityBlocking},
		{"gemini medium is advisory", geminiMediumBody, SeverityAdvisory},
		{"gemini critical is blocking", "![critical](https://www.gstatic.com/codereviewagent/critical-priority.svg) Boom", SeverityBlocking},
		{"gemini low is advisory", "![low](https://www.gstatic.com/codereviewagent/low-priority.svg) Style", SeverityAdvisory},

		// Fail closed. Every one of these must be treated as blocking: an
		// unrecognised marker is the case that turns a parser miss into a
		// silent merge, which is precisely the ce-lr7j defect.
		{"no marker at all is unknown", "Looks good to me, just a thought about naming.", SeverityUnknown},
		{"empty body is unknown", "", SeverityUnknown},
		{"unrecognised badge host is unknown", "![P1 Badge](https://example.invalid/P1.svg) hmm", SeverityUnknown},
		{"future codex format is unknown", "**Severity: P1** something new", SeverityUnknown},
		{"future gemini wording is unknown", "![urgent](https://www.gstatic.com/codereviewagent/urgent.svg)", SeverityUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyCommentSeverity(tc.body); got != tc.want {
				t.Fatalf("ClassifyCommentSeverity() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSeverityBlocksResolution pins the fail-closed rule itself: only an
// explicitly recognised advisory marker may be auto-resolved. Blocking and
// unknown both withhold.
func TestSeverityBlocksResolution(t *testing.T) {
	tests := []struct {
		sev          ThreadSeverity
		wantBlocking bool
	}{
		{SeverityAdvisory, false},
		{SeverityBlocking, true},
		{SeverityUnknown, true},
	}
	for _, tc := range tests {
		if got := tc.sev.BlocksResolution(); got != tc.wantBlocking {
			t.Fatalf("%v.BlocksResolution() = %v, want %v", tc.sev, got, tc.wantBlocking)
		}
	}
}

// TestThreadSeverity_HighestWins pins that a thread carrying several comments
// takes the most severe verdict across all of them. A P2 follow-up comment must
// not downgrade a P1 opener.
func TestThreadSeverity_HighestWins(t *testing.T) {
	tests := []struct {
		name   string
		bodies []string
		want   ThreadSeverity
	}{
		{"advisory only", []string{codexP3Body, geminiMediumBody}, SeverityAdvisory},
		{"blocking anywhere wins", []string{codexP2Body, codexP1Body}, SeverityBlocking},
		{"unknown anywhere withholds", []string{codexP3Body, "plain prose"}, SeverityUnknown},
		{"blocking outranks unknown", []string{"plain prose", codexP1Body}, SeverityBlocking},
		{"no comments is unknown", nil, SeverityUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ThreadSeverityOf(tc.bodies); got != tc.want {
				t.Fatalf("thread severity = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestClassifyCommentSeverityUnsupportedPriorityIsUnknown pins the ce-lr7j
// review finding that an unrestricted `P(\d+)` match let an unsupported
// priority such as P6 classify as advisory, which auto-resolved the thread AND
// cleared the independent merge gate with the same wrong verdict.
func TestClassifyCommentSeverityUnsupportedPriorityIsUnknown(t *testing.T) {
	for _, p := range []string{"P4", "P5", "P6", "P9", "P10"} {
		body := "**<sub><sub>![" + p + " Badge](https://img.shields.io/badge/" + p + "-orange?style=flat)</sub></sub>  Title**"
		if got := ClassifyCommentSeverity(body); got != SeverityUnknown {
			t.Errorf("ClassifyCommentSeverity(%s) = %v, want %v (unsupported priority must fail closed)", p, got, SeverityUnknown)
		}
		if !ClassifyCommentSeverity(body).BlocksResolution() {
			t.Errorf("%s must block resolution", p)
		}
	}
	// The supported range still classifies as before.
	for p, want := range map[string]ThreadSeverity{"P0": SeverityBlocking, "P1": SeverityBlocking, "P2": SeverityBlocking, "P3": SeverityAdvisory} {
		body := "**<sub><sub>![" + p + " Badge](https://img.shields.io/badge/" + p + "-orange?style=flat)</sub></sub>  Title**"
		if got := ClassifyCommentSeverity(body); got != want {
			t.Errorf("ClassifyCommentSeverity(%s) = %v, want %v", p, got, want)
		}
	}
}

// TestClassifyCommentSeverityMismatchedBadgeIsUnknown pins the ce-lr7j review
// finding that the badge regex constrained the alt-text priority and the URL
// priority independently, then classified from the alt text alone. A malformed
// or transitional badge such as `![P2 Badge](.../badge/P1-orange)` therefore
// read as advisory while carrying a blocking marker, and because the resolver
// and the independent merge gate share this classifier, BOTH protections
// cleared it with the same wrong verdict. A disagreement is not a severity this
// code understands, so it must fail closed to unknown.
func TestClassifyCommentSeverityMismatchedBadgeIsUnknown(t *testing.T) {
	mismatched := []struct{ alt, url string }{
		{"P2", "P1"}, // advisory alt hiding a blocking URL: the dangerous direction
		{"P1", "P2"},
		{"P3", "P0"},
		{"P0", "P3"},
	}
	for _, m := range mismatched {
		body := "**<sub><sub>![" + m.alt + " Badge](https://img.shields.io/badge/" + m.url + "-orange?style=flat)</sub></sub>  Title**"
		if got := ClassifyCommentSeverity(body); got != SeverityUnknown {
			t.Errorf("ClassifyCommentSeverity(alt=%s url=%s) = %v, want %v (mismatched badge must fail closed)",
				m.alt, m.url, got, SeverityUnknown)
		}
		if !ClassifyCommentSeverity(body).BlocksResolution() {
			t.Errorf("alt=%s url=%s must block resolution", m.alt, m.url)
		}
	}
}

// TestClassifyCommentSeverityMismatchedGeminiBadgeIsUnknown is the Gemini half
// of the same finding: the alt label and the priority in the SVG path were
// matched independently, so `![medium](.../high-priority.svg)` classified as
// advisory off the label alone.
func TestClassifyCommentSeverityMismatchedGeminiBadgeIsUnknown(t *testing.T) {
	mismatched := []struct{ alt, path string }{
		{"medium", "high"}, // advisory label hiding a blocking path
		{"low", "critical"},
		{"high", "low"},
	}
	for _, m := range mismatched {
		body := "![" + m.alt + "](https://www.gstatic.com/codereviewagent/" + m.path + "-priority.svg)\nFinding prose."
		if got := ClassifyCommentSeverity(body); got != SeverityUnknown {
			t.Errorf("ClassifyCommentSeverity(alt=%s path=%s) = %v, want %v (mismatched badge must fail closed)",
				m.alt, m.path, got, SeverityUnknown)
		}
	}
	// A well-formed Gemini badge is unaffected.
	for label, want := range map[string]ThreadSeverity{
		"critical": SeverityBlocking, "high": SeverityBlocking,
		"medium": SeverityAdvisory, "low": SeverityAdvisory,
	} {
		body := "![" + label + "](https://www.gstatic.com/codereviewagent/" + label + "-priority.svg)"
		if got := ClassifyCommentSeverity(body); got != want {
			t.Errorf("ClassifyCommentSeverity(%s) = %v, want %v", label, got, want)
		}
	}
}

// TestClassifyCommentSeverityMixedSupportedAndUnsupportedIsUnknown pins the
// ce-lr7j review finding that restricting the badge pattern to P0-P3 made the
// parser blind to an unsupported marker rather than suspicious of it. A comment
// carrying a valid P2 badge AND an unsupported P6 badge matched only the P2, so
// `seen` stayed true and the whole comment classified as advisory: the resolver
// auto-resolved the thread and the independent gate repeated the same verdict.
// A badge-shaped marker this code cannot read must poison the verdict even when
// a readable marker sits beside it.
func TestClassifyCommentSeverityMixedSupportedAndUnsupportedIsUnknown(t *testing.T) {
	p3 := "**<sub><sub>![P3 Badge](https://img.shields.io/badge/P3-blue?style=flat)</sub></sub>  Advisory**"
	for _, bad := range []string{"P6", "P4", "P9", "P12"} {
		body := p3 + "\n\n**<sub><sub>![" + bad + " Badge](https://img.shields.io/badge/" + bad +
			"-orange?style=flat)</sub></sub>  Unreadable**"
		if got := ClassifyCommentSeverity(body); got != SeverityUnknown {
			t.Errorf("ClassifyCommentSeverity(P3 + %s) = %v, want %v (an unreadable badge beside a "+
				"readable one must still withhold)", bad, got, SeverityUnknown)
		}
	}
	// A Gemini badge with an unreadable label alongside a valid one behaves the same.
	body := "![low](https://www.gstatic.com/codereviewagent/low-priority.svg)\n" +
		"![urgent](https://www.gstatic.com/codereviewagent/urgent-priority.svg)"
	if got := ClassifyCommentSeverity(body); got != SeverityUnknown {
		t.Errorf("ClassifyCommentSeverity(low + urgent) = %v, want %v", got, SeverityUnknown)
	}
	// A comment carrying only supported markers is unaffected.
	if got := ClassifyCommentSeverity(p3); got != SeverityAdvisory {
		t.Errorf("ClassifyCommentSeverity(plain P2) = %v, want %v", got, SeverityAdvisory)
	}
}

// TestClassifyCommentSeverityForeignHostBadgeIsUnknown pins the ce-lr7j review
// finding that the unread-marker counter validated the DESTINATION before it
// recognised the badge, so an off-host badge stayed invisible.
//
// `![P1 Badge](https://example.invalid/p1.svg)` beside a valid P2 left
// seen=true and unread==parsed, so the comment classified advisory and both the
// resolver and the merge gate cleared it. Badge SYNTAX must be detected first;
// only then does the host decide whether the marker could be read.
func TestClassifyCommentSeverityForeignHostBadgeIsUnknown(t *testing.T) {
	p3 := "**<sub><sub>![P3 Badge](https://img.shields.io/badge/P3-blue?style=flat)</sub></sub>  Advisory**"
	foreign := []string{
		"![P1 Badge](https://example.invalid/p1.svg)",
		"![P0 Badge](https://evil.example.com/badge/P0-red)",
		"![high](https://example.invalid/codereviewagent/high-priority.svg)",
	}
	for _, f := range foreign {
		if got := ClassifyCommentSeverity(p3 + "\n\n" + f); got != SeverityUnknown {
			t.Errorf("ClassifyCommentSeverity(P3 + %q) = %v, want %v (an off-host badge is still a "+
				"badge this parser cannot vouch for)", f, got, SeverityUnknown)
		}
	}
	if got := ClassifyCommentSeverity(p3); got != SeverityAdvisory {
		t.Errorf("a lone canonical P3 must stay advisory, got %v", got)
	}
}

// TestClassifyCommentSeverityAltTextVariantBadgeIsUnknown pins the ce-lr7j
// review finding that the badge-shape detector still keyed on the exact
// "P<n> Badge" alt text.
//
// A future-format marker such as `![Priority P1](.../badge/P1-orange)` was not
// counted as a badge at all, so it could hide behind a valid P2 in the same
// comment and the whole comment classified advisory. The destination is the
// part this code can actually validate, so the shape must be recognised from
// the destination independently of how the alt text is spelled.
func TestClassifyCommentSeverityAltTextVariantBadgeIsUnknown(t *testing.T) {
	p3 := "**<sub><sub>![P3 Badge](https://img.shields.io/badge/P3-blue?style=flat)</sub></sub>  Advisory**"
	for _, variant := range []string{
		"![Priority P1](https://img.shields.io/badge/P1-orange)",
		"![severity: P0](https://img.shields.io/badge/P0-red?style=flat)",
		"![](https://img.shields.io/badge/P1-orange)",
	} {
		if got := ClassifyCommentSeverity(p3 + "\n\n" + variant); got != SeverityUnknown {
			t.Errorf("ClassifyCommentSeverity(P3 + %q) = %v, want %v (a shields.io P badge is a "+
				"badge however its alt text is spelled)", variant, got, SeverityUnknown)
		}
	}
	// A well-formed badge must still be counted exactly ONCE. If the two shape
	// patterns both matched it, unread would exceed parsed and every ordinary
	// advisory comment would wrongly withhold.
	if got := ClassifyCommentSeverity(p3); got != SeverityAdvisory {
		t.Errorf("a lone canonical P3 must stay advisory, got %v (shape double-counting?)", got)
	}
	blocking := "**<sub><sub>![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat)</sub></sub>  Blocking**"
	if got := ClassifyCommentSeverity(blocking); got != SeverityBlocking {
		t.Errorf("a lone canonical P1 must stay blocking, got %v", got)
	}
}

// TestClassifyCommentSeverityQuerySuffixedBadgeIsUnknown pins the ce-lr7j
// review finding that a URL query suffix hid a badge from the shape counter.
//
// Both the strict Gemini parser and the shape detector required ")" immediately
// after ".svg", so `![high](.../high-priority.svg?v=2)` was counted by neither.
// unread then equalled parsed and a comment carrying that high marker alongside
// a valid medium badge classified advisory, clearing both the resolver and the
// merge gate.
//
// The strict parser is deliberately left narrow: a transitional URL form is not
// something this code can vouch for. The SHAPE counter is what must be liberal,
// so the marker is seen, fails to parse, and withholds.
func TestClassifyCommentSeverityQuerySuffixedBadgeIsUnknown(t *testing.T) {
	medium := "![medium](https://www.gstatic.com/codereviewagent/medium-priority.svg)"
	for _, variant := range []string{
		"![high](https://www.gstatic.com/codereviewagent/high-priority.svg?v=2)",
		"![critical](https://www.gstatic.com/codereviewagent/critical-priority.svg#frag)",
	} {
		if got := ClassifyCommentSeverity(medium + "\n\n" + variant); got != SeverityUnknown {
			t.Errorf("ClassifyCommentSeverity(medium + %q) = %v, want %v (a badge the parser cannot "+
				"read must not hide behind one it can)", variant, got, SeverityUnknown)
		}
	}
	// Well-formed badges are still counted exactly once and classify normally.
	if got := ClassifyCommentSeverity(medium); got != SeverityAdvisory {
		t.Errorf("a lone canonical medium badge = %v, want advisory (shape double-counting?)", got)
	}
	high := "![high](https://www.gstatic.com/codereviewagent/high-priority.svg)"
	if got := ClassifyCommentSeverity(high); got != SeverityBlocking {
		t.Errorf("a lone canonical high badge = %v, want blocking", got)
	}
	// A shields.io query suffix is the ORDINARY Codex form, not a transitional
	// one: the strict pattern already reads it, so it must keep classifying
	// normally rather than being swept up as unreadable.
	shields := "**<sub><sub>![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat&logo=x)</sub></sub>  T**"
	if got := ClassifyCommentSeverity(shields); got != SeverityBlocking {
		t.Errorf("a shields.io badge with query parameters = %v, want blocking", got)
	}
}

// TestClassifyCommentSeverityTruncatedBadgePrefixIsUnknown pins the ce-lr7j
// review finding about an interaction BETWEEN the two badge patterns.
//
// The strict pattern stopped at the URL prefix and did not require the closing
// parenthesis, so a truncated `![P2 Badge](.../badge/P2-yellow` counted as
// parsed. The shape pattern, which does span to ")", then swallowed the
// truncated marker and a following complete P6 badge as ONE shape. unread
// equalled parsed and the comment classified advisory, so an unsupported
// marker rode through on a malformed one.
func TestClassifyCommentSeverityTruncatedBadgePrefixIsUnknown(t *testing.T) {
	truncated := "![P2 Badge](https://img.shields.io/badge/P2-yellow"
	unsupported := "![P6 Badge](https://img.shields.io/badge/P6-orange?style=flat)"
	if got := ClassifyCommentSeverity(truncated + " " + unsupported); got != SeverityUnknown {
		t.Errorf("ClassifyCommentSeverity(truncated P2 + P6) = %v, want %v", got, SeverityUnknown)
	}
	// A truncated badge on its own is not something this parser can read.
	if got := ClassifyCommentSeverity(truncated); got != SeverityUnknown {
		t.Errorf("ClassifyCommentSeverity(truncated P2 alone) = %v, want %v", got, SeverityUnknown)
	}
	// Complete badges are unaffected: P2 is a correctness finding and blocks,
	// P3 is the advisory tier.
	completeP2 := "**<sub><sub>![P2 Badge](https://img.shields.io/badge/P2-yellow?style=flat)</sub></sub>  T**"
	if got := ClassifyCommentSeverity(completeP2); got != SeverityBlocking {
		t.Errorf("a complete P2 = %v, want blocking", got)
	}
	completeP3 := "**<sub><sub>![P3 Badge](https://img.shields.io/badge/P3-blue?style=flat)</sub></sub>  T**"
	if got := ClassifyCommentSeverity(completeP3); got != SeverityAdvisory {
		t.Errorf("a complete P3 = %v, want advisory", got)
	}
}

// TestCodexP2IsBlocking pins the ce-lr7j review finding that this change, as
// originally written, did not fix the incident it cites.
//
// The rationale at the top of this file counts the 2026-07-21 failure as "30 P1
// and 25 P2 findings" released unread. Classifying P2 as advisory would have
// auto-resolved all 25 of those again, and the independent gate, sharing this
// classifier, would have seen nothing blocking either.
//
// The parent implementation agrees: cmd/mergeloop/threads.go on main declares
// `severityP2: medium-severity finding → mergeloop must NOT auto-resolve` and
// blocks on `s >= severityP2`, reserving advisory for the P3-P5 badge form.
// Auto-resolution is for P3 and below.
func TestCodexP2IsBlocking(t *testing.T) {
	badge := func(p string) string {
		return "**<sub><sub>![" + p + " Badge](https://img.shields.io/badge/" + p +
			"-yellow?style=flat)</sub></sub>  Finding**"
	}
	for _, p := range []string{"P0", "P1", "P2"} {
		if got := ClassifyCommentSeverity(badge(p)); got != SeverityBlocking {
			t.Errorf("ClassifyCommentSeverity(%s) = %v, want %v (a correctness finding must never "+
				"auto-resolve)", p, got, SeverityBlocking)
		}
		if !ClassifyCommentSeverity(badge(p)).BlocksResolution() {
			t.Errorf("%s must block resolution", p)
		}
	}
	if got := ClassifyCommentSeverity(badge("P3")); got != SeverityAdvisory {
		t.Errorf("ClassifyCommentSeverity(P3) = %v, want %v (advisory starts at P3)", got, SeverityAdvisory)
	}
	// Gemini keeps the mapping main uses: high is the P2 equivalent and blocks,
	// medium and low are advisory.
	gem := func(l string) string {
		return "![" + l + "](https://www.gstatic.com/codereviewagent/" + l + "-priority.svg)"
	}
	for l, want := range map[string]ThreadSeverity{
		"critical": SeverityBlocking, "high": SeverityBlocking,
		"medium": SeverityAdvisory, "low": SeverityAdvisory,
	} {
		if got := ClassifyCommentSeverity(gem(l)); got != want {
			t.Errorf("ClassifyCommentSeverity(gemini %s) = %v, want %v", l, got, want)
		}
	}
}

// TestTruncatedBadgeCannotHideBehindALaterBadge pins the review finding that a
// badge-shape match could span a later, valid marker.
//
// The shape counter used [^)]* after the opening paren, so an unterminated P1
// badge swallowed everything up to the NEXT badge's closing parenthesis and
// counted the pair as one marker. The strict parser independently read the
// valid P3, leaving counted == parsed, so nothing looked unread and the
// comment classified as advisory. That auto-resolves a malformed P1: exactly
// the fail-open this classifier exists to prevent.
func TestTruncatedBadgeCannotHideBehindALaterBadge(t *testing.T) {
	body := "![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat " +
		"some prose ![P3 Badge](https://img.shields.io/badge/P3-blue?style=flat)"
	if got := ClassifyCommentSeverity(body); got != SeverityUnknown {
		t.Errorf("ClassifyCommentSeverity = %v, want SeverityUnknown: a truncated P1 "+
			"marker must not be readable as advisory just because a valid P3 follows", got)
	}
	// A well-formed pair must still classify normally, most severe wins.
	ok := "![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat) and " +
		"![P3 Badge](https://img.shields.io/badge/P3-blue?style=flat)"
	if got := ClassifyCommentSeverity(ok); got != SeverityBlocking {
		t.Errorf("ClassifyCommentSeverity(well-formed pair) = %v, want SeverityBlocking", got)
	}
}
