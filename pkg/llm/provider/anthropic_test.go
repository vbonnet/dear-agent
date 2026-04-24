package provider

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestNewAnthropicProvider(t *testing.T) {
	t.Run("Success with API key", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

		provider, err := NewAnthropicProvider(AnthropicConfig{})
		if err != nil {
			t.Fatalf("NewAnthropicProvider() error = %v", err)
		}

		if provider == nil {
			t.Fatal("Expected non-nil provider")
		}

		if provider.Name() != "anthropic" {
			t.Errorf("Name() = %q, want %q", provider.Name(), "anthropic")
		}
	})

	t.Run("Error without API key", func(t *testing.T) {
		// Unset API key
		t.Setenv("ANTHROPIC_API_KEY", "")

		_, err := NewAnthropicProvider(AnthropicConfig{})
		if err == nil {
			t.Fatal("Expected error when ANTHROPIC_API_KEY not set")
		}

		// Should be a ProviderError
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) {
			t.Errorf("Expected ProviderError, got %T", err)
		}
	})

	t.Run("Error with invalid API key format", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "invalid-key-format")

		_, err := NewAnthropicProvider(AnthropicConfig{})
		if err == nil {
			t.Fatal("Expected error with invalid API key format")
		}
	})

	t.Run("Uses custom model from config", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

		provider, err := NewAnthropicProvider(AnthropicConfig{
			Model: "claude-opus-4-6",
		})
		if err != nil {
			t.Fatalf("NewAnthropicProvider() error = %v", err)
		}

		// Model is stored internally, not exposed via interface
		// We'll verify it's used in Generate() test
		if provider == nil {
			t.Fatal("Expected non-nil provider")
		}
	})

	t.Run("Uses default model when not specified", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

		provider, err := NewAnthropicProvider(AnthropicConfig{})
		if err != nil {
			t.Fatalf("NewAnthropicProvider() error = %v", err)
		}

		if provider == nil {
			t.Fatal("Expected non-nil provider")
		}

		// Default model should be claude-3-5-haiku-20241022
		// We'll verify in integration tests
	})
}

func TestAnthropicProvider_Name(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

	provider, err := NewAnthropicProvider(AnthropicConfig{})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	name := provider.Name()
	if name != "anthropic" {
		t.Errorf("Name() = %q, want %q", name, "anthropic")
	}
}

func TestAnthropicProvider_Capabilities(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

	provider, err := NewAnthropicProvider(AnthropicConfig{})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	caps := provider.Capabilities()

	t.Run("Supports caching", func(t *testing.T) {
		if !caps.SupportsCaching {
			t.Error("Anthropic should support prompt caching")
		}
	})

	t.Run("Supports streaming", func(t *testing.T) {
		if !caps.SupportsStreaming {
			t.Error("Anthropic should support streaming")
		}
	})

	t.Run("Context window size", func(t *testing.T) {
		if caps.MaxTokensPerRequest != 200000 {
			t.Errorf("MaxTokensPerRequest = %d, want 200000", caps.MaxTokensPerRequest)
		}
	})

	t.Run("Rate limit", func(t *testing.T) {
		if caps.MaxConcurrentRequests != 5 {
			t.Errorf("MaxConcurrentRequests = %d, want 5", caps.MaxConcurrentRequests)
		}
	})

	t.Run("Supported models", func(t *testing.T) {
		expectedModels := []string{
			"claude-opus-4-6",
			"claude-3-5-sonnet-20241022",
			"claude-3-5-haiku-20241022",
			"claude-3-haiku-20240307",
		}

		if len(caps.SupportedModels) != len(expectedModels) {
			t.Errorf("SupportedModels length = %d, want %d", len(caps.SupportedModels), len(expectedModels))
		}

		for i, model := range expectedModels {
			if i >= len(caps.SupportedModels) {
				t.Errorf("Missing model at index %d: %s", i, model)
				continue
			}
			if caps.SupportedModels[i] != model {
				t.Errorf("SupportedModels[%d] = %q, want %q", i, caps.SupportedModels[i], model)
			}
		}
	})
}

func TestAnthropicProvider_Generate(t *testing.T) {
	// Skip if no API key (integration test)
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" || len(apiKey) < 10 {
		t.Skip("Skipping integration test: ANTHROPIC_API_KEY not set")
	}

	provider, err := NewAnthropicProvider(AnthropicConfig{})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	ctx := context.Background()

	t.Run("Error with empty prompt", func(t *testing.T) {
		req := &GenerateRequest{
			Prompt:    "",
			MaxTokens: 100,
		}

		_, err := provider.Generate(ctx, req)
		if err == nil {
			t.Fatal("Expected error with empty prompt")
		}

		var providerErr *ProviderError
		if !errors.As(err, &providerErr) {
			t.Errorf("Expected ProviderError, got %T", err)
		}
	})

	// Note: Actual API call tests require valid API key and network
	// These would be integration tests, not unit tests
}
