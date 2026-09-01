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
