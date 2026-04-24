package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeServerConfigs(t *testing.T) {
	tests := []struct {
		name     string
		base     []ServerConfig
		override []ServerConfig
		want     map[string]ServerConfig
	}{
		{
			name: "no overlap",
			base: []ServerConfig{
				{Name: "server1", URL: "http://localhost:8001", Type: "mcp"},
			},
			override: []ServerConfig{
				{Name: "server2", URL: "http://localhost:8002", Type: "mcp"},
			},
			want: map[string]ServerConfig{
				"server1": {Name: "server1", URL: "http://localhost:8001", Type: "mcp"},
				"server2": {Name: "server2", URL: "http://localhost:8002", Type: "mcp"},
			},
		},
		{
			name: "override existing",
			base: []ServerConfig{
				{Name: "github", URL: "http://team-github.com", Type: "mcp"},
			},
			override: []ServerConfig{
				{Name: "github", URL: "http://localhost:8002", Type: "mcp"},
			},
			want: map[string]ServerConfig{
				"github": {Name: "github", URL: "http://localhost:8002", Type: "mcp"},
			},
		},
		{
			name: "mixed",
			base: []ServerConfig{
				{Name: "server1", URL: "http://localhost:8001", Type: "mcp"},
				{Name: "server2", URL: "http://localhost:8002", Type: "mcp"},
			},
			override: []ServerConfig{
				{Name: "server2", URL: "http://override:8002", Type: "mcp"},
				{Name: "server3", URL: "http://localhost:8003", Type: "mcp"},
			},
			want: map[string]ServerConfig{
				"server1": {Name: "server1", URL: "http://localhost:8001", Type: "mcp"},
				"server2": {Name: "server2", URL: "http://override:8002", Type: "mcp"},
				"server3": {Name: "server3", URL: "http://localhost:8003", Type: "mcp"},
			},
		},
		{
			name: "empty base",
			base: []ServerConfig{},
			override: []ServerConfig{
				{Name: "server1", URL: "http://localhost:8001", Type: "mcp"},
			},
			want: map[string]ServerConfig{
				"server1": {Name: "server1", URL: "http://localhost:8001", Type: "mcp"},
			},
		},
		{
			name: "empty override",
			base: []ServerConfig{
				{Name: "server1", URL: "http://localhost:8001", Type: "mcp"},
			},
			override: []ServerConfig{},
			want: map[string]ServerConfig{
				"server1": {Name: "server1", URL: "http://localhost:8001", Type: "mcp"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeServerConfigs(tt.base, tt.override)

			// Convert to map for easier comparison
			gotMap := make(map[string]ServerConfig)
			for _, server := range got {
				gotMap[server.Name] = server
			}

			if len(gotMap) != len(tt.want) {
				t.Errorf("mergeServerConfigs() returned %d servers, want %d", len(gotMap), len(tt.want))
			}

			for name, wantServer := range tt.want {
				gotServer, exists := gotMap[name]
				if !exists {
					t.Errorf("Expected server %q not found in result", name)
					continue
				}

				if gotServer.URL != wantServer.URL {
					t.Errorf("Server %q URL = %q, want %q", name, gotServer.URL, wantServer.URL)
				}
				if gotServer.Type != wantServer.Type {
					t.Errorf("Server %q Type = %q, want %q", name, gotServer.Type, wantServer.Type)
				}
			}
		})
	}
}

func TestLoadConfigWithHierarchy(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear environment variable
	origEnv := os.Getenv("AGM_MCP_SERVERS")
	os.Unsetenv("AGM_MCP_SERVERS")
	defer os.Setenv("AGM_MCP_SERVERS", origEnv)

	// Create team config
	teamName := "engineering"
	teamDir := filepath.Join(tmpDir, ".config", "agm", "teams", teamName)
	os.MkdirAll(teamDir, 0755)
	teamConfig := `team:
  name: engineering
mcp_servers:
  - name: team-github
    url: http://team-github.com
    type: mcp
`
	os.WriteFile(filepath.Join(teamDir, "mcp.yaml"), []byte(teamConfig), 0644)

	// Set team membership
	SetTeamMembership(teamName)

	// Create user config
	userConfigDir := filepath.Join(tmpDir, ".config", "agm")
	os.MkdirAll(userConfigDir, 0755)
	userConfig := `mcp_servers:
  - name: user-local
    url: http://localhost:8001
    type: mcp
`
	os.WriteFile(filepath.Join(userConfigDir, "mcp.yaml"), []byte(userConfig), 0644)

	// Create session config
	sessionDir := filepath.Join(tmpDir, "project", ".agm")
	os.MkdirAll(sessionDir, 0755)
	sessionConfig := `mcp_servers:
  - name: session-local
    url: http://localhost:8002
    type: mcp
  - name: team-github
    url: http://override-github.com
    type: mcp
`
	os.WriteFile(filepath.Join(sessionDir, "mcp.yaml"), []byte(sessionConfig), 0644)

	// Load config with hierarchy
	projectPath := filepath.Join(tmpDir, "project")
	cfg, err := LoadConfigWithHierarchy(projectPath)
	if err != nil {
		t.Fatalf("LoadConfigWithHierarchy() error = %v", err)
	}

	// Build map for easier testing
	serverMap := make(map[string]ServerConfig)
	for _, server := range cfg.MCPServers {
		serverMap[server.Name] = server
	}

	// Verify all servers are present
	expectedServers := map[string]string{
		"team-github":   "http://override-github.com", // Session overrides team
		"user-local":    "http://localhost:8001",      // User config
		"session-local": "http://localhost:8002",      // Session config
	}

	for name, expectedURL := range expectedServers {
		server, exists := serverMap[name]
		if !exists {
			t.Errorf("Expected server %q not found in merged config", name)
			continue
		}

		if server.URL != expectedURL {
			t.Errorf("Server %q URL = %q, want %q", name, server.URL, expectedURL)
		}
	}

	if len(serverMap) != len(expectedServers) {
		t.Errorf("LoadConfigWithHierarchy() returned %d servers, want %d", len(serverMap), len(expectedServers))
	}
}

func TestLoadConfigWithHierarchy_NoTeam(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear environment variables
	origEnv := os.Getenv("AGM_MCP_SERVERS")
	os.Unsetenv("AGM_MCP_SERVERS")
	defer os.Setenv("AGM_MCP_SERVERS", origEnv)

	origTeam := os.Getenv("AGM_TEAM")
	os.Unsetenv("AGM_TEAM")
	defer os.Setenv("AGM_TEAM", origTeam)

	// Create user config only
	userConfigDir := filepath.Join(tmpDir, ".config", "agm")
	os.MkdirAll(userConfigDir, 0755)
	userConfig := `mcp_servers:
  - name: user-server
    url: http://localhost:8001
    type: mcp
`
	os.WriteFile(filepath.Join(userConfigDir, "mcp.yaml"), []byte(userConfig), 0644)

	// Load config without team or session
	cfg, err := LoadConfigWithHierarchy("")
	if err != nil {
		t.Fatalf("LoadConfigWithHierarchy() error = %v", err)
	}

	if len(cfg.MCPServers) != 1 {
		t.Errorf("LoadConfigWithHierarchy() returned %d servers, want 1", len(cfg.MCPServers))
	}

	if cfg.MCPServers[0].Name != "user-server" {
		t.Errorf("Server name = %q, want %q", cfg.MCPServers[0].Name, "user-server")
	}
}

func TestLoadConfigWithHierarchy_EnvVarOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Set environment variable
	origEnv := os.Getenv("AGM_MCP_SERVERS")
	os.Setenv("AGM_MCP_SERVERS", "server1=http://localhost:8001,server2=http://localhost:8002")
	defer os.Setenv("AGM_MCP_SERVERS", origEnv)

	// Load config with only env var
	cfg, err := LoadConfigWithHierarchy("")
	if err != nil {
		t.Fatalf("LoadConfigWithHierarchy() error = %v", err)
	}

	if len(cfg.MCPServers) != 2 {
		t.Errorf("LoadConfigWithHierarchy() returned %d servers, want 2", len(cfg.MCPServers))
	}

	// Build map for easier testing
	serverMap := make(map[string]ServerConfig)
	for _, server := range cfg.MCPServers {
		serverMap[server.Name] = server
	}

	expectedServers := map[string]string{
		"server1": "http://localhost:8001",
		"server2": "http://localhost:8002",
	}

	for name, expectedURL := range expectedServers {
		server, exists := serverMap[name]
		if !exists {
			t.Errorf("Expected server %q not found", name)
			continue
		}

		if server.URL != expectedURL {
			t.Errorf("Server %q URL = %q, want %q", name, server.URL, expectedURL)
		}
	}
}
