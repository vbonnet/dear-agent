package tmux

import (
	"errors"
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

func TestClassifyPromptSubmissionErrorDistinguishesParkedFromUncertain(t *testing.T) {
	lostReply := errors.New("tmux reply lost")
	uncertain := classifyPromptSubmissionError(lostReply)
	if !PromptSubmissionMayHaveOccurred(uncertain) || !errors.Is(uncertain, lostReply) {
		t.Fatalf("classified lost reply = %v, want uncertain wrapper preserving cause", uncertain)
	}

	parked := classifyPromptSubmissionError(ErrPasteNotSubmitted)
	if PromptSubmissionMayHaveOccurred(parked) || !errors.Is(parked, ErrPasteNotSubmitted) {
		t.Fatalf("classified parked prompt = %v, want definite unsubmitted failure", parked)
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
