package tmux

import "testing"

func TestShouldClearStaleInput(t *testing.T) {
	tests := []struct {
		name    string
		ansi    string
		want    bool
		comment string
	}{
		{
			name:    "stale agm command in input box",
			ansi:    "some output\n❯ merge PR 527\n",
			want:    true,
			comment: "un-submitted AGM text must be stashed, not treated as human typing",
		},
		{
			name:    "another stale command",
			ansi:    "❯ pkill -x gopls\n",
			want:    true,
			comment: "the classic ghost-text deadlock string",
		},
		{
			name:    "empty input box",
			ansi:    "previous response\n❯ \n",
			want:    false,
			comment: "nothing to clear",
		},
		{
			name:    "no prompt at all",
			ansi:    "just some output\n",
			want:    false,
			comment: "no input line to clear",
		},
		{
			name:    "ghost/placeholder text is left alone",
			ansi:    "❯ \x1b[2mstart the loop\x1b[0m\n",
			want:    false,
			comment: "dim ghost text has nothing real to stash and does not block submit",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldClearStaleInput(tc.ansi); got != tc.want {
				t.Errorf("shouldClearStaleInput() = %v, want %v (%s)", got, tc.want, tc.comment)
			}
		})
	}
}

func TestStashHarnessToggle(t *testing.T) {
	t.Cleanup(func() { SetStashHarness("") })

	if StashHarness() != "" {
		t.Fatalf("stash harness should default to empty, got %q", StashHarness())
	}
	SetStashHarness("codex-cli")
	if StashHarness() != "codex-cli" {
		t.Fatalf("stash harness = %q, want codex-cli", StashHarness())
	}
	SetStashHarness("")
	if StashHarness() != "" {
		t.Fatalf("stash harness should reset to empty, got %q", StashHarness())
	}
}

func TestNormalizeStashHarness(t *testing.T) {
	cases := map[string]string{
		"":            "claude-code",
		"claude":      "claude-code",
		"claude-code": "claude-code",
		"codex":       "codex-cli",
		"codex-cli":   "codex-cli",
		"agy":         "agy",
		"antigravity": "agy",
		"opencode":    "opencode-cli",
		"weird-thing": "weird-thing",
	}
	for in, want := range cases {
		if got := normalizeStashHarness(in); got != want {
			t.Errorf("normalizeStashHarness(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStashKeyForHarness(t *testing.T) {
	// Only Claude Code's stash key is verified today; every other harness falls
	// back to C-s as a best-effort mapping flagged unverified in telemetry.
	cases := []struct {
		harness      string
		wantKey      string
		wantVerified bool
	}{
		{"claude-code", "C-s", true},
		{"claude", "C-s", true},
		{"", "C-s", true},
		{"codex-cli", "C-s", false},
		{"agy", "C-s", false},
		{"opencode-cli", "C-s", false},
		{"something-new", "C-s", false},
	}
	for _, tc := range cases {
		key, verified := stashKeyForHarness(tc.harness)
		if key != tc.wantKey || verified != tc.wantVerified {
			t.Errorf("stashKeyForHarness(%q) = (%q,%v), want (%q,%v)",
				tc.harness, key, verified, tc.wantKey, tc.wantVerified)
		}
	}
}

func TestInputLineAfterPrompt(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"content after prompt", "out\n❯ merge PR 527\n", "merge PR 527"},
		{"empty prompt", "out\n❯ \n", ""},
		{"no prompt", "just output\n", ""},
		{"last prompt wins", "❯ old\n❯ new text\n", "new text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inputLineAfterPrompt(tc.in); got != tc.want {
				t.Errorf("inputLineAfterPrompt() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStashStaleInputLockedNoOp verifies the stash is a no-op (Attempted=false)
// when there is nothing real to stash — an empty composer or dim ghost text —
// regardless of harness. This path never touches tmux.
func TestStashStaleInputLockedNoOp(t *testing.T) {
	t.Cleanup(func() { SetStashHarness("") })
	SetStashHarness("claude-code")

	for _, ansi := range []string{
		"previous response\n❯ \n",          // empty composer
		"❯ \x1b[2mstart the loop\x1b[0m\n", // ghost text
		"no prompt here\n",                 // no composer
		"⠋ Thinking...\n❯ some AI output",  // active spinner — AI output, not stashable
	} {
		out := stashStaleInputLocked("/nonexistent.sock", "no-such-session", ansi)
		if out.Attempted {
			t.Errorf("stash should be a no-op for %q, got Attempted=true", ansi)
		}
		if out.Sent || out.Cleared || out.FalsePositive {
			t.Errorf("no-op stash must not set Sent/Cleared/FalsePositive for %q: %+v", ansi, out)
		}
	}
}

// TestStashStaleInputLockedAttemptedButSendFails verifies that when there IS
// real stale text, the stash is attempted, and that a failed send (bogus socket)
// leaves Sent=false without misreporting a stash or a false positive.
func TestStashStaleInputLockedAttemptedButSendFails(t *testing.T) {
	t.Cleanup(func() { SetStashHarness("") })
	SetStashHarness("claude-code")

	out := stashStaleInputLocked("/nonexistent.sock", "no-such-session", "❯ merge PR 527\n")
	if !out.Attempted {
		t.Fatal("stash should be attempted when real stale text is present")
	}
	if out.Sent {
		t.Error("send to a nonexistent socket must not report Sent=true")
	}
	if out.Cleared || out.FalsePositive {
		t.Errorf("a failed send must not classify the outcome: %+v", out)
	}
	if out.Harness != "claude-code" || !out.Verified {
		t.Errorf("outcome harness/verified wrong: %+v", out)
	}
}
