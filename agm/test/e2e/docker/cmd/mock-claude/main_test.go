package main

import (
	"strings"
	"testing"
)

// The reaper waits for the prompt before sending anything, then sends the
// harness-native exit command and waits for the pane to close. Both halves are
// the contract; a mock that printed no prompt would hang the reaper at prompt
// detection, and one that ignored /exit would hang it at pane close.
func TestServe_ShowsPromptThenExitsOnNativeCommand(t *testing.T) {
	for _, command := range []string{"/exit", "/quit"} {
		t.Run(command, func(t *testing.T) {
			var out strings.Builder
			serve(strings.NewReader(command+"\n"), &out)

			got := out.String()
			if !strings.Contains(got, prompt) {
				t.Errorf("output has no %q prompt, reaper prompt detection would time out: %q", prompt, got)
			}
			if !strings.Contains(got, "Goodbye") {
				t.Errorf("output = %q, want the exit acknowledgement", got)
			}
		})
	}
}

func TestServe_ReprompsOnOrdinaryInputAndStopsAtEOF(t *testing.T) {
	var out strings.Builder
	serve(strings.NewReader("hello\nworld\n"), &out)

	got := out.String()
	if strings.Contains(got, "Goodbye") {
		t.Errorf("exited on ordinary input, want it to keep running: %q", got)
	}
	// One prompt at startup plus one per line consumed.
	if n := strings.Count(got, prompt); n != 3 {
		t.Errorf("prompt count = %d, want 3 (startup + one per input line): %q", n, got)
	}
}
