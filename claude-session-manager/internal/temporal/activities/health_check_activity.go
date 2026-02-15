package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/temporal/workflows"
)

// HealthCheckActivity performs an HTTP health check on the MCP server
// This activity sends a GET request to the health endpoint and parses the response
func HealthCheckActivity(ctx context.Context, input workflows.HealthCheckInput) (*workflows.HealthCheckResult, error) {
	// Validate input
	if input.URL == "" {
		return nil, fmt.Errorf("health check URL cannot be empty")
	}

	// Set default timeout if not provided
	if input.Timeout == 0 {
		input.Timeout = 5 * time.Second
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: input.Timeout,
	}

	// Create request with context for cancellation
	req, err := http.NewRequestWithContext(ctx, "GET", input.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health check request: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", "AGM-MCP-Health-Check/1.0")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return &workflows.HealthCheckResult{
			Status:    "unhealthy",
			Timestamp: time.Now(),
		}, fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return &workflows.HealthCheckResult{
			Status:    "unhealthy",
			Timestamp: time.Now(),
		}, fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &workflows.HealthCheckResult{
			Status:    "unhealthy",
			Timestamp: time.Now(),
		}, fmt.Errorf("failed to read health check response: %w", err)
	}

	// Parse JSON response
	var healthData struct {
		Status       string  `json:"status"`
		Uptime       float64 `json:"uptime"`    // Uptime in seconds
		SessionCount int     `json:"sessions"`  // Number of active sessions (note: field name is "sessions")
		MCPProcess   string  `json:"mcpProcess"` // MCP process status
	}

	if err := json.Unmarshal(body, &healthData); err != nil {
		// If JSON parsing fails, consider it unhealthy
		return &workflows.HealthCheckResult{
			Status:    "unhealthy",
			Timestamp: time.Now(),
		}, fmt.Errorf("failed to parse health check response: %w", err)
	}

	// Build result
	result := &workflows.HealthCheckResult{
		Status:       healthData.Status,
		Uptime:       time.Duration(healthData.Uptime * float64(time.Second)),
		SessionCount: healthData.SessionCount,
		Timestamp:    time.Now(),
	}

	// Validate status
	if healthData.Status != "healthy" && healthData.Status != "ok" {
		return result, fmt.Errorf("server reports unhealthy status: %s", healthData.Status)
	}

	return result, nil
}

// PingMCPActivity performs a simple ping to check if the server is responding
// This is a lighter-weight alternative to full health check
func PingMCPActivity(ctx context.Context, url string) (bool, error) {
	if url == "" {
		return false, fmt.Errorf("ping URL cannot be empty")
	}

	// Create HTTP client with short timeout
	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create ping request: %w", err)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return false, nil // Server not responding
	}
	defer resp.Body.Close()

	// Any 2xx status code is considered success
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

// GetMCPMetricsActivity retrieves detailed metrics from the MCP server
// This activity can be used for monitoring and observability
func GetMCPMetricsActivity(ctx context.Context, url string) (map[string]interface{}, error) {
	if url == "" {
		return nil, fmt.Errorf("metrics URL cannot be empty")
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metrics request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics request returned status %d", resp.StatusCode)
	}

	// Read and parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metrics response: %w", err)
	}

	var metrics map[string]interface{}
	if err := json.Unmarshal(body, &metrics); err != nil {
		return nil, fmt.Errorf("failed to parse metrics response: %w", err)
	}

	return metrics, nil
}
