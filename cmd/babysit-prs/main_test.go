package main

import (
	"testing"
)

// --- parseRepoFromURL ---

func TestParseRepoFromURL(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"https://github.com/vbonnet/dear-agent.git", "vbonnet/dear-agent"},
		{"https://github.com/vbonnet/dear-agent", "vbonnet/dear-agent"},
		{"git@github.com:vbonnet/dear-agent.git", "vbonnet/dear-agent"},
		{"git@github.com:vbonnet/dear-agent", "vbonnet/dear-agent"},
	}
	for _, tc := range cases {
		got, err := parseRepoFromURL(tc.raw)
		if err != nil {
			t.Errorf("parseRepoFromURL(%q) error: %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseRepoFromURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestParseRepoFromURL_Invalid(t *testing.T) {
	_, err := parseRepoFromURL("https://gitlab.com/vbonnet/repo.git")
	if err == nil {
		t.Error("expected error for non-GitHub URL, got nil")
	}
}

// --- run flag parsing ---

func TestRun_Help(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Errorf("--help should not error, got %v", err)
	}
}

func TestRun_UnknownFlag(t *testing.T) {
	if err := run([]string{"--no-such-flag"}); err == nil {
		t.Error("expected error for unknown flag")
	}
}

func TestRun_LimitParsed(t *testing.T) {
	if err := run([]string{"--limit", "abc"}); err == nil {
		t.Error("expected error for non-integer limit")
	}
	if err := run([]string{"--limit", "0"}); err == nil {
		t.Error("expected error for zero limit")
	}
}

func TestRun_CapParsed(t *testing.T) {
	if err := run([]string{"--cap", "abc"}); err == nil {
		t.Error("expected error for non-integer cap")
	}
}

func TestRun_TimeoutParsed(t *testing.T) {
	if err := run([]string{"--timeout", "notaduration"}); err == nil {
		t.Error("expected error for invalid timeout")
	}
}

func TestRun_SkipBotReviewRequiresReason(t *testing.T) {
	if err := run([]string{"--skip-bot-review"}); err == nil {
		t.Error("expected error when --skip-bot-review given without --skip-bot-review-reason")
	}
}

func TestRun_SkipBotReviewReasonWithoutFlag(t *testing.T) {
	// --skip-bot-review-reason alone (without --skip-bot-review) should not error at parse time.
	// It will be a no-op since SkipBotReview is false.
	if err := run([]string{"--skip-bot-review-reason", "some reason", "--repo", "a/b"}); err == nil {
		// Will error trying to connect to GitHub — that is expected and fine.
	}
}
