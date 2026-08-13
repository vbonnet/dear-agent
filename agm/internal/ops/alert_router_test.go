package ops

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestAlertRouterClassifiesCriticalAgentActionable(t *testing.T) {
	dir := t.TempDir()
	router := NewAlertRouter(&OpContext{Storage: &mockStorage{}})
	router.SetQueuePath(filepath.Join(dir, "alerts.jsonl"))

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind:       "checker",
		Source:     "auth-checker",
		Title:      "Claude Auth At Risk",
		Body:       "token family may expire",
		Subject:    "claude-auth",
		OccurredAt: time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Severity != AlertSeverityCritical {
		t.Fatalf("Severity = %q, want %q", rec.Severity, AlertSeverityCritical)
	}
	if rec.Actionability != AlertAgentActionable {
		t.Fatalf("Actionability = %q, want %q", rec.Actionability, AlertAgentActionable)
	}
	if rec.Status != AlertStatusQueued {
		t.Fatalf("Status = %q, want queued fallback with no live supervisor", rec.Status)
	}
}

func TestAlertRouterDedupesRepeatingChecker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alerts.jsonl")
	router := NewAlertRouter(&OpContext{Storage: &mockStorage{}})
	router.SetQueuePath(path)
	when := time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC)

	first, err := router.Route(context.Background(), AlertRequest{
		Kind:       "checker",
		Source:     "auth-checker",
		Title:      "Claude Auth At Risk",
		Subject:    "claude-auth",
		OccurredAt: when,
	})
	if err != nil {
		t.Fatalf("first Route() error = %v", err)
	}
	second, err := router.Route(context.Background(), AlertRequest{
		Kind:       "checker",
		Source:     "auth-checker",
		Title:      "Claude Auth At Risk",
		Subject:    "claude-auth",
		OccurredAt: when.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("second Route() error = %v", err)
	}
	if second.Status != AlertStatusSuppressed {
		t.Fatalf("second Status = %q, want %q", second.Status, AlertStatusSuppressed)
	}
	records, err := ReadAlertRecords(path, 10)
	if err != nil {
		t.Fatalf("ReadAlertRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want only unsuppressed durable alert", len(records))
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprints differ: %q vs %q", first.Fingerprint, second.Fingerprint)
	}
}

func TestAlertRouterClassifiesHumanOnlyWithoutPagingAgent(t *testing.T) {
	dir := t.TempDir()
	router := NewAlertRouter(&OpContext{Storage: &mockStorage{}})
	router.SetQueuePath(filepath.Join(dir, "alerts.jsonl"))

	rec, err := router.Route(context.Background(), AlertRequest{
		Kind:       "credential",
		Source:     "oauth-refresh",
		Title:      "Credential decision needed",
		Body:       "needs Valentin to approve a new OAuth consent screen",
		Subject:    "oauth",
		OccurredAt: time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if rec.Actionability != AlertHumanOnly {
		t.Fatalf("Actionability = %q, want %q", rec.Actionability, AlertHumanOnly)
	}
	if rec.Status != AlertStatusQueued {
		t.Fatalf("Status = %q, want queued because no human recipient is configured in test", rec.Status)
	}
}

func TestAlertRouterDiscoversDispatchSupervisor(t *testing.T) {
	router := NewAlertRouter(&OpContext{Storage: &mockStorage{sessions: []*manifest.Manifest{
		testManifest("worker-1", manifest.StateWorking, time.Now()),
		testManifest("Dispatch", manifest.StateReady, time.Now()),
	}}})

	if got := router.discoverSupervisor(); got != "Dispatch" {
		t.Fatalf("discoverSupervisor() = %q, want Dispatch", got)
	}
}
