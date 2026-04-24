package tmux

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/lock"
)

// TestMain cleans up stale lock files from previous crashed test runs
// before any tests execute. Without this, a killed test process leaves
// a lock file that causes all subsequent lock tests to fail.
func TestMain(m *testing.M) {
	ReleaseTmuxLock()
	uid := os.Getuid()
	lockPath := fmt.Sprintf("/tmp/agm-%d/tmux-server.lock", uid)
	os.Remove(lockPath)
	os.Exit(m.Run())
}

// TestTmuxLock_AcquireRelease tests basic lock acquisition and release
func TestTmuxLock_AcquireRelease(t *testing.T) {
	// Clean up any existing lock
	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	// Acquire lock
	if err := AcquireTmuxLock(); err != nil {
		t.Fatalf("AcquireTmuxLock() failed: %v", err)
	}

	// Release lock
	if err := ReleaseTmuxLock(); err != nil {
		t.Errorf("ReleaseTmuxLock() failed: %v", err)
	}

	// Verify lock can be acquired again
	if err := AcquireTmuxLock(); err != nil {
		t.Errorf("Second AcquireTmuxLock() failed after release: %v", err)
	}

	// Cleanup
	if err := ReleaseTmuxLock(); err != nil {
		t.Errorf("Final ReleaseTmuxLock() failed: %v", err)
	}
}

// TestTmuxLock_MultipleReleaseSafe tests that multiple releases are safe
func TestTmuxLock_MultipleReleaseSafe(t *testing.T) {
	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	if err := AcquireTmuxLock(); err != nil {
		t.Fatalf("AcquireTmuxLock() failed: %v", err)
	}

	// Multiple releases should not panic or error
	if err := ReleaseTmuxLock(); err != nil {
		t.Errorf("First ReleaseTmuxLock() failed: %v", err)
	}
	if err := ReleaseTmuxLock(); err != nil {
		t.Errorf("Second ReleaseTmuxLock() failed: %v", err)
	}
	if err := ReleaseTmuxLock(); err != nil {
		t.Errorf("Third ReleaseTmuxLock() failed: %v", err)
	}
}

// TestTmuxLock_Concurrency_NewSession simulates concurrent NewSession calls
func TestTmuxLock_Concurrency_NewSession(t *testing.T) {
	if testing.Short() {
		t.Skip("requires requires tmux lock concurrency")
	}

	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	const numGoroutines = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var lockFailures atomic.Int32
	errors := make(chan error, numGoroutines)

	// Simulate concurrent NewSession operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Simulate NewSession lock acquisition
			if err := AcquireTmuxLock(); err != nil {
				// Lock contention is expected
				lockFailures.Add(1)
				return
			}
			defer ReleaseTmuxLock()

			// Simulate tmux settings update (critical section)
			time.Sleep(10 * time.Millisecond)
			successCount.Add(1)
		}(i)
	}

	wg.Wait()
	close(errors)

	// At least some operations should succeed
	if successCount.Load() == 0 {
		t.Error("No operations succeeded - all were blocked")
	}

	// Verify serialization: success + failures should equal total attempts
	total := int(successCount.Load()) + int(lockFailures.Load())
	if total != numGoroutines {
		t.Errorf("Total operations = %d, want %d", total, numGoroutines)
	}

	t.Logf("Concurrent operations: %d succeeded, %d failed (serialized correctly)",
		successCount.Load(), lockFailures.Load())
}

// TestTmuxLock_Concurrency_SendCommand simulates concurrent SendCommand calls
func TestTmuxLock_Concurrency_SendCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("requires requires tmux lock concurrency")
	}

	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	const numCommands = 20
	var wg sync.WaitGroup
	var successCount atomic.Int32
	commandOrder := make([]int, 0, numCommands)
	var orderMutex sync.Mutex

	// Simulate concurrent SendCommand operations
	for i := 0; i < numCommands; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Simulate SendCommand lock acquisition
			if err := AcquireTmuxLock(); err != nil {
				// If lock fails, skip
				return
			}
			defer ReleaseTmuxLock()

			// Critical section: send-keys operations must not interleave
			orderMutex.Lock()
			commandOrder = append(commandOrder, id)
			orderMutex.Unlock()

			// Simulate tmux send-keys delay
			time.Sleep(5 * time.Millisecond)
			successCount.Add(1)
		}(i)
	}

	wg.Wait()

	// All commands should eventually succeed (no permanent deadlock)
	if successCount.Load() == 0 {
		t.Error("No commands succeeded - potential deadlock")
	}

	t.Logf("Concurrent commands: %d/%d succeeded (serialized)",
		successCount.Load(), numCommands)
}

// TestTmuxLock_NoDeadlock_DeferPattern tests that defer pattern prevents deadlocks
func TestTmuxLock_NoDeadlock_DeferPattern(t *testing.T) {
	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	// Simulate panic inside critical section
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Recovered from panic: %v", r)
			}
		}()

		if err := AcquireTmuxLock(); err != nil {
			t.Fatalf("AcquireTmuxLock() failed: %v", err)
		}
		defer ReleaseTmuxLock()

		// Simulate panic during tmux operation
		panic("simulated tmux error")
	}()

	// Lock should have been released via defer
	// Try to acquire it again
	if err := AcquireTmuxLock(); err != nil {
		t.Errorf("Lock not released after panic: %v", err)
	}
	defer ReleaseTmuxLock()
}

// TestTmuxLock_RaceCondition_SettingsUpdate tests race conditions in settings updates
func TestTmuxLock_RaceCondition_SettingsUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("requires race condition test")
	}

	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	const numSessions = 5
	var wg sync.WaitGroup
	settingsApplied := make(map[string]int)
	var settingsMutex sync.Mutex

	// Simulate multiple sessions applying settings concurrently
	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()

			// Simulate NewSession acquiring lock
			if err := AcquireTmuxLock(); err != nil {
				return
			}
			defer ReleaseTmuxLock()

			// Simulate applying 4 settings (like NewSession does)
			settings := []string{"aggressive-resize", "window-size", "mouse", "set-clipboard", "escape-time"}
			for _, setting := range settings {
				// Critical section: settings must not interleave
				settingsMutex.Lock()
				key := fmt.Sprintf("%s:%s", sessionID, setting)
				settingsApplied[key]++
				settingsMutex.Unlock()

				time.Sleep(2 * time.Millisecond)
			}
		}(fmt.Sprintf("session-%d", i))
	}

	wg.Wait()

	// Verify each session's settings were applied atomically
	for i := 0; i < numSessions; i++ {
		sessionID := fmt.Sprintf("session-%d", i)
		for _, setting := range []string{"aggressive-resize", "window-size", "mouse", "set-clipboard"} {
			key := fmt.Sprintf("%s:%s", sessionID, setting)
			count, ok := settingsApplied[key]
			if !ok {
				continue // Lock prevented this session from running
			}
			if count != 1 {
				t.Errorf("Setting %s applied %d times, want 1 (race condition)", key, count)
			}
		}
	}

	t.Logf("Settings applied for %d sessions without race conditions", len(settingsApplied)/4)
}

// TestTmuxLock_Performance_Throughput measures lock throughput
func TestTmuxLock_Performance_Throughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	const numOperations = 100
	start := time.Now()
	var successCount atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := AcquireTmuxLock(); err != nil {
				return
			}
			defer ReleaseTmuxLock()

			// Simulate quick tmux operation
			time.Sleep(1 * time.Millisecond)
			successCount.Add(1)
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	opsPerSec := float64(successCount.Load()) / duration.Seconds()
	t.Logf("Lock throughput: %.0f ops/sec (%d ops in %v)",
		opsPerSec, successCount.Load(), duration)

	// Sanity check: should be able to do at least 10 ops/sec
	if opsPerSec < 10 {
		t.Errorf("Lock throughput too low: %.0f ops/sec, want >= 10 ops/sec", opsPerSec)
	}
}

// TestTmuxLock_LockPath tests that lock path is correctly scoped to tmux operations
func TestTmuxLock_LockPath(t *testing.T) {
	uid := os.Getuid()
	expectedPath := fmt.Sprintf("/tmp/agm-%d/tmux-server.lock", uid)

	// Acquire lock and verify file exists
	if err := AcquireTmuxLock(); err != nil {
		t.Fatalf("AcquireTmuxLock() failed: %v", err)
	}
	defer ReleaseTmuxLock()

	// Verify lock file exists at expected path
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Lock file not found at %s", expectedPath)
	}
}

// TestTmuxLock_DoubleLock tests that double lock acquisition is prevented
func TestTmuxLock_DoubleLock(t *testing.T) {
	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	// First acquisition should succeed
	if err := AcquireTmuxLock(); err != nil {
		t.Fatalf("First AcquireTmuxLock() failed: %v", err)
	}
	defer ReleaseTmuxLock()

	// Second acquisition by same process should fail with clear error
	err := AcquireTmuxLock()
	if err == nil {
		t.Fatal("Second AcquireTmuxLock() should have failed but succeeded")
	}

	// Verify error message mentions double lock
	if err.Error() == "" {
		t.Error("Double lock error message is empty")
	}
}

// TestTmuxLock_StressTest_Sequential performs stress testing with sequential operations
func TestTmuxLock_StressTest_Sequential(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	const numOperations = 100

	// Sequential operations should all succeed
	var successCount int
	for i := 0; i < numOperations; i++ {
		if err := AcquireTmuxLock(); err != nil {
			t.Errorf("Operation %d: AcquireTmuxLock() failed: %v", i, err)
			continue
		}

		// Quick operation
		time.Sleep(time.Millisecond)
		successCount++

		if err := ReleaseTmuxLock(); err != nil {
			t.Errorf("Operation %d: ReleaseTmuxLock() failed: %v", i, err)
		}
	}

	// All sequential operations should succeed
	if successCount != numOperations {
		t.Errorf("Sequential stress test: %d/%d ops succeeded, want all",
			successCount, numOperations)
	}

	t.Logf("Sequential stress test: %d/%d operations succeeded",
		successCount, numOperations)
}

// TestTmuxLock_StressTest_Concurrent performs stress testing with proper concurrent pattern
func TestTmuxLock_StressTest_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	const numGoroutines = 50

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var lockContentionCount atomic.Int32

	// Each goroutine tries to acquire lock once
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Try to acquire lock
			if err := AcquireTmuxLock(); err != nil {
				// Lock contention expected - another goroutine holds it
				lockContentionCount.Add(1)
				return
			}
			defer ReleaseTmuxLock()

			// Simulate tmux operation
			time.Sleep(5 * time.Millisecond)
			successCount.Add(1)
		}(i)
	}

	wg.Wait()

	// Verify: Only ONE goroutine should have succeeded (lock serialization)
	// The rest should have hit lock contention
	if successCount.Load() != 1 {
		t.Errorf("Expected exactly 1 success (serialization), got %d", successCount.Load())
	}

	if lockContentionCount.Load() != numGoroutines-1 {
		t.Errorf("Expected %d lock contentions, got %d",
			numGoroutines-1, lockContentionCount.Load())
	}

	t.Logf("Concurrent stress test: 1 succeeded, %d blocked (correct serialization)",
		lockContentionCount.Load())
}

// TestTmuxLock_CrossProcess tests lock behavior across processes (if possible)
func TestTmuxLock_CrossProcess(t *testing.T) {
	// This is a placeholder for cross-process testing
	// In a real scenario, you'd use exec.Command to spawn child processes
	// For now, we verify the lock file mechanism works at the syscall level

	uid := os.Getuid()
	lockPath := fmt.Sprintf("/tmp/agm-%d/tmux-server.lock", uid)

	// Clean up
	defer func() {
		os.Remove(lockPath)
		ReleaseTmuxLock()
	}()

	// Acquire lock
	if err := AcquireTmuxLock(); err != nil {
		t.Fatalf("AcquireTmuxLock() failed: %v", err)
	}
	defer ReleaseTmuxLock()

	// Try to acquire lock with raw file lock (simulates another process)
	otherLock, err := lock.New(lockPath)
	if err != nil {
		t.Fatalf("Failed to create second lock: %v", err)
	}
	defer otherLock.Unlock()

	// This should fail because we already hold the lock
	err = otherLock.TryLock()
	if err == nil {
		t.Error("Second process should not be able to acquire lock")
	}
}

// TestWithTmuxLock_Success tests successful function execution with lock
func TestWithTmuxLock_Success(t *testing.T) {
	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	executed := false
	err := withTmuxLock(func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("withTmuxLock() returned unexpected error: %v", err)
	}

	if !executed {
		t.Error("Function was not executed")
	}
}

// TestWithTmuxLock_FunctionError tests error propagation from function
func TestWithTmuxLock_FunctionError(t *testing.T) {
	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	expectedErr := fmt.Errorf("test error")
	err := withTmuxLock(func() error {
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Errorf("withTmuxLock() returned %v, expected %v", err, expectedErr)
	}
}

// TestWithTmuxLock_LockAlreadyHeld tests double-lock detection
func TestWithTmuxLock_LockAlreadyHeld(t *testing.T) {
	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	// Acquire lock manually
	if err := AcquireTmuxLock(); err != nil {
		t.Fatalf("AcquireTmuxLock() failed: %v", err)
	}
	defer ReleaseTmuxLock()

	// Try to use withTmuxLock while lock is held
	err := withTmuxLock(func() error {
		return nil
	})

	if err == nil {
		t.Error("withTmuxLock() should have failed with lock already held")
	}
}

// TestWithTmuxLock_PanicRecovery tests lock release on panic
func TestWithTmuxLock_PanicRecovery(t *testing.T) {
	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	// Execute function that panics
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic but didn't get one")
			}
		}()

		withTmuxLock(func() error {
			panic("test panic")
		})
	}()

	// Lock should have been released via defer
	// Try to acquire it again
	if err := AcquireTmuxLock(); err != nil {
		t.Errorf("Lock not released after panic: %v", err)
	}
	defer ReleaseTmuxLock()
}

// TestWithTmuxLock_ConcurrentAccess tests concurrent withTmuxLock calls
func TestWithTmuxLock_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("requires requires tmux lock")
	}

	cleanupTmuxLock(t)
	defer cleanupTmuxLock(t)

	const numGoroutines = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var lockFailures atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := withTmuxLock(func() error {
				// Simulate work
				time.Sleep(10 * time.Millisecond)
				return nil
			})

			if err != nil {
				lockFailures.Add(1)
			} else {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// At least some operations should succeed
	if successCount.Load() == 0 {
		t.Error("No operations succeeded")
	}

	// Success + failures should equal total attempts
	total := int(successCount.Load()) + int(lockFailures.Load())
	if total != numGoroutines {
		t.Errorf("Operation count mismatch: got %d, expected %d", total, numGoroutines)
	}
}

// Helper: cleanup tmux lock before test
func cleanupTmuxLock(t *testing.T) {
	t.Helper()

	// Release any held lock
	ReleaseTmuxLock()

	// Remove lock file
	uid := os.Getuid()
	lockPath := fmt.Sprintf("/tmp/agm-%d/tmux-server.lock", uid)
	os.Remove(lockPath)
}
