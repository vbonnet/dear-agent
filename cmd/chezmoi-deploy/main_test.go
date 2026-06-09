package main

import (
	"strings"
	"testing"
)

func TestAutoMessage(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{"single", []string{"dot_gitconfig"}, "chezmoi-deploy: update dot_gitconfig"},
		{"three", []string{"a", "b", "c"}, "chezmoi-deploy: update a, b, c"},
		{"overflow", []string{"a", "b", "c", "d", "e"}, "chezmoi-deploy: update a, b, c and 2 more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := autoMessage(tt.files); got != tt.want {
				t.Errorf("autoMessage(%v) = %q, want %q", tt.files, got, tt.want)
			}
		})
	}
}

func TestNonEmptyLines(t *testing.T) {
	got := nonEmptyLines("a\n\n  b  \n\nc\n")
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("nonEmptyLines = %v, want %v", got, want)
	}
	if n := len(nonEmptyLines("\n  \n")); n != 0 {
		t.Errorf("nonEmptyLines(blank) len = %d, want 0", n)
	}
}

// run must reject force-push flags before touching chezmoi or git.
func TestRunRejectsForce(t *testing.T) {
	for _, flag := range []string{"--force", "-f", "--force-with-lease"} {
		err := run([]string{flag})
		if err == nil || !strings.Contains(err.Error(), "safe pushes") {
			t.Errorf("run(%q) = %v, want safe-push refusal", flag, err)
		}
	}
}

func TestRunRejectsBadFlags(t *testing.T) {
	if err := run([]string{"-m"}); err == nil || !strings.Contains(err.Error(), "requires a commit message") {
		t.Errorf("run(-m) = %v, want missing-message error", err)
	}
	if err := run([]string{"--bogus"}); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("run(--bogus) = %v, want unknown-flag error", err)
	}
}
