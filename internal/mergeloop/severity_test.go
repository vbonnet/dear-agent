package mergeloop

import "testing"

// Real Codex badge markup, copied verbatim from PR #989 thread bodies (the PR
// whose four P1 findings were auto-resolved into main on 2026-07-21).
const (
	codexP1Body = "**<sub><sub>![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat)</sub></sub>  " +
		"Require delivery evidence before closing merged work**\n\nWhen a matching PR has merged but " +
		"deployment or real-system verification is still pending or has failed, this branch immediately " +
		"closes the bead using only the merge timestamp."
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
		{"codex P2 is advisory", codexP2Body, SeverityAdvisory},
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
		{"advisory only", []string{codexP2Body, geminiMediumBody}, SeverityAdvisory},
		{"blocking anywhere wins", []string{codexP2Body, codexP1Body}, SeverityBlocking},
		{"unknown anywhere withholds", []string{codexP2Body, "plain prose"}, SeverityUnknown},
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
	for p, want := range map[string]ThreadSeverity{"P0": SeverityBlocking, "P1": SeverityBlocking, "P2": SeverityAdvisory, "P3": SeverityAdvisory} {
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
	p2 := "**<sub><sub>![P2 Badge](https://img.shields.io/badge/P2-yellow?style=flat)</sub></sub>  Advisory**"
	for _, bad := range []string{"P6", "P4", "P9", "P12"} {
		body := p2 + "\n\n**<sub><sub>![" + bad + " Badge](https://img.shields.io/badge/" + bad +
			"-orange?style=flat)</sub></sub>  Unreadable**"
		if got := ClassifyCommentSeverity(body); got != SeverityUnknown {
			t.Errorf("ClassifyCommentSeverity(P2 + %s) = %v, want %v (an unreadable badge beside a "+
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
	if got := ClassifyCommentSeverity(p2); got != SeverityAdvisory {
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
	p2 := "**<sub><sub>![P2 Badge](https://img.shields.io/badge/P2-yellow?style=flat)</sub></sub>  Advisory**"
	foreign := []string{
		"![P1 Badge](https://example.invalid/p1.svg)",
		"![P0 Badge](https://evil.example.com/badge/P0-red)",
		"![high](https://example.invalid/codereviewagent/high-priority.svg)",
	}
	for _, f := range foreign {
		if got := ClassifyCommentSeverity(p2 + "\n\n" + f); got != SeverityUnknown {
			t.Errorf("ClassifyCommentSeverity(P2 + %q) = %v, want %v (an off-host badge is still a "+
				"badge this parser cannot vouch for)", f, got, SeverityUnknown)
		}
	}
	if got := ClassifyCommentSeverity(p2); got != SeverityAdvisory {
		t.Errorf("a lone canonical P2 must stay advisory, got %v", got)
	}
}
