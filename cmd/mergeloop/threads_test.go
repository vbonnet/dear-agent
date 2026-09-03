package main

import (
	"strings"
	"testing"
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
	tstCodexP2 = "**<sub><sub>![P2 Badge](https://img.shields.io/badge/P2-yellow?style=flat)</sub></sub>  " +
		"Fail closed when reconciliation is unavailable**\n\nIf this call transiently fails, the code skips it."
	tstGeminiHigh   = "![high](https://www.gstatic.com/codereviewagent/high-priority.svg)\n\nThis will not build on Windows."
	tstGeminiMedium = "![medium](https://www.gstatic.com/codereviewagent/medium-priority.svg)\n\nAdd a precondition here."
	tstUnparseable  = "I think this could be structured a little differently, but up to you."
)

func botComment(body string) threadComment {
	return threadComment{author: "chatgpt-codex-connector", body: body}
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
			thread:       reviewThread{id: "t2", comments: []threadComment{botComment(tstCodexP2)}},
			wantResolved: 1, wantWithheld: 0,
		},
		{
			name:         "gemini high is withheld",
			thread:       reviewThread{id: "t3", comments: []threadComment{{author: "gemini-code-assist", body: tstGeminiHigh}}},
			wantResolved: 0, wantWithheld: 1,
		},
		{
			name:         "gemini medium resolves",
			thread:       reviewThread{id: "t4", comments: []threadComment{{author: "gemini-code-assist", body: tstGeminiMedium}}},
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
				{author: "vbonnet", body: tstCodexP2},
			}},
			wantResolved: 0, wantWithheld: 0,
		},
		{
			// MLC-05 preserved: a human reply anywhere protects the thread.
			name: "bot thread with a human reply is never resolved",
			thread: reviewThread{id: "t7", comments: []threadComment{
				botComment(tstCodexP2), {author: "vbonnet", body: "disagree, keep it"},
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
			thread: reviewThread{id: "b3", comments: []threadComment{{author: "gemini-code-assist", body: tstGeminiHigh}}},
			want:   1,
		},
		{
			name:   "P2 does not block",
			thread: reviewThread{id: "b4", comments: []threadComment{botComment(tstCodexP2)}},
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
				botComment(tstCodexP1), {author: "vbonnet", body: "fixed in a follow-up"},
			}},
			want: 0,
		},
		{
			name:   "human-only thread does not block",
			thread: reviewThread{id: "b7", comments: []threadComment{{author: "vbonnet", body: tstCodexP1}}},
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
		{id: "p2a", comments: []threadComment{botComment(tstCodexP2)}},
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
