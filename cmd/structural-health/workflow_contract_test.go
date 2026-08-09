package main

import (
	"os"
	"path/filepath"
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
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	assertMainOnlyTrigger(t, "pull_request", workflow.On.PullRequest)
	assertMainOnlyTrigger(t, "push", workflow.On.Push)

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
	if job.If != "" {
		t.Fatalf("job condition = %q; ordinary main PRs must not be gated", job.If)
	}
	if job.ContinueOnError {
		t.Fatal("job suppresses failures with continue-on-error")
	}

	steps := make(map[string]structuralHealthWorkflowStep, len(job.Steps))
	for _, step := range job.Steps {
		if step.ContinueOnError {
			t.Fatalf("%s suppresses failures with continue-on-error", step.Name)
		}
		if step.If != "" {
			t.Fatalf("%s has status override %q", step.Name, step.If)
		}
		steps[step.Name] = step
	}
	for name, wantRun := range map[string]string{
		"Checkout":                   "actions/checkout@",
		"Set up Go":                  "actions/setup-go@",
		"Download dependencies":      "go mod download",
		"Run structural-health scan": "go run ./cmd/structural-health",
	} {
		step, ok := steps[name]
		if !ok {
			t.Fatalf("workflow lacks %q step", name)
		}
		if !strings.Contains(step.Uses+step.Run, wantRun) {
			t.Fatalf("%s does not run %q", name, wantRun)
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
	On struct {
		PullRequest structuralHealthWorkflowTrigger `yaml:"pull_request"`
		Push        structuralHealthWorkflowTrigger `yaml:"push"`
	} `yaml:"on"`
	Jobs map[string]structuralHealthWorkflowJob `yaml:"jobs"`
}

type structuralHealthWorkflowTrigger struct {
	Branches []string `yaml:"branches"`
}

type structuralHealthWorkflowJob struct {
	Name            string                         `yaml:"name"`
	TimeoutMinutes  int                            `yaml:"timeout-minutes"`
	If              string                         `yaml:"if"`
	ContinueOnError bool                           `yaml:"continue-on-error"`
	Steps           []structuralHealthWorkflowStep `yaml:"steps"`
}

type structuralHealthWorkflowStep struct {
	Name            string `yaml:"name"`
	Uses            string `yaml:"uses"`
	Run             string `yaml:"run"`
	If              string `yaml:"if"`
	ContinueOnError bool   `yaml:"continue-on-error"`
}

func assertMainOnlyTrigger(t *testing.T, name string, trigger structuralHealthWorkflowTrigger) {
	t.Helper()
	if len(trigger.Branches) != 1 || trigger.Branches[0] != "main" {
		t.Fatalf("%s branches = %v, want [main]", name, trigger.Branches)
	}
}
