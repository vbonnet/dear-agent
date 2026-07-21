package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRootCommandOwnsProcessSignalHandling(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read command package: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "main.go" {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(data), "signal.Notify") {
			t.Errorf("%s installs process signal handling; commands must consume cmd.Context() from the root owner", name)
		}
	}
}

func TestLongRunningCommandsConsumeRootContext(t *testing.T) {
	required := map[string][]string{
		"scan.go":            {"runScanLoop(cmd.Context())", "runSingleScan(cmd.Context())"},
		"heartbeat.go":       {"ctx := cmd.Context()"},
		"watch.go":           {"w.Run(cmd.Context())"},
		"watch_stalled.go":   {"ctx := cmd.Context()"},
		"send_compact.go":    {"verifyCompaction(ctx,"},
		"session_compact.go": {"monitorCompaction(cmd.Context(),"},
	}
	for file, snippets := range required {
		data, err := os.ReadFile(filepath.Clean(file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, snippet := range snippets {
			if !strings.Contains(string(data), snippet) {
				t.Errorf("%s does not consume the root command context through %q", file, snippet)
			}
		}
	}
}

func TestCommandHandlersAvoidBackgroundMultilineDelivery(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read command package: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(data), "tmux.SendMultiLinePromptSafe(") {
			t.Errorf("%s uses background-context multiline delivery", name)
		}
	}
}

func TestExecuteWithSignalContextPropagatesCancellation(t *testing.T) {
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- executeWithSignalContext(context.Background(), func(ctx context.Context) error {
			close(ready)
			<-ctx.Done()
			return ctx.Err()
		}, syscall.SIGUSR1)
	}()
	<-ready
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find current process: %v", err)
	}
	if err := process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatalf("send test signal: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("executeWithSignalContext error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("signal did not cancel root execution context")
	}
}
