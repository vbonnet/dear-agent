package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestIsKnownBotAuthor(t *testing.T) {
	cases := []struct {
		login string
		want  bool
	}{
		{"gemini-code-assist", true},
		{"gemini-code-assist[bot]", true}, // "[bot]" suffix normalizes off
		{"chatgpt-codex-connector", true},
		{"alice", false},           // human
		{"dependabot[bot]", false}, // a bot, but not one we auto-resolve
		{"", false},                // missing author
	}
	for _, c := range cases {
		if got := isKnownBotAuthor(c.login); got != c.want {
			t.Errorf("isKnownBotAuthor(%q) = %v, want %v", c.login, got, c.want)
		}
	}
}

func TestAllCommentsFromKnownBots(t *testing.T) {
	cases := []struct {
		name   string
		logins []string
		want   bool
	}{
		{"empty", nil, false},
		{"single bot comment", []string{"gemini-code-assist"}, true},
		{"single human comment", []string{"alice"}, false},
		{"all known bots", []string{"gemini-code-assist", "chatgpt-codex-connector"}, true},
		{"bot opens, human replies", []string{"gemini-code-assist", "alice"}, false},
		{"bot opens, unknown bot replies", []string{"gemini-code-assist", "dependabot[bot]"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := allCommentsFromKnownBots(c.logins); got != c.want {
				t.Errorf("allCommentsFromKnownBots(%v) = %v, want %v", c.logins, got, c.want)
			}
		})
	}
}

func TestNormalizeBotLogin(t *testing.T) {
	cases := map[string]string{
		"some-bot":      "some-bot",
		"some-bot[bot]": "some-bot",
		"alice":         "alice",
	}
	for in, want := range cases {
		if got := normalizeBotLogin(in); got != want {
			t.Errorf("normalizeBotLogin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitOwnerRepo(t *testing.T) {
	cases := []struct {
		in          string
		owner, name string
		ok          bool
	}{
		{"vbonnet/dear-agent", "vbonnet", "dear-agent", true},
		{"owner/repo/extra", "owner", "repo/extra", true}, // SplitN keeps the tail
		{"noslash", "", "", false},
		{"/repo", "", "", false},
		{"owner/", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		owner, name, ok := splitOwnerRepo(c.in)
		if owner != c.owner || name != c.name || ok != c.ok {
			t.Errorf("splitOwnerRepo(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, owner, name, ok, c.owner, c.name, c.ok)
		}
	}
}

// ---- ce-lr7j regression tests ----
//
// Bodies are real markup copied from the PRs in the incident: #989 (Codex, four
// P1s auto-resolved into main) and #945 (Gemini).

const (
	tstCodexP1 = "**<sub><sub>![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat)</sub></sub>  " +
		"Require delivery evidence before closing merged work**\n\nWhen a matching PR has merged but " +
		"deployment verification is still pending, this branch closes the bead using only the merge timestamp."
	// tstCodexP3 is the ADVISORY fixture. Advisory starts at P3: P2 is a
	// correctness finding and must never auto-resolve.
	tstCodexP3 = "**<sub><sub>![P3 Badge](https://img.shields.io/badge/P3-blue?style=flat)</sub></sub>  " +
		"Prefer a table here**\n\nA nit about presentation, not correctness."
	tstCodexP2 = "**<sub><sub>![P2 Badge](https://img.shields.io/badge/P2-yellow?style=flat)</sub></sub>  " +
		"Fail closed when reconciliation is unavailable**\n\nIf this call transiently fails, the code skips it."
	tstGeminiHigh   = "![high](https://www.gstatic.com/codereviewagent/high-priority.svg)\n\nThis will not build on Windows."
	tstGeminiMedium = "![medium](https://www.gstatic.com/codereviewagent/medium-priority.svg)\n\nAdd a precondition here."
	tstUnparseable  = "I think this could be structured a little differently, but up to you."
)

// tstPosted and tstReplied bracket the fixtures in time. GitHub always returns
// createdAt for a review comment, so fixtures carry one too: engagement is
// judged on time, and a comment with no timestamp fails closed by design.
var (
	tstPosted  = time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	tstReplied = time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
)

func botComment(body string) threadComment {
	return threadComment{author: "chatgpt-codex-connector", body: body, typename: "Bot", createdAt: tstPosted}
}

// humanReply is a person answering AFTER the bot fixtures above.
func humanReply(login, body string) threadComment {
	return threadComment{author: login, body: body, typename: "User", createdAt: tstReplied}
}

func TestPartitionResolvable(t *testing.T) {
	tests := []struct {
		name         string
		thread       reviewThread
		wantResolved int
		wantWithheld int
	}{
		{
			// The exact #989 case. This must never resolve again.
			name:         "P1 bot thread is withheld",
			thread:       reviewThread{id: "t1", comments: []threadComment{botComment(tstCodexP1)}},
			wantResolved: 0, wantWithheld: 1,
		},
		{
			name:         "P2 bot thread resolves",
			thread:       reviewThread{id: "t2", comments: []threadComment{botComment(tstCodexP3)}},
			wantResolved: 1, wantWithheld: 0,
		},
		{
			name:         "gemini high is withheld",
			thread:       reviewThread{id: "t3", comments: []threadComment{{author: "gemini-code-assist", body: tstGeminiHigh, typename: "Bot"}}},
			wantResolved: 0, wantWithheld: 1,
		},
		{
			name:         "gemini medium resolves",
			thread:       reviewThread{id: "t4", comments: []threadComment{{author: "gemini-code-assist", body: tstGeminiMedium, typename: "Bot"}}},
			wantResolved: 1, wantWithheld: 0,
		},
		{
			// Fail closed: an unrecognised marker must not be resolved.
			name:         "unparseable severity is withheld",
			thread:       reviewThread{id: "t5", comments: []threadComment{botComment(tstUnparseable)}},
			wantResolved: 0, wantWithheld: 1,
		},
		{
			// MLC-05 preserved: human threads are neither resolved nor counted.
			name: "human-authored thread is never resolved",
			thread: reviewThread{id: "t6", comments: []threadComment{
				{author: "vbonnet", body: tstCodexP2, typename: "User"},
			}},
			wantResolved: 0, wantWithheld: 0,
		},
		{
			// MLC-05 preserved: a human reply anywhere protects the thread.
			name: "bot thread with a human reply is never resolved",
			thread: reviewThread{id: "t7", comments: []threadComment{
				botComment(tstCodexP2), humanReply("vbonnet", "disagree, keep it"),
			}},
			wantResolved: 0, wantWithheld: 0,
		},
		{
			name: "already-resolved thread is skipped",
			thread: reviewThread{id: "t8", isResolved: true,
				comments: []threadComment{botComment(tstCodexP2)}},
			wantResolved: 0, wantWithheld: 0,
		},
		{
			name: "truncated thread is never resolved",
			thread: reviewThread{id: "t9", truncated: true,
				comments: []threadComment{botComment(tstCodexP2)}},
			wantResolved: 0, wantWithheld: 0,
		},
		{
			// A P2 follow-up must not downgrade a P1 opener.
			name: "mixed P1 and P2 in one thread is withheld",
			thread: reviewThread{id: "t10", comments: []threadComment{
				botComment(tstCodexP1), botComment(tstCodexP2),
			}},
			wantResolved: 0, wantWithheld: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolvable, withheld := partitionResolvable([]reviewThread{tc.thread})
			if len(resolvable) != tc.wantResolved {
				t.Errorf("resolvable = %d, want %d", len(resolvable), tc.wantResolved)
			}
			if withheld != tc.wantWithheld {
				t.Errorf("withheld = %d, want %d", withheld, tc.wantWithheld)
			}
		})
	}
}

func TestBlockingFindingsIn(t *testing.T) {
	tests := []struct {
		name   string
		thread reviewThread
		want   int
	}{
		{
			// The core of the independent gate: a P1 that something already
			// resolved is still reported, because GitHub's own gate is now
			// blind to it. This is what catches a resolver bug.
			name: "resolved P1 still blocks the merge",
			thread: reviewThread{id: "b1", isResolved: true,
				comments: []threadComment{botComment(tstCodexP1)}},
			want: 1,
		},
		{
			name:   "unresolved P1 blocks",
			thread: reviewThread{id: "b2", comments: []threadComment{botComment(tstCodexP1)}},
			want:   1,
		},
		{
			name:   "gemini high blocks",
			thread: reviewThread{id: "b3", comments: []threadComment{{author: "gemini-code-assist", body: tstGeminiHigh, typename: "Bot"}}},
			want:   1,
		},
		{
			name:   "P2 does not block",
			thread: reviewThread{id: "b4", comments: []threadComment{botComment(tstCodexP3)}},
			want:   0,
		},
		{
			// Unknown severity is handled by GitHub's gate while the thread is
			// open. Blocking here too would deadlock on ordinary bot prose.
			name:   "unparseable bot prose does not block",
			thread: reviewThread{id: "b5", comments: []threadComment{botComment(tstUnparseable)}},
			want:   0,
		},
		{
			// A person engaged with the finding. Not this gate's call to
			// override them.
			name: "P1 with a human reply is treated as addressed",
			thread: reviewThread{id: "b6", comments: []threadComment{
				botComment(tstCodexP1), humanReply("vbonnet", "fixed in a follow-up"),
			}},
			want: 0,
		},
		{
			name:   "human-only thread does not block",
			thread: reviewThread{id: "b7", comments: []threadComment{{author: "vbonnet", body: tstCodexP1, typename: "User"}}},
			want:   0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := blockingFindingsIn([]reviewThread{tc.thread})
			if len(got) != tc.want {
				t.Fatalf("blockingFindingsIn() = %d findings, want %d (%+v)", len(got), tc.want, got)
			}
		})
	}
}

// TestBlockingFindingCarriesExcerpt pins that the audit record says WHAT is
// blocking. "1 finding blocks this merge" with no detail is the kind of opaque
// record that made the original incident hard to see.
func TestBlockingFindingCarriesExcerpt(t *testing.T) {
	got := blockingFindingsIn([]reviewThread{
		{id: "b8", comments: []threadComment{botComment(tstCodexP1)}},
	})
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d", len(got))
	}
	if got[0].Excerpt == "" || got[0].Excerpt == "(no excerpt)" {
		t.Errorf("Excerpt = %q, want the finding title", got[0].Excerpt)
	}
	if !strings.Contains(got[0].Excerpt, "Require delivery evidence") {
		t.Errorf("Excerpt = %q, want it to carry the finding title", got[0].Excerpt)
	}
	if got[0].Author != "chatgpt-codex-connector" {
		t.Errorf("Author = %q", got[0].Author)
	}
}

// TestFullIncidentScenario replays PR #989's real thread mix end to end.
func TestFullIncidentScenario(t *testing.T) {
	threads := []reviewThread{
		{id: "p1a", comments: []threadComment{botComment(tstCodexP1)}},
		{id: "p1b", comments: []threadComment{botComment(tstCodexP1)}},
		{id: "p1c", comments: []threadComment{botComment(tstCodexP1)}},
		{id: "p1d", comments: []threadComment{botComment(tstCodexP1)}},
		{id: "p2a", comments: []threadComment{botComment(tstCodexP3)}},
	}
	resolvable, withheld := partitionResolvable(threads)
	if len(resolvable) != 1 || resolvable[0].id != "p2a" {
		t.Errorf("resolvable = %+v, want only the P2 thread", resolvable)
	}
	if withheld != 4 {
		t.Errorf("withheld = %d, want 4 (the P1s)", withheld)
	}
	if n := len(blockingFindingsIn(threads)); n != 4 {
		t.Errorf("blocking findings = %d, want 4: the merge must be refused", n)
	}
}

// Bot findings routinely contain non-ASCII prose. Truncating the excerpt on a
// byte index can split a multi-byte rune, so the audit record would carry
// invalid UTF-8 and render as a replacement character.
func TestExcerptFindingTruncatesOnRuneBoundaries(t *testing.T) {
	// One ASCII byte before 3-byte runes puts byte offset 120 inside a rune,
	// so a byte slice there produces invalid UTF-8.
	title := "x" + strings.Repeat("→", 200)
	body := "![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat)\n\n**" + title + "**\n"

	got := excerptFinding([]threadComment{{body: body}})

	if !utf8.ValidString(got) {
		t.Fatalf("excerptFinding returned invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("excerptFinding did not truncate a 200-rune title: %q", got)
	}
	if n := utf8.RuneCountInString(strings.TrimSuffix(got, "...")); n != 120 {
		t.Errorf("excerptFinding truncated to %d runes, want 120", n)
	}
}

// A short non-ASCII finding is returned whole.
func TestExcerptFindingKeepsShortNonASCIITitle(t *testing.T) {
	title := "Réfuser les chemins non canoniques"
	body := "![P1 Badge](https://img.shields.io/badge/P1-orange?style=flat)\n\n**" + title + "**\n"

	if got := excerptFinding([]threadComment{{body: body}}); got != title {
		t.Errorf("excerptFinding = %q, want %q", got, title)
	}
}

// TestIsHumanActor pins the ce-lr7j review finding that "human" was inferred
// from absence in a two-login bot allowlist, so any non-allowlisted automation
// account (dependabot[bot], a GitHub Actions bot, a newly introduced review
// bot) silently cleared a P1 finding from the merge gate.
func TestIsHumanActor(t *testing.T) {
	tests := []struct {
		name     string
		typename string
		login    string
		want     bool
	}{
		{"real person", "User", "vbonnet", true},
		{"allowlisted bot", "Bot", "chatgpt-codex-connector", false},
		{"non-allowlisted bot is not human", "Bot", "dependabot[bot]", false},
		{"github actions bot is not human", "Bot", "github-actions[bot]", false},
		{"unknown future review bot is not human", "Bot", "some-new-review-bot", false},
		{"organization actor is not human", "Organization", "vbonnet-org", false},
		{"missing actor type fails closed", "", "vbonnet", false},
		{"bot login claiming User still fails", "User", "chatgpt-codex-connector", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHumanActor(tt.typename, tt.login); got != tt.want {
				t.Errorf("isHumanActor(%q, %q) = %v, want %v", tt.typename, tt.login, got, tt.want)
			}
		})
	}
}

// TestBlockingFindingsInBotReplyDoesNotClearFinding is the end-to-end shape of
// the same defect: a bot reply on a P1 thread must not make it look addressed.
func TestBlockingFindingsInBotReplyDoesNotClearFinding(t *testing.T) {
	threads := []reviewThread{{
		id: "x1",
		comments: []threadComment{
			botComment(tstCodexP1),
			{author: "dependabot[bot]", body: "Bumped the dep.", typename: "Bot"},
		},
	}}
	if got := blockingFindingsIn(threads); len(got) != 1 {
		t.Fatalf("blockingFindingsIn() = %d findings, want 1 (a bot reply must not clear a P1)", len(got))
	}
}

// TestBlockingFindingsInResolvedThreadsFailClosed pins the second ce-lr7j
// review round: once a thread is RESOLVED, GitHub's conversation-resolution
// gate has nothing left to hold, so this gate is the last reader. An
// unreadable or unrecognised resolved thread must refuse the merge.
func TestBlockingFindingsInResolvedThreadsFailClosed(t *testing.T) {
	t.Run("resolved unknown severity blocks", func(t *testing.T) {
		threads := []reviewThread{{
			id: "r1", isResolved: true,
			comments: []threadComment{botComment(tstUnparseable)},
		}}
		if got := blockingFindingsIn(threads); len(got) != 1 {
			t.Fatalf("got %d findings, want 1 (resolved unknown must block)", len(got))
		}
	})
	t.Run("unresolved unknown severity does not deadlock", func(t *testing.T) {
		threads := []reviewThread{{
			id: "r2", isResolved: false,
			comments: []threadComment{botComment(tstUnparseable)},
		}}
		if got := blockingFindingsIn(threads); len(got) != 0 {
			t.Fatalf("got %d findings, want 0 (GitHub already holds unresolved threads)", len(got))
		}
	})
	t.Run("resolved truncated thread blocks even if visible page is advisory", func(t *testing.T) {
		threads := []reviewThread{{
			id: "r3", isResolved: true, truncated: true,
			comments: []threadComment{botComment(tstCodexP3)},
		}}
		got := blockingFindingsIn(threads)
		if len(got) != 1 {
			t.Fatalf("got %d findings, want 1 (truncated resolved thread must refuse)", len(got))
		}
	})
	t.Run("resolved advisory thread still merges", func(t *testing.T) {
		threads := []reviewThread{{
			id: "r4", isResolved: true,
			comments: []threadComment{botComment(tstCodexP3)},
		}}
		if got := blockingFindingsIn(threads); len(got) != 0 {
			t.Fatalf("got %d findings, want 0", len(got))
		}
	})
}

// TestThreadsRemainingUnresolved pins the ce-lr7j review finding that
// `mergeloop threads` printed "merge gate: PASS" for a PR safe-merge would
// refuse. blockingFindingsIn is only the CUSTOM bot-finding gate; a thread that
// is unresolved and not auto-resolvable (a human thread, or a bot thread whose
// severity this code does not recognise) yields no finding, yet GitHub's
// required_conversation_resolution — which safe-merge also enforces — still
// blocks the merge. This mode is advertised as reporting whether the merge
// would be refused, so a false PASS misdirects live verification.
func TestThreadsRemainingUnresolved(t *testing.T) {
	human := humanReply("alice", "please rename this")
	threads := []reviewThread{
		// Resolvable: bot, unresolved, recognised advisory. Goes away.
		{id: "advisory", comments: []threadComment{botComment(tstCodexP3)}},
		// Already resolved: nothing left to hold.
		{id: "done", isResolved: true, comments: []threadComment{botComment(tstCodexP3)}},
		// Unresolved human thread: never auto-resolved, still blocks the merge.
		{id: "human", comments: []threadComment{human}},
		// Unresolved bot thread of unrecognised severity: withheld, still blocks.
		{id: "unknown", comments: []threadComment{botComment(tstUnparseable)}},
	}

	resolvable, _ := partitionResolvable(threads)
	remaining := threadsRemainingUnresolved(threads, resolvable)

	got := map[string]bool{}
	for _, id := range remaining {
		got[id] = true
	}
	for _, want := range []string{"human", "unknown"} {
		if !got[want] {
			t.Errorf("thread %q must be reported as still-unresolved; got %v", want, remaining)
		}
	}
	for _, unwanted := range []string{"advisory", "done"} {
		if got[unwanted] {
			t.Errorf("thread %q must not be reported as still-unresolved; got %v", unwanted, remaining)
		}
	}

	// The custom bot-finding gate alone sees nothing here, which is exactly why
	// reporting only that gate as "merge gate: PASS" was wrong.
	if f := blockingFindingsIn(threads); len(f) != 0 {
		t.Fatalf("precondition: want no blocking bot findings, got %d", len(f))
	}
}

// TestVerifyPRIdentity pins the ce-lr7j review finding that a mistyped --pr was
// falsely certified as passing.
//
// GitHub's `pullRequest(number:)` field is nullable. A number that does not
// exist returns null, which unmarshals into the zero-valued response and looks
// exactly like a real PR with no review threads. `mergeloop threads` then
// printed zero threads and PASS for both gates and the overall verdict, which
// is the worst possible answer to "would this merge be refused".
func TestVerifyPRIdentity(t *testing.T) {
	if err := verifyPRIdentity(0, 1429, "vbonnet", "dear-agent"); err == nil {
		t.Error("a null pullRequest (number 0) must be an error, not an empty thread list")
	} else if !strings.Contains(err.Error(), "1429") {
		t.Errorf("error should name the requested PR, got %q", err)
	}
	if err := verifyPRIdentity(1429, 1429, "vbonnet", "dear-agent"); err != nil {
		t.Errorf("a matching identity must pass, got %v", err)
	}
	// A response for some other PR is also not the PR that was asked for.
	if err := verifyPRIdentity(1430, 1429, "vbonnet", "dear-agent"); err == nil {
		t.Error("a mismatched identity must be an error")
	}
}

// TestBlockingFindingsInHumanBeforeBotP1DoesNotAddressIt pins the ce-lr7j
// review finding that a human comment addressed findings posted AFTER it.
//
// hasHumanComment asked only whether a person appears anywhere in the thread,
// so a bot posting a P1 after an unrelated human remark made the whole thread
// look engaged and the finding was dropped. Once such a thread is resolved,
// GitHub's conversation gate cannot catch the omission either, so a green PR
// merges over a blocking finding nobody read. Only a human comment that comes
// after the finding can have addressed it.
func TestBlockingFindingsInHumanBeforeBotP1DoesNotAddressIt(t *testing.T) {
	earlier := threadComment{author: "alice", body: "unrelated remark", typename: "User",
		createdAt: tstPosted.Add(-time.Hour)}
	later := humanReply("alice", "thanks, fixed")

	before := reviewThread{id: "human-first", isResolved: true,
		comments: []threadComment{earlier, botComment(tstCodexP1)}}
	if got := blockingFindingsIn([]reviewThread{before}); len(got) != 1 {
		t.Errorf("a P1 posted AFTER a human comment must still block; got %d findings", len(got))
	}

	after := reviewThread{id: "human-last", isResolved: true,
		comments: []threadComment{botComment(tstCodexP1), later}}
	if got := blockingFindingsIn([]reviewThread{after}); len(got) != 0 {
		t.Errorf("a human reply AFTER the P1 addresses it; got %d findings", len(got))
	}

	// A bot reply between the finding and the human does not break the pairing.
	interleaved := reviewThread{id: "interleaved", isResolved: true,
		comments: []threadComment{earlier, botComment(tstCodexP1), botComment(tstCodexP2), later}}
	if got := blockingFindingsIn([]reviewThread{interleaved}); len(got) != 0 {
		t.Errorf("a human reply after the P1 addresses it even with a bot comment between; got %d", len(got))
	}
}

// TestBlockingFindingsInResolvedTruncatedRefusesDespiteVisibleHuman pins the
// ce-lr7j review finding that the thread-level human shortcut was ordered
// BEFORE the truncation safeguard.
//
// A resolved thread with more comments than one page, whose visible first page
// happens to contain a human, took the shortcut and never reached the
// fail-closed truncation branch. A blocking bot finding could sit in the unseen
// page while GitHub's conversation gate, already satisfied by the resolution,
// protected nothing. Partial-page engagement is not evidence about the part
// nobody read.
func TestBlockingFindingsInResolvedTruncatedRefusesDespiteVisibleHuman(t *testing.T) {
	human := humanReply("alice", "looks fine to me")
	thread := reviewThread{
		id: "resolved-truncated", isResolved: true, truncated: true,
		comments: []threadComment{botComment(tstCodexP2), human},
	}
	if got := blockingFindingsIn([]reviewThread{thread}); len(got) != 1 {
		t.Errorf("a resolved TRUNCATED thread must refuse even with a human on the visible page; got %d", len(got))
	}
	// An unresolved truncated thread is still held by GitHub, so it stays out.
	unresolved := thread
	unresolved.isResolved = false
	if got := blockingFindingsIn([]reviewThread{unresolved}); len(got) != 0 {
		t.Errorf("an UNRESOLVED truncated thread is held by GitHub's gate; got %d findings", len(got))
	}
}

// TestBlockingFindingsInResolvedUnknownAfterHumanStillRefuses pins the ce-lr7j
// review finding that the per-comment ordering fix covered only RECOGNISED
// blocking markers.
//
// unaddressedBlockingComment ignores unknown severities, so a resolved thread
// carrying an earlier human comment followed by a bot finding this parser
// cannot read took the blanket shortcut and never reached the resolved-unknown
// safeguard. An unreadable finding nobody answered is exactly the case that
// must fail closed.
func TestBlockingFindingsInResolvedUnknownAfterHumanStillRefuses(t *testing.T) {
	human := threadComment{author: "alice", body: "unrelated remark", typename: "User",
		createdAt: tstPosted.Add(-time.Hour)}
	unreadable := botComment("**<sub><sub>![P7 Badge](https://img.shields.io/badge/P7-orange?style=flat)</sub></sub>  Future format**")

	afterHuman := reviewThread{id: "resolved-unknown", isResolved: true,
		comments: []threadComment{human, unreadable}}
	if got := blockingFindingsIn([]reviewThread{afterHuman}); len(got) != 1 {
		t.Errorf("a resolved unknown-severity bot finding posted AFTER a human must refuse; got %d", len(got))
	}

	// A human reply AFTER the unreadable finding does address it.
	answered := reviewThread{id: "answered-unknown", isResolved: true,
		comments: []threadComment{unreadable, humanReply("alice", "acknowledged")}}
	if got := blockingFindingsIn([]reviewThread{answered}); len(got) != 0 {
		t.Errorf("a human reply after the unknown finding addresses it; got %d findings", len(got))
	}
}

// TestExcerptFindingReadsUnknownSeverityComments pins the ce-lr7j review
// finding that an unknown-severity finding always excerpted as "(no excerpt)".
//
// excerptFinding skipped every comment that did not classify as
// SeverityBlocking, but the resolved-unknown path exists precisely for
// findings this parser cannot classify. The durable escalation therefore
// described nothing.
func TestExcerptFindingReadsUnknownSeverityComments(t *testing.T) {
	unreadable := botComment("**<sub><sub>![P7 Badge](https://img.shields.io/badge/P7-orange?style=flat)</sub></sub>  " +
		"Withhold merges when the ledger cannot be read**\n\nDetail follows.")
	got := excerptFinding([]threadComment{unreadable})
	if got == "(no excerpt)" {
		t.Error("excerptFinding() = (no excerpt) for an unknown-severity finding; the escalation " +
			"must say what is blocking, not merely that something is")
	}
	if !strings.Contains(got, "Withhold merges") {
		t.Errorf("excerptFinding() = %q, want the finding title", got)
	}
}

// TestUnaddressedBlockingCommentUsesTimeNotPosition pins the ce-lr7j review
// finding that an EDITED bot comment defeated the ordering check.
//
// The per-comment rule asked whether a human appears later in the slice. A bot
// that edits an earlier advisory comment into a P1 after a human has already
// replied keeps its original position, so that human still sits in
// comments[i+1:] and was read as engagement with a finding that did not exist
// when they wrote. Once the thread is resolved, GitHub's gate protects nothing.
//
// Position is a proxy for time, and editing breaks it. The human response must
// post after the finding's LATEST revision.
func TestUnaddressedBlockingCommentUsesTimeNotPosition(t *testing.T) {
	at := func(s string) time.Time {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatalf("bad fixture time %q: %v", s, err)
		}
		return ts
	}
	edited := botComment(tstCodexP1)
	edited.createdAt = at("2026-06-15T10:00:00Z")
	edited.lastEditedAt = at("2026-06-15T12:00:00Z") // became a P1 only here
	human := threadComment{author: "alice", body: "looks fine", typename: "User",
		createdAt: at("2026-06-15T11:00:00Z")} // replied before the edit

	if _, ok := unaddressedBlockingComment([]threadComment{edited, human}); !ok {
		t.Error("a human reply that predates the edit which INTRODUCED the P1 does not address it")
	}

	// A human replying after the latest revision does address it.
	later := human
	later.createdAt = at("2026-06-15T13:00:00Z")
	if _, ok := unaddressedBlockingComment([]threadComment{edited, later}); ok {
		t.Error("a human reply after the latest revision addresses the finding")
	}

	// Missing timestamps must fail closed rather than fall back to position.
	noTimes := botComment(tstCodexP1)
	blank := threadComment{author: "alice", body: "ok", typename: "User"}
	if _, ok := unaddressedBlockingComment([]threadComment{noTimes, blank}); !ok {
		t.Error("without timestamps the gate cannot prove the reply came after the finding; it must refuse")
	}
}

// TestThreadResolvabilityIsTheSingleRule pins the ce-lr7j review finding that
// the auto-resolve decision was made once, when the thread list was read, and
// then acted on later against a thread that may have changed.
//
// The dangerous case is a human replying in that window: the mutation would
// silently close a person's newly posted disagreement, and the merge gate would
// then see an addressed, already-resolved thread and let the merge through.
// That is the MLC-05 violation this whole change exists to prevent.
//
// The fix is to re-read the thread and re-decide immediately before mutating,
// so this predicate is the one rule both decisions use and they cannot drift.
func TestThreadResolvabilityIsTheSingleRule(t *testing.T) {
	advisory := reviewThread{id: "t", comments: []threadComment{botComment(tstCodexP3)}}
	if got := threadResolvability(advisory); got != resolvabilityEligible {
		t.Errorf("an unresolved advisory bot thread = %v, want eligible", got)
	}

	// A human replies in the window: no longer ours to touch.
	humanJoined := advisory
	humanJoined.comments = append(append([]threadComment{}, advisory.comments...),
		humanReply("alice", "actually, please keep this open"))
	if got := threadResolvability(humanJoined); got != resolvabilityNotOurs {
		t.Errorf("a thread a person has joined = %v, want notOurs", got)
	}

	// The bot edits its advisory comment into a P1 in the window: withheld.
	escalated := reviewThread{id: "t", comments: []threadComment{botComment(tstCodexP1)}}
	if got := threadResolvability(escalated); got != resolvabilityWithheld {
		t.Errorf("a thread that became blocking = %v, want withheld", got)
	}

	// Someone else resolved it, or it grew past one page: not ours either way.
	already := advisory
	already.isResolved = true
	if got := threadResolvability(already); got != resolvabilityNotOurs {
		t.Errorf("an already-resolved thread = %v, want notOurs", got)
	}
	long := advisory
	long.truncated = true
	if got := threadResolvability(long); got != resolvabilityNotOurs {
		t.Errorf("a thread this code cannot read in full = %v, want notOurs", got)
	}
}

// TestBlockingFindingsInUnknownBotStillBlocks pins the ce-lr7j review finding
// that the independent gate only recognised findings from the two allowlisted
// logins.
//
// A renamed or newly introduced review bot posting a P1 in an already-resolved
// thread produced no finding at all, and GitHub's conversation gate was already
// satisfied, so the merge proceeded over an unread blocking finding.
//
// The allowlist governs what may be AUTO-RESOLVED, which is a privilege and
// must stay narrow. What may BLOCK is the opposite question and must be wide:
// any actor that is not a real person can carry a finding this gate owns.
func TestBlockingFindingsInUnknownBotStillBlocks(t *testing.T) {
	newBot := threadComment{author: "some-new-reviewer[bot]", body: tstCodexP1,
		typename: "Bot", createdAt: tstPosted}
	thread := reviewThread{id: "new-bot", isResolved: true, comments: []threadComment{newBot}}
	if got := blockingFindingsIn([]reviewThread{thread}); len(got) != 1 {
		t.Errorf("a P1 from an unallowlisted BOT must still block; got %d findings", len(got))
	}

	// An actor the API could not type is not provably a person either.
	untyped := newBot
	untyped.typename = ""
	if got := blockingFindingsIn([]reviewThread{{id: "untyped", isResolved: true,
		comments: []threadComment{untyped}}}); len(got) != 1 {
		t.Errorf("a P1 from an untyped actor must block; got %d findings", len(got))
	}

	// A real person writing badge-shaped text is still not a bot finding: that
	// is ordinary human feedback GitHub's own gate governs.
	person := threadComment{author: "alice", body: tstCodexP1, typename: "User", createdAt: tstPosted}
	if got := blockingFindingsIn([]reviewThread{{id: "human", isResolved: true,
		comments: []threadComment{person}}}); len(got) != 0 {
		t.Errorf("a person quoting P1 text is not a bot finding; got %d findings", len(got))
	}

	// Auto-resolution stays restricted to the allowlist: an unknown bot's
	// advisory thread is NOT ours to resolve.
	advisoryNew := threadComment{author: "some-new-reviewer[bot]", body: tstCodexP3,
		typename: "Bot", createdAt: tstPosted}
	if got := threadResolvability(reviewThread{id: "x", comments: []threadComment{advisoryNew}}); got != resolvabilityNotOurs {
		t.Errorf("an unallowlisted bot's thread = %v, want notOurs (auto-resolve stays narrow)", got)
	}
}
