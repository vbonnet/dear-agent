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

	decision := EvaluateProviderQuota(snapshot, "anthropic", snapshot.Generated.Add(time.Minute), QuotaPolicy{
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

	decision := EvaluateProviderQuota(snapshot, "gemini", snapshot.Generated, QuotaPolicy{
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

	decision := EvaluateProviderQuota(snapshot, "openai", snapshot.Generated.Add(3*time.Hour), QuotaPolicy{
		AvoidBelowRemainingPercent: 10,
		MaxSnapshotAge:             time.Hour,
	})
	if decision.Avoid || !decision.SnapshotStale {
		t.Fatalf("decision = %#v, want stale without avoid", decision)
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
