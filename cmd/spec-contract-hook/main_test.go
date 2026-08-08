package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestRunRevalidatesOperatorOwnedHelperForDigestBoundInvocation(t *testing.T) {
	t.Setenv(reminderStateEnv, privateStateDirectory(t))
	root := stagedContractRepository(t)
	expected := strings.Repeat("a", 64)
	previous := verifyOperatorOwnedHelperDigest
	t.Cleanup(func() { verifyOperatorOwnedHelperDigest = previous })

	verified := ""
	verifyOperatorOwnedHelperDigest = func(digest string) error {
		verified = digest
		return nil
	}
	var output bytes.Buffer
	if got := run([]string{
		"--root", root,
		"--provider", "codex",
		"--event", "Stop",
		"--expected-helper-sha256", expected,
	}, strings.NewReader(`{"session_id":"digest-bound","turn_id":"one","stop_hook_active":false}`), &output, &bytes.Buffer{}); got != 0 {
		t.Fatalf("run() = %d output=%s", got, output.String())
	}
	if verified != expected {
		t.Fatalf("revalidated digest = %q, want %q", verified, expected)
	}
}

func TestRunBoundsDigestMismatchByCodexSessionAndTurnWithoutEvaluatingCheckout(t *testing.T) {
	t.Setenv(reminderStateEnv, privateStateDirectory(t))
	previous := verifyOperatorOwnedHelperDigest
	verifyOperatorOwnedHelperDigest = func(string) error { return errors.New("digest mismatch") }
	t.Cleanup(func() { verifyOperatorOwnedHelperDigest = previous })
	previousEvaluate := evaluateSpecContract
	evaluationCalls := 0
	evaluateSpecContract = func(context.Context, specguard.Request) specguard.Result {
		evaluationCalls++
		return specguard.Result{Decision: specguard.DecisionAllow}
	}
	t.Cleanup(func() { evaluateSpecContract = previousEvaluate })

	root := t.TempDir()
	invoke := func(turnID string, active bool) hookResponse {
		t.Helper()
		var output bytes.Buffer
		input := fmt.Sprintf(`{"session_id":"digest-bound","turn_id":%q,"stop_hook_active":%t}`, turnID, active)
		if got := run([]string{
			"--root", root,
			"--provider", "codex",
			"--event", "Stop",
			"--expected-helper-sha256", strings.Repeat("a", 64),
		}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
			t.Fatalf("run() = %d output=%s", got, output.String())
		}
		return decodeResponse(t, output.Bytes())
	}

	if response := invoke("one", false); response.Decision != "block" || !strings.Contains(response.Reason, "revision-bound digest") {
		t.Fatalf("first response = %#v, want fail-closed digest mismatch", response)
	}
	if response := invoke("one", true); response.Decision != "" || !strings.Contains(response.SystemMessage, "already ran") {
		t.Fatalf("repeated response = %#v, want bounded advisory yield", response)
	}
	if response := invoke("two", true); response.Decision != "block" || !strings.Contains(response.Reason, "revision-bound digest") {
		t.Fatalf("new-turn response = %#v, want fresh fail-closed digest mismatch", response)
	}
	if response := invoke("two", true); response.Decision != "" || response.SystemMessage == "" {
		t.Fatalf("repeated new-turn response = %#v, want bounded advisory yield", response)
	}
	if evaluationCalls != 0 {
		t.Fatalf("mutable checkout evaluations = %d, want 0 after helper digest mismatch", evaluationCalls)
	}
}

func TestRunValidatesCodexEnvelopeBeforeDigestRevalidation(t *testing.T) {
	previous := verifyOperatorOwnedHelperDigest
	verificationCalls := 0
	verifyOperatorOwnedHelperDigest = func(string) error {
		verificationCalls++
		return errors.New("digest mismatch")
	}
	t.Cleanup(func() { verifyOperatorOwnedHelperDigest = previous })

	var output bytes.Buffer
	if got := run([]string{
		"--root", ".",
		"--provider", "codex",
		"--event", "Stop",
		"--expected-helper-sha256", strings.Repeat("a", 64),
	}, strings.NewReader(`{"session_id":"digest-bound","stop_hook_active":false}`), &output, &bytes.Buffer{}); got != 0 {
		t.Fatalf("run() = %d output=%s", got, output.String())
	}
	response := decodeResponse(t, output.Bytes())
	if response.Decision != "" || response.SystemMessage == "" {
		t.Fatalf("response = %#v, want bounded invalid-envelope yield", response)
	}
	if verificationCalls != 0 {
		t.Fatalf("digest verifications = %d, want 0 before native input validation", verificationCalls)
	}
}

func TestRunDigestMismatchUsesActiveFlagOnlyWhenPrivateClaimStateIsUnavailable(t *testing.T) {
	t.Setenv(reminderStateEnv, "relative-state")
	previous := verifyOperatorOwnedHelperDigest
	verifyOperatorOwnedHelperDigest = func(string) error { return errors.New("digest mismatch") }
	t.Cleanup(func() { verifyOperatorOwnedHelperDigest = previous })
	previousEvaluate := evaluateSpecContract
	evaluationCalls := 0
	evaluateSpecContract = func(context.Context, specguard.Request) specguard.Result {
		evaluationCalls++
		return specguard.Result{Decision: specguard.DecisionAllow}
	}
	t.Cleanup(func() { evaluateSpecContract = previousEvaluate })

	root := t.TempDir()
	invoke := func(active bool) hookResponse {
		t.Helper()
		var output bytes.Buffer
		input := fmt.Sprintf(`{"session_id":"unavailable-state","turn_id":"one","stop_hook_active":%t}`, active)
		if got := run([]string{
			"--root", root,
			"--provider", "codex",
			"--event", "Stop",
			"--expected-helper-sha256", strings.Repeat("b", 64),
		}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
			t.Fatalf("run() = %d output=%s", got, output.String())
		}
		return decodeResponse(t, output.Bytes())
	}

	if response := invoke(false); response.Decision != "block" || !strings.Contains(response.Reason, "revision-bound digest") {
		t.Fatalf("ordinary first stop = %#v, want fail-closed mismatch", response)
	}
	if response := invoke(true); response.Decision != "" || !strings.Contains(response.SystemMessage, "could not establish private helper-digest retry state") {
		t.Fatalf("active continuation = %#v, want bounded state-unavailable yield", response)
	}
	if evaluationCalls != 0 {
		t.Fatalf("mutable checkout evaluations = %d, want 0 after helper digest mismatch", evaluationCalls)
	}
}

func TestRunYieldsInvalidHookInvocationWithoutAStableRetrySignal(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if got := run([]string{"--root", ".", "--event", "PreToolUse"}, bytes.NewReader(nil), &output, &bytes.Buffer{}); got != 0 {
		t.Fatalf("run() = %d, want hook-protocol success", got)
	}
	var response hookResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Decision != "" || response.SystemMessage == "" {
		t.Fatalf("response = %#v", response)
	}
	if strings.Contains(response.SystemMessage, "docs/spec-authoring.md") || strings.Contains(response.SystemMessage, "spec-governance/skills/write-spec/SKILL.md") {
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
		{provider: "claude", input: `{"hook_event_name":"Stop","session_id":"claude-reminder","stop_hook_active":false}`, wantDecision: "block"},
		{provider: "codex", input: `{"hook_event_name":"Stop","session_id":"codex-reminder","turn_id":"codex-turn-reminder","stop_hook_active":false}`, wantSystem: true, wantDecision: "block"},
		{provider: "pi", input: `{"hook_event_name":"Stop","session_id":"pi-reminder","stop_hook_active":false}`, wantDecision: "block"},
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

	writeFile(t, root, "pkg/example/SPEC.md", "# Example\n\n**EXAMPLE-01** When the guard evaluates a contract, the system shall preserve reciprocal BDD traceability.\n\n**EXAMPLE-02** When the staged contract changes, the system shall review the new immutable snapshot.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/example.feature`\n")
	gittest.New(t).Run(t, root, "add", "--", "pkg/example/SPEC.md")
	changed := specguard.Evaluate(context.Background(), specguard.Request{Repository: root, Mode: specguard.ModeStaged})
	if changed.SnapshotID == initial.SnapshotID {
		t.Fatalf("staged snapshot identity did not change: %q", changed.SnapshotID)
	}
	if response := runStop(`{"conversationId":"conversation-a","executionNum":3}`); response.Decision != "continue" {
		t.Fatalf("new snapshot response = %#v, want a fresh reminder", response)
	}
}

func TestAntigravityReminderStateFailureYieldsForEitherSequenceOrigin(t *testing.T) {
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
		{execution: 0, want: "allow"},
		{execution: 1, want: "allow"},
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
			}{{0, "allow"}, {1, "allow"}} {
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

func TestTerminalFeedbackClaimDistinguishesSiblingContinuationSnapshotAndTurn(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv(reminderStateEnv, privateStateDirectory(t))
			root := stagedContractRepository(t)
			invoke := func(session string, active bool, turnID string) hookResponse {
				t.Helper()
				var output bytes.Buffer
				input := fmt.Sprintf(`{"session_id":%q,"stop_hook_active":%t}`, session, active)
				if provider == "codex" {
					input = fmt.Sprintf(`{"session_id":%q,"turn_id":%q,"stop_hook_active":%t}`, session, turnID, active)
				}
				if got := run([]string{"--root", root, "--provider", provider, "--event", "Stop"}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
					t.Fatalf("run() = %d output=%s", got, output.String())
				}
				return decodeResponse(t, output.Bytes())
			}

			if response := invoke("session-a", true, "turn-a"); response.Decision != "block" {
				t.Fatalf("fresh snapshot during sibling continuation = %#v, want one block", response)
			}
			if response := invoke("session-a", true, "turn-a"); response.Decision != "" || response.SystemMessage == "" {
				t.Fatalf("repeated snapshot = %#v, want advisory yield", response)
			}

			writeFile(t, root, "pkg/example/SPEC.md", "# Example\n\n**EXAMPLE-01** When the guard evaluates a contract, the system shall preserve reciprocal BDD traceability.\n\n**EXAMPLE-02** When the staged contract changes, the system shall review the new immutable snapshot.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/example.feature`\n")
			gittest.New(t).Run(t, root, "add", "--", "pkg/example/SPEC.md")
			if response := invoke("session-a", true, "turn-a"); response.Decision != "block" {
				t.Fatalf("new snapshot during active continuation = %#v, want fresh block", response)
			}
			if response := invoke("session-a", true, "turn-a"); response.Decision != "" {
				t.Fatalf("repeated new snapshot = %#v, want advisory yield", response)
			}
			if response := invoke("session-b", true, "turn-b"); response.Decision != "block" {
				t.Fatalf("new session = %#v, want independent block", response)
			}
			if provider == "claude" {
				var output bytes.Buffer
				if got := run([]string{"--root", root, "--provider", provider, "--event", "UserPromptSubmit"}, strings.NewReader(`{"session_id":"session-a"}`), &output, &bytes.Buffer{}); got != 0 {
					t.Fatalf("reset run() = %d output=%s", got, output.String())
				}
				if response := decodeResponse(t, output.Bytes()); response.Decision != "" || response.SystemMessage != "" || response.Reason != "" {
					t.Fatalf("user-turn reset response = %#v, want nonblocking empty envelope", response)
				}
			}
			if response := invoke("session-a", false, "turn-a-next"); response.Decision != "block" {
				t.Fatalf("new real turn with unchanged snapshot = %#v, want refreshed block", response)
			}
		})
	}
}

func TestTerminalFeedbackClaimSerializesConcurrentFirstAttempts(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv(reminderStateEnv, privateStateDirectory(t))
			root := stagedContractRepository(t)
			const workers = 16
			start := make(chan struct{})
			responses := make(chan struct {
				code int
				body []byte
			}, workers)
			var wait sync.WaitGroup
			wait.Add(workers)
			for range workers {
				go func() {
					defer wait.Done()
					<-start
					var output bytes.Buffer
					input := `{"session_id":"concurrent-first","stop_hook_active":false}`
					if provider == "codex" {
						input = `{"session_id":"concurrent-first","turn_id":"concurrent-turn-1","stop_hook_active":false}`
					}
					code := run([]string{"--root", root, "--provider", provider, "--event", "Stop"},
						strings.NewReader(input), &output, &bytes.Buffer{})
					responses <- struct {
						code int
						body []byte
					}{code: code, body: bytes.Clone(output.Bytes())}
				}()
			}
			close(start)
			wait.Wait()
			close(responses)

			blockers := 0
			for result := range responses {
				if result.code != 0 {
					t.Fatalf("concurrent run() = %d output=%s", result.code, result.body)
				}
				response := decodeResponse(t, result.body)
				if response.Decision == "block" {
					blockers++
				} else if response.Decision != "" {
					t.Fatalf("concurrent response = %#v, want one block and advisory yields", response)
				}
			}
			if blockers != 1 {
				t.Fatalf("concurrent blockers = %d, want exactly one", blockers)
			}

			invoke := func(active bool, turnID string) hookResponse {
				t.Helper()
				var output bytes.Buffer
				input := fmt.Sprintf(`{"session_id":"concurrent-first","stop_hook_active":%t}`, active)
				if provider == "codex" {
					input = fmt.Sprintf(`{"session_id":"concurrent-first","turn_id":%q,"stop_hook_active":%t}`, turnID, active)
				}
				if got := run([]string{"--root", root, "--provider", provider, "--event", "SubagentStop"}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
					t.Fatalf("run() = %d output=%s", got, output.String())
				}
				return decodeResponse(t, output.Bytes())
			}
			if response := invoke(false, "concurrent-turn-1"); response.Decision != "" {
				t.Fatalf("same native turn = %#v, want advisory yield", response)
			}
			if provider == "claude" {
				var output bytes.Buffer
				if got := run([]string{"--root", root, "--provider", provider, "--event", "UserPromptSubmit"}, strings.NewReader(`{"session_id":"concurrent-first"}`), &output, &bytes.Buffer{}); got != 0 {
					t.Fatalf("reset run() = %d output=%s", got, output.String())
				}
			}
			if response := invoke(false, "concurrent-turn-2"); response.Decision != "block" {
				t.Fatalf("next native outer turn = %#v, want refreshed one-shot block", response)
			}
		})
	}
}

func TestCleanTerminalFeedbackDoesNotConsumeCapacityAndClearsClaims(t *testing.T) {
	requirePersistentReminderState(t)
	state := privateStateDirectory(t)
	t.Setenv(reminderStateEnv, state)
	cleanRoot := baseRepository(t)
	for index := 0; index <= maxReminderMarkers; index++ {
		var output, stderr bytes.Buffer
		input := fmt.Sprintf(`{"session_id":"clean-%03d","turn_id":"clean-turn-%03d","stop_hook_active":false}`, index, index)
		if got := run([]string{"--root", cleanRoot, "--provider", "codex", "--event", "Stop"}, strings.NewReader(input), &output, &stderr); got != 0 {
			t.Fatalf("clean session %d run() = %d output=%s stderr=%s", index, got, output.String(), stderr.String())
		}
		if response := decodeResponse(t, output.Bytes()); response.Decision == "block" {
			t.Fatalf("clean session %d response = %#v", index, response)
		}
	}
	if got := reminderMarkerCount(t, state); got != 0 {
		t.Fatalf("clean session marker count = %d, want 0", got)
	}

	root := stagedContractRepository(t)
	invoke := func(session, turnID string) hookResponse {
		t.Helper()
		var output bytes.Buffer
		input := fmt.Sprintf(`{"session_id":%q,"turn_id":%q,"stop_hook_active":false}`, session, turnID)
		if got := run([]string{"--root", root, "--provider", "codex", "--event", "Stop"}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
			t.Fatalf("run() = %d output=%s", got, output.String())
		}
		return decodeResponse(t, output.Bytes())
	}
	if response := invoke("claimed-session", "claimed-turn-1"); response.Decision != "block" {
		t.Fatalf("first governed response = %#v, want block", response)
	}
	if got := reminderMarkerCount(t, state); got != 1 {
		t.Fatalf("claimed marker count = %d, want 1", got)
	}
	sandbox := gittest.New(t)
	sandbox.Run(t, root, "commit", "-m", "commit contract")
	if response := invoke("claimed-session", "claimed-turn-1"); response.Decision == "block" {
		t.Fatalf("clean response = %#v, want allow and claim removal", response)
	}
	if got := reminderMarkerCount(t, state); got != 0 {
		t.Fatalf("marker count after clean result = %d, want 0", got)
	}
	writeFile(t, root, "pkg/example/SPEC.md", "# Example\n\n**EXAMPLE-01** When the guard evaluates a contract, the system shall preserve reciprocal BDD traceability.\n\n**EXAMPLE-02** When a cleared claim is retried, the system shall admit a new bounded feedback attempt.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/example.feature`\n")
	sandbox.Run(t, root, "add", "--", "pkg/example/SPEC.md")
	if response := invoke("new-session-after-clear", "post-clear-turn"); response.Decision != "block" {
		t.Fatalf("post-clear fresh response = %#v, want admitted block", response)
	}
	if got := reminderMarkerCount(t, state); got != 1 {
		t.Fatalf("post-clear marker count = %d, want 1", got)
	}
}

func TestTerminalFeedbackClaimBoundsDeterministicValidationFailures(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			t.Setenv(reminderStateEnv, privateStateDirectory(t))
			root, sandbox := committedContractRepository(t)
			stageInvalid := func(body string) {
				writeFile(t, root, "pkg/example/SPEC.md", body)
				sandbox.Run(t, root, "add", "--", "pkg/example/SPEC.md")
			}
			invoke := func() hookResponse {
				t.Helper()
				var output bytes.Buffer
				input := `{"session_id":"validation-session","stop_hook_active":true}`
				if provider == "codex" {
					input = `{"session_id":"validation-session","turn_id":"validation-turn","stop_hook_active":true}`
				}
				if got := run([]string{"--root", root, "--provider", provider, "--event", "Stop"}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
					t.Fatalf("run() = %d output=%s", got, output.String())
				}
				return decodeResponse(t, output.Bytes())
			}

			stageInvalid("invalid contract one\n")
			if response := invoke(); response.Decision != "block" {
				t.Fatalf("fresh deterministic failure = %#v, want block", response)
			}
			if response := invoke(); response.Decision != "" {
				t.Fatalf("repeated deterministic failure = %#v, want yield", response)
			}
			stageInvalid("invalid contract two\n")
			if response := invoke(); response.Decision != "block" {
				t.Fatalf("changed deterministic failure snapshot = %#v, want fresh block", response)
			}
			if response := invoke(); response.Decision != "" {
				t.Fatalf("repeated changed failure = %#v, want yield", response)
			}
		})
	}
}

func TestPiAdapterReturnsStableFeedbackIdentityForOuterLoop(t *testing.T) {
	root := stagedContractRepository(t)
	var previous string
	for attempt := 1; attempt <= 2; attempt++ {
		var output bytes.Buffer
		input := `{"session_id":"pi-feedback","stop_hook_active":true}`
		if got := run([]string{"--root", root, "--provider", "pi", "--event", "Stop"}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
			t.Fatalf("attempt %d run() = %d output=%s", attempt, got, output.String())
		}
		response := decodeResponse(t, output.Bytes())
		if response.Decision != "block" || !isLowerHexDigest(response.DearAgentSpecFeedbackID) {
			t.Fatalf("attempt %d response = %#v, want block with bounded feedback identity", attempt, response)
		}
		if previous != "" && response.DearAgentSpecFeedbackID != previous {
			t.Fatalf("feedback identity changed without a snapshot change: %s != %s", response.DearAgentSpecFeedbackID, previous)
		}
		previous = response.DearAgentSpecFeedbackID
	}
}

func TestAntigravityDeterministicBlockSupportsZeroBasedExecutionSequence(t *testing.T) {
	t.Setenv(reminderStateEnv, privateStateDirectory(t))
	root, _ := committedContractRepository(t)
	writeFile(t, root, "pkg/example/SPEC.md", "unstaged invalid contract\n")
	for _, test := range []struct {
		execution int
		want      string
	}{{execution: 0, want: "continue"}, {execution: 1, want: "allow"}} {
		var output bytes.Buffer
		body := fmt.Sprintf(`{"conversationId":"blocked-contract","executionNum":%d}`, test.execution)
		if got := run([]string{"--root", root, "--provider", "antigravity", "--event", "Stop"}, strings.NewReader(body), &output, &bytes.Buffer{}); got != 0 {
			t.Fatalf("execution %d run() = %d output=%s", test.execution, got, output.String())
		}
		if response := decodeResponse(t, output.Bytes()); response.Decision != test.want {
			t.Fatalf("execution %d response = %#v, want %q", test.execution, response, test.want)
		}
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
		{name: "malformed bounded input", args: []string{"--provider", "antigravity", "--root", ".", "--event", "Stop"}, input: []byte(`{`)},
		{name: "missing stable identity", args: []string{"--provider", "antigravity", "--root", ".", "--event", "Stop"}, input: []byte(`{}`)},
		{name: "missing execution number", args: []string{"--provider", "antigravity", "--root", ".", "--event", "Stop"}, input: []byte(`{"conversationId":"missing-execution"}`)},
		{name: "negative execution number", args: []string{"--provider", "antigravity", "--root", ".", "--event", "Stop"}, input: []byte(`{"conversationId":"negative-execution","executionNum":-1}`)},
		{name: "oversized conversation identity", args: []string{"--provider", "antigravity", "--root", ".", "--event", "Stop"}, input: fmt.Appendf(nil, `{"conversationId":%q,"executionNum":0}`, strings.Repeat("x", 4097))},
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
				ExecutionNumber: new(0),
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
				ExecutionNumber: new(2),
				WorkspacePaths:  test.paths,
			})
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if got := run([]string{"--root-from-workspace-stdin", "--provider", "antigravity", "--event", "Stop"}, bytes.NewReader(body), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("later execution run() = %d output=%s", got, output.String())
			}
			if response := decodeResponse(t, output.Bytes()); response.Decision != "continue" {
				t.Fatalf("new conversation at a later native sequence = %#v, want one state-bound continuation", response)
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
	t.Setenv(reminderStateEnv, privateStateDirectory(t))
	root := baseRepository(t)
	writeFile(t, root, "pkg/example/SPEC.md", "untracked mutable contract\n")

	for _, test := range []struct {
		provider string
		event    string
		decision string
		system   bool
		input    string
	}{
		{provider: "claude", event: "SubagentStop", decision: "block", input: `{"session_id":"dirty-claude","stop_hook_active":false}`},
		{provider: "codex", event: "SubagentStop", decision: "block", system: true, input: `{"session_id":"dirty-codex","turn_id":"dirty-codex-turn","stop_hook_active":false}`},
		{provider: "pi", event: "SubagentStop", decision: "block", input: `{"session_id":"dirty-pi","stop_hook_active":false}`},
		{provider: "opencode", event: "Stop", decision: "block", system: true, input: `{}`},
		{provider: "antigravity", event: "Stop", decision: "continue", input: `{"conversationId":"dirty-antigravity","executionNum":0}`},
	} {
		t.Run(test.provider, func(t *testing.T) {
			var output bytes.Buffer
			if got := run([]string{"--root", root, "--provider", test.provider, "--event", test.event}, strings.NewReader(test.input), &output, &bytes.Buffer{}); got != 0 {
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

func TestRunYieldsOversizedHookInputWithoutAStableRetrySignal(t *testing.T) {
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
	if response.Decision != "" || response.SystemMessage == "" {
		t.Fatalf("response = %#v", response)
	}
}

func TestRunYieldsMalformedBoundedHookInputWithoutAStableRetrySignal(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		provider    string
		input       string
		wantSystem  bool
		wantContext bool
	}{
		{name: "claude malformed object", provider: "claude", input: `{`, wantSystem: true},
		{name: "claude missing retry field", provider: "claude", input: `{"session_id":"missing-signal"}`, wantSystem: true},
		{name: "codex array", provider: "codex", input: `[]`, wantSystem: true},
		{name: "codex missing identity", provider: "codex", input: `{}`, wantSystem: true},
		{name: "pi wrong retry type", provider: "pi", input: `{"session_id":"pi-invalid","stop_hook_active":"true"}`, wantContext: true},
		{name: "opencode null", provider: "opencode", input: `null`, wantSystem: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for attempt := 1; attempt <= 2; attempt++ {
				var output bytes.Buffer
				if got := run([]string{"--root", ".", "--provider", test.provider, "--event", "Stop"}, strings.NewReader(test.input), &output, &bytes.Buffer{}); got != 0 {
					t.Fatalf("attempt %d run() = %d", attempt, got)
				}
				response := decodeResponse(t, output.Bytes())
				if response.Decision != "" {
					t.Fatalf("attempt %d response = %#v, want immediate advisory yield", attempt, response)
				}
				if test.wantSystem && response.SystemMessage == "" {
					t.Fatalf("attempt %d response = %#v, want system advisory", attempt, response)
				}
				if test.wantContext && (response.HookSpecificOutput == nil || response.HookSpecificOutput.AdditionalContext == "") {
					t.Fatalf("attempt %d response = %#v, want Pi advisory context", attempt, response)
				}
			}
		})
	}
}

func TestOpenCodeAdapterAcceptsPluginEmptyInput(t *testing.T) {
	root := stagedContractRepository(t)
	var output bytes.Buffer
	if got := run([]string{"--root", root, "--provider", "opencode", "--event", "Stop"}, strings.NewReader(""), &output, &bytes.Buffer{}); got != 0 {
		t.Fatalf("run() = %d output=%s", got, output.String())
	}
	response := decodeResponse(t, output.Bytes())
	if response.Decision != "block" || response.SystemMessage != stagedSPECReminderMessage {
		t.Fatalf("response = %#v, want bounded plugin reminder", response)
	}
}

func TestRunBlocksGovernedRenameAndBinaryContracts(t *testing.T) {
	t.Setenv(reminderStateEnv, privateStateDirectory(t))
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
			input := fmt.Sprintf(`{"session_id":%q,"turn_id":%q,"stop_hook_active":false}`, "governed-"+test.name, "governed-turn-"+test.name)
			if got := run([]string{"--root", root, "--provider", "codex", "--event", "Stop"}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
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
	t.Run("oversized Pi response retains its deterministic feedback identity", func(t *testing.T) {
		var output bytes.Buffer
		feedbackID := strings.Repeat("a", 64)
		if got := emitJSON(&output, hookResponse{Decision: "block", Reason: strings.Repeat("x", maxHookOutputBytes), DearAgentSpecFeedbackID: feedbackID}); got != 0 {
			t.Fatalf("emitJSON() = %d", got)
		}
		response := decodeResponse(t, output.Bytes())
		if response.Decision != "block" || response.DearAgentSpecFeedbackID != feedbackID || !strings.Contains(response.Reason, "exceeded its safety limit") {
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
	writeFile(t, root, "pkg/example/SPEC.md", "# Example\n\n**EXAMPLE-01** When the guard evaluates a contract, the system shall preserve reciprocal BDD traceability.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/example.feature`\n")
	writeFile(t, root, "agm/test/bdd/features/example.feature", "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: Contract remains valid\n    Given a staged contract\n    Then the guard reports its result\n")
	sandbox.Run(t, root, "add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
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

func reminderMarkerCount(t *testing.T, directory string) int {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), reminderMarkerPrefix) {
			count++
		}
	}
	return count
}
