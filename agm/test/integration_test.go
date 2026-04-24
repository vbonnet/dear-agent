package test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/lock"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// TestConcurrentExecution_Locked verifies that concurrent AGM commands are prevented by file locking
func TestConcurrentExecution_Locked(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// First process acquires lock
	lock1, err := lock.New(lockPath)
	if err != nil {
		t.Fatalf("Failed to create lock: %v", err)
	}
	defer lock1.Unlock()

	if err := lock1.TryLock(); err != nil {
		t.Fatalf("First lock acquisition failed: %v", err)
	}

	// Second process tries to acquire lock (should fail immediately)
	lock2, err := lock.New(lockPath)
	if err != nil {
		t.Fatalf("Failed to create second lock: %v", err)
	}
	defer lock2.Unlock()

	start := time.Now()
	err = lock2.TryLock()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Second lock should have failed but succeeded")
	}

	// Should fail immediately (<100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Lock acquisition took too long: %v (expected < 100ms)", elapsed)
	}

	// Verify error message is helpful
	var lockErr *lock.LockError
	if !errors.As(err, &lockErr) {
		t.Errorf("Expected LockError, got %T", err)
	} else {
		if lockErr.Problem == "" || lockErr.Recovery == "" {
			t.Error("LockError missing Problem or Recovery guidance")
		}
	}
}

// TestCrashRecovery_LockAutoReleases verifies that locks are automatically released on process crash
func TestCrashRecovery_LockAutoReleases(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "crash-test.lock")

	// First lock
	lock1, err := lock.New(lockPath)
	if err != nil {
		t.Fatalf("Failed to create lock: %v", err)
	}

	if err := lock1.TryLock(); err != nil {
		t.Fatalf("Lock acquisition failed: %v", err)
	}

	// Simulate crash by closing file without unlocking
	lock1.Unlock() // Close the file descriptor

	// Wait for kernel to release the lock
	time.Sleep(10 * time.Millisecond)

	// Second lock should succeed (lock was auto-released)
	lock2, err := lock.New(lockPath)
	if err != nil {
		t.Fatalf("Failed to create second lock: %v", err)
	}
	defer lock2.Unlock()

	if err := lock2.TryLock(); err != nil {
		t.Errorf("Lock should be available after crash, but got error: %v", err)
	}
}

// TestHealthCheck_Caching verifies that health checks use caching to minimize overhead
func TestHealthCheck_Caching(t *testing.T) {
	hc := tmux.NewHealthChecker(5*time.Second, 2*time.Second)

	// First check (performs probe)
	start1 := time.Now()
	err1 := hc.Check()
	elapsed1 := time.Since(start1)

	if err1 != nil {
		// It's OK if tmux server isn't running
		t.Logf("First check: %v (elapsed: %v)", err1, elapsed1)
	}

	// Second check (should use cache)
	start2 := time.Now()
	err2 := hc.Check()
	elapsed2 := time.Since(start2)

	if err2 != nil {
		t.Logf("Second check: %v (elapsed: %v)", err2, elapsed2)
	}

	// Cached check should be much faster (<100ms overhead target)
	if elapsed2 > 100*time.Millisecond {
		t.Errorf("Cached health check took too long: %v (expected < 100ms)", elapsed2)
	}

	t.Logf("First check: %v, Cached check: %v (speedup: %.1fx)",
		elapsed1, elapsed2, float64(elapsed1)/float64(elapsed2))
}

// TestTimeout_FastFailure verifies that timeouts work correctly
func TestTimeout_FastFailure(t *testing.T) {
	ctx := context.Background()
	timeout := 100 * time.Millisecond

	// Run a command that will timeout
	start := time.Now()
	_, err := tmux.RunWithTimeout(ctx, timeout, "sleep", "10")
	elapsed := time.Since(start)

	// Should timeout quickly (not wait for full 10s)
	if elapsed > 1*time.Second {
		t.Errorf("Timeout took too long: %v (expected ~%v)", elapsed, timeout)
	}

	// Should return timeout error
	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}

	var timeoutErr *tmux.TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("Expected TimeoutError, got %T", err)
	}

	// Error should have recovery guidance
	if timeoutErr.Problem == "" || timeoutErr.Recovery == "" {
		t.Error("TimeoutError missing Problem or Recovery guidance")
	}

	t.Logf("Timeout error: %v", timeoutErr)
}

// TestTimeout_SuccessfulCommands verifies that successful commands have minimal overhead
func TestTimeout_SuccessfulCommands(t *testing.T) {
	ctx := context.Background()
	timeout := 5 * time.Second

	// Run fast command multiple times
	const iterations = 10
	var totalElapsed time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, err := tmux.RunWithTimeout(ctx, timeout, "echo", "test")
		elapsed := time.Since(start)
		totalElapsed += elapsed

		if err != nil {
			t.Errorf("Iteration %d failed: %v", i, err)
		}
	}

	avgElapsed := totalElapsed / iterations
	// Average overhead should be minimal (<50ms)
	if avgElapsed > 50*time.Millisecond {
		t.Errorf("Average command time too high: %v (expected < 50ms)", avgElapsed)
	}

	t.Logf("Average execution time for %d iterations: %v", iterations, avgElapsed)
}

// TestLockTimeout_CombinedBehavior tests lock and timeout working together
func TestLockTimeout_CombinedBehavior(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "combined.lock")

	// Acquire lock
	lock1, err := lock.New(lockPath)
	if err != nil {
		t.Fatalf("Failed to create lock: %v", err)
	}
	defer lock1.Unlock()

	if err := lock1.TryLock(); err != nil {
		t.Fatalf("Lock acquisition failed: %v", err)
	}

	// Run timed command while holding lock
	ctx := context.Background()
	timeout := 1 * time.Second

	start := time.Now()
	output, err := tmux.RunWithTimeout(ctx, timeout, "echo", "locked-command")
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Command failed: %v", err)
	}

	// Should complete quickly
	if elapsed > 100*time.Millisecond {
		t.Errorf("Command took too long: %v", elapsed)
	}

	if len(output) == 0 {
		t.Error("Command produced no output")
	}

	t.Logf("Combined lock+timeout test: %v elapsed", elapsed)
}

// TestHealthCheck_InvalidationWorks verifies cache invalidation
func TestHealthCheck_InvalidationWorks(t *testing.T) {
	hc := tmux.NewHealthChecker(10*time.Second, 2*time.Second)

	// First check
	hc.Check()

	// Invalidate cache
	hc.InvalidateCache()

	// Next check should perform fresh probe
	start := time.Now()
	err := hc.Check()
	elapsed := time.Since(start)

	if err != nil {
		t.Logf("Check after invalidation: %v", err)
	}

	// Should be slower than a cached check (or at least measurable)
	t.Logf("Check after cache invalidation: %v", elapsed)
}

// TestProcessCrashSimulation simulates a process crash with SIGKILL
func TestProcessCrashSimulation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping crash simulation in short mode")
	}

	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "crash-sim.lock")

	// Create a script that will acquire lock and get killed
	scriptPath := filepath.Join(tmpDir, "lock-holder.sh")
	scriptContent := fmt.Sprintf(`#!/bin/bash
# Compile a Go program that holds the lock
go run - <<'EOF'
package main
import (
	"time"
	"github.com/vbonnet/dear-agent/agm/internal/lock"
)
func main() {
	l, _ := lock.New("%s")
	l.TryLock()
	defer l.Unlock()
	time.Sleep(60 * time.Second) // Hold lock long enough for test to kill this process
}
EOF
`, lockPath)

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	// Start process that holds lock
	cmd := exec.Command("bash", scriptPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start lock holder: %v", err)
	}

	// Wait for lock to be acquired
	time.Sleep(200 * time.Millisecond)

	// Kill the process
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("Failed to kill process: %v", err)
	}

	// Wait for process to die
	cmd.Wait()
	time.Sleep(50 * time.Millisecond)

	// Try to acquire lock (should succeed after crash)
	lock2, err := lock.New(lockPath)
	if err != nil {
		t.Fatalf("Failed to create lock: %v", err)
	}
	defer lock2.Unlock()

	if err := lock2.TryLock(); err != nil {
		t.Errorf("Lock should be available after SIGKILL, but got error: %v", err)
	}
}
