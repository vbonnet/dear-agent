package cliframe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigLoader loads configuration from multiple sources with precedence
type ConfigLoader struct {
	envPrefix   string
	searchPaths []string
	defaults    map[string]interface{}
}

// NewConfigLoader creates a loader with environment prefix
func NewConfigLoader(envPrefix string) *ConfigLoader {
	return &ConfigLoader{
		envPrefix:   strings.ToUpper(envPrefix),
		searchPaths: []string{},
		defaults:    make(map[string]interface{}),
	}
}

// WithSearchPaths sets directories to search for config files
// Order matters: later paths have higher priority
func (l *ConfigLoader) WithSearchPaths(paths ...string) *ConfigLoader {
	l.searchPaths = paths
	return l
}

// WithDefaults sets default values
func (l *ConfigLoader) WithDefaults(defaults map[string]interface{}) *ConfigLoader {
	l.defaults = defaults
	return l
}

// Load reads configuration with 3-tier precedence:
// 1. Environment variables (highest priority)
// 2. Local config file (workspace-specific)
// 3. Global config file (user-wide)
// 4. Defaults (lowest priority)
func (l *ConfigLoader) Load() (*Config, error) {
	// Start with defaults
	values := make(map[string]interface{})
	for k, v := range l.defaults {
		values[k] = v
	}

	var source string

	// Load from search paths (in order, later overrides earlier)
	for _, path := range l.searchPaths {
		configPath := l.findConfigFile(path)
		if configPath != "" {
			fileValues, err := l.loadConfigFile(configPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load config from %s: %w", configPath, err)
			}
			// Merge file values
			for k, v := range fileValues {
				values[k] = v
			}
			source = configPath
		}
	}

	// Override with environment variables (highest priority)
	envValues := l.loadFromEnv()
	for k, v := range envValues {
		values[k] = v
		if source == "" {
			source = "environment"
		} else {
			source += " + environment"
		}
	}

	if source == "" {
		source = "defaults"
	}

	return &Config{
		values: values,
		source: source,
	}, nil
}

// LoadFrom loads configuration from specific file
func (l *ConfigLoader) LoadFrom(path string) (*Config, error) {
	values, err := l.loadConfigFile(path)
	if err != nil {
		return nil, err
	}

	return &Config{
		values: values,
		source: path,
	}, nil
}

// findConfigFile looks for config.yaml or config.json in directory
func (l *ConfigLoader) findConfigFile(dir string) string {
	// Expand home directory
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}

	// Try YAML first, then JSON
	for _, ext := range []string{".yaml", ".yml", ".json"} {
		path := filepath.Join(dir, "config"+ext)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// loadConfigFile loads and parses a config file
func (l *ConfigLoader) loadConfigFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var values map[string]interface{}

	// Parse YAML (works for both YAML and JSON)
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return values, nil
}

// loadFromEnv loads configuration from environment variables
func (l *ConfigLoader) loadFromEnv() map[string]interface{} {
	values := make(map[string]interface{})

	prefix := l.envPrefix + "_"

	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, prefix) {
			continue
		}

		// Split key=value
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		// Convert ENV_VAR_NAME to var_name
		key := strings.ToLower(strings.TrimPrefix(parts[0], prefix))

		// Handle nested keys: APP_DB__HOST -> db.host
		key = strings.ReplaceAll(key, "__", ".")

		values[key] = parts[1]
	}

	return values
}

// Config represents loaded configuration
type Config struct {
	values map[string]interface{}
	source string
}

// Get retrieves a value by key
func (c *Config) Get(key string) (interface{}, bool) {
	val, ok := c.values[key]
	return val, ok
}

// GetString retrieves a string value
func (c *Config) GetString(key string) string {
	val, ok := c.values[key]
	if !ok {
		return ""
	}

	if str, ok := val.(string); ok {
		return str
	}

	return fmt.Sprintf("%v", val)
}

// GetInt retrieves an integer value
func (c *Config) GetInt(key string) int {
	val, ok := c.values[key]
	if !ok {
		return 0
	}

	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		// Try to parse string as int
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

// GetBool retrieves a boolean value
func (c *Config) GetBool(key string) bool {
	val, ok := c.values[key]
	if !ok {
		return false
	}

	switch v := val.(type) {
	case bool:
		return v
	case string:
		// Parse common boolean strings
		lower := strings.ToLower(v)
		return lower == "true" || lower == "yes" || lower == "1"
	default:
		return false
	}
}

// GetDuration retrieves a duration value (parses "1h30m" format)
func (c *Config) GetDuration(key string) time.Duration {
	val, ok := c.values[key]
	if !ok {
		return 0
	}

	switch v := val.(type) {
	case string:
		dur, err := time.ParseDuration(v)
		if err != nil {
			return 0
		}
		return dur
	case int:
		// Assume seconds
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v) * time.Second
	default:
		return 0
	}
}

// GetStringSlice retrieves a string slice
func (c *Config) GetStringSlice(key string) []string {
	val, ok := c.values[key]
	if !ok {
		return nil
	}

	switch v := val.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, len(v))
		for i, item := range v {
			result[i] = fmt.Sprintf("%v", item)
		}
		return result
	case string:
		// Parse comma-separated list
		if v == "" {
			return nil
		}
		return strings.Split(v, ",")
	default:
		return nil
	}
}

// Set updates a configuration value (for overrides)
func (c *Config) Set(key string, value interface{}) {
	c.values[key] = value
}

// Source returns where the config was loaded from
func (c *Config) Source() string {
	return c.source
}

// AllKeys returns all configuration keys
func (c *Config) AllKeys() []string {
	keys := make([]string, 0, len(c.values))
	for k := range c.values {
		keys = append(keys, k)
	}
	return keys
}
