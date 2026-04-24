package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPluginExecution verifies full plugin loading and execution pipeline
func TestPluginExecution(t *testing.T) {
	// Create temporary directory for test plugin
	tmpDir, err := os.MkdirTemp("", "plugin-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test plugin directory
	createTestPluginDir(t, tmpDir)

	// Test 1: Load plugin
	t.Run("load_plugin", func(t *testing.T) {
		plugin := loadPlugins(t, []string{tmpDir})
		assertPluginManifest(t, plugin, "test-connector", "connector", 2)
	})

	// Test 2: Execute plugin command (no args)
	t.Run("execute_hello", func(t *testing.T) {
		plugin := loadPlugins(t, []string{tmpDir})
		executor := NewExecutor()
		output := executeCommand(t, executor, plugin, "hello", []string{})

		expected := "Hello from test-connector!"
		if output != expected {
			t.Errorf("Execute() output = %q, want %q", output, expected)
		}
	})

	// Test 3: Execute plugin command (with runtime args)
	t.Run("execute_greet_with_args", func(t *testing.T) {
		plugin := loadPlugins(t, []string{tmpDir})
		executor := NewExecutor()
		output := executeCommand(t, executor, plugin, "greet", []string{"World!"})

		// Should combine manifest args ["Hello"] + runtime args ["World!"]
		expected := "Hello World!"
		if output != expected {
			t.Errorf("Execute() output = %q, want %q", output, expected)
		}
	})

	// Test 4: Execute nonexistent command
	t.Run("execute_missing_command", func(t *testing.T) {
		plugin := loadPlugins(t, []string{tmpDir})
		executor := NewExecutor()
		_, err := executor.Execute(context.Background(), plugin, "nonexistent", []string{})
		if err == nil {
			t.Fatal("Execute() succeeded with nonexistent command, want error")
		}

		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("Execute() error = %q, want 'not found' error", err.Error())
		}
	})
}

// TestPluginLoading_MultiplePlugins verifies loading multiple plugins from search path
func TestPluginLoading_MultiplePlugins(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-multi-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create 3 test plugins
	plugins := []struct {
		name     string
		pattern  string
		disabled bool
	}{
		{"plugin-a", "connector", false},
		{"plugin-b", "guidance", false},
		{"plugin-c", "tool", true}, // This one will be disabled
	}

	for _, p := range plugins {
		pluginDir := filepath.Join(tmpDir, p.name)
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			t.Fatalf("failed to create plugin dir %s: %v", p.name, err)
		}

		manifest := `name: ` + p.name + `
version: 1.0.0
description: Test ` + p.pattern + ` plugin
pattern: ` + p.pattern + `
`
		manifestPath := filepath.Join(pluginDir, "plugin.yaml")
		if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
			t.Fatalf("failed to write manifest for %s: %v", p.name, err)
		}
	}

	// Load without disabled list (should load all 3)
	t.Run("load_all", func(t *testing.T) {
		loader := NewLoader([]string{tmpDir}, []string{})
		loaded, err := loader.Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		if len(loaded) != 3 {
			t.Errorf("Load() returned %d plugins, want 3", len(loaded))
		}
	})

	// Load with disabled list (should load only 2)
	t.Run("load_with_disabled", func(t *testing.T) {
		loader := NewLoader([]string{tmpDir}, []string{"plugin-c"})
		loaded, err := loader.Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}

		if len(loaded) != 2 {
			t.Errorf("Load() returned %d plugins, want 2 (plugin-c disabled)", len(loaded))
		}

		// Verify plugin-c is not in results
		for _, p := range loaded {
			if p.Manifest.Name == "plugin-c" {
				t.Error("Load() returned disabled plugin-c")
			}
		}
	})
}

// TestPluginLoading_InvalidManifest verifies handling of invalid plugin manifests
func TestPluginLoading_InvalidManifest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-invalid-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create plugin with invalid YAML
	pluginDir := filepath.Join(tmpDir, "invalid-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	invalidManifest := `name: "unclosed quote
version: 1.0.0
`
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(invalidManifest), 0644); err != nil {
		t.Fatalf("failed to write invalid manifest: %v", err)
	}

	// Load should skip invalid plugin (not error)
	loader := NewLoader([]string{tmpDir}, []string{})
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Should return 0 plugins (invalid one skipped)
	if len(plugins) != 0 {
		t.Errorf("Load() returned %d plugins, want 0 (invalid skipped)", len(plugins))
	}
}

// TestPluginLoading_MissingManifest verifies handling of directories without plugin.yaml
func TestPluginLoading_MissingManifest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-missing-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create directory without plugin.yaml
	pluginDir := filepath.Join(tmpDir, "no-manifest")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	// Load should skip directory without manifest
	loader := NewLoader([]string{tmpDir}, []string{})
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if len(plugins) != 0 {
		t.Errorf("Load() returned %d plugins, want 0 (no manifest)", len(plugins))
	}
}

// TestPluginLoading_NonexistentPath verifies handling of nonexistent search paths
func TestPluginLoading_NonexistentPath(t *testing.T) {
	// Load from nonexistent path (should not error)
	loader := NewLoader([]string{"/nonexistent/path/to/plugins"}, []string{})
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed with nonexistent path: %v", err)
	}

	if len(plugins) != 0 {
		t.Errorf("Load() returned %d plugins from nonexistent path, want 0", len(plugins))
	}
}

// TestPluginExecution_InvalidPermissions verifies permission validation
func TestPluginExecution_InvalidPermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-perms-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	// Create plugin with invalid permission (root filesystem access)
	manifest := `name: test-plugin
version: 1.0.0
description: Plugin with invalid permissions
pattern: connector
permissions:
  filesystem:
    - /
  commands:
    - /bin/echo
commands:
  - name: test
    description: Test command
    executable: /bin/echo
    args:
      - "test"
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

	// Execution should fail due to invalid permission
	executor := NewExecutor()
	_, err = executor.Execute(context.Background(), plugins[0], "test", []string{})
	if err == nil {
		t.Fatal("Execute() succeeded with invalid permissions, want error")
	}

	if !strings.Contains(err.Error(), "invalid permissions") {
		t.Errorf("Execute() error = %q, want 'invalid permissions' error", err.Error())
	}
}

// Test helper functions for TestPluginExecution

// createTestPluginDir creates a plugin directory with a test manifest
func createTestPluginDir(t *testing.T, tmpDir string) string {
	t.Helper()

	pluginDir := filepath.Join(tmpDir, "test-connector")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	manifest := `name: test-connector
version: 1.0.0
description: Test connector for integration testing
pattern: connector
permissions:
  filesystem:
    - read:/tmp
  commands:
    - /bin/echo
commands:
  - name: hello
    description: Say hello
    executable: /bin/echo
    args:
      - "Hello from test-connector!"
  - name: greet
    description: Greet someone
    executable: /bin/echo
    args:
      - "Hello"
`
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	return pluginDir
}

// loadPlugins loads plugins from search paths and returns first plugin
func loadPlugins(t *testing.T, searchPaths []string) *Plugin {
	t.Helper()
	loader := NewLoader(searchPaths, []string{})
	plugins, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("Load() returned %d plugins, want 1", len(plugins))
	}
	return plugins[0]
}

// assertPluginManifest verifies plugin manifest fields
func assertPluginManifest(t *testing.T, plugin *Plugin, name, pattern string, cmdCount int) {
	t.Helper()
	if plugin.Manifest.Name != name {
		t.Errorf("Plugin.Name = %q, want %q", plugin.Manifest.Name, name)
	}
	if plugin.Manifest.Pattern != pattern {
		t.Errorf("Plugin.Pattern = %q, want %q", plugin.Manifest.Pattern, pattern)
	}
	if len(plugin.Manifest.Commands) != cmdCount {
		t.Errorf("len(Plugin.Commands) = %d, want %d", len(plugin.Manifest.Commands), cmdCount)
	}
}

// executeCommand executes a plugin command and returns trimmed output
func executeCommand(t *testing.T, executor *Executor, plugin *Plugin, cmd string, args []string) string {
	t.Helper()
	output, err := executor.Execute(context.Background(), plugin, cmd, args)
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}
	return strings.TrimSpace(string(output))
}
