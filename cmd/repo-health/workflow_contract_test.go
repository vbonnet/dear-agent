package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	golangciLintAction       = "golangci/golangci-lint-action"
	golangciLintActionV9SHA  = "ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a"
	golangciLintActionPinned = golangciLintAction + "@" + golangciLintActionV9SHA
)

type workflowDocument struct {
	Env  map[string]yaml.Node   `yaml:"env"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Steps []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Name string               `yaml:"name"`
	Uses string               `yaml:"uses"`
	Run  string               `yaml:"run"`
	With map[string]yaml.Node `yaml:"with"`
}

func TestRepoHealthWorkflowUsesOwnedLinterInstaller(t *testing.T) {
	t.Parallel()

	health, healthSource := loadWorkflow(t, "health-check.yml")
	auditJob, ok := health.Jobs["audit"]
	if !ok {
		t.Fatal("health-check workflow has no audit job")
	}
	installer := namedWorkflowStep(t, auditJob, "Install golangci-lint")
	if installer.Uses != golangciLintActionPinned {
		t.Errorf("health installer uses %q, want immutable %q", installer.Uses, golangciLintActionPinned)
	}
	if strings.TrimSpace(installer.Run) != "" {
		t.Errorf("health installer must not use a shell command; got %q", installer.Run)
	}
	if strings.Contains(string(healthSource), "raw.githubusercontent.com/golangci/golangci-lint/master/install.sh") {
		t.Error("health workflow still references the obsolete master install script")
	}

	wantInstallerFields := []struct {
		name  string
		value string
	}{
		{name: "version", value: "${{ env.GOLANGCI_VERSION }}"},
		{name: "install-mode", value: "goinstall"},
		{name: "install-only", value: "true"},
		{name: "skip-save-cache", value: "true"},
	}
	for _, field := range wantInstallerFields {
		if got := workflowScalar(t, installer.With, field.name); got != field.value {
			t.Errorf("health installer %s = %q, want %q", field.name, got, field.value)
		}
	}

	ci, _ := loadWorkflow(t, "ci.yml")
	ciJob, ok := ci.Jobs["ci"]
	if !ok {
		t.Fatal("CI workflow has no ci job")
	}
	ciLint := usingWorkflowStep(t, ciJob, golangciLintAction)
	healthVersion := health.Env["GOLANGCI_VERSION"].Value
	if healthVersion == "" {
		t.Fatal("health workflow has no GOLANGCI_VERSION")
	}
	if ciVersion := workflowScalar(t, ciLint.With, "version"); ciVersion != healthVersion {
		t.Errorf("golangci-lint version drift: health=%q CI=%q", healthVersion, ciVersion)
	}
}

func loadWorkflow(t *testing.T, name string) (workflowDocument, []byte) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate workflow contract test")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", ".github", "workflows", name)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow %s: %v", name, err)
	}
	var workflow workflowDocument
	if err := yaml.Unmarshal(source, &workflow); err != nil {
		t.Fatalf("parse workflow %s: %v", name, err)
	}
	return workflow, source
}

func namedWorkflowStep(t *testing.T, job workflowJob, name string) workflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("workflow job has no %q step", name)
	return workflowStep{}
}

func usingWorkflowStep(t *testing.T, job workflowJob, action string) workflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, action+"@") {
			return step
		}
	}
	t.Fatalf("workflow job has no step using %q at any ref", action)
	return workflowStep{}
}

func workflowScalar(t *testing.T, values map[string]yaml.Node, field string) string {
	t.Helper()
	value, ok := values[field]
	if !ok {
		t.Errorf("workflow step has no %q field", field)
		return ""
	}
	if value.Kind != yaml.ScalarNode {
		t.Errorf("workflow field %q is not a scalar", field)
		return ""
	}
	return value.Value
}
