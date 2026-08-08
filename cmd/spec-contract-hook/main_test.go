package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/specguard"
)

const reminderLockCrashHelperEnv = "DEAR_AGENT_REMINDER_LOCK_CRASH_HELPER"

func TestRunBlocksInvalidHookInvocation(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if got := run([]string{"--root", ".", "--event", "PreToolUse"}, bytes.NewReader(nil), &output, &bytes.Buffer{}); got != 0 {
		t.Fatalf("run() = %d, want hook-protocol success", got)
	}
	var response hookResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Decision != "block" || response.Reason == "" {
		t.Fatalf("response = %#v", response)
	}
	if strings.Contains(response.Reason, "docs/spec-authoring.md") || strings.Contains(response.Reason, "spec-governance/skills/write-spec/SKILL.md") {
		t.Fatalf("protocol error falsely routed through staged-change authoring guidance: %#v", response)
	}
}

func TestRunProvidesCooperativeTerminalReminderForValidStagedContract(t *testing.T) {
	t.Setenv(reminderStateEnv, privateStateDirectory(t))
	root := stagedContractRepository(t)
	for _, test := range []struct {
		provider     string
		input        string
		wantSystem   bool
		wantDecision string
	}{
		{provider: "claude", input: `{"hook_event_name":"Stop"}`, wantDecision: "block"},
		{provider: "codex", input: `{"hook_event_name":"Stop"}`, wantSystem: true, wantDecision: "block"},
		{provider: "pi", input: `{"hook_event_name":"Stop"}`, wantDecision: "block"},
		{provider: "opencode", input: `{"hook_event_name":"Stop"}`, wantSystem: true, wantDecision: "block"},
		{provider: "antigravity", input: `{"conversationId":"contract-review","executionNum":1}`, wantDecision: "continue"},
	} {
		t.Run(test.provider+"-"+test.input, func(t *testing.T) {
			var output bytes.Buffer
			if got := run([]string{"--root", root, "--provider", test.provider, "--event", "Stop"}, bytes.NewReader([]byte(test.input)), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("run() = %d output=%s", got, output.String())
			}
			response := decodeResponse(t, output.Bytes())
			projectedReminder := response.Reason
			if test.wantSystem {
				if response.SystemMessage == "" || response.HookSpecificOutput != nil {
					t.Fatalf("response = %#v, want top-level systemMessage only", response)
				}
				projectedReminder = response.SystemMessage
			} else if response.SystemMessage != "" || response.HookSpecificOutput != nil {
				t.Fatalf("response = %#v, want reason-only native reminder envelope", response)
			}
			if projectedReminder != stagedSPECReminderMessage {
				t.Fatalf("%s projected reminder = %q, want shared reminder %q", test.provider, projectedReminder, stagedSPECReminderMessage)
			}
			assertReminderText(t, projectedReminder)
			if test.wantDecision != "" && response.Decision != test.wantDecision {
				t.Fatalf("response = %#v, want decision %q", response, test.wantDecision)
			}
		})
	}
}

func TestAntigravityReminderClaimIsScopedToConversationAndSnapshot(t *testing.T) {
	requirePersistentReminderState(t)
	t.Setenv(reminderStateEnv, privateStateDirectory(t))
	root := stagedContractRepository(t)
	initial := specguard.Evaluate(context.Background(), specguard.Request{Repository: root, Mode: specguard.ModeStaged})
	runStop := func(body string) hookResponse {
		t.Helper()
		var output, stderr bytes.Buffer
		if got := run([]string{"--root", root, "--provider", "antigravity", "--event", "Stop"}, bytes.NewReader([]byte(body)), &output, &stderr); got != 0 {
			t.Fatalf("run() = %d output=%s", got, output.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("unexpected reminder-state fallback: %s", stderr.String())
		}
		return decodeResponse(t, output.Bytes())
	}

	if response := runStop(`{"conversationId":"conversation-a","executionNum":1}`); response.Decision != "continue" {
		t.Fatalf("first snapshot response = %#v, want one continuation response attempt", response)
	}
	if response := runStop(`{"conversationId":"conversation-a","executionNum":2}`); response.Decision != "allow" {
		t.Fatalf("repeated snapshot response = %#v, want bounded yield", response)
	}
	if response := runStop(`{"conversationId":"conversation-b","executionNum":1}`); response.Decision != "continue" {
		t.Fatalf("new conversation response = %#v, want its own reminder", response)
	}

	writeFile(t, root, "pkg/example/SPEC.md", "# Example\n\n**EXAMPLE-01** When the guard evaluates a contract, the system shall preserve reciprocal BDD traceability.\n\n**EXAMPLE-02** When the staged contract changes, the system shall review the new immutable snapshot.\n\n## BDD Traceability\n\n- Feature: `features/example.feature`\n")
	gittest.New(t).Run(t, root, "add", "--", "pkg/example/SPEC.md")
	changed := specguard.Evaluate(context.Background(), specguard.Request{Repository: root, Mode: specguard.ModeStaged})
	if changed.SnapshotID == initial.SnapshotID {
		t.Fatalf("staged snapshot identity did not change: %q", changed.SnapshotID)
	}
	if response := runStop(`{"conversationId":"conversation-a","executionNum":3}`); response.Decision != "continue" {
		t.Fatalf("new snapshot response = %#v, want a fresh reminder", response)
	}
}

func TestAntigravityReminderFallbackTreatsExecutionOneAsFirstInvocation(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(stateFile, []byte("unavailable"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(reminderStateEnv, stateFile)
	root := stagedContractRepository(t)
	for _, test := range []struct {
		execution int
		want      string
	}{
		{execution: 1, want: "continue"},
		{execution: 2, want: "allow"},
	} {
		var output bytes.Buffer
		body := fmt.Sprintf(`{"conversationId":"fallback","executionNum":%d}`, test.execution)
		if got := run([]string{"--root", root, "--provider", "antigravity", "--event", "Stop"}, strings.NewReader(body), &output, &bytes.Buffer{}); got != 0 {
			t.Fatalf("run() = %d output=%s", got, output.String())
		}
		if response := decodeResponse(t, output.Bytes()); response.Decision != test.want {
			t.Fatalf("execution %d response = %#v, want %q", test.execution, response, test.want)
		}
	}
}

func TestReminderMarkerStoreIsStrictlyBounded(t *testing.T) {
	requirePersistentReminderState(t)
	directory := t.TempDir()
	snapshot := strings.Repeat("b", 64)
	for index := range 256 {
		digest := fmt.Sprintf("%02x%062x", index, index)
		already, err := claimReminderMarker(directory, digest, snapshot)
		if err != nil || already {
			t.Fatalf("claim %d = (%t, %v)", index, already, err)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	markerCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), reminderMarkerPrefix) {
			markerCount++
		}
	}
	if markerCount != 256 {
		t.Fatalf("marker count = %d, want 256 fixed slots", markerCount)
	}
	if already, err := claimReminderMarker(directory, strings.Repeat("f", 64), snapshot); err == nil || already {
		t.Fatalf("overflow claim = (%t, %v), want conservative fallback", already, err)
	}
}

func TestReminderMarkerConcurrentClaimHasOneWinner(t *testing.T) {
	requirePersistentReminderState(t)
	directory := t.TempDir()
	conversation := strings.Repeat("a", 64)
	snapshot := strings.Repeat("b", 64)
	const workers = 32
	var wait sync.WaitGroup
	wait.Add(workers)
	results := make(chan bool, workers)
	errors := make(chan error, workers)
	for range workers {
		go func() {
			defer wait.Done()
			already, err := claimReminderMarker(directory, conversation, snapshot)
			results <- already
			errors <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	claims := 0
	for err := range errors {
		if err != nil {
			t.Errorf("concurrent claim: %v", err)
		}
	}
	for already := range results {
		if !already {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("fresh claims = %d, want exactly one", claims)
	}
}

func TestReminderLockIsReleasedWhenHolderCrashes(t *testing.T) {
	requirePersistentReminderState(t)
	directory := t.TempDir()
	conversation := strings.Repeat("a", 64)
	snapshot := strings.Repeat("b", 64)
	command := exec.Command(os.Args[0], "-test.run=^TestReminderLockCrashHelper$")
	command.Env = append(os.Environ(), reminderLockCrashHelperEnv+"="+directory, "DEAR_AGENT_REMINDER_LOCK_CONVERSATION="+conversation)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("crash helper readiness = %q, %v", line, err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("crash helper exited successfully, want killed process")
	}
	waited = true

	info, err := os.Lstat(filepath.Join(directory, reminderLockName))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("persistent lock identity = %#v, %v", info, err)
	}
	already, err := claimReminderMarker(directory, conversation, snapshot)
	if err != nil || already {
		t.Fatalf("recover partial marker after holder crash = (%t, %v), want fresh repaired claim", already, err)
	}
	if already, err := claimReminderMarker(directory, conversation, snapshot); err != nil || !already {
		t.Fatalf("repeat repaired claim = (%t, %v), want durable duplicate", already, err)
	}
}

func TestReminderLockCrashHelper(t *testing.T) {
	directory := os.Getenv(reminderLockCrashHelperEnv)
	if directory == "" {
		return
	}
	release, err := acquireReminderLock(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	conversation := os.Getenv("DEAR_AGENT_REMINDER_LOCK_CONVERSATION")
	if !isLowerHexDigest(conversation) {
		t.Fatal("crash helper conversation identity is invalid")
	}
	partial := filepath.Join(directory, reminderMarkerPrefix+conversation)
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(os.Stdout, "locked"); err != nil {
		t.Fatal(err)
	}
	select {}
}

func TestAntigravityReminderCanonicalizesRepositoryAliases(t *testing.T) {
	requirePersistentReminderState(t)
	state := privateStateDirectory(t)
	t.Setenv(reminderStateEnv, state)
	root := stagedContractRepository(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	invoke := func(repository string) hookResponse {
		t.Helper()
		var output, stderr bytes.Buffer
		body := `{"conversationId":"same-conversation","executionNum":1}`
		if got := run([]string{"--root", repository, "--provider", "antigravity", "--event", "Stop"}, strings.NewReader(body), &output, &stderr); got != 0 {
			t.Fatalf("run() = %d output=%s", got, output.String())
		}
		if stderr.Len() != 0 {
			t.Fatalf("unexpected reminder-state fallback: %s", stderr.String())
		}
		return decodeResponse(t, output.Bytes())
	}
	if response := invoke(alias); response.Decision != "continue" {
		t.Fatalf("alias first response = %#v", response)
	}
	result := specguard.Evaluate(context.Background(), specguard.Request{Repository: root, Mode: specguard.ModeStaged})
	if already, err := antigravityReminderAlreadyClaimed(root, []byte(`{"conversationId":"same-conversation","executionNum":1}`), result); err != nil || !already {
		t.Fatalf("canonical repeat claim = (%t, %v), want shared persistent identity", already, err)
	}
	if response := invoke(root); response.Decision != "allow" {
		t.Fatalf("canonical repeat response = %#v, want shared claim", response)
	}
}

func requirePersistentReminderState(t *testing.T) {
	t.Helper()
	if !persistentReminderLockSupported() {
		t.Skip("persistent reminder state is unsupported on this host")
	}
}

func TestUnsafeReminderStateDirectoriesUseBoundedFallback(t *testing.T) {
	root := stagedContractRepository(t)
	unsafe := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(unsafe, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafe, 0o777); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"relative-state", unsafe} {
		t.Run(directory, func(t *testing.T) {
			t.Setenv(reminderStateEnv, directory)
			for _, test := range []struct {
				execution int
				want      string
			}{{1, "continue"}, {2, "allow"}} {
				var output bytes.Buffer
				body := fmt.Sprintf(`{"conversationId":"fallback-%d","executionNum":%d}`, test.execution, test.execution)
				if got := run([]string{"--root", root, "--provider", "antigravity", "--event", "Stop"}, strings.NewReader(body), &output, &bytes.Buffer{}); got != 0 {
					t.Fatalf("run() = %d output=%s", got, output.String())
				}
				if response := decodeResponse(t, output.Bytes()); response.Decision != test.want {
					t.Fatalf("execution %d response = %#v, want %q", test.execution, response, test.want)
				}
			}
		})
	}
}

func TestRunYieldsAfterOneReminderResponseAttempt(t *testing.T) {
	t.Parallel()
	root := stagedContractRepository(t)
	for _, test := range []struct {
		provider    string
		wantSystem  bool
		wantContext bool
	}{
		{provider: "claude", wantSystem: true},
		{provider: "codex", wantSystem: true},
		{provider: "pi", wantContext: true},
	} {
		t.Run(test.provider, func(t *testing.T) {
			var output bytes.Buffer
			if got := run([]string{"--root", root, "--provider", test.provider, "--event", "Stop"}, bytes.NewReader([]byte(`{"stop_hook_active":true}`)), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("run() = %d output=%s", got, output.String())
			}
			response := decodeResponse(t, output.Bytes())
			if response.Decision != "" {
				t.Fatalf("response = %#v, want nonblocking retry-loop yield", response)
			}
			if test.wantSystem && (response.SystemMessage == "" || response.HookSpecificOutput != nil) {
				t.Fatalf("response = %#v, want one bounded system warning", response)
			}
			if test.wantContext && (response.HookSpecificOutput == nil || response.HookSpecificOutput.AdditionalContext == "") {
				t.Fatalf("response = %#v, want Pi bridge context without another block", response)
			}
		})
	}
}

func TestAntigravityFailurePathsWithoutStableIdentityAllowTermination(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		args  []string
		input []byte
	}{
		{name: "unsupported event", args: []string{"--provider", "antigravity", "--root", ".", "--event", "SubagentStop"}, input: []byte(`{}`)},
		{name: "invalid flag", args: []string{"--provider", "antigravity", "--unknown"}, input: []byte(`{}`)},
		{name: "oversized input", args: []string{"--provider", "antigravity", "--root", ".", "--event", "Stop"}, input: bytes.Repeat([]byte("x"), maxHookInputBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if got := run(test.args, bytes.NewReader(test.input), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("run() = %d", got)
			}
			response := decodeResponse(t, output.Bytes())
			if response.Decision != "allow" || response.Reason != "" {
				t.Fatalf("response = %#v, want termination without an unbounded retry", response)
			}
		})
	}
}

func TestAntigravityDerivesRepositoryFromOneNativeWorkspacePath(t *testing.T) {
	t.Setenv(reminderStateEnv, privateStateDirectory(t))
	root := stagedContractRepository(t)
	body := fmt.Sprintf(`{"conversationId":"workspace-root","executionNum":1,"workspacePaths":[%q]}`, root)
	var output bytes.Buffer
	if got := run([]string{"--root-from-workspace-stdin", "--provider", "antigravity", "--event", "Stop"}, strings.NewReader(body), &output, &bytes.Buffer{}); got != 0 {
		t.Fatalf("run() = %d output=%s", got, output.String())
	}
	if response := decodeResponse(t, output.Bytes()); response.Decision != "continue" || response.Reason == "" {
		t.Fatalf("response = %#v, want native continuation from the supplied workspace root", response)
	}
}

func TestAntigravityWorkspaceRootFailuresContinueOncePerConversation(t *testing.T) {
	root := baseRepository(t)
	secondRoot := baseRepository(t)
	nonGitRoot := t.TempDir()
	missingRoot := filepath.Join(t.TempDir(), "missing")
	nestedWorkspace := filepath.Join(root, ".git")
	tests := []struct {
		name  string
		paths []string
	}{
		{name: "missing workspace paths"},
		{name: "relative workspace path", paths: []string{"."}},
		{name: "nonexistent workspace path", paths: []string{missingRoot}},
		{name: "non Git workspace path", paths: []string{nonGitRoot}},
		{name: "nested Git metadata path", paths: []string{nestedWorkspace}},
		{name: "ambiguous workspace paths", paths: []string{root, secondRoot}},
		{name: "duplicate workspace paths", paths: []string{root, root}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(reminderStateEnv, privateStateDirectory(t))
			body, err := json.Marshal(antigravityStopInput{
				ConversationID:  "invalid-root-" + test.name,
				ExecutionNumber: 1,
				WorkspacePaths:  test.paths,
			})
			if err != nil {
				t.Fatal(err)
			}
			for attempt, want := range []string{"continue", "allow"} {
				var output bytes.Buffer
				if got := run([]string{"--root-from-workspace-stdin", "--provider", "antigravity", "--event", "Stop"}, bytes.NewReader(body), &output, &bytes.Buffer{}); got != 0 {
					t.Fatalf("attempt %d run() = %d output=%s", attempt+1, got, output.String())
				}
				response := decodeResponse(t, output.Bytes())
				if response.Decision != want {
					t.Fatalf("attempt %d response = %#v, want %s", attempt+1, response, want)
				}
				if want == "continue" && response.Reason == "" {
					t.Fatalf("attempt %d response = %#v, want bounded remediation", attempt+1, response)
				}
			}

			body, err = json.Marshal(antigravityStopInput{
				ConversationID:  "invalid-root-later-execution-" + test.name,
				ExecutionNumber: 2,
				WorkspacePaths:  test.paths,
			})
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if got := run([]string{"--root-from-workspace-stdin", "--provider", "antigravity", "--event", "Stop"}, bytes.NewReader(body), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("later execution run() = %d output=%s", got, output.String())
			}
			if response := decodeResponse(t, output.Bytes()); response.Decision != "allow" {
				t.Fatalf("later execution response = %#v, want termination", response)
			}
		})
	}
}

func TestAntigravityWorkspaceRootFailureAllowsWhenOneShotStateIsUnavailable(t *testing.T) {
	t.Setenv(reminderStateEnv, "relative-state-directory")
	body := `{"conversationId":"bounded-state-failure","executionNum":1,"workspacePaths":[]}`
	var output bytes.Buffer
	var stderr bytes.Buffer
	if got := run([]string{"--root-from-workspace-stdin", "--provider", "antigravity", "--event", "Stop"}, strings.NewReader(body), &output, &stderr); got != 0 {
		t.Fatalf("run() = %d output=%s", got, output.String())
	}
	if response := decodeResponse(t, output.Bytes()); response.Decision != "allow" {
		t.Fatalf("response = %#v, want termination when one-shot state cannot be proven", response)
	}
	if !strings.Contains(stderr.String(), "avoid an unbounded retry") {
		t.Fatalf("stderr = %q, want bounded-state diagnostic", stderr.String())
	}
}

func TestRunBlocksTerminalHookForUntrackedGovernedPath(t *testing.T) {
	t.Parallel()
	root := baseRepository(t)
	writeFile(t, root, "pkg/example/SPEC.md", "untracked mutable contract\n")

	for _, test := range []struct {
		provider string
		event    string
		decision string
		system   bool
	}{
		{provider: "claude", event: "SubagentStop", decision: "block"},
		{provider: "codex", event: "SubagentStop", decision: "block", system: true},
		{provider: "pi", event: "SubagentStop", decision: "block"},
		{provider: "opencode", event: "Stop", decision: "block", system: true},
		{provider: "antigravity", event: "Stop", decision: "continue"},
	} {
		t.Run(test.provider, func(t *testing.T) {
			var output bytes.Buffer
			if got := run([]string{"--root", root, "--provider", test.provider, "--event", test.event}, bytes.NewReader([]byte(`{}`)), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("run() = %d output=%s", got, output.String())
			}
			response := decodeResponse(t, output.Bytes())
			if response.Decision != test.decision || !strings.Contains(response.Reason, "dirty-governed-worktree") ||
				!strings.Contains(response.Reason, "stage the intended contract state or resolve") ||
				!strings.Contains(response.Reason, "separately reviewed changed-SPEC CI and provider rollout that this hook does not attest") {
				t.Fatalf("response = %#v", response)
			}
			if strings.Contains(response.Reason, "docs/spec-authoring.md") || strings.Contains(response.Reason, "spec-governance/skills/write-spec/SKILL.md") {
				t.Fatalf("blocking diagnostic falsely routed through successful reminder guidance: %#v", response)
			}
			if test.system && response.SystemMessage != response.Reason {
				t.Fatalf("Codex/OpenCode response = %#v, want top-level system message", response)
			}
		})
	}
}

func TestRunBlocksOversizedHookInput(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	input := bytes.Repeat([]byte("x"), maxHookInputBytes+1)
	if got := run([]string{"--root", ".", "--provider", "codex", "--event", "Stop"}, bytes.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
		t.Fatalf("run() = %d", got)
	}
	var response hookResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Decision != "block" {
		t.Fatalf("response = %#v", response)
	}
}

func TestRunBlocksGovernedRenameAndBinaryContracts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		stage func(t *testing.T, root string, sandbox *gittest.Sandbox)
	}{
		{
			name: "rename",
			stage: func(t *testing.T, root string, sandbox *gittest.Sandbox) {
				if err := os.MkdirAll(filepath.Join(root, "pkg", "renamed"), 0o700); err != nil {
					t.Fatal(err)
				}
				sandbox.Run(t, root, "mv", "pkg/example/SPEC.md", "pkg/renamed/SPEC.md")
			},
		},
		{
			name: "binary",
			stage: func(t *testing.T, root string, sandbox *gittest.Sandbox) {
				if err := os.WriteFile(filepath.Join(root, "pkg/example/SPEC.md"), []byte("\x00not a text contract"), 0o600); err != nil {
					t.Fatal(err)
				}
				sandbox.Run(t, root, "add", "--", "pkg/example/SPEC.md")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, sandbox := committedContractRepository(t)
			test.stage(t, root, sandbox)
			var output bytes.Buffer
			if got := run([]string{"--root", root, "--provider", "codex", "--event", "Stop"}, bytes.NewReader([]byte(`{}`)), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("run() = %d output=%s", got, output.String())
			}
			response := decodeResponse(t, output.Bytes())
			if response.Decision != "block" || response.SystemMessage == "" {
				t.Fatalf("response = %#v, want Codex block with a top-level system message", response)
			}
		})
	}
}

func TestEmitJSONBoundsFallbackAndDetectsShortWrite(t *testing.T) {
	t.Parallel()
	t.Run("oversized response becomes compact block", func(t *testing.T) {
		var output bytes.Buffer
		if got := emitJSON(&output, hookResponse{SystemMessage: strings.Repeat("x", maxHookOutputBytes)}); got != 0 {
			t.Fatalf("emitJSON() = %d", got)
		}
		if output.Len() > maxHookOutputBytes {
			t.Fatalf("output length = %d, limit = %d", output.Len(), maxHookOutputBytes)
		}
		response := decodeResponse(t, output.Bytes())
		if response.Decision != "block" || !strings.Contains(response.Reason, "exceeded its safety limit") {
			t.Fatalf("fallback response = %#v", response)
		}
	})
	t.Run("oversized Antigravity response remains a native continuation", func(t *testing.T) {
		var output bytes.Buffer
		if got := emitJSON(&output, hookResponse{Decision: "continue", Reason: strings.Repeat("x", maxHookOutputBytes)}); got != 0 {
			t.Fatalf("emitJSON() = %d", got)
		}
		response := decodeResponse(t, output.Bytes())
		if response.Decision != "continue" || !strings.Contains(response.Reason, "exceeded its safety limit") {
			t.Fatalf("fallback response = %#v", response)
		}
	})
	t.Run("short write is a failed hook transport", func(t *testing.T) {
		if got := emitJSON(shortWriter{remaining: 1}, hookResponse{Decision: "block", Reason: "must be complete"}); got != 1 {
			t.Fatalf("emitJSON(short writer) = %d, want 1", got)
		}
	})
}

type shortWriter struct{ remaining int }

func (writer shortWriter) Write(payload []byte) (int, error) {
	if writer.remaining >= len(payload) {
		return len(payload), nil
	}
	return writer.remaining, nil
}

func assertReminderText(t *testing.T, text string) {
	t.Helper()
	for _, phrase := range []string{"Cooperative", "docs/spec-authoring.md", "spec-governance/skills/write-spec/SKILL.md", "reference that skill instead of copying its body", "does not claim native skill discovery", "mutable checkout hook", "not tamper-resistant", "separately reviewed changed-SPEC CI and provider rollout", "does not attest that enforcement is deployed, has run, or is provider-required"} {
		if !strings.Contains(text, phrase) {
			t.Errorf("reminder %q omits %q", text, phrase)
		}
	}
}

func decodeResponse(t *testing.T, encoded []byte) hookResponse {
	t.Helper()
	var response hookResponse
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func baseRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	sandbox := gittest.New(t)
	sandbox.Run(t, root, "init")
	writeFile(t, root, "README.md", "base\n")
	sandbox.Run(t, root, "add", "--", "README.md")
	sandbox.Run(t, root, "commit", "-m", "base")
	return root
}

func stagedContractRepository(t *testing.T) string {
	t.Helper()
	root := baseRepository(t)
	sandbox := gittest.New(t)
	writeFile(t, root, "pkg/example/SPEC.md", "# Example\n\n**EXAMPLE-01** When the guard evaluates a contract, the system shall preserve reciprocal BDD traceability.\n\n## BDD Traceability\n\n- Feature: `features/example.feature`\n")
	writeFile(t, root, "features/example.feature", "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: Contract remains valid\n    Given a staged contract\n    Then the guard reports its result\n")
	sandbox.Run(t, root, "add", "--", "pkg/example/SPEC.md", "features/example.feature")
	return root
}

func committedContractRepository(t *testing.T) (string, *gittest.Sandbox) {
	t.Helper()
	root := stagedContractRepository(t)
	sandbox := gittest.New(t)
	sandbox.Run(t, root, "commit", "-m", "add contract")
	return root, sandbox
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func privateStateDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}
