package main

import (
	"fmt"
	"os"
	"os/exec"
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
	planBody := strings.Contains(text, `PR_BODY="$(jq -r '.body // ""' "$AI_REVIEW_CURRENT_PR_JSON")"`)
	resolvedRevision := strings.Contains(text, "BASE_SHA: ${{ steps.pending.outputs.base_sha }}") &&
		strings.Contains(text, "HEAD_SHA: ${{ steps.pending.outputs.head_sha }}")
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
	if !relevance || !dependabotCandidate || !planBody || !resolvedRevision {
		t.Fatal("workflow plan does not authenticate review relevance from the current API-resolved revision, PR body, and deterministic escalation evidence")
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
		"GH_TOKEN":        "${{ secrets.GITHUB_TOKEN }}",
		"REPO":            "${{ github.repository }}",
		"PR":              "${{ github.event.pull_request.number }}",
		"BASE_SHA":        "${{ steps.pending.outputs.base_sha }}",
		"HEAD_SHA":        "${{ steps.pending.outputs.head_sha }}",
		"CANDIDATE":       "${{ steps.plan.outputs.dependabot_module_only_candidate }}",
		"PR_AUTHOR_LOGIN": "${{ steps.pending.outputs.pr_author_login }}",
		"PR_AUTHOR_ID":    "${{ steps.pending.outputs.pr_author_id }}",
		"PR_AUTHOR_TYPE":  "${{ steps.pending.outputs.pr_author_type }}",
		"HEAD_REPO_ID":    "${{ steps.pending.outputs.head_repo_id }}",
		"BASE_REPO_ID":    "${{ steps.pending.outputs.base_repo_id }}",
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
		`[ "$CANDIDATE" != true ]`,
		`[ "$PR_AUTHOR_LOGIN" != 'dependabot[bot]' ]`,
		`[ "$PR_AUTHOR_ID" != 49699333 ]`,
		`[ "$PR_AUTHOR_TYPE" != Bot ]`,
		`[ "$HEAD_REPO_ID" != "$BASE_REPO_ID" ]`,
		`"repos/$REPO/commits/$HEAD_SHA"`,
		`.author.id == 49699333`,
		`.committer.id == 19864447`,
		`.parents[0].sha == $base`,
		`.event == "head_ref_force_pushed" and .commit_id == $head`,
		`(.user.id == $id)`,
		`(.role_name == "maintain" or .role_name == "admin" or .permission == "admin")`,
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

func TestWorkflowDependabotHeadAuthenticationFailsClosed(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "review.yml")
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	var authRun string
	for _, step := range workflow.Jobs["review"].Steps {
		if step.Name == "Authenticate Dependabot module-only exception" {
			authRun = step.Run
			break
		}
	}
	if authRun == "" {
		t.Fatal("Dependabot authentication step is missing")
	}

	const (
		head = "1111111111111111111111111111111111111111"
		base = "2222222222222222222222222222222222222222"
	)
	validCommit := dependabotHeadCommitJSON(head, base)
	tests := []struct {
		name       string
		commitJSON string
		eventsJSON string
		permission string
		failAPI    string
		want       bool
	}{
		{
			name:       "positive original live shape",
			commitJSON: validCommit,
			eventsJSON: `[[{"event":"labeled","actor":{"login":"dependabot[bot]","id":49699333,"type":"Bot"}}]]`,
			want:       true,
		},
		{
			name:       "positive maintainer triggered rebase",
			commitJSON: validCommit,
			eventsJSON: `[[{"event":"head_ref_force_pushed","commit_id":"` + head + `","actor":{"login":"maintainer","id":42,"type":"User"}}]]`,
			permission: `{"permission":"write","role_name":"maintain","user":{"login":"maintainer","id":42,"type":"User"}}`,
			want:       true,
		},
		{
			name:       "mismatched current head SHA",
			commitJSON: dependabotHeadCommitJSON("3333333333333333333333333333333333333333", base),
			eventsJSON: `[[]]`,
		},
		{
			name:       "mismatched parent SHA",
			commitJSON: dependabotHeadCommitJSON(head, "3333333333333333333333333333333333333333"),
			eventsJSON: `[[]]`,
		},
		{
			name:       "non Dependabot author login",
			commitJSON: strings.Replace(validCommit, `"login":"dependabot[bot]"`, `"login":"attacker"`, 1),
			eventsJSON: `[[]]`,
		},
		{
			name:       "non Dependabot author id",
			commitJSON: strings.Replace(validCommit, `"id":49699333`, `"id":42`, 1),
			eventsJSON: `[[]]`,
		},
		{
			name:       "non Dependabot author type",
			commitJSON: strings.Replace(validCommit, `"type":"Bot"`, `"type":"User"`, 1),
			eventsJSON: `[[]]`,
		},
		{
			name:       "missing API identity fields",
			commitJSON: `{"sha":"` + head + `","commit":{},"parents":[{"sha":"` + base + `"}]}`,
			eventsJSON: `[[]]`,
		},
		{
			name:       "commit API failure",
			commitJSON: validCommit,
			eventsJSON: `[[]]`,
			failAPI:    "commit",
		},
		{
			name:       "timeline API failure",
			commitJSON: validCommit,
			eventsJSON: `[[]]`,
			failAPI:    "events",
		},
		{
			name:       "malformed timeline event preserves publication path",
			commitJSON: validCommit,
			eventsJSON: `[[1]]`,
		},
		{
			name:       "ordinary writer replaced head",
			commitJSON: validCommit,
			eventsJSON: `[[{"event":"head_ref_force_pushed","commit_id":"` + head + `","actor":{"login":"writer","id":43,"type":"User"}}]]`,
			permission: `{"permission":"write","user":{"login":"writer","id":43,"type":"User"}}`,
		},
		{
			name:       "permission identity does not match timeline actor",
			commitJSON: validCommit,
			eventsJSON: `[[{"event":"head_ref_force_pushed","commit_id":"` + head + `","actor":{"login":"maintainer","id":42,"type":"User"}}]]`,
			permission: `{"permission":"admin","role_name":"admin","user":{"login":"maintainer","id":99,"type":"User"}}`,
		},
		{
			name:       "permission API failure",
			commitJSON: validCommit,
			eventsJSON: `[[{"event":"head_ref_force_pushed","commit_id":"` + head + `","actor":{"login":"maintainer","id":42,"type":"User"}}]]`,
			failAPI:    "permission",
		},
		{
			name:       "force push event belongs to older head",
			commitJSON: validCommit,
			eventsJSON: `[[{"event":"head_ref_force_pushed","commit_id":"3333333333333333333333333333333333333333","actor":{"login":"dependabot[bot]","id":49699333,"type":"Bot"}}]]`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runDependabotHeadAuthentication(t, authRun, head, base, test.commitJSON, test.eventsJSON, test.permission, test.failAPI)
			if got != test.want {
				t.Fatalf("exempt = %v, want %v", got, test.want)
			}
		})
	}
}

func dependabotHeadCommitJSON(head, base string) string {
	return fmt.Sprintf(`{
		"sha":%q,
		"author":{"login":"dependabot[bot]","id":49699333,"type":"Bot"},
		"committer":{"login":"web-flow","id":19864447,"type":"User"},
		"commit":{
			"author":{"name":"dependabot[bot]","email":"49699333+dependabot[bot]@users.noreply.github.com"},
			"committer":{"name":"GitHub","email":"noreply@github.com"}
		},
		"parents":[{"sha":%q}]
	}`, head, base)
}

func runDependabotHeadAuthentication(t *testing.T, run, head, base, commitJSON, eventsJSON, permission, failAPI string) bool {
	t.Helper()
	tmp := t.TempDir()
	output := filepath.Join(tmp, "github-output")
	gh := filepath.Join(tmp, "gh")
	fakeGH := `#!/bin/sh
case "$*" in
  *"/commits/"*)
    [ "$FAKE_GH_FAIL" = commit ] && exit 1
    printf '%s\n' "$FAKE_COMMIT_JSON"
    ;;
  *"/issues/"*"/events"*)
    [ "$FAKE_GH_FAIL" = events ] && exit 1
    printf '%s\n' "$FAKE_EVENTS_JSON"
    ;;
  *"/collaborators/"*"/permission"*)
    [ "$FAKE_GH_FAIL" = permission ] && exit 1
    printf '%s\n' "$FAKE_PERMISSION_JSON"
    ;;
  *) exit 64 ;;
esac
`
	if err := os.WriteFile(gh, []byte(fakeGH), 0o700); err != nil {
		t.Fatal(err)
	}
	timeout := filepath.Join(tmp, "timeout")
	fakeTimeout := `#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in
    --signal=*|--kill-after=*) shift ;;
    *s) shift; break ;;
    *) exit 64 ;;
  esac
done
exec "$@"
`
	if err := os.WriteFile(timeout, []byte(fakeTimeout), 0o700); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Minute).Unix()
	cmd := exec.Command("bash", "-c", run)
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+":"+os.Getenv("PATH"),
		"GITHUB_OUTPUT="+output,
		"RUNNER_TEMP="+tmp,
		fmt.Sprintf("AI_REVIEW_DEADLINE_UNIX=%d", deadline),
		"GH_TOKEN=test",
		"REPO=owner/repo",
		"PR=1",
		"BASE_SHA="+base,
		"HEAD_SHA="+head,
		"CANDIDATE=true",
		"PR_AUTHOR_LOGIN=dependabot[bot]",
		"PR_AUTHOR_ID=49699333",
		"PR_AUTHOR_TYPE=Bot",
		"HEAD_REPO_ID=123",
		"BASE_REPO_ID=123",
		"FAKE_COMMIT_JSON="+commitJSON,
		"FAKE_EVENTS_JSON="+eventsJSON,
		"FAKE_PERMISSION_JSON="+permission,
		"FAKE_GH_FAIL="+failAPI,
	)
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("authentication step hard-failed instead of preserving ordinary review: %v\n%s", err, raw)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	got := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		if value, ok := strings.CutPrefix(line, "exempt="); ok {
			got = value == "true"
		}
	}
	return got
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
	if len(steps) == 0 || steps[0].Name != "Resolve current PR revision and mark pending" {
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
		"Resolve current PR revision and mark pending":    3,
		"Fetch PR head for diff":                          2,
		"Build authenticated SPEC governance review plan": 1,
		"Authenticate Dependabot module-only exception":   3,
		"Detect override label":                           1,
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
