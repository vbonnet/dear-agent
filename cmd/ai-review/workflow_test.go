package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestWorkflowBuildsSpecPlanBeforeCredentialDecision(t *testing.T) {
	workflow := filepath.Join("..", "..", ".github", "workflows", "review.yml")
	raw, err := os.ReadFile(workflow)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	plan := strings.Index(text, "name: Build authenticated SPEC governance review plan")
	version := strings.Contains(text, `(.version == "`+specContractVersion+`")`)
	relevance := strings.Contains(text, `(.review_relevant | type == "boolean")`)
	planBody := strings.Contains(text, "PR_BODY: ${{ github.event.pull_request.body }}")
	key := strings.Index(text, "name: Detect review key")
	gate := strings.Contains(text, "if: steps.plan.outcome != 'success' || steps.plan.outputs.review_relevant == 'true' || steps.key.outputs.present == 'true'")
	neutral := strings.Contains(text, `[ "$PLAN_OUTCOME" = success ]`)
	irrelevant := strings.Contains(text, `[ "$REVIEW_RELEVANT" = false ]`)
	if plan < 0 || key < 0 || plan >= key {
		t.Fatalf("authenticated plan must precede credential detection: plan=%d key=%d", plan, key)
	}
	if !version {
		t.Fatalf("workflow does not authenticate review plan version %q", specContractVersion)
	}
	if !relevance || !planBody {
		t.Fatal("workflow plan does not authenticate review relevance from the PR body and deterministic escalation evidence")
	}
	if !gate {
		t.Fatal("relevant SPEC or deterministic escalation plan does not force the existing review gate to run")
	}
	if !neutral || !irrelevant {
		t.Fatal("paused neutral decision is not explicitly limited to irrelevant plans")
	}
	if strings.Contains(text, "no changed SPEC.md or protected SPEC-review owner") {
		t.Fatal("neutral wording omits deterministic escalation evidence")
	}
}

func TestWorkflowPublishesUniqueSpecContractReviewCheck(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "review.yml")
	rawWorkflow, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Name           string `yaml:"name"`
			TimeoutMinutes int    `yaml:"timeout-minutes"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(rawWorkflow, &workflow); err != nil {
		t.Fatal(err)
	}
	text := string(rawWorkflow)
	if strings.Contains(text, "merge_group:") {
		t.Fatal("workflow must not add a merge_group trigger before a merge queue exists")
	}
	if workflow.Jobs["review"].Name != "AI review orchestration" {
		t.Fatalf("native job name = %q, want distinct orchestration name", workflow.Jobs["review"].Name)
	}
	if time.Duration(workflow.Jobs["review"].TimeoutMinutes)*time.Minute != reviewWorkflowTimeout {
		t.Fatalf("workflow review timeout = %d minutes, want %s", workflow.Jobs["review"].TimeoutMinutes, reviewWorkflowTimeout)
	}
	if !strings.Contains(text, "-f name='SPEC Contract Review'") || !strings.Contains(text, "-f output[title]='SPEC Contract Review'") {
		t.Fatal("workflow does not publish the unique authoritative SPEC Contract Review check")
	}
	if !strings.Contains(text, "name: Fail if SPEC Contract Review publication failed") {
		t.Fatal("native orchestration job does not fail when custom check publication fails")
	}
	specification, err := os.ReadFile("SPEC.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(specification), "Neither source result is yet a provider-required merge gate") ||
		!strings.Contains(string(specification), "cannot be required on the pull-request head") ||
		!strings.Contains(string(specification), "must not be made provider-required") {
		t.Fatal("source contract does not preserve the unresolved head-attached transport boundary")
	}
	if strings.Contains(text, ".github/rulesets/main.json") || strings.Contains(strings.ToLower(text), "required governance gate") {
		t.Fatal("workflow must not claim provider-required ruleset enforcement")
	}
}
