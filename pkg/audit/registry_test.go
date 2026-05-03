package audit

import (
	"context"
	"testing"
)

type stubCheck struct{ id string; cad Cadence }

func (s stubCheck) Meta() CheckMeta {
	cad := s.cad
	if cad == "" {
		cad = CadenceDaily
	}
	return CheckMeta{ID: s.id, Cadence: cad, SeverityCeiling: SeverityP1}
}

func (stubCheck) Run(_ context.Context, _ Env) (Result, error) { return Result{Status: StatusOK}, nil }

func TestRegistryDuplicateRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stubCheck{id: "x"}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := r.Register(stubCheck{id: "x"}); err == nil {
		t.Error("duplicate register should fail")
	}
}

func TestRegistryNilRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("nil register should fail")
	}
}

func TestRegistryInvalidMetaRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stubCheck{id: ""}); err == nil {
		t.Error("empty id should fail registration")
	}
}

func TestRegistryChecksForCadenceFilters(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stubCheck{id: "daily-1", cad: CadenceDaily}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Register(stubCheck{id: "weekly-1", cad: CadenceWeekly}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Register(stubCheck{id: "daily-2", cad: CadenceDaily}); err != nil {
		t.Fatalf("register: %v", err)
	}

	got := r.ChecksForCadence(CadenceDaily)
	if len(got) != 2 {
		t.Fatalf("daily count = %d, want 2", len(got))
	}
	// sorted by id
	if got[0].Meta().ID != "daily-1" || got[1].Meta().ID != "daily-2" {
		t.Errorf("unexpected order: %v", []string{got[0].Meta().ID, got[1].Meta().ID})
	}
}

func TestRegistryRefinerDuplicateRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterRefiner(stubRefiner{name: "x"}); err != nil {
		t.Fatalf("first refiner register: %v", err)
	}
	if err := r.RegisterRefiner(stubRefiner{name: "x"}); err == nil {
		t.Error("duplicate refiner should fail")
	}
}

type stubRefiner struct{ name string }

func (s stubRefiner) Name() string                                                  { return s.name }
func (stubRefiner) Propose(_ context.Context, _ []Finding) ([]Proposal, error)      { return nil, nil }
