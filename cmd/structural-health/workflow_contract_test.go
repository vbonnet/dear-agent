package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestStructuralHealthWorkflowContract(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "structural-health.yml")
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}

	var workflow structuralHealthWorkflow
	decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.On) != 2 {
		t.Fatalf("workflow events = %v, want exactly pull_request and push", workflowEventNames(workflow.On))
	}
	for _, name := range []string{"pull_request", "push"} {
		trigger, ok := workflow.On[name]
		if !ok {
			t.Fatalf("workflow lacks %s trigger", name)
		}
		assertMainOnlyTrigger(t, name, trigger)
	}
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Fatalf("workflow permissions = %v, want contents: read", workflow.Permissions)
	}
	if workflow.Concurrency.Group != "${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}" || !workflow.Concurrency.CancelInProgress {
		t.Fatalf("workflow concurrency = %+v, want PR/ref-isolated cancellation", workflow.Concurrency)
	}
	if len(workflow.Jobs) != 1 {
		t.Fatalf("workflow jobs = %d, want exactly structural-health", len(workflow.Jobs))
	}

	job, ok := workflow.Jobs["structural-health"]
	if !ok {
		t.Fatal("workflow lacks structural-health job")
	}
	if job.Name != "Structural Health (baselined)" {
		t.Fatalf("job name = %q", job.Name)
	}
	if job.TimeoutMinutes != 15 {
		t.Fatalf("job timeout = %d minutes, want 15", job.TimeoutMinutes)
	}
	if job.RunsOn != "ubuntu-latest" {
		t.Fatalf("job runs-on = %q, want ubuntu-latest", job.RunsOn)
	}
	if job.If != "" {
		t.Fatalf("job condition = %q; ordinary main PRs must not be gated", job.If)
	}
	if job.ContinueOnError {
		t.Fatal("job suppresses failures with continue-on-error")
	}
	if job.Needs != nil {
		t.Fatalf("job depends on %v; dependency failure could skip the check", job.Needs)
	}

	steps := make(map[string]structuralHealthWorkflowStep, len(job.Steps))
	for _, step := range job.Steps {
		if step.ContinueOnError {
			t.Fatalf("%s suppresses failures with continue-on-error", step.Name)
		}
		if step.If != "" {
			t.Fatalf("%s has status override %q", step.Name, step.If)
		}
		if _, duplicate := steps[step.Name]; duplicate {
			t.Fatalf("workflow repeats step name %q", step.Name)
		}
		steps[step.Name] = step
	}
	if len(steps) != 4 {
		t.Fatalf("workflow steps = %d, want exactly four required steps", len(steps))
	}
	for name, want := range map[string]struct {
		uses string
		with map[string]any
	}{
		"Checkout": {uses: "actions/checkout@"},
		"Set up Go": {
			uses: "actions/setup-go@",
			with: map[string]any{
				"go-version-file":       "go.mod",
				"cache":                 true,
				"cache-dependency-path": "go.sum",
			},
		},
	} {
		step, ok := steps[name]
		if !ok {
			t.Fatalf("workflow lacks %q step", name)
		}
		if !strings.HasPrefix(step.Uses, want.uses) || step.Run != "" || !reflect.DeepEqual(step.With, want.with) {
			t.Fatalf("%s uses %q, runs %q, and has inputs %v; want uses prefix %q with inputs %v", name, step.Uses, step.Run, step.With, want.uses, want.with)
		}
	}
	for name, wantRun := range map[string]string{
		"Download dependencies":      "go mod download",
		"Run structural-health scan": "go run ./cmd/structural-health",
	} {
		step, ok := steps[name]
		if !ok {
			t.Fatalf("workflow lacks %q step", name)
		}
		if strings.TrimSpace(step.Run) != wantRun || step.Uses != "" || len(step.With) != 0 {
			t.Fatalf("%s uses %q, runs %q, and has inputs %v; want exact failure-propagating command %q", name, step.Uses, step.Run, step.With, wantRun)
		}
	}

	text := string(raw)
	for _, forbidden := range []string{"full-ci", "paths:", "paths-ignore:", "continue-on-error:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("workflow contains forbidden activation or status override %q", forbidden)
		}
	}
}

type structuralHealthWorkflow struct {
	Name        string                                     `yaml:"name"`
	On          map[string]structuralHealthWorkflowTrigger `yaml:"on"`
	Concurrency structuralHealthWorkflowConcurrency        `yaml:"concurrency"`
	Permissions map[string]string                          `yaml:"permissions"`
	Jobs        map[string]structuralHealthWorkflowJob     `yaml:"jobs"`
}

type structuralHealthWorkflowTrigger struct {
	Branches    []string `yaml:"branches"`
	Types       []string `yaml:"types"`
	Paths       []string `yaml:"paths"`
	PathsIgnore []string `yaml:"paths-ignore"`
}

type structuralHealthWorkflowConcurrency struct {
	Group            string `yaml:"group"`
	CancelInProgress bool   `yaml:"cancel-in-progress"`
}

type structuralHealthWorkflowJob struct {
	Name            string                         `yaml:"name"`
	RunsOn          string                         `yaml:"runs-on"`
	TimeoutMinutes  int                            `yaml:"timeout-minutes"`
	If              string                         `yaml:"if"`
	Needs           any                            `yaml:"needs"`
	ContinueOnError bool                           `yaml:"continue-on-error"`
	Steps           []structuralHealthWorkflowStep `yaml:"steps"`
}

type structuralHealthWorkflowStep struct {
	Name            string         `yaml:"name"`
	Uses            string         `yaml:"uses"`
	Run             string         `yaml:"run"`
	With            map[string]any `yaml:"with"`
	If              string         `yaml:"if"`
	ContinueOnError bool           `yaml:"continue-on-error"`
}

func assertMainOnlyTrigger(t *testing.T, name string, trigger structuralHealthWorkflowTrigger) {
	t.Helper()
	if len(trigger.Branches) != 1 || trigger.Branches[0] != "main" {
		t.Fatalf("%s branches = %v, want [main]", name, trigger.Branches)
	}
	if len(trigger.Types) != 0 || len(trigger.Paths) != 0 || len(trigger.PathsIgnore) != 0 {
		t.Fatalf("%s narrows activation with types=%v paths=%v paths-ignore=%v", name, trigger.Types, trigger.Paths, trigger.PathsIgnore)
	}
}

func workflowEventNames(events map[string]structuralHealthWorkflowTrigger) []string {
	names := make([]string, 0, len(events))
	for name := range events {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
