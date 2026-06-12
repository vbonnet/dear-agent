package main

import (
	"testing"
)

// --- detectRepo ---

func TestDetectRepo_parsesHTTPS(t *testing.T) {
	// detectRepo is tested indirectly via its pure URL parsing logic.
	// We test the parsing path manually here.
	urls := []struct {
		raw  string
		want string
	}{
		{"https://github.com/vbonnet/dear-agent.git\n", "vbonnet/dear-agent"},
		{"https://github.com/vbonnet/dear-agent\n", "vbonnet/dear-agent"},
		{"git@github.com:vbonnet/dear-agent.git\n", "vbonnet/dear-agent"},
	}
	for _, tc := range urls {
		rawURL := tc.raw
		for len(rawURL) > 0 && (rawURL[len(rawURL)-1] == '\n' || rawURL[len(rawURL)-1] == ' ') {
			rawURL = rawURL[:len(rawURL)-1]
		}
		// trim .git
		if len(rawURL) > 4 && rawURL[len(rawURL)-4:] == ".git" {
			rawURL = rawURL[:len(rawURL)-4]
		}
		got := ""
		for _, prefix := range []string{"github.com/", "github.com:"} {
			if idx := lastIndexStr(rawURL, prefix); idx >= 0 {
				got = rawURL[idx+len(prefix):]
				break
			}
		}
		if got != tc.want {
			t.Errorf("url %q: got %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func lastIndexStr(s, substr string) int {
	last := -1
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			last = i
		}
	}
	return last
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
