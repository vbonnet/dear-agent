package workspace

import (
	"fmt"
	"os"
	"strings"
)

// SettingsResolver resolves settings through the 7-level cascade.
type SettingsResolver struct {
	registry           *Registry
	workspace          *Workspace
	componentOverrides map[string]interface{}
	componentGlobal    map[string]interface{}
	cliFlags           map[string]interface{}
}

// NewSettingsResolver creates a new settings resolver.
func NewSettingsResolver(registry *Registry, workspace *Workspace) *SettingsResolver {
	return &SettingsResolver{
		registry:           registry,
		workspace:          workspace,
		componentOverrides: make(map[string]interface{}),
		componentGlobal:    make(map[string]interface{}),
		cliFlags:           make(map[string]interface{}),
	}
}

// SetCLIFlags sets CLI flag values (Level 7 - highest priority).
func (r *SettingsResolver) SetCLIFlags(flags map[string]interface{}) {
	r.cliFlags = flags
}

// SetComponentGlobal sets component global config (Level 4).
func (r *SettingsResolver) SetComponentGlobal(config map[string]interface{}) {
	r.componentGlobal = config
}

// SetComponentOverrides sets component workspace overrides (Level 5).
func (r *SettingsResolver) SetComponentOverrides(overrides map[string]interface{}) {
	r.componentOverrides = overrides
}

// ResolveSetting resolves a setting through the 7-level cascade.
//
// Priority (highest to lowest):
// Level 7: CLI flags
// Level 6: Environment variables
// Level 5: Component workspace override
// Level 4: Component global config
// Level 3: Registry workspace settings
// Level 2: Registry global defaults
// Level 1: Hardcoded defaults
func (r *SettingsResolver) ResolveSetting(name string, hardcodedDefault interface{}) interface{} {
	// Level 7: CLI flags
	if val, ok := r.cliFlags[name]; ok {
		return val
	}

	// Level 6: Environment variables
	envKey := strings.ToUpper(name)
	if !strings.HasPrefix(envKey, "WORKSPACE_") {
		envKey = "WORKSPACE_" + envKey
	}
	if val := os.Getenv(envKey); val != "" {
		return val
	}

	// Also try without WORKSPACE_ prefix
	if val := os.Getenv(strings.ToUpper(name)); val != "" {
		return val
	}

	// Level 5: Component workspace override
	if val, ok := r.componentOverrides[name]; ok {
		return val
	}

	// Level 4: Component global config
	if val, ok := r.componentGlobal[name]; ok {
		return val
	}

	// Level 3: Registry workspace settings
	if r.workspace != nil && r.workspace.Settings != nil {
		if val, ok := r.workspace.Settings[name]; ok {
			return val
		}
	}

	// Level 2: Registry global defaults
	if r.registry != nil && r.registry.DefaultSettings != nil {
		if val, ok := r.registry.DefaultSettings[name]; ok {
			return val
		}
	}

	// Level 1: Hardcoded default
	return hardcodedDefault
}

// GetCascade returns the full cascade for a setting (for debugging).
//nolint:gocyclo // reason: linear settings cascade with many layers
func (r *SettingsResolver) GetCascade(name string, hardcodedDefault interface{}) []CascadeLevel {
	levels := []CascadeLevel{}

	// Level 7: CLI flags
	if val, ok := r.cliFlags[name]; ok {
		levels = append(levels, CascadeLevel{
			Level:  7,
			Source: "cli",
			Value:  val,
			Active: true,
		})
	} else {
		levels = append(levels, CascadeLevel{
			Level:  7,
			Source: "cli",
			Value:  nil,
			Active: false,
		})
	}

	// Level 6: Environment variables
	envKey := "WORKSPACE_" + strings.ToUpper(name)
	if val := os.Getenv(envKey); val != "" {
		levels = append(levels, CascadeLevel{
			Level:  6,
			Source: "env",
			Value:  val,
			Active: len(levels) == 1 || !levels[len(levels)-1].Active,
		})
	} else {
		levels = append(levels, CascadeLevel{
			Level:  6,
			Source: "env",
			Value:  nil,
			Active: false,
		})
	}

	// Level 5: Component workspace override
	if val, ok := r.componentOverrides[name]; ok {
		isActive := true
		for _, l := range levels {
			if l.Active {
				isActive = false
				break
			}
		}
		levels = append(levels, CascadeLevel{
			Level:  5,
			Source: "component_override",
			Value:  val,
			Active: isActive,
		})
	} else {
		levels = append(levels, CascadeLevel{
			Level:  5,
			Source: "component_override",
			Value:  nil,
			Active: false,
		})
	}

	// Level 4: Component global config
	if val, ok := r.componentGlobal[name]; ok {
		isActive := true
		for _, l := range levels {
			if l.Active {
				isActive = false
				break
			}
		}
		levels = append(levels, CascadeLevel{
			Level:  4,
			Source: "component_global",
			Value:  val,
			Active: isActive,
		})
	} else {
		levels = append(levels, CascadeLevel{
			Level:  4,
			Source: "component_global",
			Value:  nil,
			Active: false,
		})
	}

	// Level 3: Registry workspace settings
	var wsVal interface{}
	wsHasVal := false
	if r.workspace != nil && r.workspace.Settings != nil {
		wsVal, wsHasVal = r.workspace.Settings[name]
	}
	if wsHasVal {
		isActive := true
		for _, l := range levels {
			if l.Active {
				isActive = false
				break
			}
		}
		levels = append(levels, CascadeLevel{
			Level:  3,
			Source: "workspace",
			Value:  wsVal,
			Active: isActive,
		})
	} else {
		levels = append(levels, CascadeLevel{
			Level:  3,
			Source: "workspace",
			Value:  nil,
			Active: false,
		})
	}

	// Level 2: Registry global defaults
	var regVal interface{}
	regHasVal := false
	if r.registry != nil && r.registry.DefaultSettings != nil {
		regVal, regHasVal = r.registry.DefaultSettings[name]
	}
	if regHasVal {
		isActive := true
		for _, l := range levels {
			if l.Active {
				isActive = false
				break
			}
		}
		levels = append(levels, CascadeLevel{
			Level:  2,
			Source: "registry_default",
			Value:  regVal,
			Active: isActive,
		})
	} else {
		levels = append(levels, CascadeLevel{
			Level:  2,
			Source: "registry_default",
			Value:  nil,
			Active: false,
		})
	}

	// Level 1: Hardcoded default
	isActive := true
	for _, l := range levels {
		if l.Active {
			isActive = false
			break
		}
	}
	levels = append(levels, CascadeLevel{
		Level:  1,
		Source: "hardcoded",
		Value:  hardcodedDefault,
		Active: isActive,
	})

	return levels
}

// CascadeLevel represents one level in the settings cascade.
type CascadeLevel struct {
	Level  int
	Source string
	Value  interface{}
	Active bool // Whether this level is the active/winning value
}

// CoreProtocolSettings returns the 6 core protocol settings.
func CoreProtocolSettings() map[string]SettingDefinition {
	return map[string]SettingDefinition{
		"root": {
			Name:        "root",
			EnvVar:      "WORKSPACE_ROOT",
			Description: "Absolute path to workspace root directory",
			Type:        "string",
			Required:    true,
			Default:     nil,
		},
		"db_url": {
			Name:        "db_url",
			EnvVar:      "WORKSPACE_DB_URL",
			Description: "Dolt database connection URL",
			Type:        "string",
			Required:    false,
			Default:     "",
		},
		"log_level": {
			Name:        "log_level",
			EnvVar:      "WORKSPACE_LOG_LEVEL",
			Description: "Logging verbosity (debug|info|warn|error)",
			Type:        "enum",
			Required:    true,
			Default:     "info",
			Choices:     []string{"debug", "info", "warn", "error"},
		},
		"log_format": {
			Name:        "log_format",
			EnvVar:      "WORKSPACE_LOG_FORMAT",
			Description: "Log output format (text|json|structured)",
			Type:        "enum",
			Required:    false,
			Default:     "text",
			Choices:     []string{"text", "json", "structured"},
		},
		"output_dir": {
			Name:        "output_dir",
			EnvVar:      "WORKSPACE_OUTPUT_DIR",
			Description: "Directory for component output files",
			Type:        "string",
			Required:    false,
			Default:     "{workspace_root}/output",
		},
		"cache_dir": {
			Name:        "cache_dir",
			EnvVar:      "WORKSPACE_CACHE_DIR",
			Description: "Directory for component cache data",
			Type:        "string",
			Required:    false,
			Default:     "{workspace_root}/.cache",
		},
	}
}

// SettingDefinition describes a setting's properties.
type SettingDefinition struct {
	Name        string
	EnvVar      string
	Description string
	Type        string // "string", "integer", "boolean", "enum"
	Required    bool
	Default     interface{}
	Choices     []string // For enum type
	Min         *int     // For integer type
	Max         *int     // For integer type
}

// ValidateSetting validates a setting value against its definition.
//nolint:gocyclo // reason: linear validator with one branch per setting type
func ValidateSetting(def SettingDefinition, value interface{}) error {
	// Check required
	if def.Required && value == nil {
		return fmt.Errorf("setting '%s' is required", def.Name)
	}

	if value == nil {
		return nil
	}

	// Type-specific validation
	switch def.Type {
	case "enum":
		strVal, ok := value.(string)
		if !ok {
			return fmt.Errorf("setting '%s' must be a string", def.Name)
		}
		valid := false
		for _, choice := range def.Choices {
			if strVal == choice {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("setting '%s' must be one of: %v", def.Name, def.Choices)
		}

	case "integer":
		intVal, ok := value.(int)
		if !ok {
			return fmt.Errorf("setting '%s' must be an integer", def.Name)
		}
		if def.Min != nil && intVal < *def.Min {
			return fmt.Errorf("setting '%s' must be >= %d", def.Name, *def.Min)
		}
		if def.Max != nil && intVal > *def.Max {
			return fmt.Errorf("setting '%s' must be <= %d", def.Name, *def.Max)
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("setting '%s' must be a boolean", def.Name)
		}
	}

	return nil
}
