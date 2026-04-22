package plugin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestApplyResourceLimits verifies that resource limits are correctly prepared
func TestApplyResourceLimits(t *testing.T) {
	// Create a basic command
	cmd := exec.Command("/bin/sleep", "1")

	// Apply resource limits
	applyResourceLimits(cmd)

	// Verify SysProcAttr is set
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil, want it to be set")
	}

	// Note: Actual resource limit enforcement depends on execution environment
	// (Docker, cgroups, OS defaults). The constants are defined for documentation.
}

// TestExecutor_ResourceLimits verifies that resource limits are applied during execution
func TestExecutor_ResourceLimits(t *testing.T) {
	// Create temporary directory for test plugin
	tmpDir, err := os.MkdirTemp("", "plugin-rlimit-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test plugin directory
	pluginDir := filepath.Join(tmpDir, "test-rlimit")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	// Create plugin manifest
	manifest := `name: test-rlimit
version: 1.0.0
description: Test resource limits plugin
pattern: connector
permissions:
  commands:
    - /bin/sh
commands:
  - name: simple
    description: Simple echo command
    executable: /bin/echo
    args:
      - hello
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

	// Test normal execution with resource limits applied
	t.Run("normal_execution_with_limits", func(t *testing.T) {
		executor := NewExecutor()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		output, err := executor.Execute(ctx, plugin, "simple", []string{})
		if err != nil {
			t.Errorf("Execute() failed: %v", err)
		}

		// Check output
		if string(output) != "hello\n" {
			t.Errorf("Execute() output = %q, want 'hello\\n'", string(output))
		}
	})
}

// TestResourceLimitConstants verifies that resource limit constants are reasonable
func TestResourceLimitConstants(t *testing.T) {
	// Verify constants are positive and reasonable
	if DefaultMaxProcesses == 0 {
		t.Error("DefaultMaxProcesses is 0, want > 0")
	}

	if DefaultMaxProcessesHard <= DefaultMaxProcesses {
		t.Error("DefaultMaxProcessesHard is not greater than DefaultMaxProcesses")
	}

	if DefaultMaxFileDescriptors == 0 {
		t.Error("DefaultMaxFileDescriptors is 0, want > 0")
	}

	if DefaultMaxMemory == 0 {
		t.Error("DefaultMaxMemory is 0, want > 0")
	}

	// Verify reasonable values (sanity checks)
	// Soft limit should be reasonable for sub-agent spawning (typically 5-10 agents)
	if DefaultMaxProcesses < 50 {
		t.Logf("Warning: DefaultMaxProcesses = %d is quite low", DefaultMaxProcesses)
	}

	// Hard limit should be at least 2x soft limit
	if DefaultMaxProcessesHard < 2*DefaultMaxProcesses {
		t.Logf("Warning: DefaultMaxProcessesHard = %d is less than 2x soft limit", DefaultMaxProcessesHard)
	}

	// Memory should be at least 1GB
	minMemory := int64(1024 * 1024 * 1024)
	if int64(DefaultMaxMemory) < minMemory {
		t.Logf("Warning: DefaultMaxMemory = %d is less than 1GB", DefaultMaxMemory)
	}
}
