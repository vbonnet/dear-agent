package tmux

import (
	"context"
	"testing"
)

// trustAnswerRuntime replays a scripted sequence of pane captures so the
// navigate-then-confirm handshake can be asserted without a live Claude.
type trustAnswerRuntime struct {
	captures []string
	captured int
	keys     []string
	enters   int
	live     bool
}

func (r *trustAnswerRuntime) runtime() claudeInputProbeRuntime {
	return claudeInputProbeRuntime{
		resolve: func(context.Context, string) (activePaneTarget, bool, error) {
			return activePaneTarget{ID: "%1"}, true, nil
		},
		capture: func(context.Context, string) (string, error) {
			if r.captured >= len(r.captures) {
				return r.captures[len(r.captures)-1], nil
			}
			content := r.captures[r.captured]
			r.captured++
			return content, nil
		},
		liveness: func(context.Context, activePaneTarget) (PaneLiveness, error) {
			return PaneLiveness{SessionExists: true, HarnessAlive: r.live}, nil
		},
		sendEnter: func(context.Context, string) error {
			r.enters++
			return nil
		},
		sendKey: func(_ context.Context, _, key string) error {
			r.keys = append(r.keys, key)
			return nil
		},
	}
}

// The regression this fixes: AGM saw the trust dialog, recorded that it was not
// affirmative, and did nothing. The dialog never resolves on its own, so the
// composer wait then burned its whole budget.
func TestObserveClaudeInputMovesSelectorThenConfirms(t *testing.T) {
	runtime := &trustAnswerRuntime{
		live: true,
		captures: []string{
			realTrustDialogNegativeSelected,    // initial classification
			realTrustDialogNegativeSelected,    // re-capture before acting
			realTrustDialogAffirmativeSelected, // after the selector move
		},
	}
	observation, err := observeClaudeInput(context.Background(), "session", true, runtime.runtime())
	if err != nil {
		t.Fatalf("observeClaudeInput() error = %v", err)
	}
	if !observation.probe.TrustAnswered {
		t.Error("TrustAnswered = false, want the dialog answered")
	}
	if len(runtime.keys) != 1 || runtime.keys[0] != "Down" {
		t.Errorf("selector moves = %v, want exactly one \"Down\"", runtime.keys)
	}
	if runtime.enters != 1 {
		t.Errorf("Enter presses = %d, want 1", runtime.enters)
	}
}

// Enter is authorized by the capture taken after the move, not by assuming the
// move landed. If the selector did not move, nothing is confirmed.
func TestObserveClaudeInputDoesNotConfirmWhenSelectorDidNotMove(t *testing.T) {
	runtime := &trustAnswerRuntime{
		live: true,
		captures: []string{
			realTrustDialogNegativeSelected,
			realTrustDialogNegativeSelected,
			realTrustDialogNegativeSelected, // move did not land
		},
	}
	observation, err := observeClaudeInput(context.Background(), "session", true, runtime.runtime())
	if err != nil {
		t.Fatalf("observeClaudeInput() error = %v", err)
	}
	if observation.probe.TrustAnswered {
		t.Error("TrustAnswered = true after a selector move that did not land")
	}
	if runtime.enters != 0 {
		t.Errorf("Enter presses = %d, want 0", runtime.enters)
	}
}

// Without autoAnswerTrust the probe only reports; it must not touch the pane.
func TestObserveClaudeInputObservesOnlyWhenAutoAnswerDisabled(t *testing.T) {
	runtime := &trustAnswerRuntime{live: true, captures: []string{realTrustDialogNegativeSelected}}
	observation, err := observeClaudeInput(context.Background(), "session", false, runtime.runtime())
	if err != nil {
		t.Fatalf("observeClaudeInput() error = %v", err)
	}
	if !observation.probe.DialogOwnsInput {
		t.Error("DialogOwnsInput = false for the live trust dialog")
	}
	if len(runtime.keys) != 0 || runtime.enters != 0 {
		t.Errorf("probe sent keys=%v enters=%d with auto-answer disabled", runtime.keys, runtime.enters)
	}
}

// A pane with no live Claude behind it must never be typed into.
func TestObserveClaudeInputRefusesWithoutLiveHarness(t *testing.T) {
	runtime := &trustAnswerRuntime{live: false, captures: []string{realTrustDialogNegativeSelected}}
	if _, err := observeClaudeInput(context.Background(), "session", true, runtime.runtime()); err == nil {
		t.Fatal("observeClaudeInput() accepted a pane without live Claude evidence")
	}
	if len(runtime.keys) != 0 || runtime.enters != 0 {
		t.Errorf("probe sent keys=%v enters=%d without a live harness", runtime.keys, runtime.enters)
	}
}
