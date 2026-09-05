package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// fakeCheck is a minimal Check used to populate a test registry.
type fakeCheck struct {
	id  string
	cad audit.Cadence
}

func (f fakeCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{ID: f.id, Cadence: f.cad, SeverityCeiling: audit.SeverityP1}
}

func (fakeCheck) Run(_ context.Context, _ audit.Env) (audit.Result, error) {
	return audit.Result{Status: audit.StatusOK}, nil
}

func TestLoadMissingFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	f, err := Load(dir)
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if f != nil {
		t.Errorf("missing file should return nil, got %+v", f)
	}
}

func TestLoadAndBuildPlanFromConfig(t *testing.T) {
	dir := t.TempDir()
	yml := `version: 1
repo: demo
audits:
  schedule:
    daily:
      - check: build
      - check: test
        config: { race: true }
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	reg := audit.NewRegistry()
	for _, c := range []audit.Check{
		fakeCheck{id: "build", cad: audit.CadenceDaily},
		fakeCheck{id: "test", cad: audit.CadenceDaily},
		fakeCheck{id: "lint.go", cad: audit.CadenceDaily},
	} {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.Meta().ID, err)
		}
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	plan, err := BuildPlan(cfg, dir, audit.CadenceDaily, reg, "test")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Trees) != 1 {
		t.Fatalf("trees = %d, want 1", len(plan.Trees))
	}
	checks := plan.Trees[0].Checks
	if len(checks) != 2 {
		t.Fatalf("checks = %d, want 2 (config overrides defaults)", len(checks))
	}
	if v, _ := checks[1].Config["race"].(bool); !v {
		t.Errorf("race config should pass through; got %+v", checks[1].Config)
	}
}

func TestBuildPlanFallsBackToRegistryDefaults(t *testing.T) {
	reg := audit.NewRegistry()
	if err := reg.Register(fakeCheck{id: "build", cad: audit.CadenceDaily}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Register(fakeCheck{id: "weekly-only", cad: audit.CadenceWeekly}); err != nil {
		t.Fatalf("register: %v", err)
	}

	plan, err := BuildPlan(nil, "/tmp/demo", audit.CadenceDaily, reg, "test")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	checks := plan.Trees[0].Checks
	if len(checks) != 1 || checks[0].CheckID != "build" {
		t.Errorf("expected only daily defaults; got %+v", checks)
	}
}

func TestBuildPlanRejectsUnknownCheck(t *testing.T) {
	dir := t.TempDir()
	yml := `version: 1
audits:
  schedule:
    daily:
      - check: nonexistent-check
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	reg := audit.NewRegistry()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := BuildPlan(cfg, dir, audit.CadenceDaily, reg, "test"); err == nil {
		t.Error("unknown check should fail BuildPlan")
	}
}

func TestBuildPlanTreesOverride(t *testing.T) {
	dir := t.TempDir()
	yml := `version: 1
audits:
  schedule:
    daily:
      - check: build
  trees:
    - path: ./sub
      checks-add:
        - check: lint.go
      checks-remove:
        - check: build
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	reg := audit.NewRegistry()
	for _, id := range []string{"build", "lint.go"} {
		if err := reg.Register(fakeCheck{id: id, cad: audit.CadenceDaily}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	plan, err := BuildPlan(cfg, dir, audit.CadenceDaily, reg, "test")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Trees) != 1 {
		t.Fatalf("trees = %d, want 1", len(plan.Trees))
	}
	got := plan.Trees[0].Checks
	if len(got) != 1 || got[0].CheckID != "lint.go" {
		t.Errorf("tree should have lint.go and lose build; got %+v", got)
	}
}

func TestBuildPlanInvalidSeverityKey(t *testing.T) {
	dir := t.TempDir()
	yml := `version: 1
audits:
  severity-policy:
    p9: { fail-run: true, remediate: auto }
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	reg := audit.NewRegistry()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := BuildPlan(cfg, dir, audit.CadenceDaily, reg, "test"); err == nil {
		t.Error("invalid severity key should fail BuildPlan")
	}
}

func TestBuildPlanRejectsInvalidRemediationStrategy(t *testing.T) {
	for _, value := range []string{`""`, "future"} {
		t.Run(value, func(t *testing.T) {
			dir := t.TempDir()
			yml := fmt.Sprintf(`version: 1
audits:
  severity-policy:
    P1:
      remediate: %s
`, value)
			if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if _, err := BuildPlan(cfg, dir, audit.CadenceDaily, audit.NewRegistry(), "test"); err == nil {
				t.Fatalf("BuildPlan accepted remediation strategy %s", value)
			}
		})
	}
}

func TestLoadRejectsNullSeverityFields(t *testing.T) {
	for _, field := range []string{"fail-run", "remediate", "notify"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			yml := fmt.Sprintf(`version: 1
audits:
  severity-policy:
    P1:
      %s: null
`, field)
			if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			if _, err := Load(dir); err == nil {
				t.Fatalf("Load accepted null %s", field)
			}
		})
	}
}

func TestLoadRejectsNullSeverityRule(t *testing.T) {
	dir := t.TempDir()
	yml := `version: 1
audits:
  severity-policy:
    P1: null
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("Load accepted a null severity rule")
	}
}

func TestBuildPlanProgrammaticSeverityRuleIsComplete(t *testing.T) {
	cfg := &File{Audits: &AuditsSection{SeverityPolicy: map[string]SeverityRule{
		"P2": {FailRun: true, Remediate: "noop", Notify: true},
	}}}
	plan, err := BuildPlan(cfg, t.TempDir(), audit.CadenceDaily, audit.NewRegistry(), "test")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	got := plan.SeverityPolicy[audit.SeverityP2]
	want := (audit.SeverityRule{FailRun: true, DefaultStrategy: audit.StrategyNoop, Notify: true})
	if got != want {
		t.Fatalf("programmatic p2 rule = %+v, want %+v", got, want)
	}

	cfg.Audits.SeverityPolicy["P2"] = SeverityRule{FailRun: true}
	if _, err := BuildPlan(cfg, t.TempDir(), audit.CadenceDaily, audit.NewRegistry(), "test"); err == nil {
		t.Fatal("BuildPlan accepted an incomplete programmatic severity rule")
	}
}

func TestBuildPlanSeverityOverridePreservesDefaultStrategyWhenRemediateOmitted(t *testing.T) {
	dir := t.TempDir()
	yml := `version: 1
audits:
  severity-policy:
    P2:
      fail-run: true
      notify: true
  schedule:
    daily: []
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	plan, err := BuildPlan(cfg, dir, audit.CadenceDaily, audit.NewRegistry(), "test")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	got := plan.SeverityPolicy[audit.SeverityP2]
	if got.DefaultStrategy != audit.StrategyIssue {
		t.Errorf("p2 default strategy = %q, want %q", got.DefaultStrategy, audit.StrategyIssue)
	}
	if !got.FailRun || !got.Notify {
		t.Errorf("p2 boolean overrides not retained: %+v", got)
	}
}

func TestBuildPlanSeverityOverridePreservesOmittedBooleanDefaults(t *testing.T) {
	dir := t.TempDir()
	yml := `version: 1
audits:
  severity-policy:
    P0:
      remediate: issue
    P1:
      fail-run: false
      notify: false
  schedule:
    daily: []
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	plan, err := BuildPlan(cfg, dir, audit.CadenceDaily, audit.NewRegistry(), "test")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	p0 := plan.SeverityPolicy[audit.SeverityP0]
	if !p0.FailRun || !p0.Notify || p0.DefaultStrategy != audit.StrategyIssue {
		t.Errorf("p0 override = %+v, want retained gates with issue strategy", p0)
	}
	p1 := plan.SeverityPolicy[audit.SeverityP1]
	if p1.FailRun || p1.Notify || p1.DefaultStrategy != audit.StrategyPR {
		t.Errorf("p1 override = %+v, want explicit false gates with retained PR strategy", p1)
	}
}
