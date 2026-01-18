package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents AGM MCP server configuration
type Config struct {
	Enabled          bool     `yaml:"enabled"`
	Transport        string   `yaml:"transport"`
	Tools            []string `yaml:"tools"`
	AutoRegister     bool     `yaml:"auto_register"`
	ClaudeConfigPath string   `yaml:"claude_config_path"`
	SessionsDir      string   `yaml:"sessions_dir"`
}

// loadConfig loads configuration from YAML file with smart defaults
func loadConfig(configPath string) (*Config, error) {
	// Default configuration
	cfg := &Config{
		Enabled:          true,
		Transport:        "stdio",
		Tools:            []string{"agm_list_sessions", "agm_search_sessions", "agm_get_session_metadata"},
		AutoRegister:     true,
		ClaudeConfigPath: expandHomeDir("~/.config/claude/mcp_servers.json"),
		SessionsDir:      detectSessionsDir(),
	}

	// Expand config path
	configPath = expandHomeDir(configPath)

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Use defaults if config doesn't exist
		return cfg, nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Parse YAML
	var yamlCfg struct {
		MCPServer Config `yaml:"mcp_server"`
	}

	if err := yaml.Unmarshal(data, &yamlCfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Merge with defaults (YAML overrides)
	if yamlCfg.MCPServer.Transport != "" {
		cfg.Transport = yamlCfg.MCPServer.Transport
	}
	if len(yamlCfg.MCPServer.Tools) > 0 {
		cfg.Tools = yamlCfg.MCPServer.Tools
	}
	if yamlCfg.MCPServer.ClaudeConfigPath != "" {
		cfg.ClaudeConfigPath = expandHomeDir(yamlCfg.MCPServer.ClaudeConfigPath)
	}
	if yamlCfg.MCPServer.SessionsDir != "" {
		cfg.SessionsDir = expandHomeDir(yamlCfg.MCPServer.SessionsDir)
	}

	// Use YAML boolean values
	cfg.Enabled = yamlCfg.MCPServer.Enabled
	cfg.AutoRegister = yamlCfg.MCPServer.AutoRegister

	return cfg, nil
}

// detectSessionsDir auto-detects AGM sessions directory
func detectSessionsDir() string {
	// Check environment variable first
	if sessionsDir := os.Getenv("AGM_SESSIONS_DIR"); sessionsDir != "" {
		return expandHomeDir(sessionsDir)
	}

	// Default to ~/.config/agm/sessions
	return expandHomeDir("~/.config/agm/sessions")
}

// expandHomeDir expands ~ to user's home directory
func expandHomeDir(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// registerWithClaudeCode registers the MCP server with Claude Code
func registerWithClaudeCode(claudeConfigPath string) error {
	// TODO: Implement registration (write to mcp_servers.json)
	// For V1, this is a placeholder - manual registration via Claude Code settings
	return fmt.Errorf("auto-registration not yet implemented - register manually via Claude Code settings")
}
