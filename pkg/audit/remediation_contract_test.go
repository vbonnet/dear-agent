package audit

import (
	"context"
	"log/slog"
	"testing"
)

type contractRemediator struct {
	outcome ApplyOutcome
}

func (r contractRemediator) Apply(context.Context, Finding, Env) (ApplyOutcome, error) {
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

func TestRunnerRejectsCustomRemediatorBeforeStoreMutation(t *testing.T) {
	check := fakeCheck{id: "demo"}
	runner, store, _ := newTestRunner(t, check)
	runner.Remediator = contractRemediator{}

	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo", Checks: []ScheduledCheck{{CheckID: "demo"}}}},
	}
	_, err := runner.Run(context.Background(), plan)
	const want = "audit: Runner.Remediator must be NewNoopRemediator until remediation outcomes are durably persisted"
	if err == nil || err.Error() != want {
		t.Fatalf("Run error = %v, want %q", err, want)
	}
	if len(store.runs) != 0 || len(store.findings) != 0 {
		t.Fatalf("preflight mutated store: runs=%d findings=%d", len(store.runs), len(store.findings))
	}
}
