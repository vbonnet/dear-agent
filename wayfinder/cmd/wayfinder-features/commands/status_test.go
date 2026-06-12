package commands

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/progress"
)

// --- formatStatus ---

func TestFormatStatus_Values(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{progress.StatusPassing, "passing"},
		{progress.StatusInProgress, "in_progress"},
		{progress.StatusFailing, "not started"},
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

// --- getStatusIcon ---

func TestGetStatusIcon_Values(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"passing", "✅"},
		{"in_progress", "🔄"},
		{"failing", "⏳"},
		{"", "⏳"},
	}
	for _, tc := range cases {
		got := getStatusIcon(tc.in)
		if got != tc.want {
			t.Errorf("getStatusIcon(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- formatTimestamp ---

func TestFormatTimestamp_JustNow(t *testing.T) {
	got := formatTimestamp(time.Now().Add(-5 * time.Second))
	if got != "(just now)" {
		t.Errorf("formatTimestamp(5s ago) = %q, want (just now)", got)
	}
}

func TestFormatTimestamp_MinutesAgo(t *testing.T) {
	got := formatTimestamp(time.Now().Add(-30 * time.Minute))
	if got == "(just now)" || got == "" {
		t.Errorf("formatTimestamp(30m ago) = %q, expected minutes-ago format", got)
	}
}

func TestFormatTimestamp_HoursAgo(t *testing.T) {
	got := formatTimestamp(time.Now().Add(-3 * time.Hour))
	if got == "(just now)" || got == "" {
		t.Errorf("formatTimestamp(3h ago) = %q, expected hours-ago format", got)
	}
}

func TestFormatTimestamp_OlderDate(t *testing.T) {
	old := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	got := formatTimestamp(old)
	if got != "(2025-01-15 10:30)" {
		t.Errorf("formatTimestamp(old) = %q, want (2025-01-15 10:30)", got)
	}
}
