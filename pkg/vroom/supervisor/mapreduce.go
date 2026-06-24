package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// Map-reduce spec-driven execution (ce-6848).
//
// The MapReduceCoordinator turns a single Spec into a fan-out/fan-in
// pipeline:
//
//	     decompose                dispatch (parallel)            merge
//	Spec ──────────► []Component ─────────────────────► []ComponentResult ──► MergeReport
//	   (map: plan)      (map: run each bead)             (reduce: validate coherence)
//
//   - Map phase  — Decompose splits the spec into component beads, then each
//     bead is dispatched concurrently to a Worker. A failing bead does NOT
//     abort its siblings; it is recorded and surfaced in the report (the same
//     "never stall the whole queue on one item" invariant the Orchestrator
//     holds, ce-6as.4).
//   - Reduce phase — once every bead has settled, the coordinator validates
//     coherence across the *full* spec: no contradictions between beads, every
//     spec constraint satisfied by some bead, and every consumed/required
//     interface actually provided.
//
// The coordinator owns no long-lived loop; it is invoked once per spec by a
// supervisor (typically the Orchestrator, which both dispatches work and is
// accountable for the merge gate). All decisions are written to the
// decision trail so the Define→Execute→Audit narrative of a spec's execution
// is reconstructable.

// Spec is the unit a MapReduceCoordinator executes. It declares the
// constraints the finished work must satisfy and the interfaces it must
// expose; decomposition turns it into the component beads that, together,
// must cover all of them.
type Spec struct {
	// ID uniquely identifies the spec. Required.
	ID string

	// Title is a short human-readable summary. Required.
	Title string

	// Constraints are the requirements the assembled work must satisfy.
	// The merge phase fails if any constraint is left unsatisfied by every
	// component.
	Constraints []Constraint

	// RequiredInterfaces are interfaces the spec as a whole must expose
	// (its public surface). Each must be Provided by at least one component
	// or the merge phase reports an interface gap.
	RequiredInterfaces []string
}

// Validate returns a combined error describing all schema violations, or nil
// when the spec satisfies every required-field invariant.
func (s Spec) Validate() error {
	var errs []string
	if s.ID == "" {
		errs = append(errs, "ID required")
	}
	if s.Title == "" {
		errs = append(errs, "Title required")
	}
	for i, c := range s.Constraints {
		if c.ID == "" {
			errs = append(errs, fmt.Sprintf("Constraints[%d].ID required", i))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.New("Spec: " + strings.Join(errs, "; "))
}

// Constraint is one requirement the assembled work must satisfy. Components
// report which constraint IDs they satisfied; the merge phase fails if any
// constraint is unsatisfied across the whole result set.
type Constraint struct {
	// ID uniquely identifies the constraint within a Spec. Required.
	ID string

	// Text is the human-readable requirement (e.g. "writes are idempotent").
	Text string
}

// Component is one bead produced by decomposing a Spec. It declares the
// subset of spec constraints it owns and the interfaces it provides and
// consumes, so the merge phase can check producer/consumer alignment without
// inspecting code.
type Component struct {
	// ID uniquely identifies the component within a decomposition. Required.
	ID string

	// Title is a short human-readable summary.
	Title string

	// Worker is the name/role of the Worker the bead is dispatched to
	// (e.g. "coder", "code-reviewer"). Free-form.
	Worker string

	// Constraints lists the spec Constraint IDs this component is
	// responsible for satisfying.
	Constraints []string

	// Provides lists the interface names this component implements.
	Provides []string

	// Consumes lists the interface names this component depends on another
	// component to provide.
	Consumes []string
}

// ComponentResult is the map-phase output for one component bead. A Worker
// (via the Dispatcher) reports which constraints it actually satisfied, which
// interfaces it actually exposed, and any factual decisions it made
// (Assertions) that the merge phase cross-checks for contradictions.
type ComponentResult struct {
	// ComponentID identifies which Component this result is for.
	ComponentID string

	// SatisfiedConstraints lists the spec Constraint IDs the component
	// actually satisfied. May be a subset of the component's declared
	// Constraints if the Worker could not satisfy all of them.
	SatisfiedConstraints []string

	// ProvidedInterfaces lists the interface names the component actually
	// exposed. May differ from the component's declared Provides.
	ProvidedInterfaces []string

	// Assertions are the factual decisions the component made (e.g.
	// {"auth.method", "oauth"}). Two components asserting different values
	// for the same key is a contradiction.
	Assertions []Assertion
}

// Assertion is one factual decision a component made. The merge phase treats
// two components asserting different Values for the same Key as a
// contradiction.
type Assertion struct {
	Key   string
	Value string
}

// Decomposer is the map-phase planning seam: it splits a Spec into the
// component beads that will be dispatched in parallel. Implementations may
// call an LLM, read a plan file, or apply heuristics. A decomposition must
// return at least one component for a non-empty spec.
type Decomposer interface {
	Decompose(ctx context.Context, spec Spec) ([]Component, error)
}

// DecomposerFunc adapts a plain function to the Decomposer interface.
type DecomposerFunc func(ctx context.Context, spec Spec) ([]Component, error)

// Decompose implements Decomposer.
func (f DecomposerFunc) Decompose(ctx context.Context, spec Spec) ([]Component, error) {
	return f(ctx, spec)
}

// Dispatcher is the map-phase execution seam: it runs one component bead and
// returns its result. Implementations spawn a Worker (e.g. an AGM session)
// and collect its self-reported coverage. A returned error marks the bead as
// failed; siblings continue regardless.
type Dispatcher interface {
	Dispatch(ctx context.Context, c Component) (ComponentResult, error)
}

// DispatcherFunc adapts a plain function to the Dispatcher interface.
type DispatcherFunc func(ctx context.Context, c Component) (ComponentResult, error)

// Dispatch implements Dispatcher.
func (f DispatcherFunc) Dispatch(ctx context.Context, c Component) (ComponentResult, error) {
	return f(ctx, c)
}

// Contradiction reports two or more components asserting different values for
// the same decision key.
type Contradiction struct {
	// Key is the decision key the components disagree on.
	Key string

	// Values maps componentID → the value it asserted. Always has ≥2 entries.
	Values map[string]string
}

// InterfaceGap reports an interface that is required (by the spec) or consumed
// (by a component) but provided by no component.
type InterfaceGap struct {
	// Interface is the missing interface name.
	Interface string

	// NeededBy lists who needs it: component IDs that Consume it, plus
	// "spec:<id>" when the spec itself requires it. Sorted.
	NeededBy []string
}

// MergeReport is the reduce-phase verdict for a spec execution. The work is
// coherent iff there are no failed components, no contradictions, no
// unsatisfied constraints, and no interface gaps.
type MergeReport struct {
	// SpecID identifies the spec this report is for.
	SpecID string

	// Coherent is the overall verdict: true iff every other field is empty.
	Coherent bool

	// FailedComponents lists component IDs whose dispatch returned an error.
	// Sorted.
	FailedComponents []string

	// Contradictions lists decision keys where components disagree. Sorted by Key.
	Contradictions []Contradiction

	// UnsatisfiedConstraints lists spec Constraint IDs satisfied by no
	// component. Sorted.
	UnsatisfiedConstraints []string

	// InterfaceGaps lists required/consumed interfaces provided by no
	// component. Sorted by Interface.
	InterfaceGaps []InterfaceGap
}

// MapReduceCoordinator executes a Spec as a map-reduce pipeline: decompose
// into component beads, dispatch them in parallel, then merge-validate
// coherence across the full spec. It is invoked once per spec (it owns no
// loop) and writes every decision to the trail.
type MapReduceCoordinator struct {
	trail      decisiontrail.Trail
	decomposer Decomposer
	dispatcher Dispatcher
	role       Role // trail role; defaults to RoleOrchestrator
	maxPar     int  // max concurrent dispatches; 0 → unlimited
}

// NewMapReduceCoordinator constructs a MapReduceCoordinator. trail,
// decomposer, and dispatcher are all required.
func NewMapReduceCoordinator(trail decisiontrail.Trail, decomposer Decomposer, dispatcher Dispatcher) (*MapReduceCoordinator, error) {
	if trail == nil {
		return nil, errors.New("supervisor: MapReduceCoordinator requires a Trail")
	}
	if decomposer == nil {
		return nil, errors.New("supervisor: MapReduceCoordinator requires a Decomposer")
	}
	if dispatcher == nil {
		return nil, errors.New("supervisor: MapReduceCoordinator requires a Dispatcher")
	}
	return &MapReduceCoordinator{
		trail:      trail,
		decomposer: decomposer,
		dispatcher: dispatcher,
		role:       RoleOrchestrator,
	}, nil
}

// WithMaxParallel caps how many component beads dispatch concurrently. A value
// ≤ 0 means unlimited (the default). Returns the coordinator for chaining.
func (c *MapReduceCoordinator) WithMaxParallel(n int) *MapReduceCoordinator {
	c.maxPar = n
	return c
}

// Execute runs the full map-reduce pipeline for spec and returns the merge
// report. It returns a non-nil error only for whole-pipeline failures
// (invalid spec, decomposition error, empty decomposition, or a cancelled
// context); per-component failures are reported in MergeReport.FailedComponents
// and never abort the pipeline.
//
// An incoherent report is NOT an error — Execute returns (report, nil) with
// report.Coherent == false so the caller can gate on it. The coordinator
// emits supervisor.mapreduce.incoherent to the trail in that case so the
// failure is auditable.
func (c *MapReduceCoordinator) Execute(ctx context.Context, spec Spec) (MergeReport, error) {
	if err := spec.Validate(); err != nil {
		return MergeReport{}, err
	}
	if err := ctx.Err(); err != nil {
		return MergeReport{}, err
	}

	// Map phase — decompose.
	components, err := c.decomposer.Decompose(ctx, spec)
	if err != nil {
		_ = c.trail.Append(ctx, decisiontrail.Record{
			Role: string(c.role),
			Kind: "supervisor.mapreduce.decompose_failed",
			Payload: map[string]any{
				"spec_id": spec.ID,
				"error":   err.Error(),
			},
		})
		return MergeReport{}, fmt.Errorf("mapreduce: decompose spec %q: %w", spec.ID, err)
	}
	if len(components) == 0 {
		_ = c.trail.Append(ctx, decisiontrail.Record{
			Role: string(c.role),
			Kind: "supervisor.mapreduce.decompose_empty",
			Payload: map[string]any{
				"spec_id": spec.ID,
			},
		})
		return MergeReport{}, fmt.Errorf("mapreduce: decompose spec %q: no components", spec.ID)
	}
	_ = c.trail.Append(ctx, decisiontrail.Record{
		Role: string(c.role),
		Kind: "supervisor.mapreduce.decomposed",
		Payload: map[string]any{
			"spec_id":         spec.ID,
			"title":           spec.Title,
			"component_count": len(components),
		},
	})

	// Map phase — dispatch in parallel.
	results, failed := c.dispatchAll(ctx, spec, components)

	// Reduce phase — merge-validate coherence across the full spec.
	report := ValidateCoherence(spec, components, results)
	report.FailedComponents = failed
	report.Coherent = report.Coherent && len(failed) == 0

	c.recordMerge(ctx, report)
	return report, nil
}

// dispatchAll runs every component concurrently (bounded by maxPar) and
// returns the successful results plus the sorted IDs of failed components. A
// dispatch error or a cancelled context marks that single bead failed; it
// never aborts the siblings.
func (c *MapReduceCoordinator) dispatchAll(ctx context.Context, spec Spec, components []Component) ([]ComponentResult, []string) {
	var (
		mu      sync.Mutex
		results []ComponentResult
		failed  []string
		wg      sync.WaitGroup
		sem     chan struct{}
	)
	if c.maxPar > 0 {
		sem = make(chan struct{}, c.maxPar)
	}

	for _, comp := range components {
		wg.Go(func() {
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}

			res, err := c.dispatchOne(ctx, spec, comp)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = append(failed, comp.ID)
				return
			}
			results = append(results, res)
		})
	}
	wg.Wait()

	sort.Strings(failed)
	sort.Slice(results, func(i, j int) bool { return results[i].ComponentID < results[j].ComponentID })
	return results, failed
}

// dispatchOne dispatches a single component and records the outcome. It
// honours context cancellation before spawning so a cancelled run stops
// dispatching new beads.
func (c *MapReduceCoordinator) dispatchOne(ctx context.Context, spec Spec, comp Component) (ComponentResult, error) {
	if err := ctx.Err(); err != nil {
		c.recordComponentFailed(ctx, spec.ID, comp, err)
		return ComponentResult{}, err
	}

	res, err := c.dispatcher.Dispatch(ctx, comp)
	if err != nil {
		c.recordComponentFailed(ctx, spec.ID, comp, err)
		return ComponentResult{}, err
	}
	// Defend against a Dispatcher that drops the component association.
	if res.ComponentID == "" {
		res.ComponentID = comp.ID
	}
	_ = c.trail.Append(ctx, decisiontrail.Record{
		Role: string(c.role),
		Kind: "supervisor.mapreduce.component_completed",
		Payload: map[string]any{
			"spec_id":               spec.ID,
			"component_id":          comp.ID,
			"worker":                comp.Worker,
			"satisfied_constraints": res.SatisfiedConstraints,
			"provided_interfaces":   res.ProvidedInterfaces,
		},
	})
	return res, nil
}

func (c *MapReduceCoordinator) recordComponentFailed(ctx context.Context, specID string, comp Component, err error) {
	_ = c.trail.Append(ctx, decisiontrail.Record{
		Role: string(c.role),
		Kind: "supervisor.mapreduce.component_failed",
		Payload: map[string]any{
			"spec_id":      specID,
			"component_id": comp.ID,
			"worker":       comp.Worker,
			"error":        err.Error(),
		},
	})
}

// recordMerge writes the reduce-phase verdict. A coherent report emits
// merge_validated; an incoherent one additionally emits incoherent with the
// specific gaps so an operator or the Meta-Orchestrator can act.
func (c *MapReduceCoordinator) recordMerge(ctx context.Context, report MergeReport) {
	_ = c.trail.Append(ctx, decisiontrail.Record{
		Role: string(c.role),
		Kind: "supervisor.mapreduce.merge_validated",
		Payload: map[string]any{
			"spec_id":                 report.SpecID,
			"coherent":                report.Coherent,
			"failed_components":       report.FailedComponents,
			"contradiction_count":     len(report.Contradictions),
			"unsatisfied_constraints": report.UnsatisfiedConstraints,
			"interface_gap_count":     len(report.InterfaceGaps),
		},
	})
	if report.Coherent {
		return
	}
	_ = c.trail.Append(ctx, decisiontrail.Record{
		Role: string(c.role),
		Kind: "supervisor.mapreduce.incoherent",
		Payload: map[string]any{
			"spec_id":                 report.SpecID,
			"failed_components":       report.FailedComponents,
			"contradictions":          contradictionKeys(report.Contradictions),
			"unsatisfied_constraints": report.UnsatisfiedConstraints,
			"interface_gaps":          interfaceGapNames(report.InterfaceGaps),
			"action":                  "do not merge spec — resolve contradictions, cover constraints, fill interface gaps",
		},
	})
}

// ValidateCoherence is the reduce-phase merge validator. It checks the
// assembled component results against the spec along three axes:
//
//   - No contradictions: no two components assert different values for the
//     same decision key.
//   - All constraints satisfied: every spec Constraint ID appears in some
//     component's SatisfiedConstraints.
//   - Interfaces align: every interface a component Consumes, and every
//     interface the spec RequiredInterfaces, is in some component's
//     ProvidedInterfaces.
//
// It is a pure function (no trail, no dispatch) so it can be unit-tested and
// reused independently of the coordinator. The returned report's
// FailedComponents field is left empty — the coordinator fills it from the
// map phase. Coherent is true iff there are no contradictions, no unsatisfied
// constraints, and no interface gaps.
func ValidateCoherence(spec Spec, components []Component, results []ComponentResult) MergeReport {
	report := MergeReport{SpecID: spec.ID}

	report.Contradictions = detectContradictions(results)
	report.UnsatisfiedConstraints = detectUnsatisfiedConstraints(spec, results)
	report.InterfaceGaps = detectInterfaceGaps(spec, components, results)

	report.Coherent = len(report.Contradictions) == 0 &&
		len(report.UnsatisfiedConstraints) == 0 &&
		len(report.InterfaceGaps) == 0
	return report
}

// detectContradictions groups every assertion by key and flags any key with
// more than one distinct value across components.
func detectContradictions(results []ComponentResult) []Contradiction {
	// key → (componentID → value)
	byKey := map[string]map[string]string{}
	// key → set of distinct values (to decide if it's a contradiction)
	values := map[string]map[string]struct{}{}

	for _, r := range results {
		for _, a := range r.Assertions {
			if byKey[a.Key] == nil {
				byKey[a.Key] = map[string]string{}
				values[a.Key] = map[string]struct{}{}
			}
			byKey[a.Key][r.ComponentID] = a.Value
			values[a.Key][a.Value] = struct{}{}
		}
	}

	var out []Contradiction
	for key, distinct := range values {
		if len(distinct) < 2 {
			continue
		}
		out = append(out, Contradiction{Key: key, Values: byKey[key]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// detectUnsatisfiedConstraints returns the sorted spec Constraint IDs that no
// component reported satisfying.
func detectUnsatisfiedConstraints(spec Spec, results []ComponentResult) []string {
	satisfied := map[string]struct{}{}
	for _, r := range results {
		for _, id := range r.SatisfiedConstraints {
			satisfied[id] = struct{}{}
		}
	}
	var out []string
	for _, c := range spec.Constraints {
		if _, ok := satisfied[c.ID]; !ok {
			out = append(out, c.ID)
		}
	}
	sort.Strings(out)
	return out
}

// detectInterfaceGaps returns interfaces that are consumed (by a component) or
// required (by the spec) but provided by no component result.
func detectInterfaceGaps(spec Spec, components []Component, results []ComponentResult) []InterfaceGap {
	provided := map[string]struct{}{}
	for _, r := range results {
		for _, name := range r.ProvidedInterfaces {
			provided[name] = struct{}{}
		}
	}

	// needers: interface name → set of "who needs it"
	needers := map[string]map[string]struct{}{}
	add := func(iface, by string) {
		if _, ok := provided[iface]; ok {
			return
		}
		if needers[iface] == nil {
			needers[iface] = map[string]struct{}{}
		}
		needers[iface][by] = struct{}{}
	}

	for _, comp := range components {
		for _, iface := range comp.Consumes {
			add(iface, comp.ID)
		}
	}
	for _, iface := range spec.RequiredInterfaces {
		add(iface, "spec:"+spec.ID)
	}

	var out []InterfaceGap
	for iface, by := range needers {
		needed := make([]string, 0, len(by))
		for who := range by {
			needed = append(needed, who)
		}
		sort.Strings(needed)
		out = append(out, InterfaceGap{Interface: iface, NeededBy: needed})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Interface < out[j].Interface })
	return out
}

func contradictionKeys(cs []Contradiction) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Key)
	}
	return out
}

func interfaceGapNames(gaps []InterfaceGap) []string {
	out := make([]string, 0, len(gaps))
	for _, g := range gaps {
		out = append(out, g.Interface)
	}
	return out
}
