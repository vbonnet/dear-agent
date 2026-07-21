package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// fastConfig: near-zero waits so the loop logic is exercised without real delay.
func fastConfig() enterVerifyConfig {
	return enterVerifyConfig{
		initialSettle: 0,
		backoffs: []time.Duration{
			time.Millisecond, time.Millisecond, time.Millisecond,
			time.Millisecond, time.Millisecond,
		},
	}
}

const codexParked = "> [Pasted Content 2172 chars]\n  gpt-5.6 xhigh · ~"
const codexRunning = "• Working (3s • esc to interrupt)\n  gpt-5.6 xhigh · ~"

// TestVerifyingEnter_SubmitsAfterSlowChipConversion is the ce-mjk9 regression:
// under load, codex takes several polls to convert the large paste to a
// "[Pasted Content]" chip and accept the Enter. The loop MUST keep re-sending
// Enter until the composer clears, then report success.
func TestVerifyingEnter_SubmitsAfterSlowChipConversion(t *testing.T) {
	captures := 0
	enters := 0
	sendEnter := func() error { enters++; return nil }
	capture := func() (string, error) {
		captures++
		// Parked for the first 2 checks (slow chip conversion), then submitted.
		if captures <= 2 {
			return codexParked, nil
		}
		return codexRunning, nil
	}

	if err := verifyingEnter(sendEnter, capture, fastConfig()); err != nil {
		t.Fatalf("expected submission to be confirmed, got error: %v", err)
	}
	if enters < 3 {
		t.Errorf("expected Enter re-sent until submission (>=3), got %d", enters)
	}
}

// TestVerifyingEnter_FailsLoudWhenNeverSubmitted: the pane stays parked forever
// (the exact silent-park failure). The loop MUST return ErrPasteNotSubmitted so
// SendMessage reports Delivered:false instead of a false success.
func TestVerifyingEnter_FailsLoudWhenNeverSubmitted(t *testing.T) {
	sendEnter := func() error { return nil }
	capture := func() (string, error) { return codexParked, nil }

	err := verifyingEnter(sendEnter, capture, fastConfig())
	if !errors.Is(err, ErrPasteNotSubmitted) {
		t.Fatalf("expected ErrPasteNotSubmitted (fail loud), got: %v", err)
	}
}

// TestVerifyingEnter_SubmitsImmediately: a fast host — the very first check
// shows the composer already cleared. One Enter, no error.
func TestVerifyingEnter_SubmitsImmediately(t *testing.T) {
	enters := 0
	sendEnter := func() error { enters++; return nil }
	capture := func() (string, error) { return codexRunning, nil }

	if err := verifyingEnter(sendEnter, capture, fastConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enters != 1 {
		t.Errorf("expected exactly 1 Enter on immediate submit, got %d", enters)
	}
}

// TestVerifyingEnter_CaptureFailureIsBestEffort: if verification itself can't
// run (capture errors every time), we must NOT fail loud on infra flakiness —
// the send may well have worked. Return nil (legacy best-effort).
func TestVerifyingEnter_CaptureFailureIsBestEffort(t *testing.T) {
	sendEnter := func() error { return nil }
	capture := func() (string, error) { return "", errors.New("capture-pane: no server") }

	if err := verifyingEnter(sendEnter, capture, fastConfig()); err != nil {
		t.Fatalf("capture failure must be best-effort (nil), got: %v", err)
	}
}

// TestVerifyingEnter_SendError propagates a real send-keys failure.
func TestVerifyingEnter_SendError(t *testing.T) {
	sendEnter := func() error { return errors.New("tmux: server not found") }
	capture := func() (string, error) { return codexRunning, nil }

	if err := verifyingEnter(sendEnter, capture, fastConfig()); err == nil {
		t.Fatal("expected send-keys error to propagate")
	}
}

func TestVerifyingEnter_PreservesUncertaintyAcrossLaterDefiniteFailure(t *testing.T) {
	enters := 0
	sendEnter := func() error {
		enters++
		if enters == 1 {
			return nil
		}
		return errors.New("tmux: explicit rejection")
	}
	capture := func() (string, error) {
		return "", errors.New("capture-pane acknowledgement lost")
	}

	err := verifyingEnter(sendEnter, capture, fastConfig())
	if err == nil || !PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("later failure = %v, want uncertainty from the first accepted Enter", err)
	}
}

func TestVerifyingEnter_PreservesUncertaintyAcrossLaterParkedCaptures(t *testing.T) {
	captures := 0
	sendEnter := func() error { return nil }
	capture := func() (string, error) {
		captures++
		if captures == 1 {
			return "", errors.New("capture-pane acknowledgement lost")
		}
		return codexParked, nil
	}

	err := verifyingEnter(sendEnter, capture, fastConfig())
	if !errors.Is(err, ErrPasteNotSubmitted) || !PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("final parked state = %v, want parked cause with sticky uncertainty", err)
	}
}

func TestPromptEnterCommandHelperProcess(t *testing.T) {
	if os.Getenv("AGM_PROMPT_ENTER_HELPER") != "1" {
		return
	}
	switch os.Getenv("AGM_PROMPT_ENTER_HELPER_MODE") {
	case "reject":
		os.Exit(17)
	case "block":
		time.Sleep(30 * time.Second)
	}
}

func promptEnterHelperCommand(ctx context.Context, mode string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPromptEnterCommandHelperProcess$")
	cmd.Env = append(os.Environ(), "AGM_PROMPT_ENTER_HELPER=1", "AGM_PROMPT_ENTER_HELPER_MODE="+mode)
	return cmd
}

func TestRunPromptEnterCommandStartFailureIsDefinite(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), filepath.Join(t.TempDir(), "missing-tmux"))
	err := runPromptEnterCommand(t.Context(), cmd, time.Second)
	if err == nil || PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("start failure = %v, want definite pre-submission error", err)
	}
}

func TestRunPromptEnterCommandExplicitRejectionIsDefinite(t *testing.T) {
	err := runPromptEnterCommand(t.Context(), promptEnterHelperCommand(t.Context(), "reject"), time.Second)
	if err == nil || PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("explicit command rejection = %v, want definite pre-submission error", err)
	}
}

func TestRunPromptEnterCommandTimeoutAfterStartIsUncertain(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	err := runPromptEnterCommand(ctx, promptEnterHelperCommand(ctx, "block"), 20*time.Millisecond)
	if err == nil || !PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("post-start timeout = %v, want submission-uncertain error", err)
	}
}

// TestIsPasteStuck_CodexChip: the detection fix — the codex "[Pasted Content]"
// chip must register as stuck (the ce-mjk9 blind spot).
func TestIsPasteStuck_CodexChip(t *testing.T) {
	if !isPasteStuck(codexParked) {
		t.Error("codex '[Pasted Content ...]' chip must be detected as stuck")
	}
	if !isPasteStuck("some pane\n[Pasted text 40 chars]\n> ") {
		t.Error("legacy '[Pasted text' must still be detected")
	}
	if isPasteStuck(codexRunning) {
		t.Error("a running/cleared composer must NOT be detected as stuck")
	}
}
