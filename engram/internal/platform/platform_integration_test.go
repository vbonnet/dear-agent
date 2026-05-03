package platform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// TestPlatform_WithTelemetry_Integration verifies platform initialization reports to telemetry
// This is an integration test covering platform + telemetry interaction
func TestPlatform_WithTelemetry_Integration(t *testing.T) {
	// Skip if no API key (ecphory initialization requires it)
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: ANTHROPIC_API_KEY not set")
	}

	// Create temporary directory for test config and telemetry
	tmpDir := t.TempDir()

	// Test 1: Platform reports OS detection
	t.Run("os_detection_reported", func(t *testing.T) {
		configPath, telemetryPath := createTestConfig(t, tmpDir, "os", "claude-code", "", 10000)
		withConfigEnv(t, configPath, func() {
			platform := newPlatformWithCleanup(t, context.Background())
			if _, err := os.Stat(telemetryPath); os.IsNotExist(err) {
				t.Fatal("Telemetry file was not created")
			}
			events := readTelemetryEvents(t, telemetryPath)
			assertHasConfigLoadedEvent(t, events)
			assertEventHasAgentData(t, events)
			_ = platform // satisfy linter
		})
	})

	// Test 2: Platform reports config load success/failure
	t.Run("config_load_reported", func(t *testing.T) {
		configPath, telemetryPath := createTestConfig(t, tmpDir, "valid", "cursor", "", 5000)
		withConfigEnv(t, configPath, func() {
			platform := newPlatformWithCleanup(t, context.Background())
			cfg := platform.Config()
			if cfg == nil {
				t.Fatal("Platform.Config() returned nil")
			}
			if cfg.Platform.TokenBudget != 5000 {
				t.Errorf("Config.Platform.TokenBudget = %d, want 5000", cfg.Platform.TokenBudget)
			}
			events := readTelemetryEvents(t, telemetryPath)
			if len(events) == 0 {
				t.Fatal("No telemetry events recorded for successful config load")
			}
		})
	})

	// Test 3: Platform path resolution logging
	t.Run("path_resolution_logged", func(t *testing.T) {
		engramDir := filepath.Join(tmpDir, "engrams")
		if err := os.MkdirAll(engramDir, 0755); err != nil {
			t.Fatalf("failed to create engram dir: %v", err)
		}
		configPath, telemetryPath := createTestConfig(t, tmpDir, "path", "windsurf", engramDir, 10000)
		withConfigEnv(t, configPath, func() {
			platform := newPlatformWithCleanup(t, context.Background())
			cfg := platform.Config()
			if cfg.Platform.EngramPath != engramDir {
				t.Errorf("Config.Platform.EngramPath = %q, want %q", cfg.Platform.EngramPath, engramDir)
			}
			events := readTelemetryEvents(t, telemetryPath)
			assertHasConfigLoadedEvent(t, events)
		})
	})
}

// createTestConfig creates a test config file and returns configPath and telemetryPath
func createTestConfig(t *testing.T, tmpDir, name, agent, engramPath string, tokenBudget int) (string, string) {
	t.Helper()
	telemetryPath := filepath.Join(tmpDir, "telemetry-"+name+".jsonl")
	configPath := filepath.Join(tmpDir, "config-"+name+".yaml")
	configContent := fmt.Sprintf(`platform:
  agent: %s
  engram_path: %s
  token_budget: %d
plugins:
  paths: []
  disabled: []
telemetry:
  enabled: true
  path: %s
`, agent, engramPath, tokenBudget, telemetryPath)
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return configPath, telemetryPath
}

// withConfigEnv runs fn with ENGRAM_CONFIG_PATH set, then restores original value
func withConfigEnv(t *testing.T, configPath string, fn func()) {
	t.Helper()
	oldConfigPath := os.Getenv("ENGRAM_CONFIG_PATH")
	t.Setenv("ENGRAM_CONFIG_PATH", configPath)
	defer func() {
		if oldConfigPath != "" {
			t.Setenv("ENGRAM_CONFIG_PATH", oldConfigPath)
		} else {
			os.Unsetenv("ENGRAM_CONFIG_PATH")
		}
	}()
	fn()
}

// newPlatformWithCleanup creates platform and registers cleanup
func newPlatformWithCleanup(t *testing.T, ctx context.Context) *Platform {
	t.Helper()
	platform, err := New(ctx)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { platform.Close() })
	return platform
}

// assertHasConfigLoadedEvent checks if events contain config_loaded event
func assertHasConfigLoadedEvent(t *testing.T, events []telemetry.Event) {
	t.Helper()
	for _, event := range events {
		if event.Type == telemetry.EventConfigLoaded {
			return
		}
	}
	t.Error("Platform did not emit config_loaded telemetry event")
}

// assertEventHasAgentData checks if config_loaded event has agent data
func assertEventHasAgentData(t *testing.T, events []telemetry.Event) {
	t.Helper()
	for _, event := range events {
		if event.Type == telemetry.EventConfigLoaded {
			if event.Agent == "" {
				t.Error("config_loaded event missing agent")
			}
			if event.Data == nil {
				t.Error("config_loaded event missing data")
			} else if _, ok := event.Data["agent"]; !ok {
				t.Error("config_loaded event data missing 'agent' field")
			}
			return
		}
	}
}

// readTelemetryEvents reads telemetry events from JSONL file
func readTelemetryEvents(t *testing.T, path string) []telemetry.Event {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open telemetry file: %v", err)
	}
	defer file.Close()

	var events []telemetry.Event
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event telemetry.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("failed to unmarshal event: %v", err)
		}

		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("failed to read telemetry file: %v", err)
	}

	return events
}
