package config

import (
	"context"
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
