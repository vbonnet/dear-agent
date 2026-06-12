package commands

import (
	"testing"
	"time"
)

func TestFormatStatus(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"passing", "passing"},
		{"in_progress", "in_progress"},
		{"not_started", "not started"},
		{"", "not started"},
		{"unknown", "not started"},
	}
	for _, tc := range cases {
		got := formatStatus(tc.in)
		if got != tc.want {
			t.Errorf("formatStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGetStatusIcon(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"passing", "✅"},
		{"in_progress", "🔄"},
		{"not started", "⏳"},
		{"", "⏳"},
	}
	for _, tc := range cases {
		got := getStatusIcon(tc.in)
		if got != tc.want {
			t.Errorf("getStatusIcon(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatTimestamp_JustNow(t *testing.T) {
	t.Parallel()
	got := formatTimestamp(time.Now())
	if got != "(just now)" {
		t.Errorf("formatTimestamp(now) = %q, want \"(just now)\"", got)
	}
}

func TestFormatTimestamp_Minutes(t *testing.T) {
	t.Parallel()
	ts := time.Now().Add(-5 * time.Minute)
	got := formatTimestamp(ts)
	if got != "(5 min ago)" {
		t.Errorf("formatTimestamp(5m ago) = %q, want \"(5 min ago)\"", got)
	}
}

func TestFormatTimestamp_Hours(t *testing.T) {
	t.Parallel()
	ts := time.Now().Add(-3 * time.Hour)
	got := formatTimestamp(ts)
	if got != "(3 hours ago)" {
		t.Errorf("formatTimestamp(3h ago) = %q, want \"(3 hours ago)\"", got)
	}
}

func TestFormatTimestamp_OldDate(t *testing.T) {
	t.Parallel()
	ts := time.Date(2025, 1, 15, 9, 30, 0, 0, time.UTC)
	got := formatTimestamp(ts)
	if got != "(2025-01-15 09:30)" {
		t.Errorf("formatTimestamp(old) = %q, want \"(2025-01-15 09:30)\"", got)
	}
}
