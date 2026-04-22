// Package mcp provides MCP server implementation.
package mcp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// GlobalMCPDetector detects and validates global MCP servers
type GlobalMCPDetector struct {
	healthCheckTimeout time.Duration
}

// NewGlobalMCPDetector creates a new MCP detector with default timeout
func NewGlobalMCPDetector() *GlobalMCPDetector {
	return &GlobalMCPDetector{
		healthCheckTimeout: 3 * time.Second,
	}
}

// DetectionResult contains the result of global MCP detection
type DetectionResult struct {
	Available bool
	URL       string
	Name      string
	Status    string
	Error     error
}

// DetectGlobalMCP checks if a global MCP is available at the given URL
// It performs a health check and returns the detection result
func (d *GlobalMCPDetector) DetectGlobalMCP(ctx context.Context, serverConfig ServerConfig) DetectionResult {
	result := DetectionResult{
		Available: false,
		URL:       serverConfig.URL,
		Name:      serverConfig.Name,
	}

	// Validate URL is provided
	if serverConfig.URL == "" {
		result.Error = fmt.Errorf("server URL is empty")
		return result
	}

	// Build health check URL
	healthURL := serverConfig.URL + "/health"

	// Create retryable HTTP client for health checks
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 2 // Quick retry for health checks
	retryClient.RetryWaitMin = 500 * time.Millisecond
	retryClient.RetryWaitMax = 2 * time.Second
	retryClient.Logger = nil
	retryClient.HTTPClient.Timeout = d.healthCheckTimeout

	client := retryClient.StandardClient()

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		result.Error = fmt.Errorf("failed to create health check request: %w", err)
		return result
	}

	// Set headers
	req.Header.Set("User-Agent", "AGM-MCP-Detector/1.0")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("health check failed: %w", err)
		return result
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("health check returned status %d", resp.StatusCode)
		return result
	}

	// Server is available
	result.Available = true
	result.Status = "healthy"
	return result
}

// DetectAllGlobalMCPs checks all configured global MCPs and returns detection results
func (d *GlobalMCPDetector) DetectAllGlobalMCPs(ctx context.Context, config *ClientConfig) map[string]DetectionResult {
	results := make(map[string]DetectionResult)

	for _, server := range config.MCPServers {
		result := d.DetectGlobalMCP(ctx, server)
		results[server.Name] = result
	}

	return results
}

// IsGlobalMCPAvailable is a convenience function to check if a specific MCP is available
func IsGlobalMCPAvailable(serverName string) (bool, error) {
	// Load config
	cfg, err := loadClientConfig()
	if err != nil {
		return false, fmt.Errorf("failed to load MCP config: %w", err)
	}

	// Find server
	serverURL, found := cfg.GetServerURL(serverName)
	if !found {
		return false, fmt.Errorf("MCP server '%s' not configured", serverName)
	}

	// Detect
	detector := NewGlobalMCPDetector()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := detector.DetectGlobalMCP(ctx, ServerConfig{
		Name: serverName,
		URL:  serverURL,
		Type: "mcp",
	})

	if result.Error != nil {
		return false, result.Error
	}

	return result.Available, nil
}
