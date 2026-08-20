package main

import (
	"strings"
	"testing"
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
			comments := make([]threadComment, len(c.logins))
			for i, l := range c.logins {
				comments[i] = threadComment{login: l, body: "a comment"}
			}
			if got := allCommentsFromKnownBots(comments); got != c.want {
				t.Errorf("allCommentsFromKnownBots(%v) = %v, want %v", c.logins, got, c.want)
			}
		})
	}
}

// TestAllCommentsFromKnownBots_NoticeKeepsEligibility pins the retry path: the
// auto-resolve notice is posted by the authenticated gh user, not a bot, so a
// naive author check would strand any thread whose notice landed but whose
// resolution failed. It would never be selected again and would block required
// conversation resolution forever.
func TestAllCommentsFromKnownBots_NoticeKeepsEligibility(t *testing.T) {
	withNotice := []threadComment{
		{login: "chatgpt-codex-connector", body: "P1: fix this"},
		{login: "vbonnet", body: autoResolveNotice},
	}
	if !allCommentsFromKnownBots(withNotice) {
		t.Error("a thread carrying only mergeloop's own notice must stay eligible for retry")
	}
	if !hasAutoResolveNotice(withNotice) {
		t.Error("hasAutoResolveNotice did not recognise the notice")
	}

	// A real human reply still disqualifies the thread.
	withHuman := []threadComment{
		{login: "chatgpt-codex-connector", body: "P1: fix this"},
		{login: "vbonnet", body: "Actually this is wrong because..."},
	}
	if allCommentsFromKnownBots(withHuman) {
		t.Error("a human reply must still disqualify the thread")
	}
	if hasAutoResolveNotice(withHuman) {
		t.Error("a human reply was mistaken for the auto-resolve notice")
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

// TestAutoResolveNoticeIsHonest pins the wording of the notice mergeloop posts
// before auto-resolving. The whole point is that a reader can tell an
// auto-resolved thread from one a person actually handled, so the notice must
// say it was not reviewed and must offer a way back.
func TestAutoResolveNoticeIsHonest(t *testing.T) {
	for _, want := range []string{"Auto-resolved by mergeloop", "NOT reviewed by a person", "reopen"} {
		if !strings.Contains(autoResolveNotice, want) {
			t.Errorf("auto-resolve notice missing %q, got: %s", want, autoResolveNotice)
		}
	}
}

// TestThreadReplyMutationUsesCorrectInputField guards the one-character trap
// between the two mutations: resolveReviewThread takes threadId while
// addPullRequestReviewThreadReply takes pullRequestReviewThreadId.
func TestThreadReplyMutationUsesCorrectInputField(t *testing.T) {
	if !strings.Contains(threadReplyMutation, "pullRequestReviewThreadId:$threadId") {
		t.Errorf("reply mutation must use pullRequestReviewThreadId, got: %s", threadReplyMutation)
	}
	if !strings.Contains(threadResolveMutation, "threadId:$threadId") {
		t.Errorf("resolve mutation must use threadId, got: %s", threadResolveMutation)
	}
}
