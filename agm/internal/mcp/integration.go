package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// SessionMCPIntegration integrates MCP management into AGM sessions
type SessionMCPIntegration struct {
	manager     *MCPManager
	sessionName string
	sessionDir  string
}

// NewSessionMCPIntegration creates a new session MCP integration
func NewSessionMCPIntegration(sessionName, sessionDir string) *SessionMCPIntegration {
	return &SessionMCPIntegration{
		manager:     NewMCPManager(),
		sessionName: sessionName,
		sessionDir:  sessionDir,
	}
}

// Initialize loads global and session-specific MCP configurations
func (s *SessionMCPIntegration) Initialize(ctx context.Context) error {
	// Load global config
	if err := s.manager.LoadGlobalConfig(); err != nil {
		// Non-fatal: global config is optional
		fmt.Fprintf(os.Stderr, "Warning: failed to load global MCP config: %v\n", err)
	}

	// Load session config
	if err := s.manager.LoadSessionConfig(s.sessionDir); err != nil {
		// Non-fatal: session config is optional
		fmt.Fprintf(os.Stderr, "Warning: failed to load session MCP config: %v\n", err)
	}

	return nil
}

// GetClaudeArgs returns Claude CLI arguments with MCP configuration
// If global MCPs are available, it configures Claude to use them via HTTP
// Otherwise, it falls back to stdio MCP configuration
func (s *SessionMCPIntegration) GetClaudeArgs(ctx context.Context, workDir string) []string {
	args := []string{
		"--add-dir", workDir,
	}

	// Get merged MCP config
	merged := s.manager.GetMCPConfig()

	// Check which global MCPs are available
	available := s.manager.DetectAvailableGlobalMCPs(ctx)

	// For each global MCP that's available, add it to Claude's config
	for _, server := range merged.GlobalServers {
		if result, ok := available[server.Name]; ok && result.Available {
			// Global MCP is available - Claude will connect via HTTP
			// We need to set environment variables for Claude to use HTTP MCPs
			// This requires Claude CLI support for HTTP MCPs (future work)
			fmt.Fprintf(os.Stderr, "Global MCP '%s' available at %s\n", server.Name, server.URL)
		}
	}

	return args
}

// SetupEnvironment sets up environment variables for MCP integration
// This includes AGM_MCP_SERVERS for global MCPs
func (s *SessionMCPIntegration) SetupEnvironment() map[string]string {
	env := make(map[string]string)

	// Get merged config
	merged := s.manager.GetMCPConfig()

	// Build AGM_MCP_SERVERS environment variable
	var mcpServers []string
	for _, server := range merged.GlobalServers {
		mcpServers = append(mcpServers, fmt.Sprintf("%s=%s", server.Name, server.URL))
	}

	if len(mcpServers) > 0 {
		env["AGM_MCP_SERVERS"] = joinMCPServers(mcpServers)
	}

	// Set session name for MCP context
	env["AGM_SESSION_NAME"] = s.sessionName

	return env
}

// ConnectToMCP connects to a specific MCP server (global or session-specific)
func (s *SessionMCPIntegration) ConnectToMCP(ctx context.Context, serverName string) (*MCPConnection, error) {
	return s.manager.ConnectToMCP(ctx, serverName)
}

// Cleanup disconnects from all MCPs when session ends
// Global MCPs are NOT terminated, only session-specific ones
func (s *SessionMCPIntegration) Cleanup(ctx context.Context) error {
	return s.manager.DisconnectAll(ctx)
}

// GetManager returns the underlying MCP manager
func (s *SessionMCPIntegration) GetManager() *MCPManager {
	return s.manager
}

// joinMCPServers joins MCP server entries for AGM_MCP_SERVERS env var
func joinMCPServers(servers []string) string {
	return strings.Join(servers, ",")
}

// StartGlobalMCPsIfNeeded starts global MCP servers if they are configured but not running
// This is typically called during AGM daemon startup
func StartGlobalMCPsIfNeeded(ctx context.Context) error {
	// Load global config
	cfg, err := loadClientConfig()
	if err != nil {
		return fmt.Errorf("failed to load global MCP config: %w", err)
	}

	if len(cfg.MCPServers) == 0 {
		// No global MCPs configured
		return nil
	}

	// Check which MCPs are already running
	detector := NewGlobalMCPDetector()
	results := detector.DetectAllGlobalMCPs(ctx, cfg)

	// Start any MCPs that aren't running
	for _, server := range cfg.MCPServers {
		result := results[server.Name]
		if !result.Available {
			fmt.Fprintf(os.Stderr, "Global MCP '%s' not running, would start it here\n", server.Name)
			// TODO: Integrate with Temporal workflow to start MCP
			// This requires calling the MCPServiceWorkflow
		}
	}

	return nil
}

// GetGlobalMCPStatus returns the status of all global MCPs
func GetGlobalMCPStatus(ctx context.Context) (map[string]DetectionResult, error) {
	cfg, err := loadClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load global MCP config: %w", err)
	}

	detector := NewGlobalMCPDetector()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return detector.DetectAllGlobalMCPs(ctx, cfg), nil
}
