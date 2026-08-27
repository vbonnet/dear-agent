package workflow

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestInvariant_Validate_ShapeChecks exercises the per-kind validation
// of one Invariant. Each subtest is one cell in the kind × required-
// field matrix so a regression in the schema fails the closest test.
func TestInvariant_Validate_ShapeChecks(t *testing.T) {
	tests := []struct {
		name    string
		inv     Invariant
		wantErr string // substring; "" means must succeed
	}{
		{
			name:    "id empty",
			inv:     Invariant{Description: "x", Kind: InvariantPredicate, Predicate: "p"},
			wantErr: "id is required",
		},
		{
			name:    "id bad chars",
			inv:     Invariant{ID: "Bad_ID", Description: "x", Kind: InvariantPredicate, Predicate: "p"},
			wantErr: "id must match",
		},
		{
			name:    "description empty",
			inv:     Invariant{ID: "ok", Description: "  ", Kind: InvariantPredicate, Predicate: "p"},
			wantErr: "description is required",
		},
		{
			name:    "json_schema missing target",
			inv:     Invariant{ID: "ok", Description: "x", Kind: InvariantJSONSchema, Schema: "s.json"},
			wantErr: "requires target and schema",
		},
		{
			name:    "regex_match invalid pattern",
			inv:     Invariant{ID: "ok", Description: "x", Kind: InvariantRegexMatch, Target: "outputs.x", Pattern: "[unclosed"},
			wantErr: "invalid pattern",
		},
		{
			name:    "confidence_floor min out of range",
			inv:     Invariant{ID: "ok", Description: "x", Kind: InvariantConfidenceFloor, Target: "outputs.x", Min: 1.5},
			wantErr: "min must be in (0,1]",
		},
		{
			name:    "predicate missing text",
			inv:     Invariant{ID: "ok", Description: "x", Kind: InvariantPredicate},
			wantErr: "requires predicate",
		},
		{
			name:    "unknown kind",
			inv:     Invariant{ID: "ok", Description: "x", Kind: "made-up"},
			wantErr: "unknown kind",
		},
		{
			name: "valid json_schema",
			inv:  Invariant{ID: "ok", Description: "x", Kind: InvariantJSONSchema, Target: "outputs.report", Schema: "s.json"},
		},
		{
			name: "valid regex_match",
			inv:  Invariant{ID: "ok", Description: "x", Kind: InvariantRegexMatch, Target: "outputs.x", Pattern: "^ok$"},
		},
		{
			name: "valid confidence_floor",
			inv:  Invariant{ID: "ok", Description: "x", Kind: InvariantConfidenceFloor, Target: "outputs.x", Min: 0.7},
		},
		{
			name: "valid predicate",
			inv:  Invariant{ID: "ok", Description: "x", Kind: InvariantPredicate, Predicate: "no PII leaves the box"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.inv.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

// TestWorkflow_Validate_DuplicateInvariantIDs verifies the workflow-
// level uniqueness check on invariant IDs. Validate must reject the
// duplicate before reaching cycle detection (so the error names the
// invariant, not the DAG).
func TestWorkflow_Validate_DuplicateInvariantIDs(t *testing.T) {
	wf := &Workflow{
		Name:    "wf",
		Version: "1",
		Nodes: []Node{
			{ID: "n1", Kind: KindBash, Bash: &BashNode{Cmd: "true"}},
		},
		Invariants: []Invariant{
			{ID: "same", Description: "first", Kind: InvariantPredicate, Predicate: "p"},
			{ID: "same", Description: "second", Kind: InvariantPredicate, Predicate: "p"},
		},
	}
	err := wf.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("Validate() = %v, want duplicate-id error", err)
	}
}

func TestWorkflow_Validate_ConstitutionalInvariantPresence(t *testing.T) {
	tests := []struct {
		name           string
		constitutional *Constitutional
		invariants     []Invariant
		wantErr        string
	}{
		{name: "mode absent"},
		{name: "enforcement disabled", constitutional: &Constitutional{}},
		{
			name:           "enforcement enabled without invariants",
			constitutional: &Constitutional{Enforce: true},
			wantErr:        `workflow "wf": constitutional mode is on but declares no invariants`,
		},
		{
			name:           "enforcement enabled with invariant",
			constitutional: &Constitutional{Enforce: true},
			invariants: []Invariant{{
				ID: "declared", Description: "a human-authored rule", Kind: InvariantPredicate, Predicate: "output is reviewed",
			}},
		},
		{
			name:           "enforcement enabled with malformed invariant",
			constitutional: &Constitutional{Enforce: true},
			invariants:     []Invariant{{ID: "invalid-shape"}},
			wantErr:        `workflow "wf": invariants[0]: invariant "invalid-shape": description is required`,
		},
		{
			name:           "enforcement enabled with duplicate invariant IDs",
			constitutional: &Constitutional{Enforce: true},
			invariants: []Invariant{
				{ID: "duplicate", Description: "first rule", Kind: InvariantPredicate, Predicate: "first"},
				{ID: "duplicate", Description: "second rule", Kind: InvariantPredicate, Predicate: "second"},
			},
			wantErr: `workflow "wf": invariants[1]: duplicate id "duplicate"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wf := constitutionalTestWorkflow(tc.constitutional, tc.invariants)
			err := wf.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("Validate() = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadBytesRejectsEnforcedWorkflowWithoutInvariants(t *testing.T) {
	tests := []struct {
		name       string
		invariants string
	}{
		{name: "omitted"},
		{name: "explicit empty list", invariants: "invariants: []\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := `name: wf
version: "1"
constitutional:
  enforce: true
` + tc.invariants + `nodes:
  - id: n1
    kind: bash
    bash:
      cmd: "true"
`
			wf, err := LoadBytes([]byte(raw))
			if err == nil || !strings.Contains(err.Error(), "constitutional mode is on but declares no invariants") {
				t.Fatalf("LoadBytes() = (%#v, %v), want constitutional invariant error", wf, err)
			}
			if wf != nil {
				t.Fatalf("LoadBytes() workflow = %#v, want nil on validation failure", wf)
			}
		})
	}
}

func TestRunnerRejectsEnforcedWorkflowBeforeSideEffects(t *testing.T) {
	t.Run("run", func(t *testing.T) {
		spy := &constitutionalRunSpy{}
		runner := newConstitutionalSpyRunner(spy)

		report, err := runner.Run(context.Background(), constitutionalTestWorkflow(
			&Constitutional{Enforce: true}, nil,
		), nil)

		assertConstitutionalValidationFailure(t, report, err, spy, 0)
	})

	t.Run("resume loads state but cannot reopen invalid workflow", func(t *testing.T) {
		spy := &constitutionalRunSpy{
			snapshot: &Snapshot{
				Workflow: "wf", Inputs: map[string]string{}, Outputs: map[string]string{},
				Completed: map[string]bool{}, Started: time.Now(), UpdatedAt: time.Now(),
			},
		}
		runner := newConstitutionalSpyRunner(spy)
		runner.State = spy

		report, err := runner.Resume(context.Background(), constitutionalTestWorkflow(
			&Constitutional{Enforce: true}, nil,
		), spy)

		assertConstitutionalValidationFailure(t, report, err, spy, 1)
	})
}

func constitutionalTestWorkflow(c *Constitutional, invs []Invariant) *Workflow {
	return &Workflow{
		Name: "wf", Version: "1", Constitutional: c, Invariants: invs,
		Nodes: []Node{{ID: "n1", Kind: KindAI, AI: &AINode{Prompt: "must not execute"}}},
	}
}

type constitutionalRunSpy struct {
	aiCalls       int
	defineCalls   int
	recorderCalls int
	auditCalls    int
	loadCalls     int
	saveCalls     int
	snapshot      *Snapshot
}

func (s *constitutionalRunSpy) Generate(context.Context, *AINode, map[string]string, map[string]string) (string, error) {
	s.aiCalls++
	return "unexpected", nil
}

func (s *constitutionalRunSpy) BeginRun(context.Context, RunRecord) error {
	s.recorderCalls++
	return nil
}

func (s *constitutionalRunSpy) UpsertNode(context.Context, NodeRecord) error {
	s.recorderCalls++
	return nil
}

func (s *constitutionalRunSpy) RecordAttempt(context.Context, AttemptRecord) error {
	s.recorderCalls++
	return nil
}

func (s *constitutionalRunSpy) FinishRun(context.Context, string, RunState, time.Time, string) error {
	s.recorderCalls++
	return nil
}

func (s *constitutionalRunSpy) Emit(context.Context, AuditEvent) error {
	s.auditCalls++
	return nil
}

func (s *constitutionalRunSpy) Load(context.Context) (*Snapshot, error) {
	s.loadCalls++
	return s.snapshot, nil
}

func (s *constitutionalRunSpy) Save(context.Context, Snapshot) error {
	s.saveCalls++
	return nil
}

func newConstitutionalSpyRunner(spy *constitutionalRunSpy) *Runner {
	runner := NewRunner(spy)
	runner.Recorder = spy
	runner.Audit = spy
	runner.Hooks = &Hooks{OnDefine: func(context.Context, DefinePayload) error {
		spy.defineCalls++
		return nil
	}}
	return runner
}

func assertConstitutionalValidationFailure(
	t *testing.T,
	report *RunReport,
	err error,
	spy *constitutionalRunSpy,
	wantLoads int,
) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "constitutional mode is on but declares no invariants") {
		t.Fatalf("run = (%#v, %v), want constitutional invariant error", report, err)
	}
	if report != nil {
		t.Fatalf("report = %#v, want nil before run initialization", report)
	}
	if spy.loadCalls != wantLoads {
		t.Errorf("state loads = %d, want %d", spy.loadCalls, wantLoads)
	}
	if spy.saveCalls != 0 || spy.recorderCalls != 0 || spy.auditCalls != 0 || spy.defineCalls != 0 || spy.aiCalls != 0 {
		t.Errorf("validation leaked side effects: saves=%d recorder=%d audit=%d define=%d ai=%d",
			spy.saveCalls, spy.recorderCalls, spy.auditCalls, spy.defineCalls, spy.aiCalls)
	}
}
