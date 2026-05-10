package workflow

import (
	"context"
	"strings"
	"testing"
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

// TestConstitutionalDefineHook_OffOrEmpty checks the two paths that
// MUST NOT fail the hook: constitutional mode off, and constitutional
// mode missing entirely.
func TestConstitutionalDefineHook_OffOrEmpty(t *testing.T) {
	hook := ConstitutionalDefineHook()

	cases := []struct {
		name string
		wf   *Workflow
	}{
		{
			name: "no constitutional block",
			wf:   &Workflow{Name: "wf"},
		},
		{
			name: "enforce=false",
			wf: &Workflow{
				Name:           "wf",
				Constitutional: &Constitutional{Enforce: false},
			},
		},
		{
			name: "nil workflow",
			wf:   nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := hook(context.Background(), DefinePayload{Workflow: tc.wf}); err != nil {
				t.Fatalf("hook = %v, want nil", err)
			}
		})
	}
}

// TestConstitutionalDefineHook_EnforceMissingInvariants is the failure
// path: constitutional mode is on, invariants is empty, hook fails.
func TestConstitutionalDefineHook_EnforceMissingInvariants(t *testing.T) {
	hook := ConstitutionalDefineHook()
	wf := &Workflow{
		Name:           "wf",
		Constitutional: &Constitutional{Enforce: true},
		// Invariants intentionally empty.
	}
	err := hook(context.Background(), DefinePayload{Workflow: wf})
	if err == nil {
		t.Fatal("hook = nil, want failure for missing invariants")
	}
	if !strings.Contains(err.Error(), "constitutional mode is on") {
		t.Fatalf("hook = %v, want constitutional-mode error", err)
	}
}

// TestConstitutionalDefineHook_EnforceWithInvariants is the happy path:
// constitutional mode is on, at least one invariant is declared, hook
// passes.
func TestConstitutionalDefineHook_EnforceWithInvariants(t *testing.T) {
	hook := ConstitutionalDefineHook()
	wf := &Workflow{
		Name:           "wf",
		Constitutional: &Constitutional{Enforce: true},
		Invariants: []Invariant{
			{ID: "x", Description: "y", Kind: InvariantPredicate, Predicate: "z"},
		},
	}
	if err := hook(context.Background(), DefinePayload{Workflow: wf}); err != nil {
		t.Fatalf("hook = %v, want nil", err)
	}
}
