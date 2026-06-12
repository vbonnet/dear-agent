package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
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
