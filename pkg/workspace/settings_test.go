package workspace

import (
	"os"
	"testing"
)

func TestSettingsResolver_ResolveSetting(t *testing.T) {
	registry := &Registry{
		Version: 1,
		DefaultSettings: map[string]interface{}{
			"log_level": "info",
		},
		Workspaces: []Workspace{},
	}

	workspace := &Workspace{
		Name: "test",
		Settings: map[string]interface{}{
			"log_level": "debug",
		},
	}

	resolver := NewSettingsResolver(registry, workspace)

	tests := []struct {
		name             string
		settingName      string
		envVar           string
		envValue         string
		cliFlags         map[string]interface{}
		hardcodedDefault interface{}
		expected         interface{}
	}{
		{
			name:             "use hardcoded default",
			settingName:      "unknown_setting",
			hardcodedDefault: "default",
			expected:         "default",
		},
		{
			name:             "use registry default",
			settingName:      "log_level",
			hardcodedDefault: "error",
			expected:         "debug", // workspace overrides registry
		},
		{
			name:        "use CLI flag",
			settingName: "log_level",
			cliFlags: map[string]interface{}{
				"log_level": "trace",
			},
			hardcodedDefault: "error",
			expected:         "trace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.envVar, tt.envValue)
				defer os.Unsetenv(tt.envVar)
			}

			if tt.cliFlags != nil {
				resolver.SetCLIFlags(tt.cliFlags)
			}

			result := resolver.ResolveSetting(tt.settingName, tt.hardcodedDefault)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSettingsResolver_GetCascade(t *testing.T) {
	registry := &Registry{
		Version: 1,
		DefaultSettings: map[string]interface{}{
			"log_level": "info",
		},
		Workspaces: []Workspace{},
	}

	workspace := &Workspace{
		Name: "test",
		Settings: map[string]interface{}{
			"log_level": "debug",
		},
	}

	resolver := NewSettingsResolver(registry, workspace)

	// Get cascade
	cascade := resolver.GetCascade("log_level", "error")

	// Verify cascade has 7 levels
	if len(cascade) != 7 {
		t.Errorf("Expected 7 cascade levels, got %d", len(cascade))
	}

	// Verify level numbers
	for i, level := range cascade {
		expectedLevel := 7 - i
		if level.Level != expectedLevel {
			t.Errorf("Level %d: expected level number %d, got %d", i, expectedLevel, level.Level)
		}
	}

	// Verify workspace level (3) is active with value "debug"
	workspaceLevel := cascade[4] // Level 3 is at index 4
	if workspaceLevel.Level != 3 {
		t.Errorf("Expected level 3, got %d", workspaceLevel.Level)
	}
	if !workspaceLevel.Active {
		t.Error("Expected workspace level to be active")
	}
	if workspaceLevel.Value != "debug" {
		t.Errorf("Expected workspace value 'debug', got %v", workspaceLevel.Value)
	}
}

func TestCoreProtocolSettings(t *testing.T) {
	settings := CoreProtocolSettings()

	// Check required settings exist
	requiredSettings := []string{"root", "log_level", "log_format", "output_dir", "cache_dir"}
	for _, name := range requiredSettings {
		if _, exists := settings[name]; !exists {
			t.Errorf("Missing required setting: %s", name)
		}
	}

	// Check log_level is enum type with correct choices
	logLevel := settings["log_level"]
	if logLevel.Type != "enum" {
		t.Errorf("Expected log_level type 'enum', got '%s'", logLevel.Type)
	}

	expectedChoices := []string{"debug", "info", "warn", "error"}
	if len(logLevel.Choices) != len(expectedChoices) {
		t.Errorf("Expected %d log_level choices, got %d", len(expectedChoices), len(logLevel.Choices))
	}
}

func TestValidateSetting(t *testing.T) {
	tests := []struct {
		name      string
		def       SettingDefinition
		value     interface{}
		expectErr bool
	}{
		{
			name: "valid enum",
			def: SettingDefinition{
				Name:    "log_level",
				Type:    "enum",
				Choices: []string{"debug", "info", "warn", "error"},
			},
			value:     "debug",
			expectErr: false,
		},
		{
			name: "invalid enum",
			def: SettingDefinition{
				Name:    "log_level",
				Type:    "enum",
				Choices: []string{"debug", "info", "warn", "error"},
			},
			value:     "invalid",
			expectErr: true,
		},
		{
			name: "required setting missing",
			def: SettingDefinition{
				Name:     "required_setting",
				Type:     "string",
				Required: true,
			},
			value:     nil,
			expectErr: true,
		},
		{
			name: "integer in range",
			def: SettingDefinition{
				Name: "count",
				Type: "integer",
				Min:  intPtr(1),
				Max:  intPtr(100),
			},
			value:     50,
			expectErr: false,
		},
		{
			name: "integer out of range",
			def: SettingDefinition{
				Name: "count",
				Type: "integer",
				Min:  intPtr(1),
				Max:  intPtr(100),
			},
			value:     150,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSetting(tt.def, tt.value)
			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}

// Helper function
func intPtr(i int) *int {
	return &i
}
