package audit

import (
	"context"
	"os"
	"testing"
)

func TestRunnerAutoSuggestionRemainsInertData(t *testing.T) {
	marker := t.TempDir() + "/must-not-exist"
	suggestion := Remediation{
		Strategy: StrategyAuto,
		Command:  "touch " + marker,
	}
	check := fakeCheck{
		id: "demo",
		findings: []Finding{{
			Fingerprint: "auto-finding",
			Severity:    SeverityP0,
			Title:       "requires attention",
			Suggested:   suggestion,
		}},
	}
	runner, store, _ := newTestRunner(t, check)
	runner.IDGen = sequenceIDs("run-1", "finding-1")
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo", Checks: []ScheduledCheck{{CheckID: "demo"}}}},
	}

	report, err := runner.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	reported := report.CheckOutcomes[0].Findings[0]
	if reported.Suggested != suggestion {
		t.Fatalf("reported suggestion = %+v, want %+v", reported.Suggested, suggestion)
	}
	if reported.State != FindingOpen {
		t.Fatalf("reported finding state = %q, want %q", reported.State, FindingOpen)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("suggested command executed: marker stat error = %v", err)
	}
	stored, err := store.GetFinding(context.Background(), reported.FindingID)
	if err != nil {
		t.Fatalf("GetFinding: %v", err)
	}
	if stored.Suggested != suggestion || stored.State != FindingOpen {
		t.Fatalf("stored finding = %+v, want unchanged open suggestion", stored)
	}
}

func TestRunnerRerunPreservesAcknowledgedAutoFinding(t *testing.T) {
	check := fakeCheck{
		id: "demo",
		findings: []Finding{{
			Fingerprint: "auto-finding",
			Severity:    SeverityP0,
			Title:       "requires attention",
			Suggested: Remediation{
				Strategy: StrategyAuto,
				Command:  "unused",
			},
		}},
	}
	runner, store, _ := newTestRunner(t, check)
	runner.IDGen = sequenceIDs("run-1", "run-2")
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo", Checks: []ScheduledCheck{{CheckID: "demo"}}}},
	}
	first, err := runner.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	findingID := first.CheckOutcomes[0].Findings[0].FindingID
	if _, err := store.SetFindingState(context.Background(), findingID, FindingAcknowledged, "triaged"); err != nil {
		t.Fatalf("acknowledge finding: %v", err)
	}

	if _, err := runner.Run(context.Background(), plan); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	got, err := store.GetFinding(context.Background(), findingID)
	if err != nil {
		t.Fatalf("GetFinding: %v", err)
	}
	if got.State != FindingAcknowledged {
		t.Fatalf("finding state = %q, want %q", got.State, FindingAcknowledged)
	}
}
