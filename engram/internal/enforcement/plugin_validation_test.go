package enforcement

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/config"
	"github.com/vbonnet/dear-agent/engram/internal/identity"
	"github.com/vbonnet/dear-agent/engram/internal/plugin"
)

// Mock plugin registry for testing
type mockPluginRegistry struct {
	plugins []plugin.PluginInfo
}

func (m *mockPluginRegistry) ListPlugins() []plugin.PluginInfo {
	return m.plugins
}

// TestValidator_PluginValidation_AllInstalled tests successful plugin validation
func TestValidator_PluginValidation_AllInstalled(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Plugins: []config.PluginRequirement{
			{Name: "acme-integration", VersionMin: "1.0.0"},
			{Name: "github-connector", VersionMin: "2.0.0"},
		},
	}

	mockRegistry := &mockPluginRegistry{
		plugins: []plugin.PluginInfo{
			{Name: "acme-integration", Version: "1.5.0"},
			{Name: "github-connector", Version: "2.1.0"},
			{Name: "other-plugin", Version: "1.0.0"},
		},
	}

	validator := NewValidator(cfg, &identity.Identity{
		Email:  "user@acme.com",
		Domain: "@acme.com",
	})
	validator.SetPluginRegistry(mockRegistry)

	err := validator.Validate(context.Background())
	if err != nil {
		t.Errorf("Validate() with all required plugins installed should succeed, got error: %v", err)
	}
}

// TestValidator_PluginValidation_MissingPlugin tests missing plugin error
func TestValidator_PluginValidation_MissingPlugin(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required:       false, // Don't fail on identity
			AllowedDomains: []string{},
		},
		Plugins: []config.PluginRequirement{
			{Name: "acme-integration", VersionMin: "1.0.0"},
			{Name: "missing-plugin", VersionMin: "2.0.0"},
		},
	}

	mockRegistry := &mockPluginRegistry{
		plugins: []plugin.PluginInfo{
			{Name: "acme-integration", Version: "1.5.0"},
		},
	}

	validator := NewValidator(cfg, &identity.Identity{})
	validator.SetPluginRegistry(mockRegistry)

	err := validator.Validate(context.Background())
	if err == nil {
		t.Error("Validate() should fail when required plugin is missing")
	}

	// Error message should mention missing plugin
	errMsg := err.Error()
	if !contains(errMsg, "missing-plugin") {
		t.Errorf("Expected error message containing 'missing-plugin', got %q", errMsg)
	}
}

// TestValidator_PluginValidation_VersionMismatch tests version requirement
func TestValidator_PluginValidation_VersionMismatch(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required:       false,
			AllowedDomains: []string{},
		},
		Plugins: []config.PluginRequirement{
			{Name: "acme-integration", VersionMin: "2.0.0"},
		},
	}

	mockRegistry := &mockPluginRegistry{
		plugins: []plugin.PluginInfo{
			{Name: "acme-integration", Version: "1.5.0"}, // Too old
		},
	}

	validator := NewValidator(cfg, &identity.Identity{})
	validator.SetPluginRegistry(mockRegistry)

	err := validator.Validate(context.Background())
	if err == nil {
		t.Error("Validate() should fail when plugin version is too old")
	}

	// Error message should mention version mismatch
	errMsg := err.Error()
	if !contains(errMsg, "version mismatch") && !contains(errMsg, "acme-integration") {
		t.Errorf("Expected error message about version mismatch, got %q", errMsg)
	}
}

// TestValidator_PluginValidation_NoRegistry tests behavior without registry set
func TestValidator_PluginValidation_NoRegistry(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required:       false,
			AllowedDomains: []string{},
		},
		Plugins: []config.PluginRequirement{
			{Name: "acme-integration", VersionMin: "1.0.0"},
		},
	}

	validator := NewValidator(cfg, &identity.Identity{})
	// Don't set plugin registry

	// Should not fail - registry not set means phase 1 validation
	err := validator.Validate(context.Background())
	if err != nil {
		t.Errorf("Validate() without registry should skip plugin validation, got error: %v", err)
	}
}

// TestValidator_PluginValidation_NoRequirements tests no plugin requirements
func TestValidator_PluginValidation_NoRequirements(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required:       false,
			AllowedDomains: []string{},
		},
		Plugins: []config.PluginRequirement{}, // No requirements
	}

	mockRegistry := &mockPluginRegistry{
		plugins: []plugin.PluginInfo{},
	}

	validator := NewValidator(cfg, &identity.Identity{})
	validator.SetPluginRegistry(mockRegistry)

	err := validator.Validate(context.Background())
	if err != nil {
		t.Errorf("Validate() with no plugin requirements should succeed, got error: %v", err)
	}
}

// TestValidator_PluginValidation_MultipleIssues tests multiple missing plugins
func TestValidator_PluginValidation_MultipleIssues(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required:       false,
			AllowedDomains: []string{},
		},
		Plugins: []config.PluginRequirement{
			{Name: "plugin-a", VersionMin: "1.0.0"},
			{Name: "plugin-b", VersionMin: "2.0.0"},
			{Name: "plugin-c", VersionMin: "3.0.0"},
		},
	}

	mockRegistry := &mockPluginRegistry{
		plugins: []plugin.PluginInfo{
			{Name: "plugin-a", Version: "0.5.0"}, // Version too old
		},
	}

	validator := NewValidator(cfg, &identity.Identity{})
	validator.SetPluginRegistry(mockRegistry)

	err := validator.Validate(context.Background())
	if err == nil {
		t.Error("Validate() should fail when multiple plugins are missing or outdated")
	}

	errMsg := err.Error()
	// Should mention both missing plugins
	if !contains(errMsg, "plugin-b") || !contains(errMsg, "plugin-c") {
		t.Errorf("Error should mention missing plugins, got: %q", errMsg)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
