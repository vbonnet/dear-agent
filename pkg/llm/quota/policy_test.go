package quota_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

func snapshotAt(generated time.Time, providers ...quota.ProviderQuota) *quota.Snapshot {
	return &quota.Snapshot{Source: "test", GeneratedAt: generated, Providers: providers}
}

func readable(family string, windows ...quota.Window) quota.ProviderQuota {
	return quota.ProviderQuota{
		Family:       family,
		SourceID:     family,
		Availability: quota.AvailabilityOK,
		Windows:      windows,
	}
}

func window(label string, remaining float64) quota.Window {
	return quota.Window{ID: label, Label: label, RemainingPercent: remaining, UsedPercent: 100 - remaining}
}

func TestEvaluateClassifiesAgainstThresholds(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		remaining float64
		want      quota.Class
	}{
		{name: "plenty left", remaining: 90, want: quota.ClassHealthy},
		{name: "just above the deprioritize floor", remaining: 25.1, want: quota.ClassHealthy},
		{name: "on the deprioritize floor", remaining: 25, want: quota.ClassDeprioritized},
		{name: "below the deprioritize floor", remaining: 10, want: quota.ClassDeprioritized},
		{name: "on the avoid floor", remaining: 5, want: quota.ClassAvoid},
		{name: "spent", remaining: 0, want: quota.ClassAvoid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snap := snapshotAt(now, readable("openai", window("Weekly", tc.remaining)))
			got := quota.Evaluate(snap, "openai", now, quota.Policy{})
			if got.Class != tc.want {
				t.Errorf("Class = %q, want %q (reason: %s)", got.Class, tc.want, got.Reason)
			}
			if got.RemainingPercent != tc.remaining {
				t.Errorf("RemainingPercent = %.1f, want %.1f", got.RemainingPercent, tc.remaining)
			}
		})
	}
}

func TestEvaluateUsesMostConstrainedWindow(t *testing.T) {
	now := time.Now()
	snap := snapshotAt(now, readable("openai",
		window("Codex Spark Weekly", 100),
		window("Weekly", 20),
		window("Session", 60),
	))
	got := quota.Evaluate(snap, "openai", now, quota.Policy{})
	if got.RemainingPercent != 20 {
		t.Errorf("RemainingPercent = %.1f, want 20", got.RemainingPercent)
	}
	if got.ConstrainedWindow != "Weekly" {
		t.Errorf("ConstrainedWindow = %q, want Weekly", got.ConstrainedWindow)
	}
	if got.Class != quota.ClassDeprioritized {
		t.Errorf("Class = %q, want deprioritized", got.Class)
	}
}

// The fail-safe contract: every way of not knowing produces ClassUnknown,
// never ClassAvoid. A provider nobody can read must keep routing.
func TestEvaluateNeverAvoidsOnAMissingReading(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		snapshot *quota.Snapshot
		family   string
		policy   quota.Policy
		wantWord string
	}{
		{
			name:     "no snapshot at all",
			snapshot: nil,
			family:   "openai",
			wantWord: "no quota reading",
		},
		{
			name:     "family absent from the snapshot",
			snapshot: snapshotAt(now, readable("openai", window("Weekly", 50))),
			family:   "anthropic",
			wantWord: "no quota reading for family",
		},
		{
			name: "credentials missing",
			snapshot: snapshotAt(now, quota.ProviderQuota{
				Family:       "anthropic",
				Availability: quota.AvailabilityAuthRequired,
				Note:         "No Claude session key found in browser cookies.",
			}),
			family:   "anthropic",
			wantWord: "credentials needed",
		},
		{
			name: "provider switched off",
			snapshot: snapshotAt(now, quota.ProviderQuota{
				Family:       "gemini",
				Availability: quota.AvailabilityDisabled,
			}),
			family:   "gemini",
			wantWord: "disabled",
		},
		{
			name:     "reading past its max age",
			snapshot: snapshotAt(now.Add(-time.Hour), readable("openai", window("Weekly", 1))),
			family:   "openai",
			policy:   quota.Policy{MaxSnapshotAge: time.Minute},
			wantWord: "old",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := quota.Evaluate(tc.snapshot, tc.family, now, tc.policy)
			if got.Class != quota.ClassUnknown {
				t.Errorf("Class = %q, want unknown", got.Class)
			}
			if got.Known() {
				t.Error("Known() must be false without a usable reading")
			}
			if !strings.Contains(got.Reason, tc.wantWord) {
				t.Errorf("Reason = %q, want it to mention %q", got.Reason, tc.wantWord)
			}
		})
	}
}

// The acceptance criterion the meter exists for: an operator reading the
// output can tell a sign-in problem from a spent budget.
func TestEvaluateDistinguishesAuthFailureFromExhaustion(t *testing.T) {
	now := time.Now()
	authFailed := quota.Evaluate(snapshotAt(now, quota.ProviderQuota{
		Family:       "anthropic",
		Availability: quota.AvailabilityAuthRequired,
		Note:         "No Claude session key found in browser cookies.",
	}), "anthropic", now, quota.Policy{})
	exhausted := quota.Evaluate(snapshotAt(now,
		readable("openai", window("Weekly", 0))), "openai", now, quota.Policy{})

	if authFailed.Availability != quota.AvailabilityAuthRequired {
		t.Errorf("auth failure Availability = %q", authFailed.Availability)
	}
	if authFailed.Class == exhausted.Class {
		t.Errorf("auth failure and exhaustion share class %q", authFailed.Class)
	}
	if !strings.Contains(authFailed.Reason, "not exhaustion") {
		t.Errorf("auth failure reason must say it is not exhaustion, got %q", authFailed.Reason)
	}
	if exhausted.Class != quota.ClassAvoid {
		t.Errorf("exhausted Class = %q, want avoid", exhausted.Class)
	}
}

func TestEvaluateStaleFlag(t *testing.T) {
	now := time.Now()
	got := quota.Evaluate(snapshotAt(now.Add(-2*time.Hour), readable("openai", window("Weekly", 50))),
		"openai", now, quota.Policy{MaxSnapshotAge: time.Minute})
	if !got.Stale {
		t.Error("want Stale on a reading past its max age")
	}
}

func TestEvaluateNegativeMaxAgeAcceptsAnyAge(t *testing.T) {
	now := time.Now()
	got := quota.Evaluate(snapshotAt(now.Add(-30*24*time.Hour), readable("openai", window("Weekly", 50))),
		"openai", now, quota.Policy{MaxSnapshotAge: -1})
	if got.Class != quota.ClassHealthy {
		t.Errorf("Class = %q, want healthy when staleness is disabled", got.Class)
	}
}

func TestEvaluateUndatedSnapshotIsNotStale(t *testing.T) {
	now := time.Now()
	got := quota.Evaluate(snapshotAt(time.Time{}, readable("openai", window("Weekly", 50))),
		"openai", now, quota.Policy{MaxSnapshotAge: time.Minute})
	if got.Class != quota.ClassHealthy {
		t.Errorf("Class = %q, want healthy for an undated reading", got.Class)
	}
}

func TestBandOrdersByHeadroom(t *testing.T) {
	now := time.Now()
	band := func(remaining float64) int {
		snap := snapshotAt(now, readable("openai", window("Weekly", remaining)))
		return quota.Band(quota.Evaluate(snap, "openai", now, quota.Policy{}))
	}
	if got := band(99); got != 0 {
		t.Errorf("band(99) = %d, want 0", got)
	}
	if got := band(60); got != 1 {
		t.Errorf("band(60) = %d, want 1", got)
	}
	if got := band(30); got != 2 {
		t.Errorf("band(30) = %d, want 2", got)
	}
	if got := band(20); got != 3 {
		t.Errorf("band(20) = %d, want 3", got)
	}
	if got := band(1); got != 4 {
		t.Errorf("band(1) = %d, want 4", got)
	}
}

func TestBandPutsUnknownFirstSoRoutingIsUnchanged(t *testing.T) {
	if got := quota.Band(quota.Decision{Class: quota.ClassUnknown}); got != 0 {
		t.Errorf("Band(unknown) = %d, want 0", got)
	}
}

func TestBandHonoursTunedThresholdsOverQuartiles(t *testing.T) {
	now := time.Now()
	// 60% left sits in quartile band 1, but an operator who set the
	// deprioritize floor at 70% asked for a demotion.
	snap := snapshotAt(now, readable("openai", window("Weekly", 60)))
	decision := quota.Evaluate(snap, "openai", now, quota.Policy{DeprioritizeBelowRemainingPercent: 70})
	if decision.Class != quota.ClassDeprioritized {
		t.Fatalf("Class = %q, want deprioritized", decision.Class)
	}
	if got := quota.Band(decision); got != 3 {
		t.Errorf("Band = %d, want 3", got)
	}
}
