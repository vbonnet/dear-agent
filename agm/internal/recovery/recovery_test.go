package recovery

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/procreaper"
)

func TestConfirmedRequiresWorkProcessExit(t *testing.T) {
	before := BuildSnapshot(100, []procreaper.ProcessInfo{
		{PID: 101, PPID: 100, Command: "agy"},
		{PID: 102, PPID: 101, Command: "bash"},
		{PID: 103, PPID: 102, Command: "find"},
	})
	unchanged := before
	if Confirmed(before, unchanged, true) {
		t.Fatal("ready-looking pane with unchanged work PID must not confirm recovery")
	}
	after := BuildSnapshot(100, []procreaper.ProcessInfo{{PID: 101, PPID: 100, Command: "agy"}})
	if !Confirmed(before, after, false) {
		t.Fatal("exited work PID should confirm recovery")
	}
}

func TestConfirmedUsesPromptOnlyWithoutWorkProcess(t *testing.T) {
	snapshot := BuildSnapshot(100, []procreaper.ProcessInfo{{PID: 101, PPID: 100, Command: "codex"}})
	if Confirmed(snapshot, snapshot, false) {
		t.Fatal("capture without a ready prompt must not confirm recovery")
	}
	if !Confirmed(snapshot, snapshot, true) {
		t.Fatal("ready prompt should confirm recovery when no work process existed")
	}
}

func TestFallbackForActiveHarnesses(t *testing.T) {
	cases := map[string]Fallback{
		"claude-code":  FallbackNone,
		"codex-cli":    FallbackNone,
		"agy":          FallbackLeafInterrupt,
		"opencode-cli": FallbackNone,
	}
	for harness, want := range cases {
		if got := FallbackForHarness(harness); got != want {
			t.Errorf("FallbackForHarness(%q) = %q, want %q", harness, got, want)
		}
	}
}

func TestBuildSnapshotNeverTreatsHarnessRuntimeAsWork(t *testing.T) {
	snapshot := BuildSnapshot(100, []procreaper.ProcessInfo{
		{PID: 101, PPID: 100, Command: "/usr/local/bin/agy"},
		{PID: 102, PPID: 101, Command: "/usr/bin/find"},
	})
	if len(snapshot.WorkLeaves) != 1 || snapshot.WorkLeaves[0].PID != 102 {
		t.Fatalf("work leaves = %+v, want only find PID 102", snapshot.WorkLeaves)
	}
}
