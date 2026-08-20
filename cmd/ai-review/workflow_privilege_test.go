package main

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/mergeloop"
)

const ordinaryReadOnlyWorkflow = `name: CI
on:
  pull_request:
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
`

const criticalWorkflow = `name: privileged
on:
  issue_comment:
    types: [created]
permissions:
  contents: read
  id-token: write
jobs:
  agent:
    if: contains(fromJSON('["OWNER","MEMBER"]'), github.event.comment.author_association)
    runs-on: ubuntu-latest
    steps:
      - uses: vendor/agent@v1
        with:
          token: ${{ secrets.CUSTOM_AGENT_TOKEN }}
`

func TestWorkflowPrivilegeReason_CurrentClaudeWorkflow(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "claude.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if reason, privileged := workflowPrivilegeReason(raw); !privileged {
		t.Fatalf("current claude.yml privilege = false, reason=%q", reason)
	}
}

func TestWorkflowPrivilegeReason_LowBlastWritesStayAutomated(t *testing.T) {
	workflow := `name: discussion audit
on: push
permissions:
  contents: read
  discussions: write
jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: gh api /repos/vbonnet/dear-agent/discussions
`
	if reason, privileged := workflowPrivilegeReason([]byte(workflow)); privileged {
		t.Fatalf("discussion-write-only workflow escalated: %s", reason)
	}
}

// TestMergeGateLabelScopesAreNeverLowBlast is the there-is-no-bypass proof for
// the autonomous merge gate.
//
// [mergeloop.Policy.policyBlock] treats [mergeloop.DefaultBlockLabels] as the
// human-only block on autonomous merge, and labels are mutable repository
// state. Any workflow token scope that can mutate a pull request's labels can
// therefore erase that block before the merge loop ever classifies the PR. The
// only durable defence is that a change granting such a scope can never ride
// the automated review path itself, so this test pins every label-mutating
// scope to the critical set and proves the classifier agrees end to end.
func TestMergeGateLabelScopesAreNeverLowBlast(t *testing.T) {
	if len(mergeloop.DefaultBlockLabels) == 0 {
		t.Fatal("merge loop declares no block labels; the gate this test protects is gone")
	}
	// GitHub models pull requests as issues, so the issues API labels PRs and
	// the pull-requests scope labels them directly. Both are review control.
	labelMutatingScopes := []string{"issues", "pull-requests"}
	for _, scope := range labelMutatingScopes {
		if lowBlastWorkflowWriteScopes[scope] {
			t.Errorf("%s: write can add or remove the merge-gate labels %v but is classified low-blast",
				scope, mergeloop.DefaultBlockLabels)
		}
		if !criticalWorkflowWriteScopes[scope] {
			t.Errorf("%s: write can add or remove the merge-gate labels %v but is not classified critical",
				scope, mergeloop.DefaultBlockLabels)
		}
		workflow := `name: gate bypass
on: push
permissions:
  contents: read
  ` + scope + `: write
jobs:
  strip:
    runs-on: ubuntu-latest
    steps:
      - env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: gh pr edit 1 --remove-label ` + mergeloop.DefaultBlockLabels[0] + `
`
		reason, privileged := workflowPrivilegeReason([]byte(workflow))
		if !privileged {
			t.Errorf("workflow granting %s: write stayed on the automated path (reason=%q)", scope, reason)
		}
	}
	// The remaining low-blast scopes must be provably unable to reach labels.
	// Adding one here without that proof is the bypass, so the set is pinned.
	wantLowBlast := map[string]bool{"discussions": true}
	if !maps.Equal(lowBlastWorkflowWriteScopes, wantLowBlast) {
		t.Errorf("low-blast write scopes = %v, want %v; a new entry needs a documented proof that it cannot mutate pull-request labels, checks, statuses, reviews, or contents",
			lowBlastWorkflowWriteScopes, wantLowBlast)
	}
}

func TestClassifyWorkflowSecretReferences_ContextBoundaries(t *testing.T) {
	tests := []struct {
		name              string
		value             string
		wantCustom        bool
		wantDefaultSecret bool
		wantGitHubToken   bool
	}{
		{"root custom secret", "${{ secrets.DEPLOY_KEY }}", true, false, false},
		{"bare root secrets object", "${{ toJSON(secrets) }}", true, false, false},
		{"root secrets used as index", "${{ foo[secrets] }}", true, false, false},
		{"inputs member named secrets", "${{ inputs.secrets }}", false, false, false},
		{"spaced member named secrets", "${{ inputs . secrets }}", false, false, false},
		{"matrix member named secrets", "${{ matrix.secrets }}", false, false, false},
		{"nested member named secrets", "${{ github.event.inputs.secrets }}", false, false, false},
		{"default secrets dot token", "${{ secrets.GITHUB_TOKEN }}", false, true, false},
		{"default secrets bracket token", "${{ secrets['GITHUB_TOKEN'] }}", false, true, false},
		{"default GitHub dot token", "${{ github.token }}", false, false, true},
		{"default GitHub bracket token", "${{ github['token'] }}", false, false, true},
		{"nested GitHub member", "${{ inputs.github.token }}", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyWorkflowSecretReferences(tt.value)
			if got.custom != tt.wantCustom || got.defaultToken != tt.wantDefaultSecret || got.githubToken != tt.wantGitHubToken {
				t.Fatalf("secret references = %#v, want custom=%t default-secret=%t github-token=%t", got, tt.wantCustom, tt.wantDefaultSecret, tt.wantGitHubToken)
			}
		})
	}
}

func TestScalarIsExactDefaultTokenReference(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"secrets dot", "${{ secrets.GITHUB_TOKEN }}", true},
		{"secrets bracket", "${{ secrets['GITHUB_TOKEN'] }}", true},
		{"github dot", "${{ github.token }}", true},
		{"github bracket", "${{ github['token'] }}", true},
		{"dynamic output before default", "${{ needs.mint.outputs.token || github.token }}", false},
		{"literal containing default spelling", "github.token", false},
		{"default plus operator", "${{ github.token || secrets.GITHUB_TOKEN }}", false},
		{"nested member", "${{ inputs.github.token }}", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scalarIsExactDefaultTokenReference(tt.value); got != tt.want {
				t.Fatalf("exact default token = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestGitHubHostedRunnerLabels_MatchesPinnedStandardSet(t *testing.T) {
	want := []string{
		"macos-14", "macos-15", "macos-15-intel", "macos-26", "macos-26-intel", "macos-latest",
		"ubuntu-22.04", "ubuntu-22.04-arm", "ubuntu-24.04", "ubuntu-24.04-arm", "ubuntu-26.04", "ubuntu-26.04-arm", "ubuntu-latest", "ubuntu-slim",
		"windows-11-arm", "windows-11-vs2026-arm", "windows-2022", "windows-2025", "windows-2025-vs2026", "windows-latest",
	}
	got := make([]string, 0, len(githubHostedRunnerLabels))
	for label := range githubHostedRunnerLabels {
		got = append(got, label)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("pinned hosted runner labels = %v, want %v", got, want)
	}
}

func TestWorkflowPrivilegeReason_CurrentTreeGolden(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	var privileged []string
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, critical := workflowPrivilegeReason(raw); critical {
			privileged = append(privileged, filepath.Base(path))
		}
	}
	slices.Sort(privileged)
	// The audit workflows below hold issues: write, which labels pull
	// requests and so carries merge-gate authority (see
	// TestMergeGateLabelScopesAreNeverLowBlast).
	want := []string{
		"adr-integrity.yml",
		"branch-protection-audit.yml",
		"bypassed-merge-audit.yml",
		"ci-health-monitor.yml",
		"ci.yml",
		"claude-code-review.yml",
		"claude.yml",
		"codeql.yml",
		"deepsec.yml",
		"dependabot-automerge.yml",
		"dependency-freshness.yml",
		"doc-header-lint.yml",
		"gemini-review.yml",
		"go-ci-reusable.yml",
		"go-toolchain-bump.yml",
		"infra-repo-reconcile.yml",
		"main-health-watchdog.yml",
		"merge-audit.yml",
		"monthly-audit.yml",
		"pr-review-agent.yml",
		"pr-size-audit.yml",
		"pr-size-scope.yml",
		"release.yml",
		"review.yml",
		"routing-enforcement.yml",
		"sbom-scan.yml",
		"security-audit.yml",
		"stale-branch-audit.yml",
		"tofu-drift.yml",
		"tofu-plan.yml",
	}
	if !slices.Equal(privileged, want) {
		t.Fatalf("current privileged workflow inventory = %v, want %v", privileged, want)
	}
}

func TestWorkflowPrivilegeReason_Signals(t *testing.T) {
	workflowCallSecrets := `name: reusable
on:
  workflow_call:
    secrets:
      deploy_key:
        required: true
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
`
	matrixWorkflow := `name: matrix
on: push
permissions:
  contents: read
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - run: go test ./...
`
	matrixIncludeOverrideWorkflow := strings.Replace(
		matrixWorkflow,
		"        os: [ubuntu-latest, macos-latest]",
		"        os: [ubuntu-latest, macos-latest]\n        include:\n          - os: self-hosted",
		1,
	)
	matrixUppercaseIncludeOverrideWorkflow := strings.Replace(matrixIncludeOverrideWorkflow, "- os: self-hosted", "- OS: self-hosted", 1)
	matrixExpressionIncludeOverrideWorkflow := strings.Replace(matrixIncludeOverrideWorkflow, "- os: self-hosted", `- "${{ 'os' }}": self-hosted`, 1)
	matrixDynamicIncludeOverrideWorkflow := strings.Replace(matrixIncludeOverrideWorkflow, "- os: self-hosted", `- "${{ matrix.axis }}": self-hosted`, 1)
	matrixWhitespaceIncludeOverrideWorkflow := strings.Replace(matrixIncludeOverrideWorkflow, "- os: self-hosted", `- os: " ubuntu-latest "`, 1)
	matrixUppercaseHostedAxisWorkflow := strings.Replace(matrixWorkflow, "os: [ubuntu-latest, macos-latest]", "OS: [ubuntu-latest, macos-latest]", 1)
	matrixWhitespaceAxisWorkflow := strings.Replace(matrixWorkflow, "os: [ubuntu-latest, macos-latest]", `os: [" ubuntu-latest ", macos-latest]`, 1)
	matrixCaseFoldDuplicateWorkflow := strings.Replace(matrixWorkflow, "os: [ubuntu-latest, macos-latest]", "os: [ubuntu-latest]\n        OS: [macos-latest]", 1)
	matrixUppercaseIncludeAxisWorkflow := strings.Replace(matrixWorkflow, "os: [ubuntu-latest, macos-latest]", "os: [ubuntu-latest, macos-latest]\n        INCLUDE: [debug, release]", 1)
	matrixSequenceWorkflow := strings.Replace(matrixWorkflow, "runs-on: ${{ matrix.os }}", `runs-on: ["${{ matrix.os }}"]`, 1)
	matrixMixedMultiLabelWorkflow := strings.Replace(matrixWorkflow, "runs-on: ${{ matrix.os }}", `runs-on: [ubuntu-latest, "${{ matrix.os }}"]`, 1)
	matrixOuterWhitespaceExpressionWorkflow := strings.Replace(matrixWorkflow, "runs-on: ${{ matrix.os }}", `runs-on: " ${{ matrix.os }} "`, 1)
	credentialWorkflow := strings.Replace(ordinaryReadOnlyWorkflow, "    runs-on: ubuntu-latest", `    runs-on: ubuntu-latest
    container:
      image: registry.example/test:latest
      credentials:
        username: agent
        password: token`, 1)
	expressionPermissionKeyWorkflow := strings.Replace(ordinaryReadOnlyWorkflow, "    runs-on: ubuntu-latest", `    "${{ 'permissions' }}":
      contents: write
    runs-on: ubuntu-latest`, 1)
	expressionEnvironmentKeyWorkflow := strings.Replace(ordinaryReadOnlyWorkflow, "    runs-on: ubuntu-latest", `    "${{ 'environment' }}": production
    runs-on: ubuntu-latest`, 1)
	expressionEventKeyWorkflow := strings.Replace(ordinaryReadOnlyWorkflow, "  pull_request:", `  "${{ 'pull_request_target' }}":`, 1)
	expressionCredentialsKeyWorkflow := strings.Replace(credentialWorkflow, "      credentials:", `      "${{ 'credentials' }}":`, 1)
	uppercaseWorkflowCallSecrets := strings.Replace(workflowCallSecrets, "workflow_call:", "WORKFLOW_CALL:", 1)
	ambiguousWorkflowCallSecrets := strings.Replace(workflowCallSecrets, "  workflow_call:", "  workflow_call: {}\n  WORKFLOW_CALL:", 1)
	reusableCallerWorkflow := `name: reusable caller
on: pull_request
permissions:
  contents: read
jobs:
  delegated:
    uses: ./.github/workflows/go-ci-reusable.yml
    with:
      runner: self-hosted
`
	var deepWorkflow strings.Builder
	deepWorkflow.WriteString(ordinaryReadOnlyWorkflow)
	deepWorkflow.WriteString("extra:\n")
	for i := range maxWorkflowYAMLDepth + 2 {
		deepWorkflow.WriteString(strings.Repeat("  ", i+1))
		deepWorkflow.WriteString("nested:\n")
	}
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"ordinary read only", ordinaryReadOnlyWorkflow, false},
		{"critical write", strings.Replace(ordinaryReadOnlyWorkflow, "contents: read", "contents: write", 1), true},
		{"security gate write", strings.Replace(ordinaryReadOnlyWorkflow, "contents: read", "contents: read\n  security-events: write", 1), true},
		{"unknown write", strings.Replace(ordinaryReadOnlyWorkflow, "contents: read", "future-scope: write", 1), true},
		{"custom secret", strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", "echo '${{ secrets.DEPLOY_KEY }}'", 1), true},
		{"bare secrets object", strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", "echo '${{ toJSON(secrets) }}'", 1), true},
		{"quoted braces before custom secret", strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", `echo "${{ format('{{}}{0}', secrets.DEPLOY_KEY) }}"`, 1), true},
		{"quoted closing delimiter before custom secret", strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", `echo "${{ format('}}', secrets.DEPLOY_KEY) }}"`, 1), true},
		{"quoted braces before default token", strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", `echo "${{ format('{{}}{0}', secrets.GITHUB_TOKEN) }}"`, 1), false},
		{"secrets word only in string literals", strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", `echo "${{ contains(fromJSON('["secrets"]'), 'secrets') }}"`, 1), false},
		{"secrets word in escaped quote string", strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", `echo "${{ contains('it''s secrets', 'secrets') }}"`, 1), false},
		{"unclosed secret expression", strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", "echo '${{ secrets.DEPLOY_KEY'", 1), true},
		{"secret inherit", strings.Replace(ordinaryReadOnlyWorkflow, "    runs-on: ubuntu-latest", "    uses: org/repo/.github/workflows/reuse.yml@main\n    secrets: inherit", 1), true},
		{"workflow call declares secrets", workflowCallSecrets, true},
		{"uppercase workflow call declares secrets", uppercaseWorkflowCallSecrets, true},
		{"case-fold duplicate workflow call is ambiguous", ambiguousWorkflowCallSecrets, true},
		{"privileged event", strings.Replace(ordinaryReadOnlyWorkflow, "pull_request:", "pull_request_target:", 1), true},
		{"privileged event scalar with outer whitespace", strings.Replace(ordinaryReadOnlyWorkflow, "on:\n  pull_request:", `on: " workflow_run "`, 1), true},
		{"privileged event mapping key with outer whitespace", strings.Replace(ordinaryReadOnlyWorkflow, "pull_request:", `" workflow_run ":`, 1), true},
		{"constant-folded privileged event scalar", strings.Replace(ordinaryReadOnlyWorkflow, "on:\n  pull_request:", `on: "${{ 'pull_request_target' }}"`, 1), true},
		{"constant-folded privileged event sequence", strings.Replace(ordinaryReadOnlyWorkflow, "on:\n  pull_request:", `on: [push, "${{ 'workflow_run' }}"]`, 1), true},
		{"ordinary event scalar", strings.Replace(ordinaryReadOnlyWorkflow, "on:\n  pull_request:", "on: push", 1), false},
		{"event name in step text", strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", "echo pull_request_target", 1), false},
		{"protected environment", strings.Replace(ordinaryReadOnlyWorkflow, "    runs-on: ubuntu-latest", "    environment: production\n    runs-on: ubuntu-latest", 1), true},
		{"self hosted", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "[self-hosted, linux]", 1), true},
		{"literal hosted runner sequence", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "[ubuntu-latest]", 1), false},
		{"quoted literal runner whitespace", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", `" ubuntu-latest "`, 1), true},
		{"quoted singleton runner sequence whitespace", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", `[" ubuntu-latest "]`, 1), true},
		{"multiple hosted-looking runner labels", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "[ubuntu-latest, macos-latest]", 1), true},
		{"literal custom runner sequence", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "[ubuntu-latest, custom]", 1), true},
		{"invented Ubuntu-shaped label", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "ubuntu-99.99", 1), true},
		{"invented Windows-shaped label", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "windows-9999", 1), true},
		{"invented macOS-shaped label", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "macos-99", 1), true},
		{"current Ubuntu slim label", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "ubuntu-slim", 1), false},
		{"current Windows VS 2026 label", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "windows-2025-vs2026", 1), false},
		{"current Windows ARM VS 2026 label", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "windows-11-vs2026-arm", 1), false},
		{"current macOS Intel label", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "macos-26-intel", 1), false},
		{"dynamic runner", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "${{ inputs.runner }}", 1), true},
		{"known hosted matrix", matrixWorkflow, false},
		{"single hosted matrix sequence", matrixSequenceWorkflow, false},
		{"mixed literal and matrix multi-label sequence", matrixMixedMultiLabelWorkflow, true},
		{"matrix expression with outer scalar whitespace", matrixOuterWhitespaceExpressionWorkflow, true},
		{"matrix axis value with scalar whitespace", matrixWhitespaceAxisWorkflow, true},
		{"uppercase hosted matrix axis", matrixUppercaseHostedAxisWorkflow, false},
		{"matrix include overrides hosted runner", matrixIncludeOverrideWorkflow, true},
		{"uppercase matrix include overrides hosted runner", matrixUppercaseIncludeOverrideWorkflow, true},
		{"constant-folded matrix include key", matrixExpressionIncludeOverrideWorkflow, true},
		{"dynamic matrix include key", matrixDynamicIncludeOverrideWorkflow, true},
		{"matrix include value with scalar whitespace", matrixWhitespaceIncludeOverrideWorkflow, true},
		{"case-fold duplicate matrix runner keys", matrixCaseFoldDuplicateWorkflow, true},
		{"uppercase INCLUDE is an ordinary axis", matrixUppercaseIncludeAxisWorkflow, false},
		{"container credentials", credentialWorkflow, true},
		{"constant-folded job permissions key", expressionPermissionKeyWorkflow, true},
		{"constant-folded environment key", expressionEnvironmentKeyWorkflow, true},
		{"constant-folded event key", expressionEventKeyWorkflow, true},
		{"constant-folded credentials key", expressionCredentialsKeyWorkflow, true},
		{"reusable workflow caller authority", reusableCallerWorkflow, true},
		{"scalar read-all permission", strings.Replace(ordinaryReadOnlyWorkflow, "permissions:\n  contents: read", "permissions: read-all", 1), false},
		{"scalar read-all permission with outer whitespace", strings.Replace(ordinaryReadOnlyWorkflow, "permissions:\n  contents: read", `permissions: " read-all "`, 1), true},
		{"read permission with outer whitespace", strings.Replace(ordinaryReadOnlyWorkflow, "contents: read", `contents: " read "`, 1), true},
		{"low-blast write permission with outer whitespace", strings.Replace(ordinaryReadOnlyWorkflow, "contents: read", `issues: " write "`, 1), true},
		{"permission scope with outer whitespace", strings.Replace(ordinaryReadOnlyWorkflow, "contents: read", `" contents ": read`, 1), true},
		{"missing effective permissions", strings.Replace(ordinaryReadOnlyWorkflow, "permissions:\n  contents: read\n", "", 1), true},
		{"job permissions narrow root write", strings.Replace(strings.Replace(ordinaryReadOnlyWorkflow, "contents: read", "contents: write", 1), "    runs-on: ubuntu-latest", "    permissions:\n      contents: read\n    runs-on: ubuntu-latest", 1), false},
		{"job permissions widen root read", strings.Replace(ordinaryReadOnlyWorkflow, "    runs-on: ubuntu-latest", "    permissions:\n      contents: write\n    runs-on: ubuntu-latest", 1), true},
		{"malformed", "on: [\n", true},
		{"multiple documents", ordinaryReadOnlyWorkflow + "---\n" + ordinaryReadOnlyWorkflow, true},
		{"duplicate key", ordinaryReadOnlyWorkflow + "jobs: {}\n", true},
		{"non-string mapping key", ordinaryReadOnlyWorkflow + "123: value\n", true},
		{"anchor", strings.Replace(ordinaryReadOnlyWorkflow, "ubuntu-latest", "&runner ubuntu-latest", 1), true},
		{"excessive YAML depth", deepWorkflow.String(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := workflowPrivilegeReason([]byte(tt.raw))
			if got != tt.want {
				t.Fatalf("workflow privilege = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestPrivilegedWorkflowEscalationTriggers_BaseAndHead(t *testing.T) {
	ordinaryChanged := strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", "go test -race ./...", 1)
	// Cron frequency is a cost and unattended-execution signal that no
	// permission scope reveals, so it is compared across the revisions rather
	// than classified from a single blob.
	scheduledReadOnlyWorkflow := strings.Replace(ordinaryReadOnlyWorkflow, "on:\n  pull_request:\n",
		"on:\n  pull_request:\n  schedule:\n    - cron: '0 6 * * 1'\n", 1)
	frequentScheduleWorkflow := strings.Replace(scheduledReadOnlyWorkflow, "0 6 * * 1", "*/5 * * * *", 1)
	scheduledWorkflowChanged := strings.Replace(scheduledReadOnlyWorkflow, "go test ./...", "go test -race ./...", 1)
	ambiguousScheduleWorkflow := strings.Replace(scheduledReadOnlyWorkflow, "  schedule:\n",
		"  SCHEDULE:\n    - cron: '0 6 * * 1'\n  schedule:\n", 1)
	criticalWithoutPrivilege := strings.ReplaceAll(criticalWorkflow, "  id-token: write\n", "")
	criticalWithoutPrivilege = strings.Replace(criticalWithoutPrivilege, "${{ secrets.CUSTOM_AGENT_TOKEN }}", "${{ secrets.GITHUB_TOKEN }}", 1)
	guardWeakened := strings.Replace(criticalWorkflow, `contains(fromJSON('["OWNER","MEMBER"]'), github.event.comment.author_association)`, "contains(github.event.comment.body, '@agent')", 1)
	orderedMatrix := `name: ordered privileged matrix
on: push
permissions:
  contents: write
jobs:
  deploy:
    strategy:
      max-parallel: 1
      fail-fast: true
      matrix:
        target: [staging, production]
        gate: [safe, fail]
    runs-on: ubuntu-latest
    steps:
      - run: echo "${{ matrix.target }} ${{ matrix.gate }}"
`
	reorderedMatrix := strings.Replace(orderedMatrix, "        target: [staging, production]\n        gate: [safe, fail]", "        gate: [safe, fail]\n        target: [staging, production]", 1)
	uppercaseIncludeAxisMatrix := strings.Replace(orderedMatrix, "        gate: [safe, fail]", "        INCLUDE: [safe, fail]", 1)
	reorderedUppercaseIncludeAxisMatrix := strings.Replace(uppercaseIncludeAxisMatrix, "        target: [staging, production]\n        INCLUDE: [safe, fail]", "        INCLUDE: [safe, fail]\n        target: [staging, production]", 1)
	reservedMatrix := strings.Replace(orderedMatrix, "        target: [staging, production]\n        gate: [safe, fail]", "        os: [ubuntu-latest]\n        include:\n          - os: ubuntu-latest", 1)
	relocatedReservedMatrix := strings.Replace(reservedMatrix, "        os: [ubuntu-latest]\n        include:\n          - os: ubuntu-latest", "        include:\n          - os: ubuntu-latest\n        os: [ubuntu-latest]", 1)
	tests := []struct {
		name string
		base *string
		head *string
		want bool
	}{
		{"ordinary modification", new(ordinaryReadOnlyWorkflow), new(ordinaryChanged), false},
		{"arbitrary new privileged", nil, new(criticalWorkflow), true},
		{"remove privilege", new(criticalWorkflow), new(criticalWithoutPrivilege), true},
		{"delete privileged", new(criticalWorkflow), nil, true},
		{"delete ordinary", new(ordinaryReadOnlyWorkflow), nil, false},
		{"weaken guard on privileged workflow", new(criticalWorkflow), new(guardWeakened), true},
		{"reorder privileged matrix axes", new(orderedMatrix), new(reorderedMatrix), true},
		{"reorder uppercase INCLUDE matrix axis", new(uppercaseIncludeAxisMatrix), new(reorderedUppercaseIncludeAxisMatrix), true},
		{"relocate reserved matrix include", new(reservedMatrix), new(relocatedReservedMatrix), false},
		{"add schedule to a read-only workflow", new(ordinaryReadOnlyWorkflow), new(scheduledReadOnlyWorkflow), true},
		{"speed up an existing schedule", new(scheduledReadOnlyWorkflow), new(frequentScheduleWorkflow), true},
		{"remove a schedule", new(scheduledReadOnlyWorkflow), new(ordinaryReadOnlyWorkflow), true},
		{"edit a scheduled workflow without retiming it", new(scheduledReadOnlyWorkflow), new(scheduledWorkflowChanged), false},
		{"duplicate case-fold schedule keys", new(scheduledReadOnlyWorkflow), new(ambiguousScheduleWorkflow), true},
		{"malformed head", new(ordinaryReadOnlyWorkflow), new("on: [\n"), true},
		{"oversize head", new(ordinaryReadOnlyWorkflow), new(strings.Repeat("#", maxWorkflowBlobBytes+1)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, head, paths := workflowGitFixture(t, ".github/workflows/candidate.yml", tt.base, tt.head)
			got := privilegedWorkflowEscalationTriggers(context.Background(), base, head, paths)
			if (len(got) > 0) != tt.want {
				t.Fatalf("workflow triggers = %v, want triggered=%t", got, tt.want)
			}
		})
	}
}

func TestBuildReviewPlan_PrivilegedWorkflowProducesKeylessCannotRunEvidence(t *testing.T) {
	guardWeakened := strings.Replace(criticalWorkflow, `contains(fromJSON('["OWNER","MEMBER"]'), github.event.comment.author_association)`, "contains(github.event.comment.body, '@agent')", 1)
	base, head, _ := workflowGitFixture(t, ".github/workflows/claude.yml", new(criticalWorkflow), new(guardWeakened))

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ReviewNeeded || !plan.ReviewRelevant || plan.needsHuman() || len(plan.EscalationTriggers) == 0 {
		t.Fatalf("privileged workflow plan = %#v", plan)
	}
	if !strings.Contains(strings.Join(plan.EscalationTriggers, "\n"), "privileged workflow authority change") {
		t.Fatalf("privileged workflow triggers = %v", plan.EscalationTriggers)
	}

	c := baseConfig()
	c.baseSHA, c.headSHA = base, head
	if got := run(c); got != exitKeylessCannotRun {
		t.Fatalf("credentialless privileged workflow run = %d, want blocking cannot-run code %d", got, exitKeylessCannotRun)
	}
}

func TestPrivilegedWorkflowEscalationTriggers_PrivilegedFormattingOnlyIsNeutral(t *testing.T) {
	reordered := `jobs:
  agent:
    steps:
      - with:
          token: ${{ secrets.CUSTOM_AGENT_TOKEN }}
        uses: vendor/agent@v1
    runs-on: ubuntu-latest
    if: contains(fromJSON('["OWNER","MEMBER"]'), github.event.comment.author_association)
permissions:
  id-token: write
  contents: read
on:
  issue_comment:
    types: [created]
name: privileged
`
	base, head, paths := workflowGitFixture(t, ".github/workflows/agent.yml", new(criticalWorkflow), new(reordered))
	if got := privilegedWorkflowEscalationTriggers(context.Background(), base, head, paths); len(got) != 0 {
		t.Fatalf("semantic-equivalent privileged workflow escalated: %v", got)
	}
}

func TestPrivilegedWorkflowEscalationTriggers_CaseFoldAliasSeesPrivilegedBasePeer(t *testing.T) {
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string { return sandbox.Run(t, dir, args...) }
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	writeWorkflowFile(t, dir, ".github/workflows/claude.yml", criticalWorkflow)
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))

	shadow := filepath.Join(dir, "ordinary-alias")
	if err := os.WriteFile(shadow, []byte(ordinaryReadOnlyWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	object := trim(git("hash-object", "-w", "ordinary-alias"))
	git("update-index", "--add", "--cacheinfo", "100644,"+object+",.GitHub/Workflows/Claude.yml")
	git("commit", "-q", "-m", "add ordinary case-fold alias")
	head := trim(git("rev-parse", "HEAD"))

	chdir(t, dir)
	paths, err := gitChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != ".GitHub/Workflows/Claude.yml" {
		t.Fatalf("changed paths = %v, want only case-fold alias", paths)
	}
	if got := privilegedWorkflowEscalationTriggers(context.Background(), base, head, paths); len(got) == 0 {
		t.Fatal("ordinary alias of privileged base workflow must escalate")
	}
}

func TestPrivilegedWorkflowEscalationTriggers_UnicodeFoldAliasSeesPrivilegedBasePeer(t *testing.T) {
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string { return sandbox.Run(t, dir, args...) }
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	writeWorkflowFile(t, dir, ".github/workflows/claude.yml", criticalWorkflow)
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))

	alias := filepath.Join(dir, "ordinary-unicode-alias")
	if err := os.WriteFile(alias, []byte(ordinaryReadOnlyWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	object := trim(git("hash-object", "-w", "ordinary-unicode-alias"))
	const aliasPath = ".github/workflowſ/claude.yml"
	git("update-index", "--add", "--cacheinfo", "100644,"+object+","+aliasPath)
	git("commit", "-q", "-m", "add ordinary Unicode-fold alias")
	head := trim(git("rev-parse", "HEAD"))

	chdir(t, dir)
	paths, err := gitChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != aliasPath {
		t.Fatalf("changed paths = %v, want only Unicode-fold alias", paths)
	}
	if got := privilegedWorkflowEscalationTriggers(context.Background(), base, head, paths); len(got) == 0 {
		t.Fatal("ordinary Unicode-fold alias of privileged base workflow must escalate")
	}
}

func TestPrivilegedWorkflowEscalationTriggers_NonRegularEvidenceFailsClosed(t *testing.T) {
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string { return sandbox.Run(t, dir, args...) }
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	path := ".github/workflows/ordinary.yml"
	writeWorkflowFile(t, dir, path, ordinaryReadOnlyWorkflow)
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))

	if err := os.Chmod(filepath.Join(dir, filepath.FromSlash(path)), 0o755); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "make workflow executable")
	head := trim(git("rev-parse", "HEAD"))

	chdir(t, dir)
	paths, err := gitChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	if got := privilegedWorkflowEscalationTriggers(context.Background(), base, head, paths); len(got) == 0 {
		t.Fatal("non-regular workflow evidence must fail closed")
	}
}

func TestPrivilegedWorkflowEscalationTriggers_CanceledEvidenceReadFailsClosed(t *testing.T) {
	changed := strings.Replace(ordinaryReadOnlyWorkflow, "go test ./...", "go test -race ./...", 1)
	base, head, paths := workflowGitFixture(t, ".github/workflows/ordinary.yml", new(ordinaryReadOnlyWorkflow), new(changed))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := privilegedWorkflowEscalationTriggers(ctx, base, head, paths); len(got) == 0 {
		t.Fatal("canceled workflow evidence read must fail closed")
	}
}

func TestPrivilegedWorkflowEscalationTriggers_SeparateDependencyOutsideCurrentScope(t *testing.T) {
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string { return sandbox.Run(t, dir, args...) }
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	privilegedCaller := strings.Replace(criticalWorkflow, "    steps:\n", "    steps:\n      - run: sh ./scripts/review.sh\n", 1)
	writeWorkflowFile(t, dir, ".github/workflows/agent.yml", privilegedCaller)
	writeWorkflowFile(t, dir, "scripts/review.sh", "#!/bin/sh\necho before\n")
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))

	writeWorkflowFile(t, dir, "scripts/review.sh", "#!/bin/sh\necho after\n")
	git("add", "-A")
	git("commit", "-q", "-m", "change separate dependency")
	head := trim(git("rev-parse", "HEAD"))

	chdir(t, dir)
	paths, err := gitChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(paths, []string{"scripts/review.sh"}) {
		t.Fatalf("changed paths = %v, want only separate script dependency", paths)
	}
	if got := privilegedWorkflowEscalationTriggers(context.Background(), base, head, paths); len(got) != 0 {
		t.Fatalf("changed-workflow classifier claimed transitive dependency closure: %v", got)
	}
}

func workflowGitFixture(t *testing.T, path string, baseContent, headContent *string) (string, string, []string) {
	t.Helper()
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string { return sandbox.Run(t, dir, args...) }
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if baseContent != nil {
		writeWorkflowFile(t, dir, path, *baseContent)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))

	if headContent == nil {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(path))); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	} else {
		writeWorkflowFile(t, dir, path, *headContent)
	}
	git("add", "-A")
	git("commit", "-q", "--allow-empty", "-m", "head")
	head := trim(git("rev-parse", "HEAD"))

	chdir(t, dir)
	paths, err := gitChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	return base, head, paths
}

func writeWorkflowFile(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
