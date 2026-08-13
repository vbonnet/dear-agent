package main

import (
	"os"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dispatchstate"
)

func TestCompletionSurfacerRelayTargetUsesLiveStateFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := dispatchstate.SetRelayTarget(home, "dispatch-live"); err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	t.Setenv("AGM_COMPLETION_RELAY_TARGET", "")

	cs := &completionSurfacer{orchestrator: "vroom-orchestrator"}
	if got := cs.relayTarget(); got != "dispatch-live" {
		t.Fatalf("relayTarget() = %q, want dispatch-live", got)
	}
}

func TestCompletionSurfacerRelayTargetKeepsFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGM_COMPLETION_RELAY_TARGET", "")

	cs := &completionSurfacer{orchestrator: "vroom-orchestrator"}
	if got := cs.relayTarget(); got != "vroom-orchestrator" {
		t.Fatalf("relayTarget() = %q, want fallback", got)
	}
	if _, err := os.Stat(dispatchstate.RelayTargetPath(home)); !os.IsNotExist(err) {
		t.Fatalf("relay target state unexpectedly exists: %v", err)
	}
}
