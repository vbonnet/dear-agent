package errors

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectUserType_ConfigFileExists(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	configDir := filepath.Join(tmpDir, ".engram")
	os.MkdirAll(configDir, 0755)
	configPath := filepath.Join(configDir, "config.yml")
	os.WriteFile(configPath, []byte("telemetry:\n  enabled: true\n"), 0644)

	userType := DetectUserType(context.Background())

	if userType != UserTypeTechnical {
		t.Errorf("Expected UserTypeTechnical, got %v", userType)
	}
}

func TestDetectUserType_NoConfigFile(t *testing.T) {
	// Use non-existent home directory
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", filepath.Join(tmpDir, "nonexistent"))
	defer os.Setenv("HOME", oldHome)

	userType := DetectUserType(context.Background())

	if userType != UserTypeSimple {
		t.Errorf("Expected UserTypeSimple, got %v", userType)
	}
}

func TestDetectUserType_CLIFlags(t *testing.T) {
	ctx := context.WithValue(context.Background(), "cli_flags", map[string]string{"verbose": "true"})

	userType := DetectUserType(ctx)

	if userType != UserTypeTechnical {
		t.Errorf("Expected UserTypeTechnical with CLI flags, got %v", userType)
	}
}

func TestPluginLoadingFailed_TechnicalMessage(t *testing.T) {
	ctx := context.WithValue(context.Background(), "cli_flags", map[string]string{"verbose": "true"})

	err := PluginLoadingFailed(ctx, []string{"research", "personas"}, "Plugin manifest not found")

	rendered := err.Render()

	if !strings.Contains(rendered, "[Plugin Loading]") {
		t.Errorf("Expected category in technical message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "Plugin manifest not found") {
		t.Errorf("Expected technical reason in message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "engram telemetry health") {
		t.Errorf("Expected debug command in technical message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "https://docs.engram.ai/troubleshooting/plugins") {
		t.Errorf("Expected learn more URL in technical message, got: %s", rendered)
	}
}

func TestPluginLoadingFailed_SimpleMessage(t *testing.T) {
	// Use default context (no config file, no CLI flags)
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", filepath.Join(tmpDir, "nonexistent"))
	defer os.Setenv("HOME", oldHome)

	ctx := context.Background()

	err := PluginLoadingFailed(ctx, []string{"research", "personas"}, "Plugin manifest not found")

	rendered := err.Render()

	if !strings.Contains(rendered, "Some plugins didn't load") {
		t.Errorf("Expected plain language in simple message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "What you can do:") {
		t.Errorf("Expected actionable steps in simple message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "Need help? Contact your team's Engram administrator") {
		t.Errorf("Expected help contact in simple message, got: %s", rendered)
	}

	// Should NOT contain technical details
	if strings.Contains(rendered, "plugin.yaml") {
		t.Errorf("Simple message should not contain technical file paths, got: %s", rendered)
	}

	if strings.Contains(rendered, "engram telemetry health") {
		t.Errorf("Simple message should not contain debug commands, got: %s", rendered)
	}
}

func TestVersionCompatibilityWarning_TechnicalMessage(t *testing.T) {
	ctx := context.WithValue(context.Background(), "cli_flags", map[string]string{"verbose": "true"})

	err := VersionCompatibilityWarning(ctx, "research", "0.1.0", "0.2.0")

	rendered := err.Render()

	if !strings.Contains(rendered, "[Version Compatibility]") {
		t.Errorf("Expected category in technical message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "deprecated") {
		t.Errorf("Expected 'deprecated' in technical message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "engram plugin update research") {
		t.Errorf("Expected debug command in technical message, got: %s", rendered)
	}
}

func TestVersionCompatibilityWarning_SimpleMessage(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", filepath.Join(tmpDir, "nonexistent"))
	defer os.Setenv("HOME", oldHome)

	ctx := context.Background()

	err := VersionCompatibilityWarning(ctx, "research", "0.1.0", "0.2.0")

	rendered := err.Render()

	if !strings.Contains(rendered, "needs updating") {
		t.Errorf("Expected plain language in simple message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "0.1.0") && !strings.Contains(rendered, "0.2.0") {
		t.Errorf("Expected version numbers in simple message, got: %s", rendered)
	}
}

func TestEcphoryCoverageWarning_HighUtilization(t *testing.T) {
	ctx := context.WithValue(context.Background(), "cli_flags", map[string]string{"verbose": "true"})

	err := EcphoryCoverageWarning(ctx, 85.5, 85500)

	rendered := err.Render()

	if !strings.Contains(rendered, "85.5%") {
		t.Errorf("Expected utilization percentage in message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "85500") {
		t.Errorf("Expected token count in technical message, got: %s", rendered)
	}
}

func TestStorageNearLimit_Warning(t *testing.T) {
	ctx := context.WithValue(context.Background(), "cli_flags", map[string]string{"verbose": "true"})

	// 85 MB / 100 MB
	err := StorageNearLimit(ctx, 85*1024*1024, 100*1024*1024)

	rendered := err.Render()

	if !strings.Contains(rendered, "85.0%") {
		t.Errorf("Expected 85%% utilization in message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "85/100 MB") {
		t.Errorf("Expected storage sizes in technical message, got: %s", rendered)
	}
}

func TestTelemetryDisabled_Message(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", filepath.Join(tmpDir, "nonexistent"))
	defer os.Setenv("HOME", oldHome)

	ctx := context.Background()

	err := TelemetryDisabled(ctx)

	rendered := err.Render()

	if !strings.Contains(rendered, "turned off") {
		t.Errorf("Expected plain language in simple message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "All data is stored locally") {
		t.Errorf("Expected privacy reassurance in message, got: %s", rendered)
	}
}

func TestParseError_LineNumber(t *testing.T) {
	ctx := context.WithValue(context.Background(), "cli_flags", map[string]string{"verbose": "true"})

	err := ParseError(ctx, "~/.engram/telemetry/events.jsonl", 42, "invalid JSON syntax")

	rendered := err.Render()

	if !strings.Contains(rendered, "line 42") {
		t.Errorf("Expected line number in message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "invalid JSON syntax") {
		t.Errorf("Expected parse reason in technical message, got: %s", rendered)
	}

	if !strings.Contains(rendered, "tail -n +40") {
		t.Errorf("Expected debug command with line context, got: %s", rendered)
	}
}

func TestSimplifyRecommendation_RemovesTechnicalTerms(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Verify plugin.yaml configuration",
			expected: "Check that plugins are properly configured",
		},
		{
			input:    "Check if plugins are enabled: [research, personas]",
			expected: "Make sure the following plugins are enabled: [research, personas]",
		},
		{
			input:    "Review ~/.engram/config.yml settings",
			expected: "Review config.yml settings",
		},
		{
			input:    "Update plugins to latest version",
			expected: "Update your plugins",
		},
	}

	for _, tt := range tests {
		result := simplifyRecommendation(tt.input)
		if result != tt.expected {
			t.Errorf("simplifyRecommendation(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTelemetryError_ContextFields(t *testing.T) {
	ctx := context.WithValue(context.Background(), "cli_flags", map[string]string{"verbose": "true"})

	err := PluginLoadingFailed(ctx, []string{"research"}, "Test reason")

	if err.Category != "Plugin Loading" {
		t.Errorf("Expected category 'Plugin Loading', got: %s", err.Category)
	}

	if err.DebugCommand != "engram telemetry health" {
		t.Errorf("Expected debug command, got: %s", err.DebugCommand)
	}

	if err.LearnMoreURL != "https://docs.engram.ai/troubleshooting/plugins" {
		t.Errorf("Expected learn more URL, got: %s", err.LearnMoreURL)
	}

	if len(err.Recommendations) != 3 {
		t.Errorf("Expected 3 recommendations, got: %d", len(err.Recommendations))
	}

	missingPlugins, ok := err.Context["missing_plugins"].([]string)
	if !ok || len(missingPlugins) != 1 || missingPlugins[0] != "research" {
		t.Errorf("Expected context['missing_plugins'] = ['research'], got: %v", err.Context["missing_plugins"])
	}
}
