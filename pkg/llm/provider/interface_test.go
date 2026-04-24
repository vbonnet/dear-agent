package provider

import (
	"errors"
	"testing"
)

func TestProviderError(t *testing.T) {
	t.Run("Error message format", func(t *testing.T) {
		underlyingErr := errors.New("connection timeout")
		err := NewProviderError("anthropic", "generate", underlyingErr)

		expected := "anthropic provider generate failed: connection timeout"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		underlyingErr := errors.New("rate limit exceeded")
		err := NewProviderError("vertexai-claude", "authenticate", underlyingErr)

		unwrapped := err.Unwrap()
		if !errors.Is(unwrapped, underlyingErr) {
			t.Errorf("Unwrap() = %v, want %v", unwrapped, underlyingErr)
		}
	})

	t.Run("Fields are accessible", func(t *testing.T) {
		underlyingErr := errors.New("invalid API key")
		err := NewProviderError("openrouter", "authenticate", underlyingErr)

		if err.Provider != "openrouter" {
			t.Errorf("Provider = %q, want %q", err.Provider, "openrouter")
		}
		if err.Operation != "authenticate" {
			t.Errorf("Operation = %q, want %q", err.Operation, "authenticate")
		}
		if !errors.Is(err.Err, underlyingErr) {
			t.Errorf("Err = %v, want %v", err.Err, underlyingErr)
		}
	})
}

func TestUsage(t *testing.T) {
	t.Run("Basic usage calculation", func(t *testing.T) {
		usage := Usage{
			InputTokens:  1000,
			OutputTokens: 500,
			TotalTokens:  1500,
			CostUSD:      0.015,
		}

		if usage.InputTokens != 1000 {
			t.Errorf("InputTokens = %d, want 1000", usage.InputTokens)
		}
		if usage.OutputTokens != 500 {
			t.Errorf("OutputTokens = %d, want 500", usage.OutputTokens)
		}
		if usage.TotalTokens != 1500 {
			t.Errorf("TotalTokens = %d, want 1500", usage.TotalTokens)
		}
		if usage.CostUSD != 0.015 {
			t.Errorf("CostUSD = %.6f, want 0.015000", usage.CostUSD)
		}
	})
}

func TestCapabilities(t *testing.T) {
	t.Run("Anthropic capabilities", func(t *testing.T) {
		caps := Capabilities{
			SupportsCaching:       true,
			SupportsStreaming:     true,
			MaxTokensPerRequest:   200000,
			MaxConcurrentRequests: 5,
			SupportedModels: []string{
				"claude-opus-4-6",
				"claude-3-5-sonnet-20241022",
				"claude-3-5-haiku-20241022",
			},
		}

		if !caps.SupportsCaching {
			t.Error("Expected caching support")
		}
		if !caps.SupportsStreaming {
			t.Error("Expected streaming support")
		}
		if caps.MaxTokensPerRequest != 200000 {
			t.Errorf("MaxTokensPerRequest = %d, want 200000", caps.MaxTokensPerRequest)
		}
		if len(caps.SupportedModels) != 3 {
			t.Errorf("SupportedModels length = %d, want 3", len(caps.SupportedModels))
		}
	})

	t.Run("Gemini capabilities", func(t *testing.T) {
		caps := Capabilities{
			SupportsCaching:       false, // Gemini doesn't support prompt caching
			SupportsStreaming:     true,
			MaxTokensPerRequest:   1000000, // 1M context window
			MaxConcurrentRequests: 10,
		}

		if caps.SupportsCaching {
			t.Error("Gemini should not support caching")
		}
		if caps.MaxTokensPerRequest != 1000000 {
			t.Errorf("MaxTokensPerRequest = %d, want 1000000", caps.MaxTokensPerRequest)
		}
	})
}

func TestGenerateRequest(t *testing.T) {
	t.Run("Complete request", func(t *testing.T) {
		req := &GenerateRequest{
			Prompt:       "What is the meaning of life?",
			Model:        "claude-3-5-sonnet-20241022",
			SystemPrompt: "You are a helpful assistant.",
			MaxTokens:    1024,
			Temperature:  0.7,
			Metadata: map[string]any{
				"user_id": "test-user",
				"session": "abc123",
			},
		}

		if req.Prompt == "" {
			t.Error("Prompt should not be empty")
		}
		if req.Model == "" {
			t.Error("Model should not be empty")
		}
		if req.MaxTokens != 1024 {
			t.Errorf("MaxTokens = %d, want 1024", req.MaxTokens)
		}
		if req.Temperature != 0.7 {
			t.Errorf("Temperature = %.2f, want 0.70", req.Temperature)
		}
		if len(req.Metadata) != 2 {
			t.Errorf("Metadata length = %d, want 2", len(req.Metadata))
		}
	})

	t.Run("Minimal request", func(t *testing.T) {
		req := &GenerateRequest{
			Prompt:    "Hello",
			MaxTokens: 100,
		}

		if req.Prompt != "Hello" {
			t.Errorf("Prompt = %q, want %q", req.Prompt, "Hello")
		}
		if req.Model != "" {
			t.Error("Model should be empty (use provider default)")
		}
		if req.SystemPrompt != "" {
			t.Error("SystemPrompt should be empty")
		}
		if req.Temperature != 0 {
			t.Error("Temperature should be zero (use provider default)")
		}
	})
}

func TestGenerateResponse(t *testing.T) {
	t.Run("Complete response", func(t *testing.T) {
		resp := &GenerateResponse{
			Text:  "The meaning of life is 42.",
			Model: "claude-3-5-sonnet-20241022",
			Usage: Usage{
				InputTokens:  15,
				OutputTokens: 8,
				TotalTokens:  23,
				CostUSD:      0.000023,
			},
			Metadata: map[string]any{
				"anthropic_id": "msg_123",
				"stop_reason":  "end_turn",
			},
		}

		if resp.Text == "" {
			t.Error("Text should not be empty")
		}
		if resp.Model == "" {
			t.Error("Model should not be empty")
		}
		if resp.Usage.TotalTokens != 23 {
			t.Errorf("TotalTokens = %d, want 23", resp.Usage.TotalTokens)
		}
		if len(resp.Metadata) != 2 {
			t.Errorf("Metadata length = %d, want 2", len(resp.Metadata))
		}
	})
}
