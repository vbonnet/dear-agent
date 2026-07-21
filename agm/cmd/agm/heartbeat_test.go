package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/monitoring"
)

func TestRunHeartbeatWatchdogUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	started := time.Now()

	if err := runHeartbeatWatchdog(cmd, []string{"test-watchdog"}); err != nil {
		t.Fatalf("runHeartbeatWatchdog() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("canceled watchdog returned after %s", elapsed)
	}
}

func TestExecuteRestartContextUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	target := filepath.Join(t.TempDir(), "must-not-exist")

	err := executeRestartContext(ctx, "touch "+target)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("executeRestartContext() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("canceled restart created %s", target)
	}
}

func TestCheckStalenessWithMaxAge_OK(t *testing.T) {
	hb := &monitoring.LoopHeartbeat{
		Timestamp:    time.Now(),
		Session:      "test",
		IntervalSecs: 300,
	}
	status := monitoring.CheckStalenessWithMaxAge(hb, 20*time.Minute)
	if status != "ok" {
		t.Errorf("expected 'ok', got %q", status)
	}
}

func TestCheckStalenessWithMaxAge_Stale(t *testing.T) {
	hb := &monitoring.LoopHeartbeat{
		Timestamp:    time.Now().Add(-25 * time.Minute),
		Session:      "test",
		IntervalSecs: 300,
	}
	status := monitoring.CheckStalenessWithMaxAge(hb, 20*time.Minute)
	if status != "stale" {
		t.Errorf("expected 'stale', got %q", status)
	}
}

func TestCheckStalenessWithMaxAge_Warn(t *testing.T) {
	// 80% of 20m = 16m, so 17m should be warn
	hb := &monitoring.LoopHeartbeat{
		Timestamp:    time.Now().Add(-17 * time.Minute),
		Session:      "test",
		IntervalSecs: 300,
	}
	status := monitoring.CheckStalenessWithMaxAge(hb, 20*time.Minute)
	if status != "warn" {
		t.Errorf("expected 'warn', got %q", status)
	}
}

func TestCheckStalenessWithMaxAge_Nil(t *testing.T) {
	status := monitoring.CheckStalenessWithMaxAge(nil, 20*time.Minute)
	if status != "stale" {
		t.Errorf("expected 'stale' for nil, got %q", status)
	}
}

func TestFormatStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ok", "OK"},
		{"warn", "WARN"},
		{"stale", "STALE"},
		{"unknown", "unknown"},
	}
	for _, tc := range tests {
		got := formatStatus(tc.input)
		if got != tc.want {
			t.Errorf("formatStatus(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestHeartbeatWriteAndCheck_Integration(t *testing.T) {
	dir := t.TempDir()

	writer, err := monitoring.NewHeartbeatWriter(dir)
	if err != nil {
		t.Fatalf("NewHeartbeatWriter: %v", err)
	}

	err = writer.Write("watchdog-test", 300, 1, true)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Read back and check with max-age
	hb, err := monitoring.ReadHeartbeat(dir, "watchdog-test")
	if err != nil {
		t.Fatalf("ReadHeartbeat: %v", err)
	}

	// Fresh heartbeat should be OK with 20m max-age
	status := monitoring.CheckStalenessWithMaxAge(hb, 20*time.Minute)
	if status != "ok" {
		t.Errorf("fresh heartbeat: expected 'ok', got %q", status)
	}

	// Same heartbeat should be stale with 0 max-age
	status = monitoring.CheckStalenessWithMaxAge(hb, 0)
	if status != "stale" {
		t.Errorf("zero max-age: expected 'stale', got %q", status)
	}
}

func TestHeartbeatWatchdog_NoHeartbeatFile(t *testing.T) {
	dir := t.TempDir()

	// Verify no heartbeat file exists
	_, err := monitoring.ReadHeartbeat(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent heartbeat")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got: %v", err)
	}
}

func TestExecuteRestart(t *testing.T) {
	// Test that executeRestart runs a simple command
	tmpFile := filepath.Join(t.TempDir(), "restart-marker")
	err := executeRestart("touch " + tmpFile)
	if err != nil {
		t.Fatalf("executeRestart: %v", err)
	}

	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Fatal("restart command did not execute")
	}
}

func TestExecuteRestart_FailingCommand(t *testing.T) {
	err := executeRestart("false")
	if err == nil {
		t.Fatal("expected error for failing command")
	}
}
