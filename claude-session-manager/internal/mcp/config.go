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
// Priority: 1. AGM_MCP_SERVERS env var, 2. ~/.config/agm/mcp.yaml
func loadClientConfig() (*ClientConfig, error) {
	// Check environment variable first
	if envServers := os.Getenv("AGM_MCP_SERVERS"); envServers != "" {
		return parseEnvServers(envServers)
	}

	// Load from YAML file
	configPath := expandHomeDir("~/.config/agm/mcp.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &ClientConfig{MCPServers: []ServerConfig{}}, nil // Empty config
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
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
