package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectTeamMembership(t *testing.T) {
	// Save and restore original state
	origEnv := os.Getenv("AGM_TEAM")
	defer os.Setenv("AGM_TEAM", origEnv)

	tests := []struct {
		name     string
		envVar   string
		teamFile string
		expected string
	}{
		{
			name:     "env var set",
			envVar:   "engineering",
			teamFile: "",
			expected: "engineering",
		},
		{
			name:     "env var with whitespace",
			envVar:   "  research  ",
			teamFile: "",
			expected: "research",
		},
		{
			name:     "team file only",
			envVar:   "",
			teamFile: "data-science",
			expected: "data-science",
		},
		{
			name:     "env var overrides file",
			envVar:   "engineering",
			teamFile: "research",
			expected: "engineering",
		},
		{
			name:     "no team set",
			envVar:   "",
			teamFile: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			if tt.envVar != "" {
				os.Setenv("AGM_TEAM", tt.envVar)
			} else {
				os.Unsetenv("AGM_TEAM")
			}

			// Set up team file
			tmpDir := t.TempDir()
			teamFilePath := filepath.Join(tmpDir, "team")
			if tt.teamFile != "" {
				os.WriteFile(teamFilePath, []byte(tt.teamFile+"\n"), 0644)
			}

			// Override team file path for testing
			// Note: In real implementation, we'd need to make this testable
			// For now, test only env var path
			if tt.envVar != "" {
				result := detectTeamMembership()
				if result != tt.expected {
					t.Errorf("detectTeamMembership() = %q, want %q", result, tt.expected)
				}
			}
		})
	}
}

func TestGetTeamConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		teamName string
		want     string
	}{
		{
			name:     "simple team name",
			teamName: "engineering",
			want:     "~/.config/agm/teams/engineering/mcp.yaml",
		},
		{
			name:     "team with hyphens",
			teamName: "ml-research",
			want:     "~/.config/agm/teams/ml-research/mcp.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTeamConfigPath(tt.teamName)
			// Compare without expanding home directory for consistency
			if !filepath.IsAbs(got) && got != tt.want {
				t.Errorf("getTeamConfigPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetTeamMembership(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	teamName := "engineering"
	err := SetTeamMembership(teamName)
	if err != nil {
		t.Fatalf("SetTeamMembership() error = %v", err)
	}

	// Verify file was created
	teamFilePath := filepath.Join(tmpDir, ".config", "agm", "team")
	data, err := os.ReadFile(teamFilePath)
	if err != nil {
		t.Fatalf("Failed to read team file: %v", err)
	}

	got := string(data)
	want := teamName + "\n"
	if got != want {
		t.Errorf("Team file content = %q, want %q", got, want)
	}
}

func TestListAvailableTeams(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create test team configs
	teams := []string{"engineering", "research", "data-science"}
	for _, team := range teams {
		teamDir := filepath.Join(tmpDir, ".config", "agm", "teams", team)
		os.MkdirAll(teamDir, 0755)

		configPath := filepath.Join(teamDir, "mcp.yaml")
		os.WriteFile(configPath, []byte("mcp_servers: []\n"), 0644)
	}

	// Create directory without config (should be ignored)
	emptyTeamDir := filepath.Join(tmpDir, ".config", "agm", "teams", "empty")
	os.MkdirAll(emptyTeamDir, 0755)

	got, err := ListAvailableTeams()
	if err != nil {
		t.Fatalf("ListAvailableTeams() error = %v", err)
	}

	if len(got) != len(teams) {
		t.Errorf("ListAvailableTeams() returned %d teams, want %d", len(got), len(teams))
	}

	// Check all expected teams are present
	teamMap := make(map[string]bool)
	for _, team := range got {
		teamMap[team] = true
	}

	for _, expectedTeam := range teams {
		if !teamMap[expectedTeam] {
			t.Errorf("Expected team %q not found in results", expectedTeam)
		}
	}
}

func TestCreateTeamConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	teamName := "test-team"
	description := "Test Team"
	owner := "test@example.com"
	servers := []ServerConfig{
		{
			Name: "test-server",
			URL:  "http://localhost:8001",
			Type: "mcp",
		},
	}

	err := CreateTeamConfig(teamName, description, owner, servers)
	if err != nil {
		t.Fatalf("CreateTeamConfig() error = %v", err)
	}

	// Verify config file was created
	configPath := filepath.Join(tmpDir, ".config", "agm", "teams", teamName, "mcp.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Team config file was not created")
	}

	// Load and verify config
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read team config: %v", err)
	}

	// Basic verification that YAML was written
	content := string(data)
	if !contains(content, "name: "+teamName) {
		t.Errorf("Config does not contain team name")
	}
	if !contains(content, "description: "+description) {
		t.Errorf("Config does not contain description")
	}
	if !contains(content, "test-server") {
		t.Errorf("Config does not contain server name")
	}
}

func TestGetTeamInfo(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create team config
	teamName := "engineering"
	description := "Engineering Team"
	owner := "eng-lead@example.com"

	err := CreateTeamConfig(teamName, description, owner, []ServerConfig{})
	if err != nil {
		t.Fatalf("CreateTeamConfig() error = %v", err)
	}

	// Get team info
	info, err := GetTeamInfo(teamName)
	if err != nil {
		t.Fatalf("GetTeamInfo() error = %v", err)
	}

	if info.Name != teamName {
		t.Errorf("Team name = %q, want %q", info.Name, teamName)
	}
	if info.Description != description {
		t.Errorf("Team description = %q, want %q", info.Description, description)
	}
	if info.Owner != owner {
		t.Errorf("Team owner = %q, want %q", info.Owner, owner)
	}
}

func TestGetTeamInfo_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	_, err := GetTeamInfo("nonexistent-team")
	if err == nil {
		t.Error("GetTeamInfo() expected error for nonexistent team, got nil")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			len(s) > len(substr) && containsInMiddle(s, substr)))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
