package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// MCPType indicates whether an MCP is global (HTTP) or session-specific (stdio)
type MCPType string

const (
	// MCPTypeGlobal indicates a global HTTP/SSE MCP server
	MCPTypeGlobal MCPType = "global"
	// MCPTypeSession indicates a session-specific stdio MCP server
	MCPTypeSession MCPType = "session"
)

// MCPConnection represents a connection to an MCP server
type MCPConnection struct {
	Name     string
	Type     MCPType
	Client   mcpClient
	IsGlobal bool
	URL      string
}

// MCPManager manages MCP connections for AGM sessions
type MCPManager struct {
	globalConfig  *ClientConfig
	sessionConfig *SessionMCPConfig
	connections   map[string]*MCPConnection
	mu            sync.RWMutex
	detector      *GlobalMCPDetector
}

// SessionMCPConfig represents session-specific MCP configuration
type SessionMCPConfig struct {
	// MCPServers lists session-specific MCP servers (stdio-based)
	MCPServers []SessionServerConfig `yaml:"mcp_servers"`
}

// SessionServerConfig represents a session-specific MCP server
type SessionServerConfig struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env,omitempty"`
}

// NewMCPManager creates a new MCP manager
func NewMCPManager() *MCPManager {
	return &MCPManager{
		connections: make(map[string]*MCPConnection),
		detector:    NewGlobalMCPDetector(),
	}
}

// LoadGlobalConfig loads the global MCP configuration
func (m *MCPManager) LoadGlobalConfig() error {
	cfg, err := loadClientConfig()
	if err != nil {
		return fmt.Errorf("failed to load global MCP config: %w", err)
	}

	m.mu.Lock()
	m.globalConfig = cfg
	m.mu.Unlock()

	return nil
}

// LoadSessionConfig loads session-specific MCP configuration
func (m *MCPManager) LoadSessionConfig(sessionDir string) error {
	configPath := filepath.Join(sessionDir, ".agm", "mcp.yaml")

	// Session config is optional
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		m.mu.Lock()
		m.sessionConfig = &SessionMCPConfig{MCPServers: []SessionServerConfig{}}
		m.mu.Unlock()
		return nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read session MCP config: %w", err)
	}

	var cfg SessionMCPConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse session MCP config: %w", err)
	}

	m.mu.Lock()
	m.sessionConfig = &cfg
	m.mu.Unlock()

	return nil
}

// GetMCPConfig returns the merged MCP configuration
// Session-specific MCPs override global MCPs with the same name
func (m *MCPManager) GetMCPConfig() *MergedMCPConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	merged := &MergedMCPConfig{
		GlobalServers:  []ServerConfig{},
		SessionServers: []SessionServerConfig{},
	}

	// Add global servers
	if m.globalConfig != nil {
		merged.GlobalServers = m.globalConfig.MCPServers
	}

	// Add session-specific servers
	if m.sessionConfig != nil {
		merged.SessionServers = m.sessionConfig.MCPServers
	}

	return merged
}

// MergedMCPConfig represents the merged global + session MCP configuration
type MergedMCPConfig struct {
	GlobalServers  []ServerConfig
	SessionServers []SessionServerConfig
}

// ConnectToMCP connects to an MCP server (global or session-specific)
// It automatically detects if a global MCP is available and falls back to stdio if not
func (m *MCPManager) ConnectToMCP(ctx context.Context, serverName string) (*MCPConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already connected
	if conn, exists := m.connections[serverName]; exists {
		return conn, nil
	}

	// Try global MCP first
	if m.globalConfig != nil {
		serverURL, found := m.globalConfig.GetServerURL(serverName)
		if found {
			// Check if global MCP is available
			result := m.detector.DetectGlobalMCP(ctx, ServerConfig{
				Name: serverName,
				URL:  serverURL,
				Type: "mcp",
			})

			if result.Available {
				// Connect to global HTTP MCP
				client := NewHTTPClient(serverURL)

				// Create session
				if err := client.CreateSession(ctx, "agm-session"); err != nil {
					return nil, fmt.Errorf("failed to create global MCP session: %w", err)
				}

				conn := &MCPConnection{
					Name:     serverName,
					Type:     MCPTypeGlobal,
					Client:   client,
					IsGlobal: true,
					URL:      serverURL,
				}

				m.connections[serverName] = conn
				return conn, nil
			}
		}
	}

	// Fallback to session-specific stdio MCP
	if m.sessionConfig != nil {
		for _, server := range m.sessionConfig.MCPServers {
			if server.Name == serverName {
				// TODO: Implement stdio MCP client
				// For now, return mock client
				client := &mockMCPClient{url: "stdio://" + server.Command}

				conn := &MCPConnection{
					Name:     serverName,
					Type:     MCPTypeSession,
					Client:   client,
					IsGlobal: false,
					URL:      "stdio://" + server.Command,
				}

				m.connections[serverName] = conn
				return conn, nil
			}
		}
	}

	return nil, fmt.Errorf("MCP server '%s' not found in global or session config", serverName)
}

// DisconnectMCP disconnects from an MCP server
// Global MCPs are NOT terminated (reference counted)
// Session-specific MCPs are terminated
func (m *MCPManager) DisconnectMCP(ctx context.Context, serverName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[serverName]
	if !exists {
		return nil // Already disconnected
	}

	// For global MCPs, just close the session (don't kill the server)
	if conn.IsGlobal {
		if err := conn.Client.Close(); err != nil {
			return fmt.Errorf("failed to close global MCP session: %w", err)
		}
	} else {
		// For session-specific MCPs, terminate the process
		if err := conn.Client.Close(); err != nil {
			return fmt.Errorf("failed to close session MCP: %w", err)
		}
	}

	delete(m.connections, serverName)
	return nil
}

// DisconnectAll disconnects from all MCP servers
func (m *MCPManager) DisconnectAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	for name, conn := range m.connections {
		if conn.IsGlobal {
			// Close session but don't kill global server
			if err := conn.Client.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close global MCP '%s': %w", name, err))
			}
		} else {
			// Terminate session-specific MCP
			if err := conn.Client.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close session MCP '%s': %w", name, err))
			}
		}
		delete(m.connections, name)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors disconnecting MCPs: %v", errs)
	}

	return nil
}

// GetConnection returns an existing MCP connection
func (m *MCPManager) GetConnection(serverName string) (*MCPConnection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[serverName]
	return conn, exists
}

// ListConnections returns all active MCP connections
func (m *MCPManager) ListConnections() []*MCPConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	connections := make([]*MCPConnection, 0, len(m.connections))
	for _, conn := range m.connections {
		connections = append(connections, conn)
	}

	return connections
}

// DetectAvailableGlobalMCPs returns a list of available global MCPs
func (m *MCPManager) DetectAvailableGlobalMCPs(ctx context.Context) map[string]DetectionResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.globalConfig == nil {
		return make(map[string]DetectionResult)
	}

	return m.detector.DetectAllGlobalMCPs(ctx, m.globalConfig)
}
