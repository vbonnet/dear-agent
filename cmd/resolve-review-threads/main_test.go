package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestCleanBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"collapses whitespace", "a\n\tb   c", "a b c"},
		{"trims ends", "  hello world  ", "hello world"},
		{"empty", "", ""},
		{"only whitespace", "  \n\t ", ""},
		{"truncates to preview length", repeat("x", bodyPreviewLen+50), repeat("x", bodyPreviewLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanBody(tt.in); got != tt.want {
				t.Errorf("cleanBody(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanBodyRuneSafe(t *testing.T) {
	// A multibyte string longer than the preview must be cut on a rune
	// boundary, never mid-codepoint, and yield exactly bodyPreviewLen runes.
	in := repeat("é", bodyPreviewLen+10)
	got := cleanBody(in)
	if n := len([]rune(got)); n != bodyPreviewLen {
		t.Fatalf("got %d runes, want %d", n, bodyPreviewLen)
	}
}

func TestFilterThreads(t *testing.T) {
	threads := []thread{
		{ID: "1", IsResolved: false, Author: "gemini-code-assist"},
		{ID: "2", IsResolved: true, Author: "gemini-code-assist"},
		{ID: "3", IsResolved: false, Author: "alice"},
		{ID: "4", IsResolved: false, Author: "gemini-code-assist"},
	}

	t.Run("no author keeps all unresolved", func(t *testing.T) {
		got := filterThreads(threads, "")
		if want := []string{"1", "3", "4"}; !idsEqual(got, want) {
			t.Errorf("got %v, want %v", ids(got), want)
		}
	})

	t.Run("author filter never matches resolved", func(t *testing.T) {
		got := filterThreads(threads, "gemini-code-assist")
		if want := []string{"1", "4"}; !idsEqual(got, want) {
			t.Errorf("got %v, want %v", ids(got), want)
		}
	})

	t.Run("author with no unresolved threads yields empty", func(t *testing.T) {
		if got := filterThreads(threads, "nobody"); len(got) != 0 {
			t.Errorf("got %v, want empty", ids(got))
		}
	})
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}

func ids(ts []thread) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}

func idsEqual(ts []thread, want []string) bool {
	got := ids(ts)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// mkNode builds a threadNode whose comment authors are the given logins, in
// order, mirroring what the opening/recent GraphQL aliases return: the first
// login opens the thread, the last one had the most recent word.
func mkNode(id string, outdated bool, authors ...string) threadNode {
	var n threadNode
	n.ID = id
	n.IsOutdated = outdated
	n.Path = "pkg/thing.go"
	n.Opening.TotalCount = len(authors)
	if len(authors) == 0 {
		return n
	}
	var open struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Body string `json:"body"`
	}
	open.Author.Login = authors[0]
	open.Body = "comment from " + authors[0]
	n.Opening.Nodes = append(n.Opening.Nodes, open)

	// Recent mirrors comments(last:2): newest last.
	for i := max(0, len(authors)-2); i < len(authors); i++ {
		var c struct {
			ID     string `json:"id"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body string `json:"body"`
		}
		c.ID = fmt.Sprintf("%s-c%d", id, i)
		c.Author.Login = authors[i]
		c.Body = "comment from " + authors[i]
		n.Recent.Nodes = append(n.Recent.Nodes, c)
	}
	return n
}

// mkLongNode builds a thread whose comment count exceeds any single page, to
// prove the answered check reads the true last commenter rather than the end
// of a fetched page.
func mkLongNode(id string, total int, opening, latest string) threadNode {
	n := mkNode(id, false, opening, latest)
	n.Opening.TotalCount = total
	return n
}

// TestToThreadAnswered pins the evidence gate that resolve-all depends on.
// Answered is the only signal standing between "we addressed the review" and
// a blind bulk resolve, so each transition is asserted explicitly.
func TestToThreadAnswered(t *testing.T) {
	tests := []struct {
		name         string
		node         threadNode
		wantAnswered bool
		wantLast     string
	}{
		{
			name:         "reviewer comment alone is unanswered",
			node:         mkNode("1", false, "gemini-code-assist"),
			wantAnswered: false,
			wantLast:     "gemini-code-assist",
		},
		{
			name:         "reply from someone else answers it",
			node:         mkNode("2", false, "gemini-code-assist", "vbonnet"),
			wantAnswered: true,
			wantLast:     "vbonnet",
		},
		{
			name:         "reviewer replying after us takes the ball back",
			node:         mkNode("3", false, "gemini-code-assist", "vbonnet", "gemini-code-assist"),
			wantAnswered: false,
			wantLast:     "gemini-code-assist",
		},
		{
			name:         "reviewer talking to itself is not an answer",
			node:         mkNode("4", false, "gemini-code-assist", "gemini-code-assist"),
			wantAnswered: false,
			wantLast:     "gemini-code-assist",
		},
		{
			name:         "outdated alone never answers a thread",
			node:         mkNode("5", true, "chatgpt-codex-connector"),
			wantAnswered: false,
			wantLast:     "chatgpt-codex-connector",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toThread(tt.node)
			if got.Answered != tt.wantAnswered {
				t.Errorf("Answered = %t, want %t", got.Answered, tt.wantAnswered)
			}
			if got.LastAuthor != tt.wantLast {
				t.Errorf("LastAuthor = %q, want %q", got.LastAuthor, tt.wantLast)
			}
		})
	}
}

// TestToThreadOutdatedIsReportedNotActedOn guards the trap that motivated this
// change: on dear-agent#1242 three outdated threads were live P1 findings, so
// outdated must survive into output without ever implying resolvability.
func TestToThreadOutdatedIsReportedNotActedOn(t *testing.T) {
	got := toThread(mkNode("x", true, "chatgpt-codex-connector"))
	if !got.IsOutdated {
		t.Fatal("IsOutdated was dropped; refusals could not label the thread")
	}
	if got.Answered {
		t.Fatal("an outdated thread was treated as answered")
	}
	if note := outdatedNote(got); note == "" {
		t.Error("outdatedNote said nothing for an outdated thread")
	}
	if note := outdatedNote(toThread(mkNode("y", false, "bot"))); note != "" {
		t.Errorf("outdatedNote spoke for a current thread: %q", note)
	}
}

// TestToThreadNoComments covers the defensive path: a thread with no comments
// must not panic on the last-comment lookup.
func TestToThreadNoComments(t *testing.T) {
	got := toThread(threadNode{ID: "empty"})
	if got.Author != "unknown" || got.Answered {
		t.Errorf("got %+v, want unknown author and unanswered", got)
	}
}

// TestToThreadLongThreadUsesTrueLastCommenter covers the pagination finding:
// with more comments than one page holds, the verdict must still come from the
// genuinely most recent comment, which the `recent` alias supplies directly.
func TestToThreadLongThreadUsesTrueLastCommenter(t *testing.T) {
	answered := toThread(mkLongNode("long-1", 250, "gemini-code-assist", "vbonnet"))
	if !answered.Answered || answered.LastAuthor != "vbonnet" {
		t.Errorf("250-comment thread ending with our reply: got %+v, want answered by vbonnet", answered)
	}

	// The dangerous direction: the reviewer got the last word far past any
	// page boundary. This must not be treated as answered.
	reopened := toThread(mkLongNode("long-2", 250, "gemini-code-assist", "gemini-code-assist"))
	if reopened.Answered {
		t.Error("reviewer had the last word on a long thread but it counted as answered")
	}
}

// TestToThreadSingleCommentNeverAnswered pins that a lone opening comment is
// unanswered no matter what, since TotalCount is the only length signal.
func TestToThreadSingleCommentNeverAnswered(t *testing.T) {
	n := mkNode("solo", false, "gemini-code-assist")
	if got := toThread(n); got.Answered {
		t.Errorf("single-comment thread counted as answered: %+v", got)
	}
}

// mkNodeLogins builds a thread from explicit logins so the empty-login cases
// (deleted or hidden accounts) can be exercised directly.
func mkNodeLogins(id, opening, latest string, total int) threadNode {
	var n threadNode
	n.ID = id
	n.Path = "pkg/thing.go"
	n.Opening.TotalCount = total
	var open struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Body string `json:"body"`
	}
	open.Author.Login = opening
	n.Opening.Nodes = append(n.Opening.Nodes, open)
	var last struct {
		ID     string `json:"id"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Body string `json:"body"`
	}
	last.ID = id + "-last"
	last.Author.Login = latest
	n.Recent.Nodes = append(n.Recent.Nodes, last)
	return n
}

// TestToThreadFailsClosedOnMissingAuthor pins that a missing login never
// manufactures the second participant that Answered asserts. Comparing an
// absent login against the "unknown" display placeholder would read as two
// distinct actors and license resolving a finding nobody answered.
func TestToThreadFailsClosedOnMissingAuthor(t *testing.T) {
	tests := []struct {
		name            string
		opening, latest string
	}{
		{"latest author missing", "gemini-code-assist", ""},
		{"opening author missing", "", "vbonnet"},
		{"both missing", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toThread(mkNodeLogins("x", tt.opening, tt.latest, 2)); got.Answered {
				t.Errorf("answered on unobservable authors: %+v", got)
			}
		})
	}

	// Control: two observed, distinct logins still answer the thread.
	if got := toThread(mkNodeLogins("y", "gemini-code-assist", "vbonnet", 2)); !got.Answered {
		t.Errorf("two observed authors should answer the thread: %+v", got)
	}
}

// TestUnansweredErrorIsDistinguishable pins the sweep's error split: an
// evidence refusal must be recognisable via errors.As so operational failures
// (auth, permissions, outage) can abort instead of repeating per thread.
func TestUnansweredErrorIsDistinguishable(t *testing.T) {
	var target *unansweredError
	if !errors.As(error(&unansweredError{msg: "unanswered"}), &target) {
		t.Fatal("unansweredError not matched by errors.As")
	}
	if errors.As(errors.New("gh api graphql: permission denied"), &target) {
		t.Error("an operational error was misclassified as an evidence refusal")
	}
}

// TestFailedReopenErrorIsDistinguishable pins the same split one level up:
// a resolution that went through on stale evidence AND whose automatic
// reopen also failed must be recognisable via errors.As and NOT match
// unansweredError, because the two need opposite retry advice — the thread
// is still resolved, so "just retry reply-resolve" would silently no-op at
// readOrExit's IsResolved check instead of reopening and re-reading it.
func TestFailedReopenErrorIsDistinguishable(t *testing.T) {
	var reopenTarget *failedReopenError
	if !errors.As(error(&failedReopenError{msg: "reopen failed"}), &reopenTarget) {
		t.Fatal("failedReopenError not matched by errors.As")
	}
	var unansweredTarget *unansweredError
	if errors.As(error(&failedReopenError{msg: "reopen failed"}), &unansweredTarget) {
		t.Error("failedReopenError was misclassified as unansweredError")
	}
	if errors.As(error(&unansweredError{msg: "unanswered"}), &reopenTarget) {
		t.Error("unansweredError was misclassified as failedReopenError")
	}
}

// TestSameReplyBody pins the retry guard's comparison. cleanBody collapses
// whitespace and truncates to a preview, so using it here would fail to match
// any multi-line or long reply and repost it on every retry.
func TestSameReplyBody(t *testing.T) {
	multiline := "Fixed in abc1234 — moved the check.\n\nCovered by TestThing."
	long := repeat("x", bodyPreviewLen+80)

	tests := []struct {
		name       string
		last, want string
		match      bool
	}{
		{"identical", "Fixed - one line", "Fixed - one line", true},
		{"multiline survives", multiline, multiline, true},
		{"longer than the preview survives", long, long, true},
		{"surrounding whitespace ignored", "  Fixed - one line\n", "Fixed - one line", true},
		{"different replies do not match", "Fixed - A", "Fixed - B", false},
		{"truncated preview is not the reply", cleanBody(long), long, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameReplyBody(tt.last, tt.want); got != tt.match {
				t.Errorf("sameReplyBody = %t, want %t", got, tt.match)
			}
		})
	}
}

// TestShellQuote pins the retry-guidance safety property %q broke: a
// multiline body or one with shell metacharacters must survive a literal
// copy-paste into bash unchanged, or the retried command posts a body that
// no longer matches the original and classifyPriorReply treats it as new.
func TestShellQuote(t *testing.T) {
	tests := []struct {
		name, in string
	}{
		{"plain", "Fixed - moved the check"},
		{"multiline", "Fixed in abc1234\n\nCovered by TestThing."},
		{"embedded single quote", "it's fixed now"},
		{"dollar and backtick", "cost is $5 via `cmd`"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := shellQuote(tt.in)
			got := unquoteSingleQuoted(t, quoted)
			if got != tt.in {
				t.Errorf("shellQuote(%q) = %q, round-tripped through a POSIX "+
					"shell as %q, want %q", tt.in, quoted, got, tt.in)
			}
		})
	}
}

// unquoteSingleQuoted runs the shell's own single-quote parsing rather than
// reimplementing it, so the test proves what bash would actually see.
func unquoteSingleQuoted(t *testing.T, quoted string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", "printf '%s' "+quoted).Output()
	if err != nil {
		t.Fatalf("sh -c failed on %q: %v", quoted, err)
	}
	return string(out)
}

// TestIsAccessDenied separates a refusal by GitHub from a transient failure.
// Retrying a denial repeats it, so the two get different advice.
func TestIsAccessDenied(t *testing.T) {
	denied := []string{
		"gh api graphql: HTTP 403: Resource not accessible by integration",
		"gh api graphql: exit status 1: Bad credentials",
		"Must have push permission to resolve",
		"HTTP 401: requires authentication",
	}
	for _, m := range denied {
		if !isAccessDenied(errors.New(m)) {
			t.Errorf("expected access denial for %q", m)
		}
	}
	transient := []string{
		"gh api graphql: exit status 1: server error 502",
		"context deadline exceeded",
		"gh: Not Found",
	}
	for _, m := range transient {
		if isAccessDenied(errors.New(m)) {
			t.Errorf("misread %q as an access denial", m)
		}
	}
	if isAccessDenied(nil) {
		t.Error("nil is not an access denial")
	}
}

// TestToThreadRecentUsesNewestComment guards the indexing after widening the
// recent window to two comments: the newest is LAST in the connection, so
// reading nodes[0] would silently take the second-to-last commenter and could
// call a thread answered when the reviewer actually spoke last.
func TestToThreadRecentUsesNewestComment(t *testing.T) {
	n := mkNode("t", false, "gemini-code-assist", "vbonnet", "gemini-code-assist")
	got := toThread(n)
	if got.LastAuthor != "gemini-code-assist" {
		t.Errorf("LastAuthor = %q, want the newest comment's author", got.LastAuthor)
	}
	if got.Answered {
		t.Error("reviewer spoke last; thread must not be answered")
	}
	if got.LastID == "" || got.PrevID == "" {
		t.Errorf("both comment IDs must be populated, got last=%q prev=%q", got.LastID, got.PrevID)
	}
	if got.LastID == got.PrevID {
		t.Error("last and previous comment IDs must differ")
	}
}

// TestToThreadPrevIDEmptyOnSingleComment pins that a one-comment thread has no
// predecessor. reply-resolve compares PrevID against the comment it read, so a
// bogus non-empty value here would let an interleaved comment pass unnoticed.
func TestToThreadPrevIDEmptyOnSingleComment(t *testing.T) {
	got := toThread(mkNode("solo", false, "gemini-code-assist"))
	if got.PrevID != "" {
		t.Errorf("PrevID = %q, want empty for a single-comment thread", got.PrevID)
	}
	if got.LastID == "" {
		t.Error("LastID must still be populated")
	}
}

// TestClassifyPriorReply covers the retry-after-follow-up fail-open. If a run
// posts a reply and dies before resolving, and the reviewer then comments, a
// naive last-comment check reposts the same body; our copy becomes the last
// comment, the thread reads as answered, and the follow-up is resolved away
// unread. The classifier must see the prior reply wherever it sits.
func TestClassifyPriorReply(t *testing.T) {
	reply := "Fixed - moved the check into the helper."
	tests := []struct {
		name string
		tail []tailComment
		want priorReplyState
	}{
		{
			name: "not posted yet",
			tail: []tailComment{{Login: "bot", Body: "P1: fix this"}},
			want: noPriorReply,
		},
		{
			name: "posted and still last",
			tail: []tailComment{{Login: "bot", Body: "P1: fix this"}, {Login: "vbonnet", Body: reply}},
			want: priorReplyIsLast,
		},
		{
			name: "posted then reviewer followed up",
			tail: []tailComment{
				{Login: "bot", Body: "P1: fix this"},
				{Login: "vbonnet", Body: reply},
				{Login: "bot", Body: "That does not cover the nil case."},
			},
			want: priorReplySuperseded,
		},
		{
			name: "empty tail",
			tail: nil,
			want: noPriorReply,
		},
		{
			name: "similar but different reply is not ours",
			tail: []tailComment{{Login: "vbonnet", Body: "Fixed - something else entirely."}},
			want: noPriorReply,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPriorReply(tt.tail, reply); got != tt.want {
				t.Errorf("classifyPriorReply = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFindReplyID pins the ambiguous-reply-mutation recovery path: after a
// client-side postReply error, postReplyOrExit re-reads history and uses
// this to find the comment it may have already posted, rather than trust a
// failure that doesn't prove the mutation never applied.
func TestFindReplyID(t *testing.T) {
	reply := "Fixed - moved the check into the helper."
	tests := []struct {
		name string
		tail []tailComment
		want string
	}{
		{
			name: "found",
			tail: []tailComment{{ID: "c1", Body: "P1: fix this"}, {ID: "c2", Body: reply}},
			want: "c2",
		},
		{
			name: "not present",
			tail: []tailComment{{ID: "c1", Body: "P1: fix this"}},
			want: "",
		},
		{
			name: "empty tail",
			tail: nil,
			want: "",
		},
		{
			name: "last match wins on duplicates",
			tail: []tailComment{{ID: "c1", Body: reply}, {ID: "c2", Body: "other"}, {ID: "c3", Body: reply}},
			want: "c3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findReplyID(tt.tail, reply); got != tt.want {
				t.Errorf("findReplyID = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestClassifyPriorReplyMultiLine pins that the prior-reply scan uses the same
// lossless comparison as the retry guard, since real replies are multi-line.
func TestClassifyPriorReplyMultiLine(t *testing.T) {
	reply := "Fixed in abc1234.\n\nCovered by TestThing, mutation-checked."
	tail := []tailComment{
		{Login: "bot", Body: "P2: consider this"},
		{Login: "vbonnet", Body: reply},
		{Login: "bot", Body: "Still wrong."},
	}
	if got := classifyPriorReply(tail, reply); got != priorReplySuperseded {
		t.Errorf("multi-line prior reply not recognised: got %v", got)
	}
}

// TestParseReplyCommentID pins the anchor the whole reply-resolve safety
// argument rests on. Resolution is allowed only when a NAMED comment is the
// thread's last one, so a missing ID must be a hard error: a blank anchor
// would compare equal to nothing and silently disable the check.
func TestParseReplyCommentID(t *testing.T) {
	good := []byte(`{"data":{"addPullRequestReviewThreadReply":{"comment":{"id":"PRRC_abc"}}}}`)
	id, err := parseReplyCommentID(good)
	if err != nil || id != "PRRC_abc" {
		t.Fatalf("got (%q, %v), want (PRRC_abc, nil)", id, err)
	}

	// errReplyIDMissing distinguishes "GitHub accepted the mutation but the
	// response omitted the ID" (the reply is live; postReplyOrExit forbids
	// reposting) from a genuine parse failure (unknown whether it posted at
	// all): they need opposite advice, so the sentinel must fire on exactly
	// the first kind.
	bad := map[string]struct {
		raw        []byte
		wantIDMiss bool
	}{
		"empty id":        {[]byte(`{"data":{"addPullRequestReviewThreadReply":{"comment":{"id":""}}}}`), true},
		"missing comment": {[]byte(`{"data":{"addPullRequestReviewThreadReply":{}}}`), true},
		"empty payload":   {[]byte(`{}`), true},
		"not json":        {[]byte(`<html>502</html>`), false},
	}
	for name, tc := range bad {
		t.Run(name, func(t *testing.T) {
			id, err := parseReplyCommentID(tc.raw)
			if err == nil {
				t.Fatalf("want an error, got id=%q", id)
			}
			if got := errors.Is(err, errReplyIDMissing); got != tc.wantIDMiss {
				t.Errorf("errors.Is(err, errReplyIDMissing) = %v, want %v (err: %v)",
					got, tc.wantIDMiss, err)
			}
		})
	}
}

// TestReplyMutationRequestsCommentID guards the query itself: if the mutation
// stopped selecting the comment ID, parseReplyCommentID would fail every time
// and reply-resolve would never resolve anything.
func TestReplyMutationRequestsCommentID(t *testing.T) {
	if !strings.Contains(replyMutation, "comment { id }") {
		t.Errorf("reply mutation must select the comment ID, got: %s", replyMutation)
	}
}

// TestResolveMutationRequestsLastComment guards the query resolveWithEvidence's
// post-mutation reconciliation depends on: if resolveMutation stopped
// selecting the thread's last comment, mutateThread would always report an
// empty lastCommentID, silently disabling the check for a comment landing
// while the resolve mutation itself was in flight.
func TestResolveMutationRequestsLastComment(t *testing.T) {
	if !strings.Contains(resolveMutation, "comments(last:1)") {
		t.Errorf("resolve mutation must select the last comment, got: %s", resolveMutation)
	}
}

// TestCheckReplyPlacement pins BOTH conditions reply-resolve depends on, and
// exists because dropping one of them is exactly how this regressed.
//
// SPEC-28 required verifying that our reply directly follows the comment we
// read. A later revision introduced an identity check on the reply itself and
// REPLACED the predecessor check with it, which silently reopened the race: a
// follow-up landing between the read and the post leaves our reply last, so
// the identity check passes while our reply sits on top of an unread comment.
// Neither condition implies the other, so both are asserted here.
func TestCheckReplyPlacement(t *testing.T) {
	const (
		read    = "PRRC_read"    // last comment when we looked
		ours    = "PRRC_ours"    // the reply we posted
		sneaked = "PRRC_sneaked" // a comment we never saw
		later   = "PRRC_later"   // a comment posted after ours
	)
	tests := []struct {
		name             string
		gotPrev, gotLast string
		want             replyPlacement
	}{
		{
			name:    "clean: ours is last and follows what we read",
			gotPrev: read, gotLast: ours,
			want: replyExact,
		},
		{
			name:    "buried: someone spoke after our reply",
			gotPrev: ours, gotLast: later,
			want: replyBuried,
		},
		{
			name:    "jumped: a comment arrived between our read and our post",
			gotPrev: sneaked, gotLast: ours,
			want: replyJumped,
		},
		{
			name:    "buried takes precedence when both went wrong",
			gotPrev: sneaked, gotLast: later,
			want: replyBuried,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkReplyPlacement(tt.gotPrev, tt.gotLast, read, ours)
			if got != tt.want {
				t.Errorf("checkReplyPlacement = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCheckReplyPlacementRejectsBlankAnchors guards the degenerate case: an
// empty expected ID must not compare equal to an empty observed one and wave
// the resolution through. A single-comment thread legitimately has no
// predecessor, so that pairing is the one blank case allowed.
func TestCheckReplyPlacementRejectsBlankAnchors(t *testing.T) {
	if got := checkReplyPlacement("", "", "PRRC_read", "PRRC_ours"); got == replyExact {
		t.Error("blank observed IDs must not satisfy a named anchor")
	}
	if got := checkReplyPlacement("PRRC_x", "", "", ""); got == replyExact {
		t.Error("blank expected last ID must not be satisfied by a real comment")
	}
}

// TestRunHelpExitsZero pins that the documented help path works. The Makefile
// points agents at `resolve-review-threads --help`, and a help that exits 1
// aborts any caller running under `set -e`.
func TestRunHelpExitsZero(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		if code := run([]string{flag}); code != 0 {
			t.Errorf("run(%q) = %d, want 0", flag, code)
		}
	}
	if code := run([]string{"not-a-subcommand"}); code == 0 {
		t.Error("an unknown subcommand must still fail")
	}
	if code := run(nil); code == 0 {
		t.Error("no arguments must still fail")
	}
}

// TestClassifyPriorReplyBeyondTail is the regression for the bounded-window
// fail-open. A reply buried under later discussion must still be found, or it
// gets reposted and anchored to the newest follow-up, which passes the
// placement check and resolves everything in between unread. classifyPriorReply
// is now fed the full paginated history, so depth must not matter.
func TestClassifyPriorReplyBeyondTail(t *testing.T) {
	reply := "Fixed - handled the nil case."
	history := []tailComment{
		{Login: "chatgpt-codex-connector", Body: "P1: nil deref"},
		{Login: "vbonnet", Body: reply},
	}
	// Bury it far deeper than any bounded window would reach.
	for i := range 60 {
		history = append(history, tailComment{
			Login: "chatgpt-codex-connector",
			Body:  fmt.Sprintf("later finding %d", i),
		})
	}
	if got := classifyPriorReply(history, reply); got != priorReplySuperseded {
		t.Errorf("prior reply buried under %d comments not found: got %v",
			len(history)-2, got)
	}
}

// TestCheckCursorAdvances pins the pagination progress guard. A response
// claiming another page while returning an empty or repeated cursor would
// otherwise loop forever, so reply-resolve would never finish and would
// hammer the API while not finishing.
func TestCheckCursorAdvances(t *testing.T) {
	if err := checkCursorAdvances("cursor2", "cursor1"); err != nil {
		t.Errorf("a genuinely advancing cursor was rejected: %v", err)
	}
	if err := checkCursorAdvances("cursor1", "cursor1"); err == nil {
		t.Error("a repeated cursor must abort pagination")
	}
	if err := checkCursorAdvances("", "cursor1"); err == nil {
		t.Error("an empty cursor must abort pagination")
	}
	if err := checkCursorAdvances("", ""); err == nil {
		t.Error("an empty first cursor must abort pagination")
	}
}
