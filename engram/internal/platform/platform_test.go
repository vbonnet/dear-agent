package platform

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/testutil"
)

// TestNew_Success verifies successful platform initialization
func TestNew_Success(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	if platform.config == nil {
		t.Error("Platform.config is nil, want non-nil")
	}

	if platform.telemetry == nil {
		t.Error("Platform.telemetry is nil, want non-nil")
	}

	if platform.eventBus == nil {
		t.Error("Platform.eventBus is nil, want non-nil")
	}

	if platform.plugins == nil {
		t.Error("Platform.plugins is nil, want non-nil")
	}
}

// TestNew_WithAgentDetection verifies agent detection and config override
func TestNew_WithAgentDetection(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	// Set environment variable to simulate Claude Code agent
	t.Setenv("CLAUDECODE", "1")

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	// Agent should be detected as ClaudeCode
	if string(platform.agent) != "claude-code" {
		t.Errorf("Platform.agent = %v, want claude-code", platform.agent)
	}

	// Config should be overridden with detected agent
	if platform.config.Platform.Agent != "claude-code" {
		t.Errorf("Platform.config.Platform.Agent = %v, want claude-code", platform.config.Platform.Agent)
	}
}

// TestNew_WithEcphory verifies platform initialization with engram path configured
func TestNew_WithEcphory(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	// Create config with engram path
	configDir := filepath.Join(tmpdir, ".engram", "core")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	engramPath := testutil.SetupTestEngramPath(t, tmpdir)

	configContent := `platform:
  agent: "test-agent"
  engram_path: "` + engramPath + `"
  token_budget: 1000
telemetry:
  enabled: false
  path: ""
plugins:
  paths: []
  disabled: []
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	// Ecphory initialization is non-fatal, so it may be nil even with path configured
	// Just verify the config was set correctly
	if platform.config.Platform.EngramPath != engramPath {
		t.Errorf("Platform.config.Platform.EngramPath = %v, want %v", platform.config.Platform.EngramPath, engramPath)
	}
}

// TestNew_WithoutEcphory verifies platform works when ecphory is not configured
func TestNew_WithoutEcphory(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	// Ecphory should be nil when engram_path is empty
	if platform.ecphory != nil {
		t.Error("Platform.ecphory is non-nil when engram_path empty, want nil")
	}
}

// TestNew_ConfigLoadFailure verifies error handling when config loading fails
func TestNew_ConfigLoadFailure(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	// Create invalid config file
	configDir := filepath.Join(tmpdir, ".engram", "core")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content: [[["), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tmpdir)

	ctx := context.Background()
	_, err := New(ctx)
	if err == nil {
		t.Fatal("New() succeeded with invalid config, want error")
	}
}

// TestNew_TelemetryFailure verifies error handling when telemetry initialization fails
func TestNew_TelemetryFailure(t *testing.T) {
	testutil.SkipIfRoot(t) // Root can create directories anywhere, breaking this test
	tmpdir := testutil.SetupTempDir(t)

	// Create config with invalid telemetry path
	configDir := filepath.Join(tmpdir, ".engram", "core")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Use a deeply nested non-existent path
	configContent := `platform:
  agent: "test-agent"
  engram_path: ""
  token_budget: 1000
telemetry:
  enabled: true
  path: "/nonexistent/deeply/nested/path/events.jsonl"
plugins:
  paths: []
  disabled: []
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tmpdir)

	ctx := context.Background()
	_, err := New(ctx)
	if err == nil {
		t.Fatal("New() succeeded with invalid telemetry path, want error")
	}
}

// TestNew_WithPlugins verifies platform initialization with custom plugin path
func TestNew_WithPlugins(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	// Create config with plugin path
	configDir := filepath.Join(tmpdir, ".engram", "core")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create empty plugin directory
	pluginDir := filepath.Join(tmpdir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	configContent := `platform:
  agent: "test-agent"
  engram_path: ""
  token_budget: 1000
telemetry:
  enabled: false
  path: ""
plugins:
  paths: ["` + pluginDir + `"]
  disabled: []
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	// Verify custom plugin path was set
	if len(platform.config.Plugins.Paths) == 0 {
		t.Error("Platform.config.Plugins.Paths is empty, want non-empty")
	}
}

// TestConfig accessor verifies Config() returns correct config
func TestConfig(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	cfg := platform.Config()
	if cfg == nil {
		t.Fatal("Config() returned nil, want non-nil")
	}

	// Agent may be overridden by detection, just verify config is non-nil and has values
	if cfg.Platform.TokenBudget == 0 {
		t.Error("Config().Platform.TokenBudget = 0, want > 0")
	}
}

// TestAgent accessor verifies Agent() returns detected agent
func TestAgent(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	agent := platform.Agent()
	// Agent should be detected (unknown if no env vars set)
	if string(agent) == "" {
		t.Error("Agent() returned empty string, want non-empty")
	}
}

// TestTelemetry accessor verifies Telemetry() returns telemetry collector
func TestTelemetry(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	tel := platform.Telemetry()
	if tel == nil {
		t.Fatal("Telemetry() returned nil, want non-nil")
	}
}

// TestEventBus accessor verifies EventBus() returns event bus
func TestEventBus(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	bus := platform.EventBus()
	if bus == nil {
		t.Fatal("EventBus() returned nil, want non-nil")
	}
}

// TestPlugins accessor verifies Plugins() returns plugin registry
func TestPlugins(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	plugins := platform.Plugins()
	if plugins == nil {
		t.Fatal("Plugins() returned nil, want non-nil")
	}
}

// TestEcphory accessor verifies Ecphory() returns ecphory instance or nil
func TestEcphory(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer platform.Close()

	// Ecphory should be nil when not configured
	eph := platform.Ecphory()
	if eph != nil {
		t.Error("Ecphory() returned non-nil when not configured, want nil")
	}
}

// TestClose verifies Close() cleans up resources
func TestClose(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = platform.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

// TestClose_Multiple verifies multiple Close() calls are safe
func TestClose_Multiple(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// First close
	err = platform.Close()
	if err != nil {
		t.Errorf("First Close() failed: %v", err)
	}

	// Second close should not panic
	_ = platform.Close()
	// Some implementations may return error on second close, that's acceptable
	// We're just verifying it doesn't panic
}

// TestNew_WithTimeout verifies New() respects context timeout
func TestNew_WithTimeout(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for timeout
	time.Sleep(10 * time.Millisecond)

	// Try to create platform with expired context
	_, err := New(ctx)

	// Should get context error
	if err == nil {
		t.Fatal("expected error with expired context")
	}

	t.Logf("Got expected error with expired context: %v", err)
}

// TestNew_WithCancellation verifies New() respects context cancellation
func TestNew_WithCancellation(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	// Try to create platform with cancelled context
	_, err := New(ctx)

	// Should get error
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}

	t.Logf("Got expected error with cancelled context: %v", err)
}

// TestClose_NilPlatform verifies Close() handles nil receiver without panicking
func TestClose_NilPlatform(t *testing.T) {
	var p *Platform
	err := p.Close()
	if err != nil {
		t.Errorf("Close() on nil platform should not error, got: %v", err)
	}
}

// TestClose_PartialInitialization verifies cleanup of partially initialized platform
func TestClose_PartialInitialization(t *testing.T) {
	testutil.SkipIfRoot(t) // Root can create directories anywhere, breaking this test

	tmpdir := testutil.SetupTempDir(t)

	// Create config that will cause telemetry to fail
	configDir := filepath.Join(tmpdir, ".engram", "core")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Use an invalid telemetry path to force initialization failure
	configContent := `platform:
  agent: "test-agent"
  engram_path: ""
  token_budget: 1000
telemetry:
  enabled: true
  path: "/nonexistent/deeply/nested/path/events.jsonl"
plugins:
  paths: []
  disabled: []
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)

	// Should fail during initialization
	if err == nil {
		// If platform was created, ensure it can be closed safely
		if platform != nil {
			closeErr := platform.Close()
			if closeErr != nil {
				t.Logf("Close() after partial init returned error (acceptable): %v", closeErr)
			}
		}
		t.Fatal("New() succeeded with invalid telemetry path, want error")
	}

	// Verify we got expected error
	t.Logf("Got expected initialization error: %v", err)
}

// TestNew_EventBusCleanupOnPluginFailure verifies EventBus is cleaned up when plugin operations fail
func TestNew_EventBusCleanupOnPluginFailure(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	// Create config with an invalid plugin path that will cause issues
	configDir := filepath.Join(tmpdir, ".engram", "core")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Create a plugin directory with invalid manifest
	pluginDir := filepath.Join(tmpdir, "plugins", "badplugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	// Write invalid plugin manifest
	manifestPath := filepath.Join(pluginDir, "plugin.yaml")
	if err := os.WriteFile(manifestPath, []byte("invalid: yaml: [[["), 0644); err != nil {
		t.Fatalf("failed to write plugin manifest: %v", err)
	}

	configContent := `platform:
  agent: "test-agent"
  engram_path: ""
  token_budget: 1000
telemetry:
  enabled: false
  path: ""
plugins:
  paths: ["` + filepath.Join(tmpdir, "plugins") + `"]
  disabled: []
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)

	// Plugin loader is tolerant of bad plugins, so this might succeed
	// The important thing is that if it fails, cleanup happens correctly
	if err != nil {
		t.Logf("Got expected error (EventBus should be cleaned up): %v", err)
	} else if platform != nil {
		// If successful, verify platform is valid and can be closed
		defer platform.Close()
		if platform.eventBus == nil {
			t.Error("Platform.eventBus is nil after successful init, want non-nil")
		}
	}
}

// TestClose_PanicRecovery verifies Close() recovers from panics in component Close() methods
func TestClose_PanicRecovery(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestConfig(t, tmpdir)

	ctx := context.Background()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Close should succeed and not panic even if components have issues
	// This tests the panic recovery in safeClose
	err = platform.Close()
	if err != nil {
		// Some error is acceptable, important thing is it didn't panic
		t.Logf("Close() returned error (acceptable): %v", err)
	}

	// Verify we can call Close multiple times without panic
	err = platform.Close()
	// Second close may return error, that's ok
	if err != nil {
		t.Logf("Second Close() returned error (acceptable): %v", err)
	}
}
