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
		t.Fatalf("event head has %d uses, want only the exact-head override-event guard", strings.Count(string(raw), eventHead))
	}
	for _, step := range reviewWorkflowRevisionSteps(t) {
		if step.Name != "Detect override label" {
			continue
		}
		wantEnv := map[string]string{
			"EVENT_NAME":        "$" + "{{ github.event_name }}",
			"EVENT_ACTION":      "$" + "{{ github.event.action }}",
			"EVENT_LABEL":       "$" + "{{ github.event.label.name }}",
			"EVENT_HEAD_SHA":    "$" + "{{ github.event.pull_request.head.sha }}",
			"EVENT_ACTOR_LOGIN": "$" + "{{ github.event.sender.login }}",
			"EVENT_ACTOR_ID":    "$" + "{{ github.event.sender.id }}",
			"EVENT_ACTOR_TYPE":  "$" + "{{ github.event.sender.type }}",
		}
		for name, want := range wantEnv {
			if got := step.Env[name]; got != want {
				t.Errorf("override %s = %q, want trusted event value %q", name, got, want)
			}
		}
		return
	}
	t.Fatal("Detect override label step is missing")
}

func TestWorkflowOverrideActivationIsExactHeadLabeledEvent(t *testing.T) {
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
	if strings.Contains(detect.Run, "/comments?") || strings.Contains(detect.Run, "ai-review-override-sha") {
		t.Fatal("override detection still treats a forgeable bot-authored marker as authority")
	}
	const (
		currentHead = "1111111111111111111111111111111111111111"
		oldHead     = "2222222222222222222222222222222222222222"
	)
	tests := []struct {
		name           string
		eventName      string
		action         string
		eventLabel     string
		eventHead      string
		actorLogin     string
		actorID        string
		actorType      string
		permissionJSON string
		labels         string
		markerHead     string
		failAPI        bool
		want           bool
	}{
		{
			name:       "verified maintainer exact-head labeled event",
			eventName:  "pull_request_target",
			action:     "labeled",
			eventLabel: "ai-review:override",
			eventHead:  currentHead,
			actorLogin: "maintainer",
			actorID:    "101",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"maintainer", 101, "User", "write", "maintain",
			),
			labels: `["ai-review:override"]`,
			want:   true,
		},
		{
			name:       "stale labeled event head mismatch",
			eventName:  "pull_request_target",
			action:     "labeled",
			eventLabel: "ai-review:override",
			eventHead:  oldHead,
			actorLogin: "maintainer",
			actorID:    "101",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"maintainer", 101, "User", "write", "maintain",
			),
			labels: `["ai-review:override"]`,
		},
		{
			name:       "verified administrator exact-head labeled event",
			eventName:  "pull_request_target",
			action:     "labeled",
			eventLabel: "ai-review:override",
			eventHead:  currentHead,
			actorLogin: "administrator",
			actorID:    "102",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"administrator", 102, "User", "admin", "admin",
			),
			labels: `["ai-review:override"]`,
			want:   true,
		},
		{
			name:       "synchronize with retained label and forged marker",
			eventName:  "pull_request_target",
			action:     "synchronize",
			eventHead:  currentHead,
			actorLogin: "writer",
			actorID:    "103",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"writer", 103, "User", "write", "write",
			),
			labels:     `["ai-review:override"]`,
			markerHead: currentHead,
		},
		{
			name:       "other event with forged marker",
			eventName:  "pull_request_target",
			action:     "edited",
			eventHead:  currentHead,
			actorLogin: "maintainer",
			actorID:    "101",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"maintainer", 101, "User", "write", "maintain",
			),
			labels:     `["ai-review:override"]`,
			markerHead: currentHead,
		},
		{
			name:       "exact-head labeled event from ordinary writer",
			eventName:  "pull_request_target",
			action:     "labeled",
			eventLabel: "ai-review:override",
			eventHead:  currentHead,
			actorLogin: "writer",
			actorID:    "103",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"writer", 103, "User", "write", "write",
			),
			labels: `["ai-review:override"]`,
		},
		{
			name:       "actor permission API failure",
			eventName:  "pull_request_target",
			action:     "labeled",
			eventLabel: "ai-review:override",
			eventHead:  currentHead,
			actorLogin: "maintainer",
			actorID:    "101",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"maintainer", 101, "User", "write", "maintain",
			),
			labels:  `["ai-review:override"]`,
			failAPI: true,
		},
		{
			name:       "bot actor cannot authorize override",
			eventName:  "pull_request_target",
			action:     "labeled",
			eventLabel: "ai-review:override",
			eventHead:  currentHead,
			actorLogin: "automation-bot",
			actorID:    "104",
			actorType:  "Bot",
			permissionJSON: collaboratorPermissionJSON(
				"automation-bot", 104, "Bot", "admin", "admin",
			),
			labels: `["ai-review:override"]`,
		},
		{
			name:       "permission response identity mismatch",
			eventName:  "pull_request_target",
			action:     "labeled",
			eventLabel: "ai-review:override",
			eventHead:  currentHead,
			actorLogin: "maintainer",
			actorID:    "101",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"different-user", 999, "User", "admin", "admin",
			),
			labels: `["ai-review:override"]`,
		},
		{
			name:           "malformed permission response",
			eventName:      "pull_request_target",
			action:         "labeled",
			eventLabel:     "ai-review:override",
			eventHead:      currentHead,
			actorLogin:     "maintainer",
			actorID:        "101",
			actorType:      "User",
			permissionJSON: `{`,
			labels:         `["ai-review:override"]`,
		},
		{
			name:           "oversized permission response",
			eventName:      "pull_request_target",
			action:         "labeled",
			eventLabel:     "ai-review:override",
			eventHead:      currentHead,
			actorLogin:     "maintainer",
			actorID:        "101",
			actorType:      "User",
			permissionJSON: strings.Repeat("x", 65537),
			labels:         `["ai-review:override"]`,
		},
		{
			name:       "wrong label event",
			eventName:  "pull_request_target",
			action:     "labeled",
			eventLabel: "dependencies",
			eventHead:  currentHead,
			actorLogin: "maintainer",
			actorID:    "101",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"maintainer", 101, "User", "write", "maintain",
			),
			labels: `["ai-review:override"]`,
		},
		{
			name:       "override absent from current API snapshot",
			eventName:  "pull_request_target",
			action:     "labeled",
			eventLabel: "ai-review:override",
			eventHead:  currentHead,
			actorLogin: "maintainer",
			actorID:    "101",
			actorType:  "User",
			permissionJSON: collaboratorPermissionJSON(
				"maintainer", 101, "User", "write", "maintain",
			),
			labels: `[]`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runOverrideDetection(t, detect.Run, currentHead, overrideDetectionInput{
				EventName:      test.eventName,
				Action:         test.action,
				EventLabel:     test.eventLabel,
				EventHead:      test.eventHead,
				ActorLogin:     test.actorLogin,
				ActorID:        test.actorID,
				ActorType:      test.actorType,
				PermissionJSON: test.permissionJSON,
				Labels:         test.labels,
				MarkerHead:     test.markerHead,
				FailAPI:        test.failAPI,
			})
			if got != test.want {
				t.Fatalf("active = %v, want %v", got, test.want)
			}
		})
	}
}

type overrideDetectionInput struct {
	EventName      string
	Action         string
	EventLabel     string
	EventHead      string
	ActorLogin     string
	ActorID        string
	ActorType      string
	PermissionJSON string
	Labels         string
	MarkerHead     string
	FailAPI        bool
}

func runOverrideDetection(t *testing.T, run, head string, input overrideDetectionInput) bool {
	t.Helper()
	tmp := t.TempDir()
	output := filepath.Join(tmp, "github-output")
	permissionFile := filepath.Join(tmp, "permission.json")
	if err := os.WriteFile(permissionFile, []byte(input.PermissionJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	gh := filepath.Join(tmp, "gh")
	fakeGH := strings.Join([]string{
		"#!/bin/sh",
		"case \"$*\" in",
		"  *\"/comments?\"*) printf '<!-- ai-review-override-sha:%s -->\\n' \"$FAKE_MARKER_HEAD\" ;;",
		"  *\"/collaborators/\"*\"/permission\"*) [ \"$FAKE_GH_FAIL\" = true ] && exit 1; cat \"$FAKE_PERMISSION_FILE\" ;;",
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
		"RUNNER_TEMP="+tmp,
		"AI_REVIEW_DEADLINE_UNIX="+fmt.Sprint(time.Now().Add(time.Minute).Unix()),
		"LABELS="+input.Labels,
		"GH_TOKEN=test",
		"REPO=owner/repo",
		"HEAD_SHA="+head,
		"EVENT_NAME="+input.EventName,
		"EVENT_ACTION="+input.Action,
		"EVENT_LABEL="+input.EventLabel,
		"EVENT_HEAD_SHA="+input.EventHead,
		"EVENT_ACTOR_LOGIN="+input.ActorLogin,
		"EVENT_ACTOR_ID="+input.ActorID,
		"EVENT_ACTOR_TYPE="+input.ActorType,
		"FAKE_PERMISSION_FILE="+permissionFile,
		"FAKE_MARKER_HEAD="+input.MarkerHead,
		"FAKE_GH_FAIL="+fmt.Sprint(input.FailAPI),
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

func collaboratorPermissionJSON(login string, id int, actorType, permission, roleName string) string {
	return fmt.Sprintf(
		`{"permission":%q,"role_name":%q,"user":{"login":%q,"id":%d,"type":%q}}`,
		permission, roleName, login, id, actorType,
	)
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
		isFork      bool
		keyPresent  bool
		keyless     bool // gate step's keyless_cannot_run output (exit 78)
		want        string
	}{
		{name: "approved current-base review", currentBase: base, outcome: "success", want: "success"},
		{name: "neutral current-base Dependabot review", currentBase: base, outcome: "skipped", dependabot: true, want: "neutral"},
		{name: "protected main advanced during review", currentBase: advanced, outcome: "success", want: "failure"},
		{name: "freshness API failure", currentBase: base, failAPI: true, outcome: "success", want: "failure"},
		// Only the gate's explicit cannot-run-without-credential exit (78,
		// surfaced as keyless_cannot_run=true) publishes neutral-with-warning,
		// and only same-repository with the key affirmatively absent. That
		// exit is the AIREV-26 predicate: the review could never run. A
		// keyless failure WITHOUT that discrimination stays a failure, as do
		// fork PRs, stale-base publication, and any failure while a key is
		// present.
		{name: "keyless same-repo cannot-run failure is neutral with warning", currentBase: base, outcome: "failure", keyless: true, want: "neutral"},
		{name: "keyless failure without cannot-run discrimination stays failure", currentBase: base, outcome: "failure", want: "failure"},
		{name: "keyless fork cannot-run failure stays failure", currentBase: base, outcome: "failure", keyless: true, isFork: true, want: "failure"},
		{name: "keyless cannot-run failure with stale base stays failure", currentBase: advanced, outcome: "failure", keyless: true, want: "failure"},
		{name: "gate failure with key present stays failure", currentBase: base, outcome: "failure", keyPresent: true, want: "failure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runReviewPublication(t, publish.Run, base, test.currentBase, test.outcome, test.dependabot, test.isFork, test.keyPresent, test.keyless, test.failAPI)
			if got != test.want {
				t.Fatalf("conclusion = %q, want %q", got, test.want)
			}
		})
	}
}

func runReviewPublication(t *testing.T, run, base, currentBase, outcome string, dependabot, isFork, keyPresent, keyless, failAPI bool) string {
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
		"KEY_PRESENT="+fmt.Sprint(keyPresent),
		"OVERRIDE=false",
		"PLAN_OUTCOME=success",
		"REVIEW_RELEVANT=false",
		"DEPENDABOT_OUTCOME="+dependabotOutcome,
		"DEPENDABOT_EXEMPT="+dependabotExempt,
		"IS_FORK="+fmt.Sprint(isFork),
		"KEYLESS_CANNOT_RUN="+fmt.Sprint(keyless),
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

// TestWorkflowKeylessNeutralIsExitSeventyEightOnly pins the AIREV-26 boundary
// that separates "the review could never run" from "the review concluded and
// found a problem". Only cmd/ai-review's explicit cannot-run exit 78 —
// surfaced by the gate step as keyless_cannot_run=true — may be published as
// neutral-with-warning. Every disposition AIREV-26 enumerates as fail-closed
// reaches the workflow with keyless_cannot_run=false (or never writes the
// output at all, when the failure precedes the binary), and each must keep
// blocking even though no reviewer credential is configured.
//
// Widening the publish branch to "any keyless gate failure" turns each of
// these genuine findings into a false pass, so they are asserted by name.
func TestWorkflowKeylessNeutralIsExitSeventyEightOnly(t *testing.T) {
	steps := reviewWorkflowRevisionSteps(t)
	var publish, failGate workflowRevisionStep
	for _, step := range steps {
		switch step.Name {
		case "Publish check on reviewed head":
			publish = step
		case "Fail if review gate failed":
			failGate = step
		}
	}
	if publish.Run == "" {
		t.Fatal("Publish check on reviewed head step is missing")
	}
	if failGate.If == "" {
		t.Fatal("Fail if review gate failed step is missing")
	}
	// The native orchestration job must go green for exit 78 and ONLY for
	// exit 78; without this clause every keyless gate failure stops blocking.
	if !strings.Contains(failGate.If, "steps.gate.outputs.keyless_cannot_run != 'true'") {
		t.Errorf("orchestration failure condition = %q, want it gated on keyless_cannot_run", failGate.If)
	}
	// The published semantic verdict must apply the same discrimination.
	if !strings.Contains(publish.Run, `[ "${KEYLESS_CANNOT_RUN:-}" = true ]`) {
		t.Error("publish step no longer discriminates the cannot-run exit; every keyless gate failure would publish neutral")
	}

	const base = "1111111111111111111111111111111111111111"
	// Each case is a keyless, same-repository, current-base run whose gate
	// failed. keyless=false is how the workflow observes every AIREV-26
	// fail-closed disposition, because cmd/ai-review reserves exit 78 for the
	// cannot-run predicate alone.
	failClosed := []string{
		"conclusive SPEC-governance verdict needing no model (ownership edge)",
		"conclusive SPEC-governance verdict needing no model (reviewer dependency)",
		"conclusive SPEC-governance verdict needing no model (BDD traceability)",
		"conclusive SPEC-governance verdict needing no model (stale-base evidence)",
		"oversize or uncomputable diff",
		"needs-human-review evidence comment could not be posted",
		"authenticated plan construction error",
		"expired trusted review deadline",
		"reviewer build failure before the gate binary ran",
	}
	for _, name := range failClosed {
		t.Run(name, func(t *testing.T) {
			got := runReviewPublication(t, publish.Run, base, base, "failure", false, false, false, false, false)
			if got != "failure" {
				t.Fatalf("conclusion = %q, want %q: a keyless review that CAN conclude must not be excused", got, "failure")
			}
		})
	}
}
