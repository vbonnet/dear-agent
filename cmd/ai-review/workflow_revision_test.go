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

type workflowRevisionStep struct {
	Name string
	If   string
	Env  map[string]string
	Run  string
}

func reviewWorkflowRevisionSteps(t *testing.T) []workflowRevisionStep {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []workflowRevisionStep
		}
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow.Jobs["review"].Steps
}

func TestWorkflowResolvesCurrentPRRevisionBeforeReview(t *testing.T) {
	steps := reviewWorkflowRevisionSteps(t)
	if len(steps) == 0 || steps[0].Name != "Resolve current PR revision and mark pending" {
		t.Fatal("current PR revision is not resolved by the first trusted workflow step")
	}
	resolveRun := steps[0].Run
	for _, required := range []string{
		"\"repos/$REPO/pulls/$PR\"",
		".state == \"open\"",
		".base.ref == \"main\"",
		".base.repo.full_name == $repo",
		"\"repos/$REPO/git/ref/heads/main\"",
		".object.sha | test(\"^[0-9a-f]{40}$\")",
		"echo \"head_sha=$head_sha\"",
		"-f head_sha=\"$head_sha\"",
	} {
		if !strings.Contains(resolveRun, required) {
			t.Errorf("current revision resolver omits %s", required)
		}
	}

	const (
		currentHead = "1111111111111111111111111111111111111111"
		currentBase = "2222222222222222222222222222222222222222"
	)
	valid := currentPRRevisionJSON(currentHead, currentBase, "open", "main", "owner/repo", 123, 123, "body")
	outputs, env, err := runCurrentPRRevisionResolver(t, resolveRun, valid, "")
	if err != nil {
		t.Fatalf("resolve current PR: %v", err)
	}
	wantOutputs := map[string]string{
		"head_sha":        currentHead,
		"base_sha":        currentBase,
		"head_repo_id":    "123",
		"base_repo_id":    "123",
		"pr_author_login": "dependabot[bot]",
		"pr_author_id":    "49699333",
		"pr_author_type":  "Bot",
		"labels":          "[\"dependencies\"]",
		"is_fork":         "false",
		"check_id":        "987",
	}
	for name, want := range wantOutputs {
		if got := outputs[name]; got != want {
			t.Errorf("resolved output %s = %q, want %q", name, got, want)
		}
	}
	if !strings.Contains(env, "AI_REVIEW_DEADLINE_UNIX=") ||
		!strings.Contains(env, "AI_REVIEW_CURRENT_PR_JSON=") {
		t.Fatalf("resolver did not export its deadline and authenticated snapshot path:\n%s", env)
	}

	forkJSON := currentPRRevisionJSON(currentHead, currentBase, "open", "main", "owner/repo", 456, 123, "body")
	forkOutputs, _, err := runCurrentPRRevisionResolver(t, resolveRun, forkJSON, "")
	if err != nil {
		t.Fatalf("resolve fork PR: %v", err)
	}
	if forkOutputs["is_fork"] != "true" {
		t.Fatalf("fork resolution is_fork = %q, want true", forkOutputs["is_fork"])
	}
	stalePRBase := currentPRRevisionJSON(currentHead, "3333333333333333333333333333333333333333", "open", "main", "owner/repo", 123, 123, "body")
	staleOutputs, _, err := runCurrentPRRevisionResolver(t, resolveRun, stalePRBase, "")
	if err != nil {
		t.Fatalf("resolve PR with stale embedded base: %v", err)
	}
	if staleOutputs["base_sha"] != currentBase {
		t.Fatalf("resolved base = %q, want live protected-main %q", staleOutputs["base_sha"], currentBase)
	}

	tests := []struct {
		name     string
		response string
		failAPI  string
	}{
		{name: "pull request API failure", response: valid, failAPI: "pull"},
		{name: "protected main API failure", response: valid, failAPI: "protected"},
		{name: "malformed protected main", response: valid, failAPI: "protected-malformed"},
		{name: "check creation failure", response: valid, failAPI: "check"},
		{name: "malformed check response", response: valid, failAPI: "check-malformed"},
		{name: "closed pull request", response: currentPRRevisionJSON(currentHead, currentBase, "closed", "main", "owner/repo", 123, 123, "body")},
		{name: "wrong base branch", response: currentPRRevisionJSON(currentHead, currentBase, "open", "release", "owner/repo", 123, 123, "body")},
		{name: "wrong base repository", response: currentPRRevisionJSON(currentHead, currentBase, "open", "main", "attacker/repo", 123, 123, "body")},
		{name: "malformed head SHA", response: currentPRRevisionJSON("not-a-sha", currentBase, "open", "main", "owner/repo", 123, 123, "body")},
		{name: "NUL in body", response: strings.Replace(valid, `:"body"`, `:"body\u0000"`, 1)},
		{name: "oversized body", response: currentPRRevisionJSON(currentHead, currentBase, "open", "main", "owner/repo", 123, 123, strings.Repeat("x", 65537))},
		{name: "oversized multibyte body", response: currentPRRevisionJSON(currentHead, currentBase, "open", "main", "owner/repo", 123, 123, strings.Repeat("é", 40000))},
		{name: "oversized response", response: currentPRRevisionJSON(currentHead, currentBase, "open", "main", "owner/repo", 123, 123, strings.Repeat("x", 1<<20))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := runCurrentPRRevisionResolver(t, resolveRun, test.response, test.failAPI); err == nil {
				t.Fatal("uncertain current PR evidence unexpectedly resolved")
			}
		})
	}
}

func TestWorkflowFetchesResolvedSHAInsteadOfMutablePRRef(t *testing.T) {
	for _, step := range reviewWorkflowRevisionSteps(t) {
		if step.Name != "Fetch PR head for diff" {
			continue
		}
		wantHeadOutput := "$" + "{{ steps.pending.outputs.head_sha }}"
		if step.Env["HEAD_SHA"] != wantHeadOutput {
			t.Fatalf("fetch HEAD_SHA = %q, want current API-resolved output %q", step.Env["HEAD_SHA"], wantHeadOutput)
		}
		if !strings.Contains(step.Run, "\"+$"+"{HEAD_SHA}:refs/ai-review/head\"") {
			t.Fatal("fetch does not request the exact API-resolved Git object")
		}
		if strings.Contains(step.Run, "refs/pull/") {
			t.Fatal("fetch still races against the mutable pull-request head ref")
		}
		return
	}
	t.Fatal("Fetch PR head for diff step is missing")
}

func TestWorkflowExactHeadFetchFailsClosed(t *testing.T) {
	var fetch workflowRevisionStep
	for _, step := range reviewWorkflowRevisionSteps(t) {
		if step.Name == "Fetch PR head for diff" {
			fetch = step
			break
		}
	}
	if fetch.Run == "" {
		t.Fatal("Fetch PR head for diff step is missing")
	}
	const head = "1111111111111111111111111111111111111111"
	for _, test := range []struct {
		name     string
		failMode string
		wantErr  bool
	}{
		{name: "exact object fetched"},
		{name: "fetch failure", failMode: "fetch", wantErr: true},
		{name: "missing fetched object", failMode: "object", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			args, err := runExactHeadFetch(t, fetch.Run, head, test.failMode)
			if (err != nil) != test.wantErr {
				t.Fatalf("fetch error = %v, wantErr %v", err, test.wantErr)
			}
			if test.failMode == "" {
				wantRefspec := "+" + head + ":refs/ai-review/head"
				if !strings.Contains(args, wantRefspec) || strings.Contains(args, "refs/pull/") {
					t.Fatalf("fetch arguments = %q, want immutable refspec %q", args, wantRefspec)
				}
			}
		})
	}
}

func runExactHeadFetch(t *testing.T, run, head, failMode string) (string, error) {
	t.Helper()
	tmp := t.TempDir()
	argsFile := filepath.Join(tmp, "git-args")
	git := filepath.Join(tmp, "git")
	fakeGit := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$*\" >> \"$FAKE_GIT_ARGS\"",
		"case \"$*\" in",
		"  *\" fetch \"*|fetch\\ *) [ \"$FAKE_GIT_FAIL\" = fetch ] && exit 1; exit 0 ;;",
		"  *\"cat-file -e\"*) [ \"$FAKE_GIT_FAIL\" = object ] && exit 1; exit 0 ;;",
		"  *) exit 64 ;;",
		"esac",
		"",
	}, "\n")
	if err := os.WriteFile(git, []byte(fakeGit), 0o700); err != nil {
		t.Fatal(err)
	}
	timeout := filepath.Join(tmp, "timeout")
	fakeTimeout := strings.Join([]string{
		"#!/bin/sh",
		"while [ \"$#\" -gt 0 ]; do",
		"  case \"$1\" in",
		"    --signal=*|--kill-after=*) shift ;;",
		"    *s) shift; break ;;",
		"    *) exit 64 ;;",
		"  esac",
		"done",
		"exec \"$@\"",
		"",
	}, "\n")
	if err := os.WriteFile(timeout, []byte(fakeTimeout), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "bash", "-c", run)
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+":"+os.Getenv("PATH"),
		"AI_REVIEW_DEADLINE_UNIX="+fmt.Sprint(time.Now().Add(time.Minute).Unix()),
		"GH_TOKEN=test",
		"HEAD_SHA="+head,
		"REPO=owner/repo",
		"FAKE_GIT_ARGS="+argsFile,
		"FAKE_GIT_FAIL="+failMode,
	)
	raw, runErr := cmd.CombinedOutput()
	argsRaw, err := os.ReadFile(argsFile)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if runErr != nil {
		return string(argsRaw), fmt.Errorf("%w: %s", runErr, raw)
	}
	return string(argsRaw), nil
}

func TestWorkflowDoesNotReuseMutableEventRevisionEvidence(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	eventExpression := "$" + "{{ github.event.pull_request."
	for _, mutable := range []string{"base.sha }}", "body }}", "user.login }}", "head.repo.id }}"} {
		if strings.Contains(string(raw), eventExpression+mutable) {
			t.Errorf("workflow still binds review evidence to mutable event field %s", mutable)
		}
	}
	eventHead := "github.event.pull_request.head.sha"
	if strings.Count(string(raw), eventHead) != 1 {
		t.Fatalf("event head has %d uses, want only the label-attestation mutation guard", strings.Count(string(raw), eventHead))
	}
	for _, step := range reviewWorkflowRevisionSteps(t) {
		if step.Name == "Attest override to the reviewed revision" {
			if !strings.Contains(step.If, "steps.pending.outputs.head_sha == "+eventHead) {
				t.Errorf("%s does not bind its event mutation to the resolved head", step.Name)
			}
		}
	}
}

func TestWorkflowSynchronizeOrderingUsesRevisionAttestation(t *testing.T) {
	var detect workflowRevisionStep
	for _, step := range reviewWorkflowRevisionSteps(t) {
		if step.Name == "Detect override label" {
			detect = step
			break
		}
	}
	if detect.Run == "" {
		t.Fatal("Detect override label step is missing")
	}
	const (
		currentHead = "1111111111111111111111111111111111111111"
		oldHead     = "2222222222222222222222222222222222222222"
	)
	tests := []struct {
		name         string
		eventHead    string
		attestedHead string
		want         bool
	}{
		{name: "same-head delayed synchronize preserves newer valid override", eventHead: currentHead, attestedHead: currentHead, want: true},
		{name: "stale synchronize preserves newer valid override", eventHead: oldHead, attestedHead: currentHead, want: true},
		{name: "stale synchronize cannot revive old override", eventHead: oldHead, attestedHead: oldHead},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runOverrideDetection(t, detect.Run, currentHead, test.eventHead, test.attestedHead)
			if got != test.want {
				t.Fatalf("active = %v, want %v", got, test.want)
			}
		})
	}
}

func runOverrideDetection(t *testing.T, run, head, eventHead, attestedHead string) bool {
	t.Helper()
	tmp := t.TempDir()
	output := filepath.Join(tmp, "github-output")
	gh := filepath.Join(tmp, "gh")
	fakeGH := strings.Join([]string{
		"#!/bin/sh",
		"case \"$*\" in",
		"  *\"/comments?\"*) printf '<!-- ai-review-override-sha:%s -->\\n' \"$FAKE_ATTESTED_HEAD\" ;;",
		"  *\"/events?\"*) printf 'maintainer\\n' ;;",
		"  *\"/collaborators/\"*\"/permission\"*) printf 'admin\\n' ;;",
		"  *) exit 64 ;;",
		"esac",
		"",
	}, "\n")
	if err := os.WriteFile(gh, []byte(fakeGH), 0o700); err != nil {
		t.Fatal(err)
	}
	timeout := filepath.Join(tmp, "timeout")
	fakeTimeout := strings.Join([]string{
		"#!/bin/sh",
		"while [ \"$#\" -gt 0 ]; do",
		"  case \"$1\" in",
		"    --signal=*|--kill-after=*) shift ;;",
		"    *s) shift; break ;;",
		"    *) exit 64 ;;",
		"  esac",
		"done",
		"exec \"$@\"",
		"",
	}, "\n")
	if err := os.WriteFile(timeout, []byte(fakeTimeout), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "bash", "-c", run)
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+":"+os.Getenv("PATH"),
		"GITHUB_OUTPUT="+output,
		"AI_REVIEW_DEADLINE_UNIX="+fmt.Sprint(time.Now().Add(time.Minute).Unix()),
		"LABELS=[\"ai-review:override\"]",
		"GH_TOKEN=test",
		"REPO=owner/repo",
		"ACTION=synchronize",
		"EVENT_HEAD_SHA="+eventHead,
		"PR=1",
		"HEAD_SHA="+head,
		"FAKE_ATTESTED_HEAD="+attestedHead,
	)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("override detection hard-failed: %v\n%s", err, raw)
	}
	outputRaw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	active := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(outputRaw)), "\n") {
		if value, ok := strings.CutPrefix(line, "active="); ok {
			active = value == "true"
		}
	}
	return active
}

func TestWorkflowPublicationRevalidatesProtectedMain(t *testing.T) {
	var publish workflowRevisionStep
	for _, step := range reviewWorkflowRevisionSteps(t) {
		if step.Name == "Publish check on reviewed head" {
			publish = step
			break
		}
	}
	if publish.Run == "" {
		t.Fatal("Publish check on reviewed head step is missing")
	}
	if publish.Env["BASE_SHA"] != "$"+"{{ steps.pending.outputs.base_sha }}" {
		t.Fatalf("publish BASE_SHA = %q, want resolved protected-main output", publish.Env["BASE_SHA"])
	}
	const (
		base     = "1111111111111111111111111111111111111111"
		advanced = "2222222222222222222222222222222222222222"
	)
	tests := []struct {
		name        string
		currentBase string
		failAPI     bool
		outcome     string
		dependabot  bool
		want        string
	}{
		{name: "approved current-base review", currentBase: base, outcome: "success", want: "success"},
		{name: "neutral current-base Dependabot review", currentBase: base, outcome: "skipped", dependabot: true, want: "neutral"},
		{name: "protected main advanced during review", currentBase: advanced, outcome: "success", want: "failure"},
		{name: "freshness API failure", currentBase: base, failAPI: true, outcome: "success", want: "failure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runReviewPublication(t, publish.Run, base, test.currentBase, test.outcome, test.dependabot, test.failAPI)
			if got != test.want {
				t.Fatalf("conclusion = %q, want %q", got, test.want)
			}
		})
	}
}

func runReviewPublication(t *testing.T, run, base, currentBase, outcome string, dependabot, failAPI bool) string {
	t.Helper()
	tmp := t.TempDir()
	output := filepath.Join(tmp, "github-output")
	gh := filepath.Join(tmp, "gh")
	fakeGH := strings.Join([]string{
		"#!/bin/sh",
		"case \"$*\" in",
		"  *\"/git/ref/heads/main\"*)",
		"    [ \"$FAKE_GH_FAIL\" = true ] && exit 1",
		"    printf '%s\\n' \"$FAKE_CURRENT_BASE\"",
		"    ;;",
		"  *\"/check-runs/987\"*) exit 0 ;;",
		"  *) exit 64 ;;",
		"esac",
		"",
	}, "\n")
	if err := os.WriteFile(gh, []byte(fakeGH), 0o700); err != nil {
		t.Fatal(err)
	}
	timeout := filepath.Join(tmp, "timeout")
	fakeTimeout := strings.Join([]string{
		"#!/bin/sh",
		"while [ \"$#\" -gt 0 ]; do",
		"  case \"$1\" in",
		"    --signal=*|--kill-after=*) shift ;;",
		"    *s) shift; break ;;",
		"    *) exit 64 ;;",
		"  esac",
		"done",
		"exec \"$@\"",
		"",
	}, "\n")
	if err := os.WriteFile(timeout, []byte(fakeTimeout), 0o700); err != nil {
		t.Fatal(err)
	}
	checkExpression := "$" + "{{ steps.pending.outputs.check_id }}"
	run = strings.Replace(run, checkExpression, "987", 1)
	dependabotOutcome := "skipped"
	dependabotExempt := "false"
	if dependabot {
		dependabotOutcome = "success"
		dependabotExempt = "true"
	}
	cmd := exec.CommandContext(t.Context(), "bash", "-c", run)
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+":"+os.Getenv("PATH"),
		"GITHUB_OUTPUT="+output,
		"GH_TOKEN=test",
		"BASE_SHA="+base,
		"HEAD_SHA=3333333333333333333333333333333333333333",
		"REPO=owner/repo",
		"OUTCOME="+outcome,
		"KEY_OUTCOME=success",
		"KEY_PRESENT=false",
		"OVERRIDE=false",
		"PLAN_OUTCOME=success",
		"REVIEW_RELEVANT=false",
		"DEPENDABOT_OUTCOME="+dependabotOutcome,
		"DEPENDABOT_EXEMPT="+dependabotExempt,
		"FAKE_CURRENT_BASE="+currentBase,
		"FAKE_GH_FAIL="+fmt.Sprint(failAPI),
	)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("publication step hard-failed: %v\n%s", err, raw)
	}
	outputRaw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(outputRaw)), "\n") {
		if value, ok := strings.CutPrefix(line, "conclusion="); ok {
			return value
		}
	}
	t.Fatalf("publication produced no conclusion:\n%s", outputRaw)
	return ""
}

func currentPRRevisionJSON(head, base, state, baseRef, baseRepo string, headRepoID, baseRepoID int, body string) string {
	return fmt.Sprintf("{"+
		"\"number\":1,"+
		"\"state\":%q,"+
		"\"body\":%q,"+
		"\"user\":{\"login\":\"dependabot[bot]\",\"id\":49699333,\"type\":\"Bot\"},"+
		"\"labels\":[{\"name\":\"dependencies\"}],"+
		"\"head\":{\"sha\":%q,\"repo\":{\"id\":%d,\"full_name\":\"owner/head\"}},"+
		"\"base\":{\"sha\":%q,\"ref\":%q,\"repo\":{\"id\":%d,\"full_name\":%q}}"+
		"}", state, body, head, headRepoID, base, baseRef, baseRepoID, baseRepo)
}

func runCurrentPRRevisionResolver(t *testing.T, run, response, failAPI string) (map[string]string, string, error) {
	t.Helper()
	tmp := t.TempDir()
	output := filepath.Join(tmp, "github-output")
	envFile := filepath.Join(tmp, "github-env")
	responseFile := filepath.Join(tmp, "pr.json")
	if err := os.WriteFile(responseFile, []byte(response), 0o600); err != nil {
		t.Fatal(err)
	}
	gh := filepath.Join(tmp, "gh")
	fakeGH := strings.Join([]string{
		"#!/bin/sh",
		"case \"$*\" in",
		"  *\"/pulls/\"*)",
		"    [ \"$FAKE_GH_FAIL\" = pull ] && exit 1",
		"    cat \"$FAKE_PR_FILE\"",
		"    ;;",
		"  *\"/git/ref/heads/main\"*)",
		"    [ \"$FAKE_GH_FAIL\" = protected ] && exit 1",
		"    [ \"$FAKE_GH_FAIL\" = protected-malformed ] && { printf '{}\\n'; exit 0; }",
		"    printf '{\"ref\":\"refs/heads/main\",\"object\":{\"type\":\"commit\",\"sha\":\"%s\"}}\\n' \"$FAKE_PROTECTED_BASE\"",
		"    ;;",
		"  *\"check-runs\"*)",
		"    [ \"$FAKE_GH_FAIL\" = check ] && exit 1",
		"    [ \"$FAKE_GH_FAIL\" = check-malformed ] && { printf 'not-an-id\\n'; exit 0; }",
		"    printf '987\\n'",
		"    ;;",
		"  *) exit 64 ;;",
		"esac",
		"",
	}, "\n")
	if err := os.WriteFile(gh, []byte(fakeGH), 0o700); err != nil {
		t.Fatal(err)
	}
	timeout := filepath.Join(tmp, "timeout")
	fakeTimeout := strings.Join([]string{
		"#!/bin/sh",
		"while [ \"$#\" -gt 0 ]; do",
		"  case \"$1\" in",
		"    --signal=*|--kill-after=*) shift ;;",
		"    *s) shift; break ;;",
		"    *) exit 64 ;;",
		"  esac",
		"done",
		"exec \"$@\"",
		"",
	}, "\n")
	if err := os.WriteFile(timeout, []byte(fakeTimeout), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "bash", "-c", run)
	cmd.Env = append(os.Environ(),
		"PATH="+tmp+":"+os.Getenv("PATH"),
		"GITHUB_OUTPUT="+output,
		"GITHUB_ENV="+envFile,
		"RUNNER_TEMP="+tmp,
		"GH_TOKEN=test",
		"PR=1",
		"REPO=owner/repo",
		"REVIEW_DEADLINE_SECONDS=4020",
		"FAKE_PR_FILE="+responseFile,
		"FAKE_PROTECTED_BASE=2222222222222222222222222222222222222222",
		"FAKE_GH_FAIL="+failAPI,
	)
	raw, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return nil, string(raw), fmt.Errorf("%w: %s", runErr, raw)
	}
	outputRaw, err := os.ReadFile(output)
	if err != nil {
		return nil, "", err
	}
	envRaw, err := os.ReadFile(envFile)
	if err != nil {
		return nil, "", err
	}
	outputs := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSpace(string(outputRaw)), "\n") {
		if name, value, ok := strings.Cut(line, "="); ok {
			outputs[name] = value
		}
	}
	return outputs, string(envRaw), nil
}
