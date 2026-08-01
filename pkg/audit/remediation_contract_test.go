package audit

import (
	"context"
	"log/slog"
	"testing"
)

type contractRemediator struct {
	outcome ApplyOutcome
	calls   *int
}

func (r contractRemediator) Apply(context.Context, Finding, Env) (ApplyOutcome, error) {
	if r.calls != nil {
		(*r.calls)++
	}
	return r.outcome, nil
}

type recordingStateStore struct {
	Store
	calls int
	state FindingState
	note  string
}

func (s *recordingStateStore) SetFindingState(
	_ context.Context,
	findingID string,
	state FindingState,
	note string,
) (Finding, error) {
	s.calls++
	s.state = state
	s.note = note
	return Finding{FindingID: findingID, State: state}, nil
}

func TestRunnerRemediatorOutcomePersistenceBoundary(t *testing.T) {
	stored := Finding{
		FindingID: "finding-1",
		State:     FindingOpen,
		Suggested: Remediation{Strategy: StrategyAuto, Command: "unused"},
	}

	tests := []struct {
		name      string
		outcome   ApplyOutcome
		wantCalls int
		wantState FindingState
		wantNote  string
	}{
		{
			name: "unchanged state drops all descriptive fields",
			outcome: ApplyOutcome{
				Status: "applied", State: FindingOpen,
				Note: "not persisted", Reference: "artifact-ignored",
			},
		},
		{
			name: "invalid state drops all descriptive fields",
			outcome: ApplyOutcome{
				Status: "applied", State: FindingState("unknown"),
				Note: "not persisted", Reference: "artifact-ignored",
			},
		},
		{
			name: "changed state passes only state and note to store",
			outcome: ApplyOutcome{
				Status: "applied", State: FindingResolved,
				Note: "state transition note", Reference: "artifact-ignored",
			},
			wantCalls: 1,
			wantState: FindingResolved,
			wantNote:  "state transition note",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingStateStore{}
			runner := &Runner{
				Store:      store,
				Remediator: contractRemediator{outcome: tt.outcome},
			}

			runner.applyInlineRemediation(context.Background(), stored, Env{}, slog.Default())

			if store.calls != tt.wantCalls {
				t.Fatalf("SetFindingState calls = %d, want %d", store.calls, tt.wantCalls)
			}
			if store.state != tt.wantState {
				t.Errorf("state = %q, want %q", store.state, tt.wantState)
			}
			if store.note != tt.wantNote {
				t.Errorf("note = %q, want %q", store.note, tt.wantNote)
			}
		})
	}
}

func TestRunnerNoopRemediatorPreservesAcknowledgedAutoFinding(t *testing.T) {
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

func TestRunnerCustomRemediatorRemainsCallableForCompatibility(t *testing.T) {
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
	calls := 0
	runner.Remediator = contractRemediator{
		calls: &calls,
		outcome: ApplyOutcome{
			State: FindingResolved,
			Note:  "compatibility outcome",
		},
	}
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo", Checks: []ScheduledCheck{{CheckID: "demo"}}}},
	}

	report, err := runner.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 1 {
		t.Fatalf("custom Remediator.Apply calls = %d, want 1", calls)
	}
	got, err := store.GetFinding(context.Background(), report.CheckOutcomes[0].Findings[0].FindingID)
	if err != nil {
		t.Fatalf("GetFinding: %v", err)
	}
	if got.State != FindingResolved {
		t.Fatalf("finding state = %q, want %q", got.State, FindingResolved)
	}
}
