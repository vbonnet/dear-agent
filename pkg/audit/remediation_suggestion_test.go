package audit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRunnerAutoSuggestionRemainsInertData(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
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

func TestRunnerDefaultsPayloadlessP1SuggestionToPR(t *testing.T) {
	check := fakeCheck{
		id: "demo",
		findings: []Finding{{
			Fingerprint: "default-pr",
			Severity:    SeverityP1,
			Title:       "requires investigation",
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
	if len(report.CheckOutcomes[0].Findings) != 1 {
		t.Fatalf("findings = %+v, want one", report.CheckOutcomes[0].Findings)
	}
	reported := report.CheckOutcomes[0].Findings[0]
	if reported.Suggested != (Remediation{Strategy: StrategyPR}) {
		t.Fatalf("reported suggestion = %+v, want payloadless PR", reported.Suggested)
	}
	stored, err := store.GetFinding(context.Background(), reported.FindingID)
	if err != nil {
		t.Fatalf("GetFinding: %v", err)
	}
	if stored.Suggested != reported.Suggested {
		t.Fatalf("stored suggestion = %+v, want %+v", stored.Suggested, reported.Suggested)
	}
}

func TestRunnerRejectsInvalidSeverityPolicyBeforeState(t *testing.T) {
	missingP1 := DefaultSeverityPolicy()
	delete(missingP1, SeverityP1)
	tests := map[string]map[Severity]SeverityRule{
		"empty supplied policy": {},
		"missing severity":      missingP1,
		"unknown severity": {
			Severity("PX"): {DefaultStrategy: StrategyPR},
		},
		"unknown default strategy": {
			SeverityP1: {DefaultStrategy: Strategy("future")},
		},
		"unspecified default strategy": {
			SeverityP1: {DefaultStrategy: StrategyUnspecified},
		},
	}
	for name, policy := range tests {
		t.Run(name, func(t *testing.T) {
			runner, memory, _ := newTestRunner(t)
			store := &observingFindingStore{Store: memory}
			runner.Store = store
			runner.IDGen = func() string {
				t.Fatal("IDGen called before invalid policy was rejected")
				return ""
			}
			plan := Plan{
				Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceDaily,
				Trees:          []TreePlan{{WorkingDir: "/tmp/demo"}},
				SeverityPolicy: policy,
			}

			report, err := runner.Run(context.Background(), plan)
			if err == nil {
				t.Fatal("Run accepted an invalid severity policy")
			}
			if report != nil {
				t.Fatalf("report = %+v, want nil on preflight failure", report)
			}
			if store.beginAuditRunCalls != 0 {
				t.Fatalf("BeginAuditRun calls = %d, want zero", store.beginAuditRunCalls)
			}
		})
	}
}

func TestRunnerRejectsUnknownSuggestionStrategy(t *testing.T) {
	check := fakeCheck{
		id: "demo",
		findings: []Finding{{
			Fingerprint: "unknown-strategy",
			Severity:    SeverityP1,
			Title:       "must be rejected",
			Suggested:   Remediation{Strategy: Strategy("future")},
		}},
	}
	runner, store, _ := newTestRunner(t, check)
	runner.IDGen = sequenceIDs("run-1")
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo", Checks: []ScheduledCheck{{CheckID: "demo"}}}},
	}

	report, err := runner.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.CheckOutcomes[0].Findings) != 0 {
		t.Fatalf("findings = %+v, want unknown strategy dropped", report.CheckOutcomes[0].Findings)
	}
	if report.AuditRun.State != AuditRunPartial || report.CheckOutcomes[0].Err == nil {
		t.Fatalf("run = %q, outcome error = %v; want partial with error", report.AuditRun.State, report.CheckOutcomes[0].Err)
	}
	stored, err := store.ListFindings(context.Background(), FindingFilter{Repo: "demo"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("stored findings = %+v, want none", stored)
	}
}

func TestRunnerFindingStoreFailureTransitionsPartial(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
	}{
		{name: "P2", severity: SeverityP2},
		{name: "P1", severity: SeverityP1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			check := fakeCheck{
				id: "demo",
				findings: []Finding{{
					Fingerprint: "store-failure",
					Severity:    tc.severity,
					Title:       "cannot persist",
				}},
			}
			runner, memory, _ := newTestRunner(t, check)
			store := &observingFindingStore{Store: memory, upsertFindingErr: errors.New("write failed")}
			runner.Store = store
			runner.IDGen = sequenceIDs("run-1")
			plan := Plan{
				Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceDaily,
				Trees: []TreePlan{{WorkingDir: "/tmp/demo", Checks: []ScheduledCheck{{CheckID: "demo"}}}},
			}

			report, err := runner.Run(context.Background(), plan)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if report.AuditRun.State != AuditRunPartial || report.CheckOutcomes[0].Err == nil {
				t.Fatalf("run = %q, outcome error = %v; want partial with store error", report.AuditRun.State, report.CheckOutcomes[0].Err)
			}
			if store.upsertFindingCalls != 1 {
				t.Fatalf("UpsertFinding calls = %d, want one", store.upsertFindingCalls)
			}
		})
	}
}

func TestRunnerPersistedFailRunFindingOutranksRejectedFinding(t *testing.T) {
	check := fakeCheck{
		id: "demo",
		findings: []Finding{
			{
				Fingerprint: "invalid-strategy",
				Severity:    SeverityP2,
				Title:       "rejected",
				Suggested:   Remediation{Strategy: Strategy("future")},
			},
			{
				Fingerprint: "valid-fail-run",
				Severity:    SeverityP1,
				Title:       "persisted",
			},
		},
	}
	runner, _, _ := newTestRunner(t, check)
	runner.IDGen = sequenceIDs("run-1")
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo", Checks: []ScheduledCheck{{CheckID: "demo"}}}},
	}

	report, err := runner.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	outcome := report.CheckOutcomes[0]
	if outcome.Err == nil {
		t.Fatal("outcome error is nil, want rejected-finding error")
	}
	if len(outcome.Findings) != 1 || outcome.Findings[0].Fingerprint != "valid-fail-run" {
		t.Fatalf("persisted findings = %+v, want only valid-fail-run", outcome.Findings)
	}
	if report.AuditRun.State != AuditRunFailed {
		t.Fatalf("run state = %q, want failed from persisted P1 finding", report.AuditRun.State)
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

type observingFindingStore struct {
	Store
	beginAuditRunCalls int
	upsertFindingCalls int
	upsertFindingErr   error
}

func (s *observingFindingStore) BeginAuditRun(ctx context.Context, record AuditRunRecord) error {
	s.beginAuditRunCalls++
	return s.Store.BeginAuditRun(ctx, record)
}

func (s *observingFindingStore) UpsertFinding(ctx context.Context, finding Finding) (Finding, error) {
	s.upsertFindingCalls++
	if s.upsertFindingErr != nil {
		return Finding{}, s.upsertFindingErr
	}
	return s.Store.UpsertFinding(ctx, finding)
}
