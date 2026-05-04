package mcp

import (
	"context"
	"fmt"
	"time"
)

// QueryMCPServer connects to external MCP server and queries for context
// Implements graceful degradation with timeouts (2s connection, 5s read)
func QueryMCPServer(serverName string, query string) (string, error) {
	// 1. Load client configuration
	cfg, err := loadClientConfig()
	if err != nil {
		return "", fmt.Errorf("config error: %w", err)
	}

	// 2. Find server URL
	serverURL, found := cfg.GetServerURL(serverName)
	if !found {
		return "", fmt.Errorf("MCP server '%s' not configured. Check ~/.config/agm/mcp.yaml or AGM_MCP_SERVERS", serverName)
	}

	// 3. Connect with 2s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := connectMCPServer(ctx, serverURL)
	if err != nil {
		return "", fmt.Errorf("connection timeout (2s): %w", err)
	}
	defer client.Close()

	// 4. Query with 5s timeout
	queryCtx, queryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queryCancel()

	result, err := client.Query(queryCtx, query)
	if err != nil {
		return "", fmt.Errorf("query timeout (5s): %w", err)
	}

	return result, nil
}

// mcpClient interface (stub for V1)
type mcpClient interface {
	Query(ctx context.Context, query string) (string, error)
	Close() error
}

// connectMCPServer connects to MCP server at given URL
// TODO: Implement actual MCP client connection using official SDK
func connectMCPServer(_ context.Context, serverURL string) (mcpClient, error) {
	// V1 stub: Return mock client
	return &mockMCPClient{url: serverURL}, nil
}

// mockMCPClient is a placeholder for V1 implementation
type mockMCPClient struct {
	url string
}

func (m *mockMCPClient) Query(ctx context.Context, query string) (string, error) {
	// TODO: Implement actual MCP query via official SDK
	return fmt.Sprintf("Mock response from %s for query: %s", m.url, query), nil
}

func (m *mockMCPClient) Close() error {
	return nil
}
