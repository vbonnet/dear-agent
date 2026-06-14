package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

func TestMetaOrchestrator_Role(t *testing.T) {
	rm := NewInMemoryRoadmap()
	trail, _ := newBufferTrail()
	m, err := NewMetaOrchestrator(trail, rm)
	if err != nil {
		t.Fatalf("NewMetaOrchestrator: %v", err)
	}
	if m.Role() != RoleMetaOrchestrator {
		t.Errorf("Role = %q, want %q", m.Role(), RoleMetaOrchestrator)
	}
}

func TestNewMetaOrchestrator_RejectsNilDeps(t *testing.T) {
	rm := NewInMemoryRoadmap()
	trail, _ := newBufferTrail()
	if _, err := NewMetaOrchestrator(nil, rm); err == nil {
		t.Error("nil trail accepted")
	}
	if _, err := NewMetaOrchestrator(trail, nil); err == nil {
		t.Error("nil roadmap accepted")
	}
}

func TestMetaOrchestrator_Tick_AcceptsWithReason(t *testing.T) {
	rm := NewInMemoryRoadmap()
	must(t, rm.Submit(WorkProposal{ID: "p1", Title: "Add memory pair lint", Reason: "prevents engram drift"}))
	must(t, rm.Submit(WorkProposal{ID: "p2", Title: "Refactor", Reason: ""})) // missing Reason

	trail, buf := newBufferTrail()
	m, err := NewMetaOrchestrator(trail, rm)
	if err != nil {
		t.Fatalf("NewMetaOrchestrator: %v", err)
	}

	if err := m.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	if got := rm.Accepted(); !equalSorted(got, []string{"p1"}) {
		t.Errorf("Accepted = %v, want [p1]", got)
	}
	if got := rm.Rejected(); !equalSorted(got, []string{"p2"}) {
		t.Errorf("Rejected = %v, want [p2]", got)
	}

	// Trail records both decisions.
	records := parseTrail(t, buf)
	var evaluations []map[string]any
	for _, r := range records {
		if r["kind"] == "supervisor.metao.roadmap.evaluated" {
			evaluations = append(evaluations, r["payload"].(map[string]any))
		}
	}
	if len(evaluations) != 2 {
		t.Fatalf("evaluated records = %d, want 2", len(evaluations))
	}
	// Each evaluation captures the accepted flag.
	for _, ev := range evaluations {
		switch ev["proposal_id"] {
		case "p1":
			if ev["accepted"] != true {
				t.Errorf("p1 accepted = %v, want true", ev["accepted"])
			}
		case "p2":
			if ev["accepted"] != false {
				t.Errorf("p2 accepted = %v, want false", ev["accepted"])
			}
		}
	}
}

func TestMetaOrchestrator_Tick_RoadmapErrorPropagates(t *testing.T) {
	rm := &errorRoadmap{err: errors.New("boom")}
	trail, _ := newBufferTrail()
	m, err := NewMetaOrchestrator(trail, rm)
	if err != nil {
		t.Fatalf("NewMetaOrchestrator: %v", err)
	}
	if err := m.Tick(context.Background()); err == nil {
		t.Error("Tick returned nil when PendingProposals errored")
	}
}

func TestMetaOrchestrator_Tick_AcceptErrorRecordedButContinues(t *testing.T) {
	rm := &flakyRoadmap{
		InMemoryRoadmap: NewInMemoryRoadmap(),
		failAccept:      map[string]bool{"p1": true},
	}
	must(t, rm.Submit(WorkProposal{ID: "p1", Title: "ok", Reason: "ok"}))
	must(t, rm.Submit(WorkProposal{ID: "p2", Title: "ok2", Reason: "ok2"}))

	trail, buf := newBufferTrail()
	m, err := NewMetaOrchestrator(trail, rm)
	if err != nil {
		t.Fatalf("NewMetaOrchestrator: %v", err)
	}
	if err := m.Tick(context.Background()); err != nil {
		t.Fatalf("Tick returned err = %v; per design, single-proposal failures should not abort the tick", err)
	}
	// p2 should still get through despite p1's Accept failing.
	if got := rm.Accepted(); len(got) != 1 || got[0] != "p2" {
		t.Errorf("Accepted = %v, want [p2]", got)
	}
	// accept_failed record present.
	records := parseTrail(t, buf)
	saw := false
	for _, r := range records {
		if r["kind"] == "supervisor.metao.roadmap.accept_failed" {
			saw = true
		}
	}
	if !saw {
		t.Error("no supervisor.metao.roadmap.accept_failed record in trail")
	}
}

func TestMetaOrchestrator_Tick_NoWork_EmitsNoWorkRecord(t *testing.T) {
	rm := NewInMemoryRoadmap() // empty
	trail, buf := newBufferTrail()
	m, err := NewMetaOrchestrator(trail, rm)
	if err != nil {
		t.Fatalf("NewMetaOrchestrator: %v", err)
	}
	if err := m.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	saw := false
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.metao.no_work" {
			saw = true
		}
	}
	if !saw {
		t.Error("no supervisor.metao.no_work record after idle tick on empty roadmap")
	}
}

func TestMetaOrchestrator_Tick_IdleEscalation(t *testing.T) {
	rm := NewInMemoryRoadmap() // always empty
	trail, buf := newBufferTrail()
	m, err := NewMetaOrchestrator(trail, rm)
	if err != nil {
		t.Fatalf("NewMetaOrchestrator: %v", err)
	}
	for i := range metaoIdleEscalationThreshold {
		if err := m.Tick(context.Background()); err != nil {
			t.Fatalf("Tick %d: %v", i, err)
		}
	}
	saw := false
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.metao.idle_escalation" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("no supervisor.metao.idle_escalation after %d idle ticks", metaoIdleEscalationThreshold)
	}
}

func TestMetaOrchestrator_Tick_IdleResetOnEvaluation(t *testing.T) {
	rm := NewInMemoryRoadmap()
	trail, buf := newBufferTrail()
	m, err := NewMetaOrchestrator(trail, rm)
	if err != nil {
		t.Fatalf("NewMetaOrchestrator: %v", err)
	}
	// Run 6 idle ticks (one below threshold).
	for i := range metaoIdleEscalationThreshold - 1 {
		if err := m.Tick(context.Background()); err != nil {
			t.Fatalf("idle Tick %d: %v", i, err)
		}
	}
	// Submit + evaluate a proposal — resets the streak.
	must(t, rm.Submit(WorkProposal{ID: "p1", Title: "x", Reason: "y"}))
	if err := m.Tick(context.Background()); err != nil {
		t.Fatalf("evaluation Tick: %v", err)
	}
	// Run 6 more idle ticks. No escalation should fire.
	for i := range metaoIdleEscalationThreshold - 1 {
		if err := m.Tick(context.Background()); err != nil {
			t.Fatalf("post-reset idle Tick %d: %v", i, err)
		}
	}
	for _, r := range parseTrail(t, buf) {
		if r["kind"] == "supervisor.metao.idle_escalation" {
			t.Error("idle_escalation fired even though evaluation reset the streak")
		}
	}
}

// errorRoadmap always returns an error from PendingProposals.
type errorRoadmap struct {
	Roadmap // unused — only PendingProposals is called in the path under test
	err     error
}

func (e *errorRoadmap) PendingProposals(context.Context) ([]WorkProposal, error) {
	return nil, e.err
}
func (e *errorRoadmap) Accept(context.Context, string) error          { return nil }
func (e *errorRoadmap) Reject(context.Context, string, string) error  { return nil }

// flakyRoadmap fails Accept for specific IDs.
type flakyRoadmap struct {
	*InMemoryRoadmap
	failAccept map[string]bool
}

func (f *flakyRoadmap) Accept(ctx context.Context, id string) error {
	if f.failAccept[id] {
		return errors.New("simulated accept failure")
	}
	return f.InMemoryRoadmap.Accept(ctx, id)
}

// helpers

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func equalSorted(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

func parseTrail(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("parse trail line: %v (line=%q)", err, line)
		}
		out = append(out, rec)
	}
	return out
}

// TestWorkProposal_Validate covers required-field enforcement.
func TestWorkProposal_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		p       WorkProposal
		wantErr bool
		errFrag string
	}{
		{name: "valid_minimal", p: WorkProposal{ID: "x", Title: "T", Reason: "R"}},
		{
			name: "valid_with_optional_fields",
			p: WorkProposal{
				ID:             "x",
				Title:          "T",
				Reason:         "R",
				SubmittedBy:    "worker/sess-abc",
				SubmitterChain: []string{"orchestrator/sess-000", "worker/sess-abc"},
				ScopeHint:      ScopeFeature,
				Tags:           []string{"vroom", "ce-6as80"},
			},
		},
		{name: "missing_id", p: WorkProposal{Title: "T", Reason: "R"}, wantErr: true, errFrag: "ID required"},
		{name: "missing_title", p: WorkProposal{ID: "x", Reason: "R"}, wantErr: true, errFrag: "Title required"},
		{name: "missing_reason", p: WorkProposal{ID: "x", Title: "T"}, wantErr: true, errFrag: "Reason required"},
		{name: "all_required_missing", p: WorkProposal{}, wantErr: true, errFrag: "ID required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.p.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.errFrag != "" && err != nil && !strings.Contains(err.Error(), tc.errFrag) {
				t.Errorf("Validate() error = %q, want it to contain %q", err, tc.errFrag)
			}
		})
	}
}

func TestWorkProposal_Validate_MultiError(t *testing.T) {
	t.Parallel()
	err := WorkProposal{ID: "x"}.Validate() // missing Title + Reason
	if err == nil {
		t.Fatal("expected error for missing Title and Reason, got nil")
	}
	for _, frag := range []string{"Title required", "Reason required"} {
		if !strings.Contains(err.Error(), frag) {
			t.Errorf("error %q missing fragment %q", err, frag)
		}
	}
}

func TestWorkProposal_ScopeHint_Constants(t *testing.T) {
	t.Parallel()
	want := map[ScopeHint]string{
		ScopeUnspecified: "",
		ScopeFix:         "fix",
		ScopeRefactor:    "refactor",
		ScopeFeature:     "feature",
		ScopeResearch:    "research",
		ScopeInfra:       "infra",
	}
	for hint, str := range want {
		if string(hint) != str {
			t.Errorf("ScopeHint %q has value %q, want %q", hint, string(hint), str)
		}
	}
}

// TestMetaOrchestrator_Tick_RejectsMissingID confirms that the Validate-based
// admission policy rejects proposals missing required fields beyond Reason.
func TestMetaOrchestrator_Tick_RejectsMissingID(t *testing.T) {
	t.Parallel()
	rm := NewInMemoryRoadmap()
	rm.pending[""] = WorkProposal{ID: "", Title: "No-ID proposal", Reason: "test"}

	trail, _ := newBufferTrail()
	m, err := NewMetaOrchestrator(trail, rm)
	if err != nil {
		t.Fatalf("NewMetaOrchestrator: %v", err)
	}
	if err := m.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := rm.Accepted(); len(got) != 0 {
		t.Errorf("expected no accepted proposals, got %v", got)
	}
}
