package main

import (
	"bufio"
	"bytes"
	"context"
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
