package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPManager_LoadGlobalConfig(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mcp.yaml")

	configContent := `mcp_servers:
  - name: googledocs
    url: http://localhost:8001
    type: mcp
  - name: github
    url: http://localhost:8002
    type: mcp
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Set up environment to use test config
	homeDir := os.Getenv("HOME")
	testConfigDir := filepath.Join(tmpDir, ".config", "agm")
	os.MkdirAll(testConfigDir, 0755)
	testConfigPath := filepath.Join(testConfigDir, "mcp.yaml")
	os.WriteFile(testConfigPath, []byte(configContent), 0644)

	// Temporarily override HOME
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", homeDir)

	// Create manager
	manager := NewMCPManager()

	// Load config
	err := manager.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("Failed to load global config: %v", err)
	}

	// Verify config loaded
	if manager.globalConfig == nil {
		t.Fatal("Global config is nil")
	}

	if len(manager.globalConfig.MCPServers) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(manager.globalConfig.MCPServers))
	}
}

func TestMCPManager_ConnectToGlobalMCP(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy"}`))
		case "/mcp/sessions":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"sessionId":"test-session-123","createdAt":"2024-01-01T00:00:00Z"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create manager with test config
	manager := NewMCPManager()
	manager.globalConfig = &ClientConfig{
		MCPServers: []ServerConfig{
			{Name: "test-mcp", URL: server.URL, Type: "mcp"},
		},
	}

	// Connect to MCP
	ctx := context.Background()
	conn, err := manager.ConnectToMCP(ctx, "test-mcp")
	if err != nil {
		t.Fatalf("Failed to connect to MCP: %v", err)
	}

	if conn == nil {
		t.Fatal("Connection is nil")
	}

	if conn.Name != "test-mcp" {
		t.Errorf("Expected name 'test-mcp', got '%s'", conn.Name)
	}

	if conn.Type != MCPTypeGlobal {
		t.Errorf("Expected type %s, got %s", MCPTypeGlobal, conn.Type)
	}

	if !conn.IsGlobal {
		t.Error("Expected IsGlobal to be true")
	}
}

func TestMCPManager_DisconnectGlobalMCP(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy"}`))
		case "/mcp/sessions":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"sessionId":"test-session-123","createdAt":"2024-01-01T00:00:00Z"}`))
		default:
			if r.Method == "DELETE" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"success":true}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))
	defer server.Close()

	// Create manager and connect
	manager := NewMCPManager()
	manager.globalConfig = &ClientConfig{
		MCPServers: []ServerConfig{
			{Name: "test-mcp", URL: server.URL, Type: "mcp"},
		},
	}

	ctx := context.Background()
	conn, err := manager.ConnectToMCP(ctx, "test-mcp")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if conn == nil {
		t.Fatal("Connection is nil")
	}

	// Disconnect
	err = manager.DisconnectMCP(ctx, "test-mcp")
	if err != nil {
		t.Fatalf("Failed to disconnect: %v", err)
	}

	// Verify connection removed
	if _, exists := manager.GetConnection("test-mcp"); exists {
		t.Error("Connection still exists after disconnect")
	}
}

func TestMCPManager_FallbackToSessionMCP(t *testing.T) {
	// Create manager with session config (no global)
	manager := NewMCPManager()
	manager.sessionConfig = &SessionMCPConfig{
		MCPServers: []SessionServerConfig{
			{
				Name:    "local-mcp",
				Command: "node",
				Args:    []string{"server.js"},
			},
		},
	}

	// Connect to MCP (should fall back to session-specific)
	ctx := context.Background()
	conn, err := manager.ConnectToMCP(ctx, "local-mcp")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if conn.Type != MCPTypeSession {
		t.Errorf("Expected type %s, got %s", MCPTypeSession, conn.Type)
	}

	if conn.IsGlobal {
		t.Error("Expected IsGlobal to be false")
	}
}

func TestMCPManager_ListConnections(t *testing.T) {
	manager := NewMCPManager()

	// Add mock connections
	manager.connections["mcp1"] = &MCPConnection{
		Name:     "mcp1",
		Type:     MCPTypeGlobal,
		IsGlobal: true,
	}
	manager.connections["mcp2"] = &MCPConnection{
		Name:     "mcp2",
		Type:     MCPTypeSession,
		IsGlobal: false,
	}

	// List connections
	connections := manager.ListConnections()

	if len(connections) != 2 {
		t.Errorf("Expected 2 connections, got %d", len(connections))
	}
}

func TestMCPManager_GetMergedConfig(t *testing.T) {
	manager := NewMCPManager()

	// Set global and session configs
	manager.globalConfig = &ClientConfig{
		MCPServers: []ServerConfig{
			{Name: "global-mcp", URL: "http://localhost:8001", Type: "mcp"},
		},
	}

	manager.sessionConfig = &SessionMCPConfig{
		MCPServers: []SessionServerConfig{
			{Name: "session-mcp", Command: "node", Args: []string{"server.js"}},
		},
	}

	// Get merged config
	merged := manager.GetMCPConfig()

	if len(merged.GlobalServers) != 1 {
		t.Errorf("Expected 1 global server, got %d", len(merged.GlobalServers))
	}

	if len(merged.SessionServers) != 1 {
		t.Errorf("Expected 1 session server, got %d", len(merged.SessionServers))
	}

	if merged.GlobalServers[0].Name != "global-mcp" {
		t.Errorf("Expected global server 'global-mcp', got '%s'", merged.GlobalServers[0].Name)
	}

	if merged.SessionServers[0].Name != "session-mcp" {
		t.Errorf("Expected session server 'session-mcp', got '%s'", merged.SessionServers[0].Name)
	}
}
