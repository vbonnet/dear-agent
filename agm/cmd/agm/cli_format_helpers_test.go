package main

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/budget"
	"github.com/vbonnet/dear-agent/agm/internal/verify"
)

// These tests cover the pure presentation/parsing helpers scattered across the
// agm CLI. They have no command-execution side effects, so they can be tested
// directly without a cobra harness or external services. (ce-6as.44)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{"shorter than max", "abc", 10, "abc"},
		{"equal to max", "abcde", 5, "abcde"},
		{"longer than max", "abcdefgh", 6, "abc..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.in, tt.maxLen); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestFormatState(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"READY", "✓ READY"},
		{"THINKING", "● THINKING"},
		{"PERMISSION_PROMPT", "? PERMISSION"},
		{"COMPACTING", "⟳ COMPACTING"},
		{"OFFLINE", "✗ OFFLINE"},
		{"SOMETHING_ELSE", "SOMETHING_ELSE"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := formatState(tt.in); got != tt.want {
				t.Errorf("formatState(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatUncommitted(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  string
	}{
		{"negative is unknown", -1, "unknown"},
		{"zero is clean", 0, "clean"},
		{"positive count", 3, "3 files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatUncommitted(tt.count); got != tt.want {
				t.Errorf("formatUncommitted(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}

func TestFormatBudget(t *testing.T) {
	tests := []struct {
		name string
		bs   *budget.Status
		want string
	}{
		{"nil status", nil, "—"},
		{"critical", &budget.Status{Level: budget.LevelCritical, PercentageUsed: 95}, "!! 95%"},
		{"warning", &budget.Status{Level: budget.LevelWarning, PercentageUsed: 80}, "! 80%"},
		{"ok", &budget.Status{Level: budget.LevelOK, PercentageUsed: 42}, "42%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatBudget(tt.bs); got != tt.want {
				t.Errorf("formatBudget(%+v) = %q, want %q", tt.bs, got, tt.want)
			}
		})
	}
}

func TestFormatWorktree(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/Users/x/worktrees/repo/branch", "worktree"},
		{"/Users/x/wf/repo/branch", "worktree"},
		{"/Users/x/src/repo", "main"},
		{"", "main"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := formatWorktree(tt.path); got != tt.want {
				t.Errorf("formatWorktree(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1234, "1,234"},
		{1000005, "1000,005"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatTokens(tt.n); got != tt.want {
				t.Errorf("formatTokens(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{1073741824, "1.0 GiB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatBytes(tt.bytes); got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"seconds", 2 * time.Second, "2.00s"},
		{"milliseconds", 5 * time.Millisecond, "5.00ms"},
		{"microseconds", 5 * time.Microsecond, "5.00us"},
		{"nanoseconds", 42 * time.Nanosecond, "42ns"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDuration(tt.d); got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestParseGoVersion(t *testing.T) {
	tests := []struct {
		in        string
		wantMajor int
		wantMinor int
	}{
		{"go1.26", 1, 26},
		{"1.26.4", 1, 26},
		{"go1.21.0", 1, 21},
		{"go1", 0, 0},     // fewer than 2 parts
		{"garbage", 0, 0}, // no dot
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			major, minor := parseGoVersion(tt.in)
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Errorf("parseGoVersion(%q) = (%d, %d), want (%d, %d)",
					tt.in, major, minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestTruncateMsg(t *testing.T) {
	tests := []struct {
		name   string
		msg    string
		maxLen int
		want   string
	}{
		{"first line only", "line1\nline2", 100, "line1"},
		{"under max", "short", 100, "short"},
		{"over max", "abcdefghij", 5, "abcde..."},
		{"newline then over max", "abcdefghij\nrest", 5, "abcde..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateMsg(tt.msg, tt.maxLen); got != tt.want {
				t.Errorf("truncateMsg(%q, %d) = %q, want %q", tt.msg, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestParseDurationSafe(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"not-a-duration", 0},
		{"1h", time.Hour},
		{"7d", 7 * 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := parseDurationSafe(tt.in); got != tt.want {
				t.Errorf("parseDurationSafe(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatList(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  string
	}{
		{"empty", nil, "none"},
		{"single", []string{"a"}, "a"},
		{"multiple", []string{"a", "b", "c"}, "a, b, c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatList(tt.items); got != tt.want {
				t.Errorf("formatList(%v) = %q, want %q", tt.items, got, tt.want)
			}
		})
	}
}

func TestFormatWindowDuration(t *testing.T) {
	tests := []struct {
		seconds int
		want    string
	}{
		{30, "0m"},
		{60, "1m"},
		{1800, "30m"},
		{3600, "1h"},
		{7200, "2h"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatWindowDuration(tt.seconds); got != tt.want {
				t.Errorf("formatWindowDuration(%d) = %q, want %q", tt.seconds, got, tt.want)
			}
		})
	}
}

func TestFormatCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{-1, "N/A"},
		{0, "0"},
		{7, "7"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatCount(tt.n); got != tt.want {
				t.Errorf("formatCount(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestParseExtraAssertion(t *testing.T) {
	t.Run("neg without glob", func(t *testing.T) {
		a, err := parseExtraAssertion("neg:go.temporal.io")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Type != verify.Negative || a.Pattern != "go.temporal.io" || a.GlobPattern != "" {
			t.Errorf("got %+v", a)
		}
	})

	t.Run("pos with glob", func(t *testing.T) {
		a, err := parseExtraAssertion("pos:broadcastFromMPC:*.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Type != verify.Positive || a.Pattern != "broadcastFromMPC" || a.GlobPattern != "*.go" {
			t.Errorf("got %+v", a)
		}
	})

	t.Run("dir-neg sets PathCheck", func(t *testing.T) {
		a, err := parseExtraAssertion("dir-neg:coordinator")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Type != verify.Negative || a.PathCheck != "coordinator" {
			t.Errorf("got %+v", a)
		}
	})

	t.Run("dir-pos sets PathCheck", func(t *testing.T) {
		a, err := parseExtraAssertion("dir-pos:pkg/vroom")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a.Type != verify.Positive || a.PathCheck != "pkg/vroom" {
			t.Errorf("got %+v", a)
		}
	})

	t.Run("missing value errors", func(t *testing.T) {
		if _, err := parseExtraAssertion("neg"); err == nil {
			t.Error("expected error for input without colon")
		}
	})

	t.Run("unknown type errors", func(t *testing.T) {
		if _, err := parseExtraAssertion("bogus:value"); err == nil {
			t.Error("expected error for unknown assertion type")
		}
	})
}
