package router

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeCommandRunner struct {
	name string
	args []string
	out  []byte
	err  error
}

func (f *fakeCommandRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.name = name
	f.args = append([]string(nil), args...)
	return f.out, f.err
}

func TestParseCodexBarDashboard(t *testing.T) {
	data := []byte(`{
		"generatedAt": "2026-08-10T06:00:00Z",
		"providers": [
			{
				"id": "anthropic",
				"family": "anthropic",
				"account": "redacted",
				"windows": [
					{"name": "weekly", "remainingPercent": 7.5, "usedPercent": 92.5, "resetAt": "2026-08-14T00:00:00Z"},
					{"name": "daily", "remainingPercent": 41.0, "usedPercent": 59.0, "resetAt": "2026-08-11T00:00:00Z"}
				]
			},
			{
				"id": "openai",
				"account": "redacted",
				"windows": [
					{"name": "monthly", "remainingPercent": 68.0, "usedPercent": 32.0}
				]
			}
		]
	}`)

	snapshot, err := ParseCodexBarDashboard(data, time.Time{})
	if err != nil {
		t.Fatalf("ParseCodexBarDashboard: %v", err)
	}
	if snapshot.Source != "codexbar" {
		t.Fatalf("source = %q, want codexbar", snapshot.Source)
	}
	if got := snapshot.Generated.Format(time.RFC3339); got != "2026-08-10T06:00:00Z" {
		t.Fatalf("generated = %s", got)
	}
	if len(snapshot.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(snapshot.Providers))
	}
	if snapshot.Providers[0].Family != "anthropic" || len(snapshot.Providers[0].Windows) != 2 {
		t.Fatalf("anthropic provider = %#v", snapshot.Providers[0])
	}
	if snapshot.Providers[1].Family != "openai" {
		t.Fatalf("provider without family should fall back to id, got %#v", snapshot.Providers[1])
	}
}

// TestParseCodexBarDashboardCanonicalizesAliases guards against CodexBar's
// own labels ("claude", "codex", "google") never matching the router's
// resolved families ("anthropic", "openai", "gemini"), which would make
// EvaluateProviderQuota's exact family match silently ignore the provider's
// quota (codex review on #1197, ADR-038 alias table).
func TestParseCodexBarDashboardCanonicalizesAliases(t *testing.T) {
	data := []byte(`{
		"providers": [
			{"id": "claude", "windows": [{"name": "daily", "remainingPercent": 40}]},
			{"family": "codex", "windows": [{"name": "daily", "remainingPercent": 40}]},
			{"id": "google", "windows": [{"name": "daily", "remainingPercent": 40}]}
		]
	}`)

	snapshot, err := ParseCodexBarDashboard(data, time.Time{})
	if err != nil {
		t.Fatalf("ParseCodexBarDashboard: %v", err)
	}
	want := []string{"anthropic", "openai", "gemini"}
	for i, w := range want {
		if snapshot.Providers[i].Family != w {
			t.Fatalf("provider[%d].Family = %q, want %q", i, snapshot.Providers[i].Family, w)
		}
	}
}

// TestParseCodexBarDashboardSkipsWindowsWithoutRemainingPercent guards
// against a missing/null remainingPercent silently unmarshalling to zero,
// which would read as exhausted quota and could avoid a provider even
// though the ADR requires unavailable data not to affect availability
// (codex review on #1197).
func TestParseCodexBarDashboardSkipsWindowsWithoutRemainingPercent(t *testing.T) {
	data := []byte(`{
		"providers": [
			{"id": "anthropic", "windows": [
				{"name": "daily", "usedPercent": 10},
				{"name": "weekly", "remainingPercent": 55, "usedPercent": 45}
			]}
		]
	}`)

	snapshot, err := ParseCodexBarDashboard(data, time.Time{})
	if err != nil {
		t.Fatalf("ParseCodexBarDashboard: %v", err)
	}
	windows := snapshot.Providers[0].Windows
	if len(windows) != 1 {
		t.Fatalf("windows = %#v, want only the window with an explicit remainingPercent", windows)
	}
	if windows[0].Name != "weekly" {
		t.Fatalf("windows[0] = %#v, want the weekly window", windows[0])
	}
}

func TestEvaluateProviderQuotaAvoidsMostConstrainedWindow(t *testing.T) {
	snapshot := &QuotaSnapshot{
		Generated: time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC),
		Providers: []ProviderQuota{{
			Family: "anthropic",
			Windows: []QuotaWindow{
				{Name: "daily", RemainingPercent: 35},
				{Name: "weekly", RemainingPercent: 6},
			},
		}},
	}

	decision := EvaluateProviderQuota(snapshot, "anthropic", "", snapshot.Generated.Add(time.Minute), QuotaPolicy{
		AvoidBelowRemainingPercent:        10,
		DeprioritizeBelowRemainingPercent: 25,
		MaxSnapshotAge:                    time.Hour,
	})
	if !decision.Avoid || decision.Deprioritize {
		t.Fatalf("decision = %#v, want avoid only", decision)
	}
	if decision.MinRemaining != 6 {
		t.Fatalf("min remaining = %v, want 6", decision.MinRemaining)
	}
	if !strings.Contains(decision.Reason, "avoid threshold") {
		t.Fatalf("reason = %q, want avoid threshold", decision.Reason)
	}
}

func TestEvaluateProviderQuotaDeprioritizes(t *testing.T) {
	snapshot := &QuotaSnapshot{
		Generated: time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC),
		Providers: []ProviderQuota{{
			Family:  "gemini",
			Windows: []QuotaWindow{{Name: "daily", RemainingPercent: 18}},
		}},
	}

	decision := EvaluateProviderQuota(snapshot, "gemini", "", snapshot.Generated, QuotaPolicy{
		AvoidBelowRemainingPercent:        10,
		DeprioritizeBelowRemainingPercent: 25,
	})
	if decision.Avoid || !decision.Deprioritize {
		t.Fatalf("decision = %#v, want deprioritize only", decision)
	}
}

func TestEvaluateProviderQuotaStaleSnapshotDoesNotAvoid(t *testing.T) {
	snapshot := &QuotaSnapshot{
		Generated: time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC),
		Providers: []ProviderQuota{{
			Family:  "openai",
			Windows: []QuotaWindow{{Name: "daily", RemainingPercent: 1}},
		}},
	}

	decision := EvaluateProviderQuota(snapshot, "openai", "", snapshot.Generated.Add(3*time.Hour), QuotaPolicy{
		AvoidBelowRemainingPercent: 10,
		MaxSnapshotAge:             time.Hour,
	})
	if decision.Avoid || !decision.SnapshotStale {
		t.Fatalf("decision = %#v, want stale without avoid", decision)
	}
}

// TestEvaluateProviderQuotaZeroGeneratedIsStale guards against a zero
// Generated timestamp bypassing freshness enforcement: its age cannot be
// established, so it must be treated as stale rather than allowed to
// influence routing indefinitely (codex review on #1197).
func TestEvaluateProviderQuotaZeroGeneratedIsStale(t *testing.T) {
	snapshot := &QuotaSnapshot{
		Providers: []ProviderQuota{{
			Family:  "openai",
			Windows: []QuotaWindow{{Name: "daily", RemainingPercent: 1}},
		}},
	}

	decision := EvaluateProviderQuota(snapshot, "openai", "", time.Now(), QuotaPolicy{
		AvoidBelowRemainingPercent: 10,
		MaxSnapshotAge:             time.Hour,
	})
	if decision.Avoid || !decision.SnapshotStale {
		t.Fatalf("decision = %#v, want stale without avoid", decision)
	}
}

// TestEvaluateProviderQuotaSkipsExpiredWindow guards against an exhausted
// window whose reset time has already passed continuing to drag the
// provider's minimum down after its quota period actually reset (codex
// review on #1197).
func TestEvaluateProviderQuotaSkipsExpiredWindow(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	snapshot := &QuotaSnapshot{
		Generated: now,
		Providers: []ProviderQuota{{
			Family: "anthropic",
			Windows: []QuotaWindow{
				// Exhausted, but its reset time is in the past: expired.
				{Name: "daily", RemainingPercent: 0, ResetAt: now.Add(-time.Hour)},
				{Name: "weekly", RemainingPercent: 60, ResetAt: now.Add(6 * 24 * time.Hour)},
			},
		}},
	}

	decision := EvaluateProviderQuota(snapshot, "anthropic", "", now, QuotaPolicy{
		AvoidBelowRemainingPercent: 10,
	})
	if decision.Avoid {
		t.Fatalf("decision = %#v, want no avoid once the exhausted window has reset", decision)
	}
	if decision.MinRemaining != 60 {
		t.Fatalf("min remaining = %v, want 60 (expired window excluded)", decision.MinRemaining)
	}
}

// TestEvaluateProviderQuotaScopesToAccount guards against an exhausted
// inactive account under the same family causing Avoid for the healthy
// account actually routed to (codex review on #1197).
func TestEvaluateProviderQuotaScopesToAccount(t *testing.T) {
	snapshot := &QuotaSnapshot{
		Generated: time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC),
		Providers: []ProviderQuota{
			{
				Family:  "anthropic",
				Account: "work",
				Windows: []QuotaWindow{{Name: "daily", RemainingPercent: 1}},
			},
			{
				Family:  "anthropic",
				Account: "personal",
				Windows: []QuotaWindow{{Name: "daily", RemainingPercent: 80}},
			},
		},
	}

	decision := EvaluateProviderQuota(snapshot, "anthropic", "personal", snapshot.Generated, QuotaPolicy{
		AvoidBelowRemainingPercent: 10,
	})
	if decision.Avoid {
		t.Fatalf("decision = %#v, want no avoid for the healthy scoped account", decision)
	}
	if decision.MinRemaining != 80 {
		t.Fatalf("min remaining = %v, want 80 (exhausted other account excluded)", decision.MinRemaining)
	}
}

func TestCodexBarQuotaReaderUsesRedactedDashboardCommand(t *testing.T) {
	runner := &fakeCommandRunner{out: []byte(`{
		"generatedAt": "2026-08-10T06:00:00Z",
		"providers": [{"id": "anthropic", "windows": [{"name": "daily", "remainingPercent": 50}]}]
	}`)}
	reader := CodexBarQuotaReader{
		Command: "codexbar-test",
		Runner:  runner,
		Timeout: time.Second,
	}

	snapshot, err := reader.ReadQuota(context.Background())
	if err != nil {
		t.Fatalf("ReadQuota: %v", err)
	}
	if runner.name != "codexbar-test" {
		t.Fatalf("command = %q, want codexbar-test", runner.name)
	}
	wantArgs := []string{"dashboard", "--identity", "redacted"}
	if len(runner.args) != len(wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.args, wantArgs)
	}
	for i := range wantArgs {
		if runner.args[i] != wantArgs[i] {
			t.Fatalf("args = %#v, want %#v", runner.args, wantArgs)
		}
	}
	if snapshot.Providers[0].Family != "anthropic" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestCodexBarQuotaReaderWrapsCommandErrors(t *testing.T) {
	reader := CodexBarQuotaReader{
		Runner: &fakeCommandRunner{err: errors.New("missing binary")},
	}
	_, err := reader.ReadQuota(context.Background())
	if err == nil || !strings.Contains(err.Error(), "read dashboard") {
		t.Fatalf("err = %v, want wrapped command error", err)
	}
}
