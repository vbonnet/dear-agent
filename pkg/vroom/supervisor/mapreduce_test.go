package supervisor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// trailKinds and hasKind are shared helpers defined in loop_test.go and
// failover_test.go respectively.

// coherentSpec is a spec whose two components together satisfy both
// constraints, provide what they consume, and assert no conflicting values.
func coherentSpec() (Spec, Decomposer, Dispatcher) {
	spec := Spec{
		ID:    "spec-1",
		Title: "user service",
		Constraints: []Constraint{
			{ID: "c1", Text: "stores users"},
			{ID: "c2", Text: "authenticates users"},
		},
		RequiredInterfaces: []string{"UserAPI"},
	}
	dec := DecomposerFunc(func(_ context.Context, _ Spec) ([]Component, error) {
		return []Component{
			{ID: "store", Worker: "coder", Constraints: []string{"c1"}, Provides: []string{"UserStore"}},
			{ID: "api", Worker: "coder", Constraints: []string{"c2"}, Provides: []string{"UserAPI"}, Consumes: []string{"UserStore"}},
		}, nil
	})
	disp := DispatcherFunc(func(_ context.Context, c Component) (ComponentResult, error) {
		return ComponentResult{
			ComponentID:          c.ID,
			SatisfiedConstraints: c.Constraints,
			ProvidedInterfaces:   c.Provides,
			Assertions:           []Assertion{{Key: "db", Value: "postgres"}},
		}, nil
	})
	return spec, dec, disp
}

func TestMapReduce_CoherentSpec(t *testing.T) {
	trail, buf := newBufferTrail()
	spec, dec, disp := coherentSpec()
	c, err := NewMapReduceCoordinator(trail, dec, disp)
	if err != nil {
		t.Fatal(err)
	}
	report, err := c.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !report.Coherent {
		t.Fatalf("want coherent, got %+v", report)
	}
	if len(report.FailedComponents) != 0 || len(report.Contradictions) != 0 ||
		len(report.UnsatisfiedConstraints) != 0 || len(report.InterfaceGaps) != 0 {
		t.Fatalf("want empty failure fields, got %+v", report)
	}
	kinds := trailKinds(t, buf)
	for _, want := range []string{"supervisor.mapreduce.decomposed", "supervisor.mapreduce.component_completed", "supervisor.mapreduce.merge_validated"} {
		if !hasKind(kinds, want) {
			t.Errorf("missing trail kind %q in %v", want, kinds)
		}
	}
	if hasKind(kinds, "supervisor.mapreduce.incoherent") {
		t.Errorf("coherent run should not emit incoherent, kinds=%v", kinds)
	}
}

func TestMapReduce_Contradiction(t *testing.T) {
	spec := Spec{ID: "spec-c", Title: "t", Constraints: []Constraint{{ID: "c1"}}}
	dec := DecomposerFunc(func(_ context.Context, _ Spec) ([]Component, error) {
		return []Component{{ID: "a", Constraints: []string{"c1"}}, {ID: "b"}}, nil
	})
	disp := DispatcherFunc(func(_ context.Context, comp Component) (ComponentResult, error) {
		val := "oauth"
		if comp.ID == "b" {
			val = "apikey"
		}
		return ComponentResult{
			ComponentID:          comp.ID,
			SatisfiedConstraints: comp.Constraints,
			Assertions:           []Assertion{{Key: "auth.method", Value: val}},
		}, nil
	})
	trail, buf := newBufferTrail()
	c, _ := NewMapReduceCoordinator(trail, dec, disp)
	report, err := c.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if report.Coherent {
		t.Fatal("want incoherent due to contradiction")
	}
	if len(report.Contradictions) != 1 {
		t.Fatalf("want 1 contradiction, got %+v", report.Contradictions)
	}
	got := report.Contradictions[0]
	if got.Key != "auth.method" {
		t.Errorf("want key auth.method, got %q", got.Key)
	}
	if got.Values["a"] != "oauth" || got.Values["b"] != "apikey" {
		t.Errorf("want a=oauth b=apikey, got %v", got.Values)
	}
	if !hasKind(trailKinds(t, buf), "supervisor.mapreduce.incoherent") {
		t.Error("incoherent run must emit incoherent record")
	}
}

func TestMapReduce_UnsatisfiedConstraint(t *testing.T) {
	spec := Spec{ID: "spec-u", Title: "t", Constraints: []Constraint{{ID: "c1"}, {ID: "c2"}}}
	dec := DecomposerFunc(func(_ context.Context, _ Spec) ([]Component, error) {
		return []Component{{ID: "a", Constraints: []string{"c1"}}}, nil
	})
	disp := DispatcherFunc(func(_ context.Context, comp Component) (ComponentResult, error) {
		// Only satisfies c1; c2 is left uncovered.
		return ComponentResult{ComponentID: comp.ID, SatisfiedConstraints: []string{"c1"}}, nil
	})
	c, _ := NewMapReduceCoordinator(mustTrail(t), dec, disp)
	report, err := c.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if report.Coherent {
		t.Fatal("want incoherent due to unsatisfied constraint")
	}
	if len(report.UnsatisfiedConstraints) != 1 || report.UnsatisfiedConstraints[0] != "c2" {
		t.Fatalf("want [c2] unsatisfied, got %v", report.UnsatisfiedConstraints)
	}
}

func TestMapReduce_InterfaceGap(t *testing.T) {
	spec := Spec{ID: "spec-i", Title: "t", RequiredInterfaces: []string{"PublicAPI"}}
	dec := DecomposerFunc(func(_ context.Context, _ Spec) ([]Component, error) {
		// "api" consumes UserStore which nobody provides; spec requires
		// PublicAPI which nobody provides either.
		return []Component{{ID: "api", Consumes: []string{"UserStore"}}}, nil
	})
	disp := DispatcherFunc(func(_ context.Context, comp Component) (ComponentResult, error) {
		return ComponentResult{ComponentID: comp.ID}, nil
	})
	c, _ := NewMapReduceCoordinator(mustTrail(t), dec, disp)
	report, err := c.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if report.Coherent {
		t.Fatal("want incoherent due to interface gaps")
	}
	if len(report.InterfaceGaps) != 2 {
		t.Fatalf("want 2 gaps (PublicAPI, UserStore), got %+v", report.InterfaceGaps)
	}
	// Sorted by interface name: PublicAPI, UserStore.
	if report.InterfaceGaps[0].Interface != "PublicAPI" {
		t.Errorf("want PublicAPI first, got %q", report.InterfaceGaps[0].Interface)
	}
	if report.InterfaceGaps[0].NeededBy[0] != "spec:spec-i" {
		t.Errorf("PublicAPI should be needed by spec, got %v", report.InterfaceGaps[0].NeededBy)
	}
	if report.InterfaceGaps[1].Interface != "UserStore" || report.InterfaceGaps[1].NeededBy[0] != "api" {
		t.Errorf("UserStore should be needed by api, got %+v", report.InterfaceGaps[1])
	}
}

func TestMapReduce_FailedComponentDoesNotAbortSiblings(t *testing.T) {
	spec := Spec{ID: "spec-f", Title: "t", Constraints: []Constraint{{ID: "c1"}, {ID: "c2"}}}
	dec := DecomposerFunc(func(_ context.Context, _ Spec) ([]Component, error) {
		return []Component{
			{ID: "ok", Constraints: []string{"c1"}},
			{ID: "boom", Constraints: []string{"c2"}},
		}, nil
	})
	var okRan atomic.Bool
	disp := DispatcherFunc(func(_ context.Context, comp Component) (ComponentResult, error) {
		if comp.ID == "boom" {
			return ComponentResult{}, errors.New("dispatch exploded")
		}
		okRan.Store(true)
		return ComponentResult{ComponentID: comp.ID, SatisfiedConstraints: comp.Constraints}, nil
	})
	trail, buf := newBufferTrail()
	c, _ := NewMapReduceCoordinator(trail, dec, disp)
	report, err := c.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("Execute should not error on per-component failure: %v", err)
	}
	if !okRan.Load() {
		t.Error("sibling component should still run when another fails")
	}
	if len(report.FailedComponents) != 1 || report.FailedComponents[0] != "boom" {
		t.Fatalf("want [boom] failed, got %v", report.FailedComponents)
	}
	if report.Coherent {
		t.Error("a failed component must make the spec incoherent")
	}
	// c2 was the failed component's constraint → unsatisfied.
	if len(report.UnsatisfiedConstraints) != 1 || report.UnsatisfiedConstraints[0] != "c2" {
		t.Errorf("want c2 unsatisfied, got %v", report.UnsatisfiedConstraints)
	}
	if !hasKind(trailKinds(t, buf), "supervisor.mapreduce.component_failed") {
		t.Error("failed component must emit component_failed record")
	}
}

func TestMapReduce_DecomposeErrorAndEmpty(t *testing.T) {
	disp := DispatcherFunc(func(_ context.Context, _ Component) (ComponentResult, error) {
		t.Fatal("dispatch must not run when decompose fails")
		return ComponentResult{}, nil
	})

	// Decompose returns an error.
	errDec := DecomposerFunc(func(_ context.Context, _ Spec) ([]Component, error) {
		return nil, errors.New("planner down")
	})
	c, _ := NewMapReduceCoordinator(mustTrail(t), errDec, disp)
	if _, err := c.Execute(context.Background(), Spec{ID: "s", Title: "t"}); err == nil {
		t.Error("want error from failed decompose")
	}

	// Decompose returns no components.
	emptyDec := DecomposerFunc(func(_ context.Context, _ Spec) ([]Component, error) {
		return nil, nil
	})
	c2, _ := NewMapReduceCoordinator(mustTrail(t), emptyDec, disp)
	if _, err := c2.Execute(context.Background(), Spec{ID: "s", Title: "t"}); err == nil {
		t.Error("want error from empty decomposition")
	}
}

func TestMapReduce_InvalidSpec(t *testing.T) {
	_, dec, disp := coherentSpec()
	c, _ := NewMapReduceCoordinator(mustTrail(t), dec, disp)
	if _, err := c.Execute(context.Background(), Spec{Title: "no id"}); err == nil {
		t.Error("want validation error for spec missing ID")
	}
	if _, err := c.Execute(context.Background(), Spec{ID: "x", Title: "t", Constraints: []Constraint{{ID: ""}}}); err == nil {
		t.Error("want validation error for constraint missing ID")
	}
}

func TestMapReduce_ContextCancelStopsDispatch(t *testing.T) {
	spec := Spec{ID: "spec-x", Title: "t"}
	dec := DecomposerFunc(func(_ context.Context, _ Spec) ([]Component, error) {
		return []Component{{ID: "a"}, {ID: "b"}, {ID: "c"}}, nil
	})
	var dispatched atomic.Int64
	disp := DispatcherFunc(func(ctx context.Context, comp Component) (ComponentResult, error) {
		dispatched.Add(1)
		return ComponentResult{ComponentID: comp.ID}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Execute
	c, _ := NewMapReduceCoordinator(mustTrail(t), dec, disp)
	if _, err := c.Execute(ctx, spec); err == nil {
		t.Error("want context.Canceled from a pre-cancelled Execute")
	}
	if dispatched.Load() != 0 {
		t.Errorf("no component should dispatch under a cancelled context, got %d", dispatched.Load())
	}
}

func TestMapReduce_MaxParallelBound(t *testing.T) {
	comps := make([]Component, 50)
	for i := range comps {
		comps[i] = Component{ID: string(rune('A'+i%26)) + string(rune('0'+i/26))}
	}
	dec := DecomposerFunc(func(_ context.Context, _ Spec) ([]Component, error) { return comps, nil })

	var mu sync.Mutex
	var inflight, maxSeen int
	disp := DispatcherFunc(func(_ context.Context, comp Component) (ComponentResult, error) {
		mu.Lock()
		inflight++
		if inflight > maxSeen {
			maxSeen = inflight
		}
		mu.Unlock()
		// busy a little so concurrency overlaps
		for i := range 1000 {
			_ = i
		}
		mu.Lock()
		inflight--
		mu.Unlock()
		return ComponentResult{ComponentID: comp.ID}, nil
	})
	c, _ := NewMapReduceCoordinator(mustTrail(t), dec, disp)
	c.WithMaxParallel(4)
	if _, err := c.Execute(context.Background(), Spec{ID: "s", Title: "t"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if maxSeen > 4 {
		t.Errorf("max parallel breached: saw %d concurrent dispatches, cap was 4", maxSeen)
	}
	if maxSeen == 0 {
		t.Error("dispatcher never ran")
	}
}

// ValidateCoherence is exercised directly to lock the pure reduce-phase logic.
func TestValidateCoherence_Pure(t *testing.T) {
	spec := Spec{
		ID:                 "s",
		Constraints:        []Constraint{{ID: "c1"}},
		RequiredInterfaces: []string{"API"},
	}
	comps := []Component{{ID: "a", Provides: []string{"API"}, Constraints: []string{"c1"}}}
	results := []ComponentResult{{ComponentID: "a", SatisfiedConstraints: []string{"c1"}, ProvidedInterfaces: []string{"API"}}}
	if rep := ValidateCoherence(spec, comps, results); !rep.Coherent {
		t.Fatalf("want coherent, got %+v", rep)
	}
	// Drop the provided interface → gap.
	results[0].ProvidedInterfaces = nil
	if rep := ValidateCoherence(spec, comps, results); rep.Coherent || len(rep.InterfaceGaps) != 1 {
		t.Fatalf("want one interface gap, got %+v", rep)
	}
}

// mustTrail returns a discardable buffer-backed trail for tests that don't
// assert on records.
func mustTrail(t *testing.T) decisiontrail.Trail {
	t.Helper()
	trail, _ := newBufferTrail()
	return trail
}
