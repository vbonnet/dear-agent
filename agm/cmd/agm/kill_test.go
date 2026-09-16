package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestKill_AgentMode_BackendFailureReturnsJSON(t *testing.T) {
	setAgentJSON(t)
	wantErr := errors.New("tmux socket unavailable")

	var done bool
	var retErr error
	out := captureStdout(t, func() {
		_, done, retErr = handleKillError(nil, "demo", nil, wantErr)
	})

	if !done || retErr == nil {
		t.Fatalf("done = %v, error = %v; want terminal failure", done, retErr)
	}
	var response map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &response); err != nil {
		t.Fatalf("stdout is not JSON: %v\noutput: %q", err, out)
	}
	if response["error"] != wantErr.Error() || response["session"] != "demo" {
		t.Fatalf("response = %#v, want backend error and exact session", response)
	}
}

// setAgentJSON flips the package-global output state into agent/JSON mode for
// the duration of a test and restores it afterwards, mirroring how
// PersistentPreRunE resolves it at runtime.
func setAgentJSON(t *testing.T) {
	t.Helper()
	origFmt, origMode := outputFormat, outputMode
	outputFormat, outputMode = "json", ModeAgent
	t.Cleanup(func() { outputFormat, outputMode = origFmt, origMode })
}

// setHumanText flips the globals into human/text mode and restores them after.
func setHumanText(t *testing.T) {
	t.Helper()
	origFmt, origMode := outputFormat, outputMode
	outputFormat, outputMode = "text", ModeHuman
	t.Cleanup(func() { outputFormat, outputMode = origFmt, origMode })
}

// TestEmitConfirmationEnvelope_Exit4 checks the helper prints a JSON envelope
// to stdout and returns an exitError carrying the state-conflict code (4).
func TestEmitConfirmationEnvelope_Exit4(t *testing.T) {
	setAgentJSON(t)

	var retErr error
	out := captureStdout(t, func() {
		retErr = emitConfirmationEnvelope(ConfirmationEnvelope{
			Action:   "session_kill",
			Target:   "demo",
			RerunCmd: "agm session kill --confirmed-stuck demo",
			Reason:   "session is active; add --confirmed-stuck to force",
		})
	})

	if got := exitCodeFromError(retErr); got != ExitStateConflict {
		t.Fatalf("exit code = %d, want %d", got, ExitStateConflict)
	}

	var env ConfirmationEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\noutput: %q", err, out)
	}
	if env.ExitCode != ExitStateConflict {
		t.Errorf("envelope exit_code = %d, want %d", env.ExitCode, ExitStateConflict)
	}
	if env.Error != "confirmation required" {
		t.Errorf("envelope error = %q, want %q", env.Error, "confirmation required")
	}
}

// TestKill_AgentMode_NoForce_ReturnsEnvelope covers the headline scenario:
// killing an active session in agent-mode without --confirmed-stuck must NOT
// block on a prompt. It returns a JSON envelope (exit 4) with the exact re-run
// command carrying --confirmed-stuck.
func TestKill_AgentMode_NoForce_ReturnsEnvelope(t *testing.T) {
	setAgentJSON(t)

	var killResult *ops.KillSessionResult
	var done bool
	var retErr error
	out := captureStdout(t, func() {
		killResult, done, retErr = handleKillError(nil, "demo", nil, ops.ErrActiveSessionKill("demo"))
	})

	if !done {
		t.Fatalf("done = false, want true (envelope path is terminal)")
	}
	if killResult != nil {
		t.Errorf("killResult = %+v, want nil", killResult)
	}
	if got := exitCodeFromError(retErr); got != ExitStateConflict {
		t.Fatalf("exit code = %d, want %d", got, ExitStateConflict)
	}

	var env ConfirmationEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\noutput: %q", err, out)
	}
	if env.Action != "session_kill" {
		t.Errorf("action = %q, want %q", env.Action, "session_kill")
	}
	if env.Target != "demo" {
		t.Errorf("target = %q, want %q", env.Target, "demo")
	}
	if !strings.Contains(env.RerunCmd, "--confirmed-stuck") || !strings.Contains(env.RerunCmd, "demo") {
		t.Errorf("rerun_cmd = %q, want it to contain --confirmed-stuck and the target", env.RerunCmd)
	}
}

// TestKill_AgentMode_KillProtected_ReturnsEnvelope covers the recently-active
// (kill-protected) case: agent-mode emits an envelope whose bypass flag is
// --force rather than --confirmed-stuck.
func TestKill_AgentMode_KillProtected_ReturnsEnvelope(t *testing.T) {
	setAgentJSON(t)

	var done bool
	var retErr error
	out := captureStdout(t, func() {
		_, done, retErr = handleKillError(nil, "demo", nil, ops.ErrKillProtected("demo", time.Time{}))
	})

	if !done {
		t.Fatalf("done = false, want true")
	}
	if got := exitCodeFromError(retErr); got != ExitStateConflict {
		t.Fatalf("exit code = %d, want %d", got, ExitStateConflict)
	}

	var env ConfirmationEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\noutput: %q", err, out)
	}
	if !strings.Contains(env.RerunCmd, "--force") {
		t.Errorf("rerun_cmd = %q, want it to contain --force", env.RerunCmd)
	}
}

// TestKill_HumanMode_NoEnvelope confirms the human (TTY) path is unchanged: an
// active-session kill renders the text warning and returns the existing error
// instead of emitting a JSON confirmation envelope. (The interactive huh
// prompt itself is exercised by the kill-protected path below.)
func TestKill_HumanMode_NoEnvelope(t *testing.T) {
	setHumanText(t)

	var retErr error
	out := captureStdout(t, func() {
		_, _, retErr = handleKillError(nil, "demo", nil, ops.ErrActiveSessionKill("demo"))
	})

	if retErr == nil {
		t.Fatalf("retErr = nil, want the active-session error")
	}
	if strings.Contains(out, "\"exit_code\"") || strings.Contains(out, "\"rerun_cmd\"") {
		t.Errorf("human mode must not emit a JSON envelope, got: %q", out)
	}
}

// TestKill_HumanMode_Prompts confirms the kill-protected path in human mode
// takes the interactive prompt branch rather than the envelope branch.
func TestKill_HumanMode_Prompts(t *testing.T) {
	setHumanText(t)
	origPrompt := confirmKillProtected
	var promptCalled bool
	confirmKillProtected = func(sessionName string) (bool, error) {
		promptCalled = true
		return false, errors.New("prompt cancelled")
	}
	t.Cleanup(func() { confirmKillProtected = origPrompt })

	var done bool
	var retErr error
	out := captureStdout(t, func() {
		_, done, retErr = handleKillError(nil, "demo", nil, ops.ErrKillProtected("demo", time.Time{}))
	})

	if !promptCalled {
		t.Fatalf("prompt was not called in human mode")
	}
	if !done {
		t.Fatalf("done = false, want true (cancelled prompt is terminal)")
	}
	if retErr != nil {
		t.Errorf("retErr = %v, want nil (cancellation is not an error)", retErr)
	}
	if strings.Contains(out, "\"rerun_cmd\"") {
		t.Errorf("human mode must not emit a JSON envelope, got: %q", out)
	}
}
