package output

import (
	"strings"
	"testing"
	"time"
)

func TestFormatNextFeature_Empty_AllComplete(t *testing.T) {
	got := formatNextFeature("", 0)
	if !strings.Contains(got, "complete") && !strings.Contains(got, "Complete") {
		t.Errorf("formatNextFeature(\"\",0) = %q, want 'all complete' message", got)
	}
}

func TestFormatNextFeature_WithInProgress(t *testing.T) {
	got := formatNextFeature("F-42", 2)
	if !strings.Contains(got, "F-42") {
		t.Errorf("formatNextFeature(F-42,2) = %q, want to contain feature ID", got)
	}
}

func TestFormatNextFeature_NotStarted(t *testing.T) {
	got := formatNextFeature("F-01", 0)
	if !strings.Contains(got, "F-01") {
		t.Errorf("formatNextFeature(F-01,0) = %q, want to contain feature ID", got)
	}
}

func TestFormatTime_JustNow(t *testing.T) {
	got := formatTime(time.Now().Add(-5 * time.Second))
	if got != "just now" {
		t.Errorf("formatTime(5s ago) = %q, want 'just now'", got)
	}
}

func TestFormatTime_Minutes(t *testing.T) {
	got := formatTime(time.Now().Add(-10 * time.Minute))
	if !strings.Contains(got, "min") {
		t.Errorf("formatTime(10m ago) = %q, want 'N min ago'", got)
	}
}

func TestFormatTime_Hours(t *testing.T) {
	got := formatTime(time.Now().Add(-3 * time.Hour))
	if !strings.Contains(got, "hours") {
		t.Errorf("formatTime(3h ago) = %q, want 'N hours ago'", got)
	}
}

func TestFormatTime_OldDate(t *testing.T) {
	old := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	got := formatTime(old)
	if !strings.Contains(got, "2024") {
		t.Errorf("formatTime(old) = %q, want date format", got)
	}
}
