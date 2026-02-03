package research

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewClient_WithEnvVar(t *testing.T) {
	// Set environment variable
	os.Setenv("GEMINI_API_KEY", "test-api-key-123")
	defer os.Unsetenv("GEMINI_API_KEY")

	client, err := NewClient("")
	if err != nil {
		t.Fatalf("NewClient() error = %v, want nil", err)
	}

	if client.apiKey != "test-api-key-123" {
		t.Errorf("client.apiKey = %q, want %q", client.apiKey, "test-api-key-123")
	}
}

func TestNewClient_NoAPIKey(t *testing.T) {
	// Ensure no API key is set
	os.Unsetenv("GEMINI_API_KEY")

	_, err := NewClient("")
	if err == nil {
		t.Error("NewClient() error = nil, want error for missing API key")
	}

	// Check that it's the right error type
	var researchErr *ResearchError
	if !errors.As(err, &researchErr) {
		t.Errorf("error type = %T, want *ResearchError", err)
	}
}

func TestStartResearch_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != http.MethodPost {
			t.Errorf("request method = %s, want POST", r.Method)
		}

		// Verify URL path
		if !strings.Contains(r.URL.Path, "/interactions") {
			t.Errorf("URL path = %s, want to contain /interactions", r.URL.Path)
		}

		// Verify API key in query
		apiKey := r.URL.Query().Get("key")
		if apiKey != "test-key" {
			t.Errorf("API key = %s, want test-key", apiKey)
		}

		// Verify request body
		var req InteractionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.Agent != DefaultAgent {
			t.Errorf("agent = %s, want %s", req.Agent, DefaultAgent)
		}

		if !strings.Contains(req.Input, "topic1") {
			t.Errorf("input = %s, want to contain topic1", req.Input)
		}

		// Return success response
		resp := InteractionResponse{
			ID:     "interaction-123",
			Status: "pending",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override BaseURL for testing
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Start research
	ctx := context.Background()
	interactionID, err := client.StartResearch(ctx, []string{"topic1", "topic2"})
	if err != nil {
		t.Fatalf("StartResearch() error = %v, want nil", err)
	}

	if interactionID != "interaction-123" {
		t.Errorf("interactionID = %s, want interaction-123", interactionID)
	}
}

func TestStartResearch_AuthError(t *testing.T) {
	// Create test server that returns 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid API key"))
	}))
	defer server.Close()

	// Override BaseURL for testing
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "invalid-key",
	}

	// Start research
	ctx := context.Background()
	_, err := client.StartResearch(ctx, []string{"topic1"})
	if err == nil {
		t.Error("StartResearch() error = nil, want auth error")
	}

	// Check error contains auth failure
	if !strings.Contains(err.Error(), "authentication") && !strings.Contains(err.Error(), "auth") {
		t.Errorf("error = %v, want auth-related error", err)
	}
}

func TestStartResearch_RateLimitError(t *testing.T) {
	// Create test server that returns 429
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limit exceeded"))
	}))
	defer server.Close()

	// Override BaseURL for testing
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Start research with short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.StartResearch(ctx, []string{"topic1"})
	if err == nil {
		t.Error("StartResearch() error = nil, want rate limit error")
	}
}

func TestGetInteractionStatus_Success(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != http.MethodGet {
			t.Errorf("request method = %s, want GET", r.Method)
		}

		// Verify URL contains interaction ID
		if !strings.Contains(r.URL.Path, "interaction-123") {
			t.Errorf("URL path = %s, want to contain interaction-123", r.URL.Path)
		}

		// Return status response
		resp := InteractionResponse{
			ID:     "interaction-123",
			Status: "in_progress",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override BaseURL for testing
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Get status
	ctx := context.Background()
	status, err := client.GetInteractionStatus(ctx, "interaction-123")
	if err != nil {
		t.Fatalf("GetInteractionStatus() error = %v, want nil", err)
	}

	if status.Status != "in_progress" {
		t.Errorf("status.Status = %s, want in_progress", status.Status)
	}
}

func TestGetInteractionStatus_NotFound(t *testing.T) {
	// Create test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Interaction not found"))
	}))
	defer server.Close()

	// Override BaseURL for testing
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Get status
	ctx := context.Background()
	_, err := client.GetInteractionStatus(ctx, "invalid-id")
	if err == nil {
		t.Error("GetInteractionStatus() error = nil, want not found error")
	}

	// Check it's the right error type
	var researchErr *ResearchError
	if !errors.As(err, &researchErr) {
		t.Errorf("error type = %T, want *ResearchError", err)
	}
}

func TestGetInteractionStatus_Completed(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := InteractionResponse{
			ID:     "interaction-123",
			Status: "completed",
			Outputs: []InteractionOutputItem{
				{Text: "Research report content here..."},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override BaseURL for testing
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Get status
	ctx := context.Background()
	status, err := client.GetInteractionStatus(ctx, "interaction-123")
	if err != nil {
		t.Fatalf("GetInteractionStatus() error = %v, want nil", err)
	}

	if status.Status != "completed" {
		t.Errorf("status.Status = %s, want completed", status.Status)
	}

	if len(status.Outputs) != 1 {
		t.Errorf("len(status.Outputs) = %d, want 1", len(status.Outputs))
	}
}

func TestHandleHTTPError(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantType   string
	}{
		{
			name:       "401 unauthorized",
			statusCode: 401,
			body:       "Invalid credentials",
			wantType:   "*research.ResearchError",
		},
		{
			name:       "429 rate limit",
			statusCode: 429,
			body:       "Too many requests",
			wantType:   "*research.ResearchError",
		},
		{
			name:       "404 not found",
			statusCode: 404,
			body:       "Resource not found",
			wantType:   "*research.HTTPError",
		},
		{
			name:       "500 server error",
			statusCode: 500,
			body:       "Internal server error",
			wantType:   "*research.HTTPError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.handleHTTPError(tt.statusCode, tt.body)
			if err == nil {
				t.Error("handleHTTPError() error = nil, want error")
			}

			errType := fmt.Sprintf("%T", err)
			if errType != tt.wantType {
				t.Errorf("error type = %s, want %s", errType, tt.wantType)
			}
		})
	}
}

func TestInteractionRequest_JSONMarshaling(t *testing.T) {
	req := InteractionRequest{
		Input:      "Test input",
		Agent:      "test-agent",
		Background: true,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded InteractionRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Input != req.Input {
		t.Errorf("decoded.Input = %s, want %s", decoded.Input, req.Input)
	}
	if decoded.Agent != req.Agent {
		t.Errorf("decoded.Agent = %s, want %s", decoded.Agent, req.Agent)
	}
	if decoded.Background != req.Background {
		t.Errorf("decoded.Background = %v, want %v", decoded.Background, req.Background)
	}
}

func TestInteractionResponse_JSONMarshaling(t *testing.T) {
	resp := InteractionResponse{
		ID:     "test-123",
		Status: "completed",
		Outputs: []InteractionOutputItem{
			{Text: "Output text"},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded InteractionResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.ID != resp.ID {
		t.Errorf("decoded.ID = %s, want %s", decoded.ID, resp.ID)
	}
	if decoded.Status != resp.Status {
		t.Errorf("decoded.Status = %s, want %s", decoded.Status, resp.Status)
	}
	if len(decoded.Outputs) != 1 {
		t.Fatalf("len(decoded.Outputs) = %d, want 1", len(decoded.Outputs))
	}
	if decoded.Outputs[0].Text != "Output text" {
		t.Errorf("decoded.Outputs[0].Text = %s, want 'Output text'", decoded.Outputs[0].Text)
	}
}
