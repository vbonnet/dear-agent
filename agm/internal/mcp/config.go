package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClientConfig represents MCP client configuration
type ClientConfig struct {
	MCPServers []ServerConfig `yaml:"mcp_servers"`
}

// ServerConfig represents a single MCP server
type ServerConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Type string `yaml:"type"` // "mcp"
}

// loadClientConfig loads MCP client configuration
// Priority (highest to lowest): Session > User > Team > Global (env var)
// Session config is loaded separately via loadSessionConfig()
func loadClientConfig() (*ClientConfig, error) {
	// Check environment variable first (lowest priority of global configs)
	var globalServers []ServerConfig
	if envServers := os.Getenv("AGM_MCP_SERVERS"); envServers != "" {
		envCfg, err := parseEnvServers(envServers)
		if err != nil {
			return nil, err
		}
		globalServers = envCfg.MCPServers
	}

	// Load team config (if team membership exists)
	teamCfg, err := loadTeamConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load team config: %w", err)
	}
	if teamCfg != nil {
		// Team config overrides global env config
		globalServers = mergeServerConfigs(globalServers, teamCfg.MCPServers)
	}

	// Load user config (overrides team and env)
	userCfg, err := loadUserConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load user config: %w", err)
	}
	if userCfg != nil {
		globalServers = mergeServerConfigs(globalServers, userCfg.MCPServers)
	}

	return &ClientConfig{MCPServers: globalServers}, nil
}

// loadUserConfig loads user-level MCP configuration from ~/.config/agm/mcp.yaml
func loadUserConfig() (*ClientConfig, error) {
	configPath := expandHomeDir("~/.config/agm/mcp.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, nil // No user config (not an error)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read user config: %w", err)
	}

	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	return &cfg, nil
}

// loadSessionConfig loads session-specific MCP configuration from <project>/.agm/mcp.yaml
func loadSessionConfig(projectPath string) (*ClientConfig, error) {
	// Try .agm/mcp.yaml first
	configPath := filepath.Join(projectPath, ".agm", "mcp.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Try .config/claude-code/mcp.json as fallback
		jsonPath := filepath.Join(projectPath, ".config", "claude-code", "mcp.json")
		if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
			return nil, nil // No session config (not an error)
		}
		// Note: JSON format support can be added later if needed
		return nil, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session config: %w", err)
	}

	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse session config: %w", err)
	}

	return &cfg, nil
}

// mergeServerConfigs merges two sets of server configs
// Later configs override earlier ones (by name)
func mergeServerConfigs(base, override []ServerConfig) []ServerConfig {
	// Create map of base configs
	serverMap := make(map[string]ServerConfig)
	for _, server := range base {
		serverMap[server.Name] = server
	}

	// Override with new configs
	for _, server := range override {
		serverMap[server.Name] = server
	}

	// Convert back to slice
	result := make([]ServerConfig, 0, len(serverMap))
	for _, server := range serverMap {
		result = append(result, server)
	}

	return result
}

// LoadConfigWithHierarchy loads MCP configuration with full hierarchy
// Priority: Session > User > Team > Global (env var)
func LoadConfigWithHierarchy(projectPath string) (*ClientConfig, error) {
	// Load global config (env + team + user)
	globalCfg, err := loadClientConfig()
	if err != nil {
		return nil, err
	}

	// Load session config if project path provided
	if projectPath != "" {
		sessionCfg, err := loadSessionConfig(projectPath)
		if err != nil {
			return nil, err
		}
		if sessionCfg != nil {
			// Session config overrides global
			globalCfg.MCPServers = mergeServerConfigs(globalCfg.MCPServers, sessionCfg.MCPServers)
		}
	}

	return globalCfg, nil
}

// parseEnvServers parses AGM_MCP_SERVERS environment variable
// Format: "name1=url1,name2=url2"
func parseEnvServers(envValue string) (*ClientConfig, error) {
	cfg := &ClientConfig{MCPServers: []ServerConfig{}}

	pairs := strings.Split(envValue, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid AGM_MCP_SERVERS format: %s (expected name=url)", pair)
		}

		name := strings.TrimSpace(parts[0])
		url := strings.TrimSpace(parts[1])

		cfg.MCPServers = append(cfg.MCPServers, ServerConfig{
			Name: name,
			URL:  url,
			Type: "mcp",
		})
	}

	return cfg, nil
}

// GetServerURL looks up MCP server URL by name
func (cfg *ClientConfig) GetServerURL(name string) (string, bool) {
	for _, server := range cfg.MCPServers {
		if server.Name == name {
			return server.URL, true
		}
	}
	return "", false
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
