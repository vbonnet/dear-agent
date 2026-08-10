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
	dependabotCandidate := strings.Contains(text, `(.dependabot_module_only_candidate | type == "boolean")`)
	planBody := strings.Contains(text, "PR_BODY: ${{ github.event.pull_request.body }}")
	key := strings.Index(text, "name: Detect review key")
	gate := strings.Contains(text, "steps.plan.outcome != 'success' || steps.plan.outputs.review_relevant == 'true' || steps.key.outputs.present == 'true'")
	dependabotGate := strings.Contains(text, "steps.dependabot.outputs.exempt != 'true' || steps.override.outputs.active == 'true'")
	neutral := strings.Contains(text, `[ "$PLAN_OUTCOME" = success ]`)
	irrelevant := strings.Contains(text, `[ "$REVIEW_RELEVANT" = false ]`)
	if plan < 0 || key < 0 || plan >= key {
		t.Fatalf("authenticated plan must precede credential detection: plan=%d key=%d", plan, key)
	}
	if !version {
		t.Fatalf("workflow does not authenticate review plan version %q", specContractVersion)
	}
	if !relevance || !dependabotCandidate || !planBody {
		t.Fatal("workflow plan does not authenticate review relevance from the PR body and deterministic escalation evidence")
	}
	if !gate || !dependabotGate {
		t.Fatal("relevant SPEC or deterministic escalation plan does not force the existing review gate to run")
	}
	if !neutral || !irrelevant {
		t.Fatal("paused neutral decision is not explicitly limited to irrelevant plans")
	}
	if strings.Contains(text, "no changed SPEC.md or protected SPEC-review owner") {
		t.Fatal("neutral wording omits deterministic escalation evidence")
	}
}

func TestWorkflowDependabotExceptionRequiresTrustedIdentityAndGitEvidence(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "review.yml")
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string            `yaml:"name"`
				If   string            `yaml:"if"`
				Env  map[string]string `yaml:"env"`
				Run  string            `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	steps := make(map[string]struct {
		If  string
		Env map[string]string
		Run string
	})
	for _, step := range workflow.Jobs["review"].Steps {
		steps[step.Name] = struct {
			If  string
			Env map[string]string
			Run string
		}{If: step.If, Env: step.Env, Run: step.Run}
	}
	auth := steps["Authenticate Dependabot module-only exception"]
	if auth.If != "steps.plan.outcome == 'success'" {
		t.Fatalf("Dependabot authentication condition = %q", auth.If)
	}
	wantEnv := map[string]string{
		"CANDIDATE":       "${{ steps.plan.outputs.dependabot_module_only_candidate }}",
		"PR_AUTHOR_LOGIN": "${{ github.event.pull_request.user.login }}",
		"PR_AUTHOR_ID":    "${{ github.event.pull_request.user.id }}",
		"PR_AUTHOR_TYPE":  "${{ github.event.pull_request.user.type }}",
		"HEAD_REPO_ID":    "${{ github.event.pull_request.head.repo.id }}",
		"BASE_REPO_ID":    "${{ github.event.repository.id }}",
	}
	if len(auth.Env) != len(wantEnv) {
		t.Fatalf("Dependabot authentication environment keys = %v, want exactly %v", auth.Env, wantEnv)
	}
	for name, want := range wantEnv {
		if auth.Env[name] != want {
			t.Errorf("Dependabot authentication %s = %q, want %q", name, auth.Env[name], want)
		}
	}
	for _, predicate := range []string{
		`[ "$CANDIDATE" = true ]`,
		`[ "$PR_AUTHOR_LOGIN" = 'dependabot[bot]' ]`,
		`[ "$PR_AUTHOR_ID" = 49699333 ]`,
		`[ "$PR_AUTHOR_TYPE" = Bot ]`,
		`[ "$HEAD_REPO_ID" = "$BASE_REPO_ID" ]`,
	} {
		if !strings.Contains(auth.Run, predicate) {
			t.Errorf("Dependabot authentication omits %s", predicate)
		}
	}
	for _, untrusted := range []string{"github.actor", "head.ref", "labels"} {
		if strings.Contains(auth.Run, untrusted) {
			t.Errorf("Dependabot authentication trusts %q", untrusted)
		}
	}
	gate := steps["Run AI review gate"]
	if !strings.Contains(gate.If, "steps.dependabot.outputs.exempt != 'true' || steps.override.outputs.active == 'true'") {
		t.Fatal("authenticated Dependabot exception does not guard model execution")
	}
	publish := steps["Publish check on reviewed head"]
	if publish.Env["DEPENDABOT_EXEMPT"] != "${{ steps.dependabot.outputs.exempt }}" ||
		!strings.Contains(publish.Run, `[ "$DEPENDABOT_EXEMPT" = true ]`) ||
		!strings.Contains(publish.Run, "authenticated same-repository Dependabot dependency-version-led Go module update") {
		t.Fatal("Dependabot exception does not publish an explicit neutral, non-model verdict")
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
			Steps          []struct {
				Name           string `yaml:"name"`
				TimeoutMinutes int    `yaml:"timeout-minutes"`
				Env            struct {
					ReviewDeadlineSeconds int `yaml:"REVIEW_DEADLINE_SECONDS"`
				} `yaml:"env"`
				Run string `yaml:"run"`
			} `yaml:"steps"`
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
	steps := workflow.Jobs["review"].Steps
	if len(steps) == 0 || steps[0].Name != "Mark reviewed head pending" {
		t.Fatal("trusted review deadline is not established by the first workflow step")
	}
	if time.Duration(steps[0].Env.ReviewDeadlineSeconds)*time.Second != reviewWorkflowDeadlineOffset {
		t.Fatalf("workflow absolute cutoff offset = %d seconds, want %s", steps[0].Env.ReviewDeadlineSeconds, reviewWorkflowDeadlineOffset)
	}
	if !strings.Contains(steps[0].Run, "date -u +%s") || !strings.Contains(steps[0].Run, reviewAbsoluteDeadlineEnv+"=$review_deadline") || !strings.Contains(steps[0].Run, ">> \"$GITHUB_ENV\"") {
		t.Fatal("first workflow step does not export the trusted absolute review deadline")
	}
	if reviewWorkflowTimeout-reviewWorkflowDeadlineOffset != reviewWorkflowPublicationReserve || reviewWorkflowPublicationReserve < 2*time.Minute {
		t.Fatalf("workflow publication reserve = %s, want a bound of at least two minutes", reviewWorkflowPublicationReserve)
	}
	stepByName := make(map[string]struct {
		TimeoutMinutes int
		Run            string
	}, len(steps))
	for _, step := range steps {
		stepByName[step.Name] = struct {
			TimeoutMinutes int
			Run            string
		}{TimeoutMinutes: step.TimeoutMinutes, Run: step.Run}
	}
	for _, name := range []string{"Checkout trusted base revision", "Set up Go"} {
		step := stepByName[name]
		if step.TimeoutMinutes <= 0 || step.TimeoutMinutes > 10 {
			t.Fatalf("%s timeout = %d minutes, want a bounded setup step", name, step.TimeoutMinutes)
		}
	}
	boundedCalls := map[string]int{
		"Fetch PR head for diff":                          3,
		"Build authenticated SPEC governance review plan": 1,
		"Clear override for a new pull request revision":  1,
		"Attest override to the reviewed revision":        1,
		"Detect override label":                           3,
		"Run AI review gate":                              2,
	}
	for name, minimum := range boundedCalls {
		run := stepByName[name].Run
		if strings.Count(run, "timeout --signal=TERM --kill-after=5s") < minimum || !strings.Contains(run, reviewAbsoluteDeadlineEnv) {
			t.Fatalf("%s does not bound all pre-publication work to the absolute cutoff", name)
		}
	}
	if !strings.Contains(text, "90s gh api --method PATCH") {
		t.Fatal("workflow does not bound final check publication inside the reserved window")
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
