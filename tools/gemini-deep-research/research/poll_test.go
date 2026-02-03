package research

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExtractOutput_Success(t *testing.T) {
	client := &Client{}

	resp := &InteractionResponse{
		ID:     "test-123",
		Status: "completed",
		Outputs: []InteractionOutputItem{
			{Text: strings.Repeat("a", 150)}, // >100 chars
		},
	}

	output, err := client.extractOutput("test-123", resp)
	if err != nil {
		t.Fatalf("extractOutput() error = %v, want nil", err)
	}

	if len(output) < 100 {
		t.Errorf("output length = %d, want >= 100", len(output))
	}
}

func TestExtractOutput_EmptyOutput(t *testing.T) {
	client := &Client{}

	resp := &InteractionResponse{
		ID:     "test-123",
		Status: "completed",
		Outputs: []InteractionOutputItem{
			{Text: "short"}, // <100 chars
		},
	}

	_, err := client.extractOutput("test-123", resp)
	if err == nil {
		t.Error("extractOutput() error = nil, want error for short output")
	}

	var researchErr *ResearchError
	if !errors.As(err, &researchErr) {
		t.Errorf("error type = %T, want *ResearchError", err)
	}
}

func TestExtractOutput_NoOutputs(t *testing.T) {
	client := &Client{}

	resp := &InteractionResponse{
		ID:      "test-123",
		Status:  "completed",
		Outputs: []InteractionOutputItem{},
	}

	_, err := client.extractOutput("test-123", resp)
	if err == nil {
		t.Error("extractOutput() error = nil, want error for missing outputs")
	}
}

func TestLogStatus(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name    string
		status  string
		elapsed time.Duration
	}{
		{
			name:    "in_progress status",
			status:  "in_progress",
			elapsed: 65 * time.Second, // 1m 5s
		},
		{
			name:    "pending status",
			status:  "pending",
			elapsed: 30 * time.Second,
		},
		{
			name:    "processing status",
			status:  "processing",
			elapsed: 120 * time.Second, // 2m 0s
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just ensure it doesn't panic
			client.logStatus(tt.status, tt.elapsed)
		})
	}
}

func TestPollStatus_ImmediateCompletion(t *testing.T) {
	// Create test server that returns completed status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := InteractionResponse{
			ID:     "interaction-123",
			Status: "completed",
			Outputs: []InteractionOutputItem{
				{Text: strings.Repeat("Research report content here. ", 10)}, // >100 chars
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override BaseURL
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Poll status
	ctx := context.Background()
	config := &PollConfig{
		IntervalSeconds: 1,
		TimeoutMinutes:  1,
	}

	report, err := client.PollStatus(ctx, "interaction-123", config)
	if err != nil {
		t.Fatalf("PollStatus() error = %v, want nil", err)
	}

	if len(report) < 100 {
		t.Errorf("report length = %d, want >= 100", len(report))
	}
}

func TestPollStatus_PollingUntilComplete(t *testing.T) {
	// Track number of polls
	pollCount := 0

	// Create test server that transitions from pending to completed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pollCount++

		var resp InteractionResponse
		if pollCount < 3 {
			// First two polls: in_progress
			resp = InteractionResponse{
				ID:     "interaction-123",
				Status: "in_progress",
			}
		} else {
			// Third poll: completed
			resp = InteractionResponse{
				ID:     "interaction-123",
				Status: "completed",
				Outputs: []InteractionOutputItem{
					{Text: strings.Repeat("Research complete. ", 10)},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override BaseURL
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Poll status with short interval
	ctx := context.Background()
	config := &PollConfig{
		IntervalSeconds: 1, // 1 second interval for fast test
		TimeoutMinutes:  1,
	}

	report, err := client.PollStatus(ctx, "interaction-123", config)
	if err != nil {
		t.Fatalf("PollStatus() error = %v, want nil", err)
	}

	if pollCount < 3 {
		t.Errorf("pollCount = %d, want >= 3", pollCount)
	}

	if len(report) == 0 {
		t.Error("report is empty, want non-empty report")
	}
}

func TestPollStatus_Timeout(t *testing.T) {
	// Create test server that never completes
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := InteractionResponse{
			ID:     "interaction-123",
			Status: "in_progress",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override BaseURL
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Poll status with very short timeout
	ctx := context.Background()
	config := &PollConfig{
		IntervalSeconds: 1,
		TimeoutMinutes:  0, // This will be 0 minutes = immediate timeout
	}

	// Adjust timeout to a few seconds for test
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_, err := client.PollStatus(ctx, "interaction-123", config)
	if err == nil {
		t.Error("PollStatus() error = nil, want timeout error")
	}

	// Check for timeout error
	if !strings.Contains(err.Error(), "timeout") && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want timeout-related error", err)
	}
}

func TestPollStatus_Failed(t *testing.T) {
	// Create test server that returns failed status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := InteractionResponse{
			ID:     "interaction-123",
			Status: "failed",
			Error:  "Research processing failed",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override BaseURL
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Poll status
	ctx := context.Background()
	config := &PollConfig{
		IntervalSeconds: 1,
		TimeoutMinutes:  1,
	}

	_, err := client.PollStatus(ctx, "interaction-123", config)
	if err == nil {
		t.Error("PollStatus() error = nil, want failed error")
	}

	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error = %v, want error containing 'failed'", err)
	}
}

func TestPollStatus_DefaultConfig(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := InteractionResponse{
			ID:     "interaction-123",
			Status: "completed",
			Outputs: []InteractionOutputItem{
				{Text: strings.Repeat("Report. ", 20)},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override BaseURL
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Poll status with nil config (should use default)
	ctx := context.Background()
	report, err := client.PollStatus(ctx, "interaction-123", nil)
	if err != nil {
		t.Fatalf("PollStatus() error = %v, want nil", err)
	}

	if report == "" {
		t.Error("report is empty, want non-empty")
	}
}

func TestRunResearch_Success(t *testing.T) {
	// Track request count
	requestCount := 0

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if r.Method == http.MethodPost {
			// Create interaction
			resp := InteractionResponse{
				ID:     "interaction-456",
				Status: "pending",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else {
			// Poll status - return completed immediately
			resp := InteractionResponse{
				ID:     "interaction-456",
				Status: "completed",
				Outputs: []InteractionOutputItem{
					{Text: strings.Repeat("Full research report. ", 15)},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	// Override BaseURL
	oldBaseURL := BaseURL
	BaseURL = server.URL
	defer func() { BaseURL = oldBaseURL }()

	// Create client
	client := &Client{
		httpClient: &http.Client{},
		apiKey:     "test-key",
	}

	// Run research
	ctx := context.Background()
	config := &PollConfig{
		IntervalSeconds: 1,
		TimeoutMinutes:  1,
	}

	report, err := client.RunResearch(ctx, []string{"topic1", "topic2"}, config)
	if err != nil {
		t.Fatalf("RunResearch() error = %v, want nil", err)
	}

	if report == "" {
		t.Error("report is empty, want non-empty")
	}

	// Should have at least 2 requests: POST to create, GET to poll
	if requestCount < 2 {
		t.Errorf("requestCount = %d, want >= 2", requestCount)
	}
}

func TestDefaultPollConfig(t *testing.T) {
	config := DefaultPollConfig()

	if config.IntervalSeconds != 10 {
		t.Errorf("IntervalSeconds = %d, want 10", config.IntervalSeconds)
	}

	if config.TimeoutMinutes != 60 {
		t.Errorf("TimeoutMinutes = %d, want 60", config.TimeoutMinutes)
	}
}
