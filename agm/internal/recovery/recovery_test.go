package recovery

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestInterruptWorkLeavesSignalsOnlyValidLeaves(t *testing.T) {
	snapshot := Snapshot{WorkLeaves: []Process{{PID: 0}, {PID: 1}, {PID: 42}, {PID: 84}}}
	var signaled []int
	count, err := interruptWorkLeaves(snapshot, func(pid int) error {
		signaled = append(signaled, pid)
		return nil
	})
	if err != nil {
		t.Fatalf("interruptWorkLeaves() error = %v", err)
	}
	if count != 2 || !reflect.DeepEqual(signaled, []int{42, 84}) {
		t.Fatalf("interruptWorkLeaves() = count %d, PIDs %v; want 2, [42 84]", count, signaled)
	}
}

func TestInterruptWorkLeavesReportsSignalFailures(t *testing.T) {
	snapshot := Snapshot{WorkLeaves: []Process{{PID: 42}, {PID: 84}}}
	count, err := interruptWorkLeaves(snapshot, func(pid int) error {
		if pid == 42 {
			return fmt.Errorf("denied")
		}
		return nil
	})
	if count != 1 || err == nil || !strings.Contains(err.Error(), "pid 42: denied") {
		t.Fatalf("interruptWorkLeaves() = count %d, error %v; want one success and PID-specific failure", count, err)
	}
}

func TestWaitForConfirmationReturnsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	err := WaitForConfirmation(ctx, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForConfirmation() error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("WaitForConfirmation() took %v after cancellation", elapsed)
	}
}

func TestWaitForConfirmationReturnsAfterDuration(t *testing.T) {
	if err := WaitForConfirmation(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("WaitForConfirmation() error = %v", err)
	}
}
