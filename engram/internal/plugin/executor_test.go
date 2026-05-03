package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestExecutor_Timeout verifies that plugin execution respects timeout
func TestExecutor_Timeout(t *testing.T) {
	// Create temporary directory for test plugin
	tmpDir := t.TempDir()

	// Create test plugin directory
	pluginDir := filepath.Join(tmpDir, "test-timeout")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	// Create plugin manifest with sleep command
	manifest := `name: test-timeout
version: 1.0.0
description: Test timeout plugin
pattern: connector
permissions:
  commands:
    - /bin/sleep
commands:
  - name: sleep
    description: Sleep for a duration
    executable: /bin/sleep
    args: []
`
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Load plugin
	loader := NewLoader([]string{tmpDir}, []string{})
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(plugins) != 1 {
		t.Fatalf("Load() returned %d plugins, want 1", len(plugins))
	}

	plugin := plugins[0]

	// Test with short timeout
	t.Run("timeout_exceeded", func(t *testing.T) {
		// Create executor with 100ms timeout
		executor := NewExecutorWithTimeout(100 * time.Millisecond)

		// Try to execute sleep for 5 seconds (should timeout)
		start := time.Now()
		_, err := executor.Execute(context.Background(), plugin, "sleep", []string{"5"})
		elapsed := time.Since(start)

		// Should timeout
		if err == nil {
			t.Fatal("Execute() succeeded, want timeout error")
		}

		// Verify error message
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Execute() error = %q, want 'timeout' error", err.Error())
		}

		// Verify timeout occurred around 100ms (allow 200ms buffer for goroutine scheduling)
		if elapsed > 300*time.Millisecond {
			t.Errorf("Execute() took %v, want ~100ms timeout", elapsed)
		}
	})

	// Test with sufficient timeout
	t.Run("timeout_not_exceeded", func(t *testing.T) {
		// Create executor with 5s timeout
		executor := NewExecutorWithTimeout(5 * time.Second)

		// Execute sleep for 100ms (should succeed)
		_, err := executor.Execute(context.Background(), plugin, "sleep", []string{"0.1"})
		if err != nil {
			t.Fatalf("Execute() failed: %v", err)
		}
	})

	// Test with default timeout
	t.Run("default_timeout", func(t *testing.T) {
		executor := NewExecutor()

		// Verify default timeout is set
		if executor.timeout != DefaultExecutionTimeout {
			t.Errorf("NewExecutor() timeout = %v, want %v", executor.timeout, DefaultExecutionTimeout)
		}
	})
}

// TestExecutor_ContextCancellation verifies that parent context cancellation is respected
func TestExecutor_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	pluginDir := filepath.Join(tmpDir, "test-cancel")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	manifest := `name: test-cancel
version: 1.0.0
description: Test cancellation plugin
pattern: connector
permissions:
  commands:
    - /bin/sleep
commands:
  - name: sleep
    description: Sleep for a duration
    executable: /bin/sleep
    args: []
`
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	loader := NewLoader([]string{tmpDir}, []string{})
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	plugin := plugins[0]

	// Test parent context cancellation
	t.Run("parent_context_cancelled", func(t *testing.T) {
		executor := NewExecutorWithTimeout(10 * time.Second) // Long timeout

		// Create cancellable context
		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after 100ms
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		// Try to execute sleep for 5 seconds (should be cancelled)
		start := time.Now()
		_, err := executor.Execute(ctx, plugin, "sleep", []string{"5"})
		elapsed := time.Since(start)

		// Should be cancelled
		if err == nil {
			t.Fatal("Execute() succeeded, want cancellation error")
		}

		// Verify error message indicates cancellation
		if !strings.Contains(err.Error(), "cancel") {
			t.Errorf("Execute() error = %q, want 'cancel' error", err.Error())
		}

		// Verify cancellation occurred around 100ms
		if elapsed > 300*time.Millisecond {
			t.Errorf("Execute() took %v, want ~100ms for cancellation", elapsed)
		}
	})
}

// TestExecutor_TimeoutVsCompletion verifies timeout behavior with fast and slow commands
func TestExecutor_TimeoutVsCompletion(t *testing.T) {
	tmpDir := t.TempDir()

	pluginDir := filepath.Join(tmpDir, "test-timing")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	manifest := `name: test-timing
version: 1.0.0
description: Test timing plugin
pattern: connector
permissions:
  commands:
    - /bin/echo
    - /bin/sleep
commands:
  - name: fast
    description: Fast command
    executable: /bin/echo
    args:
      - "fast"
  - name: slow
    description: Slow command
    executable: /bin/sleep
    args:
      - "10"
`
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	loader := NewLoader([]string{tmpDir}, []string{})
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	plugin := plugins[0]
	executor := NewExecutorWithTimeout(500 * time.Millisecond)

	// Test fast command (completes before timeout)
	t.Run("fast_command_completes", func(t *testing.T) {
		output, err := executor.Execute(context.Background(), plugin, "fast", []string{})
		if err != nil {
			t.Fatalf("Execute() failed: %v", err)
		}

		outputStr := strings.TrimSpace(string(output))
		if outputStr != "fast" {
			t.Errorf("Execute() output = %q, want %q", outputStr, "fast")
		}
	})

	// Test slow command (times out)
	t.Run("slow_command_times_out", func(t *testing.T) {
		start := time.Now()
		_, err := executor.Execute(context.Background(), plugin, "slow", []string{})
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("Execute() succeeded for slow command, want timeout error")
		}

		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Execute() error = %q, want 'timeout' error", err.Error())
		}

		// Should timeout around 500ms
		if elapsed > 1*time.Second {
			t.Errorf("Execute() took %v, want ~500ms timeout", elapsed)
		}
	})
}

// TestExecutor_CustomTimeout verifies custom timeout configuration
func TestExecutor_CustomTimeout(t *testing.T) {
	testCases := []struct {
		name    string
		timeout time.Duration
	}{
		{"1ms", 1 * time.Millisecond},
		{"100ms", 100 * time.Millisecond},
		{"1s", 1 * time.Second},
		{"1m", 1 * time.Minute},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			executor := NewExecutorWithTimeout(tc.timeout)
			if executor.timeout != tc.timeout {
				t.Errorf("NewExecutorWithTimeout(%v) timeout = %v, want %v", tc.timeout, executor.timeout, tc.timeout)
			}
		})
	}
}

// TestExecutor_TimeoutErrorMessage verifies timeout error messages are descriptive
func TestExecutor_TimeoutErrorMessage(t *testing.T) {
	tmpDir := t.TempDir()

	pluginDir := filepath.Join(tmpDir, "my-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	manifest := `name: my-plugin
version: 1.0.0
description: Test error message plugin
pattern: connector
permissions:
  commands:
    - /bin/sleep
commands:
  - name: my-command
    description: My command
    executable: /bin/sleep
    args:
      - "10"
`
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	loader := NewLoader([]string{tmpDir}, []string{})
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	plugin := plugins[0]
	executor := NewExecutorWithTimeout(50 * time.Millisecond)

	_, err = executor.Execute(context.Background(), plugin, "my-command", []string{})
	if err == nil {
		t.Fatal("Execute() succeeded, want timeout error")
	}

	errMsg := err.Error()

	// Error message should contain timeout duration, plugin name, and command name
	if !strings.Contains(errMsg, "50ms") {
		t.Errorf("error message missing timeout duration: %q", errMsg)
	}
	if !strings.Contains(errMsg, "my-plugin") {
		t.Errorf("error message missing plugin name: %q", errMsg)
	}
	if !strings.Contains(errMsg, "my-command") {
		t.Errorf("error message missing command name: %q", errMsg)
	}
}
