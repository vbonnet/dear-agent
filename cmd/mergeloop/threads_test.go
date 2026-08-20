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
