package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDetectGlobalMCP_Success(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create detector
	detector := NewGlobalMCPDetector()

	// Test detection
	ctx := context.Background()
	result := detector.DetectGlobalMCP(ctx, ServerConfig{
		Name: "test-mcp",
		URL:  server.URL,
		Type: "mcp",
	})

	if !result.Available {
		t.Errorf("Expected MCP to be available, got unavailable: %v", result.Error)
	}

	if result.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", result.Status)
	}
}

func TestDetectGlobalMCP_ServerDown(t *testing.T) {
	// Create detector
	detector := NewGlobalMCPDetector()

	// Test detection with non-existent server
	ctx := context.Background()
	result := detector.DetectGlobalMCP(ctx, ServerConfig{
		Name: "test-mcp",
		URL:  "http://localhost:99999",
		Type: "mcp",
	})

	if result.Available {
		t.Errorf("Expected MCP to be unavailable, got available")
	}

	if result.Error == nil {
		t.Errorf("Expected error for unavailable MCP, got nil")
	}
}

func TestDetectGlobalMCP_Timeout(t *testing.T) {
	// Create test HTTP server that sleeps
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create detector with short timeout
	detector := &GlobalMCPDetector{
		healthCheckTimeout: 100 * time.Millisecond,
	}

	// Test detection
	ctx := context.Background()
	result := detector.DetectGlobalMCP(ctx, ServerConfig{
		Name: "test-mcp",
		URL:  server.URL,
		Type: "mcp",
	})

	if result.Available {
		t.Errorf("Expected MCP to be unavailable due to timeout")
	}

	if result.Error == nil {
		t.Errorf("Expected timeout error, got nil")
	}
}

func TestDetectGlobalMCP_EmptyURL(t *testing.T) {
	detector := NewGlobalMCPDetector()

	ctx := context.Background()
	result := detector.DetectGlobalMCP(ctx, ServerConfig{
		Name: "test-mcp",
		URL:  "",
		Type: "mcp",
	})

	if result.Available {
		t.Errorf("Expected MCP to be unavailable with empty URL")
	}

	if result.Error == nil {
		t.Errorf("Expected error for empty URL, got nil")
	}
}

func TestDetectAllGlobalMCPs(t *testing.T) {
	// Create two test servers
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy"}`))
		}
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy"}`))
		}
	}))
	defer server2.Close()

	// Create config with two servers
	config := &ClientConfig{
		MCPServers: []ServerConfig{
			{Name: "mcp1", URL: server1.URL, Type: "mcp"},
			{Name: "mcp2", URL: server2.URL, Type: "mcp"},
		},
	}

	// Create detector
	detector := NewGlobalMCPDetector()

	// Test detection
	ctx := context.Background()
	results := detector.DetectAllGlobalMCPs(ctx, config)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	if !results["mcp1"].Available {
		t.Errorf("Expected mcp1 to be available")
	}

	if !results["mcp2"].Available {
		t.Errorf("Expected mcp2 to be available")
	}
}

func TestDetectGlobalMCP_NonOKStatus(t *testing.T) {
	// Create test HTTP server that returns 503
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"degraded"}`))
		}
	}))
	defer server.Close()

	// Create detector
	detector := NewGlobalMCPDetector()

	// Test detection
	ctx := context.Background()
	result := detector.DetectGlobalMCP(ctx, ServerConfig{
		Name: "test-mcp",
		URL:  server.URL,
		Type: "mcp",
	})

	if result.Available {
		t.Errorf("Expected MCP to be unavailable with 503 status")
	}

	if result.Error == nil {
		t.Errorf("Expected error for 503 status, got nil")
	}
}
