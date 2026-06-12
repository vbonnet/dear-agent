package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents AGM MCP server configuration
type Config struct {
	Enabled          bool      `yaml:"enabled"`
	Transport        string    `yaml:"transport"`
	Tools            []string  `yaml:"tools"`
	AutoRegister     bool      `yaml:"auto_register"`
	ClaudeConfigPath string    `yaml:"claude_config_path"`
	SessionsDir      string    `yaml:"sessions_dir"`
	EngramMCPURL     string    `yaml:"engram_mcp_url"` // kept for future HTTP transport; wayfinder tools now use WayfinderDir
	WayfinderDir     string    `yaml:"wayfinder_dir"`  // path to engram-research wf/ directory
	// Workspace is the Dolt workspace name to use for session storage.
	// When set, this value is applied as the WORKSPACE environment variable
	// at startup so that tools work correctly when the server is launched
	// from Claude Desktop (which does not inherit shell environment).
	// Example: "oss" for the open-source dear-agent workspace.
	Workspace string    `yaml:"workspace"`
	A2A       A2AConfig `yaml:"a2a"`
}

// A2AConfig configures the A2A HTTP endpoint.
type A2AConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Bind    string `yaml:"bind"`
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
		EngramMCPURL:     "http://localhost:8081", // Default Engram MCP server URL
		WayfinderDir:     expandHomeDir(defaultWayfinderDir),
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
	if yamlCfg.MCPServer.EngramMCPURL != "" {
		cfg.EngramMCPURL = yamlCfg.MCPServer.EngramMCPURL
	}
	if yamlCfg.MCPServer.WayfinderDir != "" {
		cfg.WayfinderDir = expandHomeDir(yamlCfg.MCPServer.WayfinderDir)
	}
	if yamlCfg.MCPServer.Workspace != "" {
		cfg.Workspace = yamlCfg.MCPServer.Workspace
	}

	// Use YAML boolean values
	cfg.Enabled = yamlCfg.MCPServer.Enabled
	cfg.AutoRegister = yamlCfg.MCPServer.AutoRegister

	// Merge A2A config
	cfg.A2A.Enabled = yamlCfg.MCPServer.A2A.Enabled
	if yamlCfg.MCPServer.A2A.Port != 0 {
		cfg.A2A.Port = yamlCfg.MCPServer.A2A.Port
	}
	if yamlCfg.MCPServer.A2A.Bind != "" {
		cfg.A2A.Bind = yamlCfg.MCPServer.A2A.Bind
	}

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

// registerWithClaudeCode idempotently registers the AGM MCP server in the
// Claude Code MCP config at claudeConfigPath. It merges an "agm" entry pointing
// at the current executable while preserving any servers already configured,
// and is a no-op when the entry already points at the same command.
//
// Two on-disk shapes are supported: the flat name->entry map documented for
// mcp_servers.json, and the nested {"mcpServers": {...}} shape used by
// ~/.claude.json / .mcp.json. The existing shape is preserved; a missing file
// is created in the flat shape.
func registerWithClaudeCode(claudeConfigPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(exePath); rerr == nil {
		exePath = resolved
	}

	// Load existing config, tolerating a missing or empty file.
	root := map[string]any{}
	switch data, rerr := os.ReadFile(claudeConfigPath); {
	case rerr == nil:
		if len(data) > 0 {
			if jerr := json.Unmarshal(data, &root); jerr != nil {
				return fmt.Errorf("parse %s: %w", claudeConfigPath, jerr)
			}
		}
	case !os.IsNotExist(rerr):
		return fmt.Errorf("read %s: %w", claudeConfigPath, rerr)
	}

	// Servers live under a nested "mcpServers" object when present, otherwise
	// at the top level (the flat mcp_servers.json shape).
	servers := root
	if existing, ok := root["mcpServers"]; ok {
		nested, ok := existing.(map[string]any)
		if !ok {
			return fmt.Errorf("unexpected mcpServers type in %s", claudeConfigPath)
		}
		servers = nested
	}

	// Update only the command, preserving any user-defined fields (args, env)
	// on an existing entry. Skip the write entirely when already current.
	if cur, ok := servers["agm"].(map[string]any); ok {
		if cmd, _ := cur["command"].(string); cmd == exePath {
			return nil
		}
		cur["command"] = exePath
	} else {
		servers["agm"] = map[string]any{"command": exePath}
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(claudeConfigPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp := claudeConfigPath + ".tmp"
	defer func() { _ = os.Remove(tmp) }() // best-effort cleanup if we return before the rename
	if err := os.WriteFile(tmp, append(out, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, claudeConfigPath); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
