package tmux

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCommandWithTimeout_Success(t *testing.T) {
	ctx := context.Background()
	timeout := 2 * time.Second

	cmd, cancel := CommandWithTimeout(ctx, timeout, "echo", "hello")
	defer cancel()

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("CommandWithTimeout() failed: %v", err)
	}

	if !strings.Contains(string(output), "hello") {
		t.Errorf("Expected output to contain 'hello', got: %s", output)
	}
}

func TestCommandWithTimeout_Timeout(t *testing.T) {
	ctx := context.Background()
	timeout := 100 * time.Millisecond

	// Use sleep command that exceeds timeout
	cmd, cancel := CommandWithTimeout(ctx, timeout, "sleep", "10")
	defer cancel()

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	// Should timeout quickly (not wait for 10s)
	if elapsed > 1*time.Second {
		t.Errorf("Command took too long: %v (expected ~100ms)", elapsed)
	}

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

func TestRunWithTimeout_Success(t *testing.T) {
	ctx := context.Background()
	timeout := 2 * time.Second

	output, err := RunWithTimeout(ctx, timeout, "echo", "test")
	if err != nil {
		t.Errorf("RunWithTimeout() failed: %v", err)
	}

	if !strings.Contains(string(output), "test") {
		t.Errorf("Expected output to contain 'test', got: %s", output)
	}
}

func TestRunWithTimeout_DeadlineExceeded(t *testing.T) {
	ctx := context.Background()
	timeout := 100 * time.Millisecond

	start := time.Now()
	_, err := RunWithTimeout(ctx, timeout, "sleep", "10")
	elapsed := time.Since(start)

	// Should timeout quickly
	if elapsed > 1*time.Second {
		t.Errorf("RunWithTimeout took too long: %v (expected ~100ms)", elapsed)
	}

	// Should return TimeoutError
	if err == nil {
		t.Fatal("Expected TimeoutError, got nil")
	}

	timeoutErr := &TimeoutError{}
	ok := errors.As(err, &timeoutErr)
	if !ok {
		t.Fatalf("Expected *TimeoutError, got %T", err)
	}

	if timeoutErr.Problem == "" {
		t.Error("TimeoutError.Problem is empty")
	}
	if timeoutErr.Recovery == "" {
		t.Error("TimeoutError.Recovery is empty")
	}
	if timeoutErr.Duration != timeout {
		t.Errorf("TimeoutError.Duration = %v, expected %v", timeoutErr.Duration, timeout)
	}
}

func TestTimeoutError_Format(t *testing.T) {
	err := &TimeoutError{
		Problem:  "Test problem",
		Recovery: "Test recovery",
		Duration: 5 * time.Second,
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("TimeoutError.Error() returned empty string")
	}

	// Verify both fields are in the error message
	if !strings.Contains(errStr, "Test problem") {
		t.Error("Error message missing Problem field")
	}
	if !strings.Contains(errStr, "Test recovery") {
		t.Error("Error message missing Recovery field")
	}
}

func TestSetTimeout_GetTimeout(t *testing.T) {
	originalTimeout := GetTimeout()
	defer SetTimeout(originalTimeout) // Restore after test

	newTimeout := 10 * time.Second
	SetTimeout(newTimeout)

	if GetTimeout() != newTimeout {
		t.Errorf("GetTimeout() = %v, expected %v", GetTimeout(), newTimeout)
	}
}

func TestRunWithTimeout_FastCommand(t *testing.T) {
	ctx := context.Background()
	timeout := 5 * time.Second

	// Fast command should complete without overhead
	start := time.Now()
	_, err := RunWithTimeout(ctx, timeout, "echo", "fast")
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("RunWithTimeout() failed: %v", err)
	}

	// Should complete very quickly (< 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Fast command took too long: %v (expected < 100ms)", elapsed)
	}
}

func TestCommandWithTimeout_CancelContext(t *testing.T) {
	ctx, cancelCtx := context.WithCancel(context.Background())
	timeout := 5 * time.Second

	cmd, cancel := CommandWithTimeout(ctx, timeout, "sleep", "10")
	defer cancel()

	// Start command in goroutine
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	// Cancel context immediately
	time.Sleep(10 * time.Millisecond)
	cancelCtx()

	// Wait for command to finish
	select {
	case err := <-done:
		// Command should have been killed
		if err == nil {
			t.Error("Expected error from canceled context, got nil")
		}
	case <-time.After(1 * time.Second):
		t.Error("Command did not exit after context cancellation")
	}
}
