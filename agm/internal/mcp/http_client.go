package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// HTTPClient represents an HTTP/SSE MCP client
type HTTPClient struct {
	baseURL    string
	sessionID  string
	httpClient *http.Client
	sseClient  *http.Client
}

// NewHTTPClient creates a new HTTP MCP client
func NewHTTPClient(baseURL string) *HTTPClient {
	// Create retryable HTTP client with MCP-appropriate retry settings
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 30 * time.Second

	// Custom retry policy for MCP requests
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		// Network/timeout errors are retryable
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return true, nil
		}

		// HTTP errors
		if resp != nil {
			// Retry on 500-level errors (server errors)
			if resp.StatusCode >= 500 && resp.StatusCode < 600 {
				return true, nil
			}
			// Retry on 429 (rate limit)
			if resp.StatusCode == http.StatusTooManyRequests {
				return true, nil
			}
			// Don't retry on 4xx errors (client errors) except 429
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return false, nil
			}
		}

		return false, nil
	}

	// Disable built-in logging
	retryClient.Logger = nil

	// Set timeout on the retryable client
	retryClient.HTTPClient.Timeout = 30 * time.Second

	return &HTTPClient{
		baseURL:    baseURL,
		httpClient: retryClient.StandardClient(),
		sseClient: &http.Client{
			Timeout: 0, // No timeout for SSE connections
		},
	}
}

// SessionResponse represents the response from creating a session
type SessionResponse struct {
	SessionID string    `json:"sessionId"`
	CreatedAt time.Time `json:"createdAt"`
}

// MCPRequest represents a JSON-RPC request to MCP server
type MCPRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC response from MCP server
type MCPResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Result  map[string]any `json:"result,omitempty"`
	Error   *MCPError      `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC error
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// CreateSession creates a new session on the HTTP MCP server
func (c *HTTPClient) CreateSession(ctx context.Context, clientID string) error {
	url := c.baseURL + "/mcp/sessions"

	reqBody := map[string]string{
		"clientId": clientID,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("session creation failed with status %d: %s", resp.StatusCode, string(body))
	}

	var sessionResp SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return fmt.Errorf("failed to decode session response: %w", err)
	}

	c.sessionID = sessionResp.SessionID
	return nil
}

// SendRequest sends a JSON-RPC request to the MCP server
func (c *HTTPClient) SendRequest(ctx context.Context, method string, params map[string]any) (*MCPResponse, error) {
	if c.sessionID == "" {
		return nil, fmt.Errorf("no active session - call CreateSession first")
	}

	url := c.baseURL + "/mcp/message"

	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("X-Session-ID", c.sessionID)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var mcpResp MCPResponse
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if mcpResp.Error != nil {
		return nil, fmt.Errorf("MCP error %d: %s", mcpResp.Error.Code, mcpResp.Error.Message)
	}

	return &mcpResp, nil
}

// Initialize sends the initialize request to the MCP server
func (c *HTTPClient) Initialize(ctx context.Context, params map[string]any) (*MCPResponse, error) {
	return c.SendRequest(ctx, "initialize", params)
}

// ListTools lists available tools from the MCP server
func (c *HTTPClient) ListTools(ctx context.Context) (*MCPResponse, error) {
	return c.SendRequest(ctx, "tools/list", nil)
}

// CallTool calls a tool on the MCP server
func (c *HTTPClient) CallTool(ctx context.Context, toolName string, arguments map[string]any) (*MCPResponse, error) {
	params := map[string]any{
		"name":      toolName,
		"arguments": arguments,
	}
	return c.SendRequest(ctx, "tools/call", params)
}

// CloseSession closes the session on the HTTP MCP server
func (c *HTTPClient) CloseSession(ctx context.Context) error {
	if c.sessionID == "" {
		return nil // No active session
	}

	url := c.baseURL + "/mcp/sessions/" + c.sessionID

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create close request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to close session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("session close failed with status %d: %s", resp.StatusCode, string(body))
	}

	c.sessionID = ""
	return nil
}

// GetSessionID returns the current session ID
func (c *HTTPClient) GetSessionID() string {
	return c.sessionID
}

// Close closes the HTTP client and cleans up resources
func (c *HTTPClient) Close() error {
	ctx := context.Background()
	return c.CloseSession(ctx)
}

// Query implements the mcpClient interface for compatibility
func (c *HTTPClient) Query(ctx context.Context, query string) (string, error) {
	// This is a simplified implementation for V1 compatibility
	// In practice, you'd want to use specific MCP methods
	resp, err := c.SendRequest(ctx, "query", map[string]any{
		"query": query,
	})
	if err != nil {
		return "", err
	}

	// Extract result as string
	if result, ok := resp.Result["result"].(string); ok {
		return result, nil
	}

	// Fallback: marshal entire result
	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}
