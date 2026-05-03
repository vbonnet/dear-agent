package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeCheck is a Check implementation that returns the findings it
// was constructed with. Used to drive runner tests deterministically.
type fakeCheck struct {
	id       string
	cadence  Cadence
	ceiling  Severity
	findings []Finding
	err      error
	status   CheckStatus
}

func (f fakeCheck) Meta() CheckMeta {
	cad := f.cadence
	if cad == "" {
		cad = CadenceDaily
	}
	ceil := f.ceiling
	if ceil == "" {
		ceil = SeverityP0
	}
	return CheckMeta{ID: f.id, Cadence: cad, SeverityCeiling: ceil}
}

func (f fakeCheck) Run(_ context.Context, _ Env) (Result, error) {
	if f.err != nil {
		return Result{Status: StatusError}, f.err
	}
	st := f.status
	if st == "" {
		st = StatusOK
	}
	return Result{Status: st, Findings: f.findings}, nil
}

func newTestRunner(t *testing.T, checks ...Check) (*Runner, *MemoryStore, *Registry) {
	t.Helper()
	reg := NewRegistry()
	for _, c := range checks {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.Meta().ID, err)
		}
	}
	store := NewMemoryStore()
	r := NewRunner()
	r.Registry = reg
	r.Store = store
	r.Now = func() time.Time { return time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC) }
	r.IDGen = func() string { return "test-id" }
	return r, store, reg
}

func TestRunnerHappyPath(t *testing.T) {
	check := fakeCheck{
		id: "demo",
		findings: []Finding{
			{Fingerprint: "f1", Severity: SeverityP2, Title: "minor"},
		},
	}
	r, store, _ := newTestRunner(t, check)
	r.IDGen = sequenceIDs("run-1", "find-1")

	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo", Checks: []ScheduledCheck{{CheckID: "demo"}}}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.AuditRun.State != AuditRunSucceeded {
		t.Errorf("state = %s, want succeeded", report.AuditRun.State)
	}
	if len(report.CheckOutcomes) != 1 || len(report.CheckOutcomes[0].Findings) != 1 {
		t.Fatalf("expected 1 finding via 1 outcome, got %+v", report.CheckOutcomes)
	}
	got := report.CheckOutcomes[0].Findings[0]
	if got.Repo != "demo" || got.CheckID != "demo" || got.State != FindingOpen {
		t.Errorf("finding metadata wrong: %+v", got)
	}
	c, _ := store.CountFindings(context.Background(), "demo")
	if c.Open != 1 {
		t.Errorf("counts: %+v", c)
	}
}

func TestRunnerSeverityClamp(t *testing.T) {
	check := fakeCheck{
		id:      "limited",
		ceiling: SeverityP2,
		findings: []Finding{
			{Fingerprint: "f1", Severity: SeverityP0, Title: "trying to scream"},
		},
	}
	r, _, _ := newTestRunner(t, check)
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/d", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/d", Checks: []ScheduledCheck{{CheckID: "limited"}}}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := report.CheckOutcomes[0].Findings[0]
	if got.Severity != SeverityP2 {
		t.Errorf("severity = %s, want clamped to P2", got.Severity)
	}
}

func TestRunnerCheckErrorMakesPartial(t *testing.T) {
	bad := fakeCheck{id: "broken", err: errors.New("toolchain missing")}
	r, _, _ := newTestRunner(t, bad)
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/d", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/d", Checks: []ScheduledCheck{{CheckID: "broken"}}}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.AuditRun.State != AuditRunPartial {
		t.Errorf("state = %s, want partial", report.AuditRun.State)
	}
}

func TestRunnerP0FindingFails(t *testing.T) {
	check := fakeCheck{
		id: "fatal",
		findings: []Finding{
			{Fingerprint: "f1", Severity: SeverityP0, Title: "broken build"},
		},
	}
	r, _, _ := newTestRunner(t, check)
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/d", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/d", Checks: []ScheduledCheck{{CheckID: "fatal"}}}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.AuditRun.State != AuditRunFailed {
		t.Errorf("state = %s, want failed", report.AuditRun.State)
	}
}

func TestRunnerPreflightUnknownCheck(t *testing.T) {
	r, _, _ := newTestRunner(t)
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/d", Cadence: CadenceDaily,
		Trees: []TreePlan{{WorkingDir: "/tmp/d", Checks: []ScheduledCheck{{CheckID: "ghost"}}}},
	}
	if _, err := r.Run(context.Background(), plan); err == nil {
		t.Error("expected error for unknown check")
	}
}

// sequenceIDs returns an idGen that yields the given strings in
// order, then panics if asked for more — keeps test failures loud
// when the runner unexpectedly creates extra rows.
func sequenceIDs(ids ...string) func() string {
	i := 0
	return func() string {
		if i >= len(ids) {
			panic("sequenceIDs exhausted")
		}
		out := ids[i]
		i++
		return out
	}
}
