package tmux

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPiReadyPatternUsesLatestManagedStatus(t *testing.T) {
	ready := "────────────────────────\n \n────────────────────────\n/work • pi-worker\n0.0%/0 (auto)  AGM plan/ready  unknown"
	if !containsPiReadyPattern(ready) {
		t.Fatal("managed Pi ready footer was not detected")
	}
	working := ready + "\nAGM plan/working"
	if containsPiReadyPattern(working) {
		t.Fatal("stale ready status overrode a later working status")
	}
	if containsPiReadyPattern("────────────────────────\n────────────────────────\n/work • pi-worker") {
		t.Fatal("unmanaged Pi chrome was accepted without the authorization extension status")
	}
}

func TestGenericHarnessPromptDetectionHonorsLatestPiStatus(t *testing.T) {
	ready := "/work • pi-worker\nAGM default/ready"
	if !containsAnyHarnessPromptPattern(ready) {
		t.Fatal("generic prompt detection omitted managed Pi readiness")
	}
	if containsAnyHarnessPromptPattern(ready + "\nAGM default/working") {
		t.Fatal("generic prompt detection accepted a stale Pi ready marker")
	}
}

func TestWaitForPiPromptFailsFastOnExtensionLoadError(t *testing.T) {
	runtime := piPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			return []byte(`Failed to load extension "/tmp/agm-authorization.js": syntax error`), nil
		},
		sleep: func(context.Context, time.Duration) error { return nil },
	}
	err := waitForPiPromptWithRuntime(t.Context(), "pi-broken", "launch-new", time.Second, runtime)
	var startupErr *PiStartupError
	if !errors.As(err, &startupErr) || !strings.Contains(startupErr.Detail, "Failed to load extension") {
		t.Fatalf("error = %v, want PiStartupError", err)
	}
}

func TestWaitForPiPromptObservesManagedReadyStatus(t *testing.T) {
	outputs := [][]byte{
		[]byte("pi v0.81.0\nloading"),
		[]byte("/work • pi-worker\nAGM default/ready launch-new"),
	}
	index := 0
	runtime := piPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			output := outputs[index]
			if index < len(outputs)-1 {
				index++
			}
			return output, nil
		},
		sleep: func(context.Context, time.Duration) error { return nil },
	}
	if err := waitForPiPromptWithRuntime(t.Context(), "pi-ready", "launch-new", time.Second, runtime); err != nil {
		t.Fatal(err)
	}
}

func TestWaitForPiPromptRejectsStaleLaunchReadiness(t *testing.T) {
	outputs := [][]byte{
		[]byte("/work • pi-worker\nAGM default/ready launch-old"),
		[]byte("/work • pi-worker\nAGM default/working launch-new"),
		[]byte("/work • pi-worker\nAGM default/ready launch-new"),
	}
	index := 0
	runtime := piPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			output := outputs[index]
			if index < len(outputs)-1 {
				index++
			}
			return output, nil
		},
		alive: func(context.Context, string) (bool, error) { return true, nil },
		sleep: func(context.Context, time.Duration) error { return nil },
	}
	if err := waitForPiPromptWithRuntime(t.Context(), "pi-ready", "launch-new", time.Second, runtime); err != nil {
		t.Fatal(err)
	}
	if index != len(outputs)-1 {
		t.Fatalf("readiness accepted stale launch marker after capture %d", index)
	}
}

func TestWaitForPiPromptFailsClosedWhenExactLivenessCannotBeProved(t *testing.T) {
	checks := 0
	runtime := piPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			checks++
			return []byte("Pi loading"), nil
		},
		alive: func(context.Context, string) (bool, error) {
			return false, errors.New("ps unavailable")
		},
		sleep: func(context.Context, time.Duration) error { return nil },
	}
	err := waitForPiPromptWithRuntime(t.Context(), "pi-unknown", "launch-new", time.Second, runtime)
	var startupErr *PiStartupError
	if !errors.As(err, &startupErr) || !strings.Contains(startupErr.Detail, "cannot prove exact Pi process liveness") {
		t.Fatalf("error = %v, want fail-closed liveness error", err)
	}
	if checks != 3 {
		t.Fatalf("capture checks = %d, want 3 before liveness gate", checks)
	}
}

func TestWaitForPiPromptHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	captured := false
	runtime := piPromptRuntime{
		capture: func(context.Context, string) ([]byte, error) {
			captured = true
			return nil, nil
		},
		sleep: func(context.Context, time.Duration) error { return nil },
	}
	err := waitForPiPromptWithRuntime(ctx, "pi-cancelled", "launch-new", time.Minute, runtime)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if captured {
		t.Fatal("pane capture ran after cancellation")
	}
}
