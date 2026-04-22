package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TeamConfig represents team-level MCP configuration
type TeamConfig struct {
	Team       TeamMetadata   `yaml:"team"`
	MCPServers []ServerConfig `yaml:"mcp_servers"`
}

// TeamMetadata contains team ownership and identification
type TeamMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Owner       string `yaml:"owner,omitempty"`
}

// loadTeamConfig loads team MCP configuration based on team membership
// Priority for team detection:
// 1. AGM_TEAM environment variable
// 2. ~/.config/agm/team file
// Returns nil if no team membership detected
func loadTeamConfig() (*TeamConfig, error) {
	teamName := detectTeamMembership()
	if teamName == "" {
		return nil, nil // No team membership detected
	}

	teamConfigPath := getTeamConfigPath(teamName)
	if _, err := os.Stat(teamConfigPath); os.IsNotExist(err) {
		return nil, nil // Team config doesn't exist (not an error)
	}

	data, err := os.ReadFile(teamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read team config: %w", err)
	}

	var cfg TeamConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse team config: %w", err)
	}

	// Validate team name matches config
	if cfg.Team.Name != teamName {
		return nil, fmt.Errorf("team config mismatch: expected %s, got %s", teamName, cfg.Team.Name)
	}

	return &cfg, nil
}

// detectTeamMembership determines which team the user belongs to
// Priority:
// 1. AGM_TEAM environment variable
// 2. ~/.config/agm/team file
// 3. Git repository ownership (future enhancement)
func detectTeamMembership() string {
	// Check environment variable first
	if teamName := os.Getenv("AGM_TEAM"); teamName != "" {
		return strings.TrimSpace(teamName)
	}

	// Check team membership file
	teamFilePath := expandHomeDir("~/.config/agm/team")
	if data, err := os.ReadFile(teamFilePath); err == nil {
		return strings.TrimSpace(string(data))
	}

	// No team membership detected
	return ""
}

// getTeamConfigPath returns the path to a team's MCP configuration
func getTeamConfigPath(teamName string) string {
	return expandHomeDir(fmt.Sprintf("~/.config/agm/teams/%s/mcp.yaml", teamName))
}

// SetTeamMembership sets the user's team membership
func SetTeamMembership(teamName string) error {
	teamFilePath := expandHomeDir("~/.config/agm/team")
	teamDir := filepath.Dir(teamFilePath)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(teamDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write team name
	if err := os.WriteFile(teamFilePath, []byte(teamName+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write team membership: %w", err)
	}

	return nil
}

// GetTeamMembership returns the current team membership, if any
func GetTeamMembership() string {
	return detectTeamMembership()
}

// ListAvailableTeams lists all available team configurations
func ListAvailableTeams() ([]string, error) {
	teamsDir := expandHomeDir("~/.config/agm/teams")

	if _, err := os.Stat(teamsDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(teamsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read teams directory: %w", err)
	}

	teams := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if team has mcp.yaml config
			configPath := filepath.Join(teamsDir, entry.Name(), "mcp.yaml")
			if _, err := os.Stat(configPath); err == nil {
				teams = append(teams, entry.Name())
			}
		}
	}

	return teams, nil
}

// GetTeamInfo loads team metadata without server configs
func GetTeamInfo(teamName string) (*TeamMetadata, error) {
	teamConfigPath := getTeamConfigPath(teamName)

	if _, err := os.Stat(teamConfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("team config not found: %s", teamName)
	}

	data, err := os.ReadFile(teamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read team config: %w", err)
	}

	var cfg TeamConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse team config: %w", err)
	}

	return &cfg.Team, nil
}

// CreateTeamConfig creates a new team configuration
func CreateTeamConfig(teamName, description, owner string, servers []ServerConfig) error {
	teamConfigPath := getTeamConfigPath(teamName)
	teamDir := filepath.Dir(teamConfigPath)

	// Create team directory
	if err := os.MkdirAll(teamDir, 0755); err != nil {
		return fmt.Errorf("failed to create team directory: %w", err)
	}

	// Create team config
	cfg := TeamConfig{
		Team: TeamMetadata{
			Name:        teamName,
			Description: description,
			Owner:       owner,
		},
		MCPServers: servers,
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal team config: %w", err)
	}

	if err := os.WriteFile(teamConfigPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write team config: %w", err)
	}

	return nil
}
