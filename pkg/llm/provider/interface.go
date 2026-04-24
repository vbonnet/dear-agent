// Package provider implements LLM provider abstraction for unified agent execution.
//
// This package defines the Provider interface and common types for interacting
// with different LLM providers (Anthropic, Vertex AI, OpenRouter).
//
// Providers are created via NewProvider() factory which auto-detects
// authentication and returns the appropriate implementation.
package provider

import (
	"context"
)

// Provider abstracts LLM provider for text generation.
//
// Implementations handle provider-specific SDK calls, authentication,
// rate limiting, and error handling.
//
// Available implementations:
//   - AnthropicProvider (via API key)
//   - VertexAIClaudeProvider (via Vertex AI ADC)
//   - VertexAIGeminiProvider (via Vertex AI ADC)
//   - OpenRouterProvider (optional, via API key)
type Provider interface {
	// Name returns provider identifier (e.g., "anthropic", "vertexai-claude")
	Name() string

	// Generate executes text generation with the provider
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// Capabilities returns provider capabilities
	Capabilities() Capabilities
}

// GenerateRequest contains parameters for LLM text generation.
type GenerateRequest struct {
	// Prompt is the user message or instruction
	Prompt string

	// Model is the specific model identifier (e.g., "claude-3-5-sonnet-20241022")
	Model string

	// SystemPrompt is optional system instruction
	SystemPrompt string

	// MaxTokens is the maximum response length
	MaxTokens int

	// Temperature controls randomness (0.0 = deterministic, 1.0 = creative)
	Temperature float64

	// Metadata contains additional context for logging/tracking
	Metadata map[string]any
}

// GenerateResponse contains the LLM response and usage information.
type GenerateResponse struct {
	// Text is the generated response
	Text string

	// Model is the actual model used (may differ from request if overridden)
	Model string

	// Usage contains token consumption and cost
	Usage Usage

	// Metadata contains provider-specific response data
	Metadata map[string]any
}

// Usage tracks token consumption and cost.
type Usage struct {
	// InputTokens consumed by the prompt
	InputTokens int

	// OutputTokens generated in the response
	OutputTokens int

	// TotalTokens is InputTokens + OutputTokens
	TotalTokens int

	// CostUSD is the estimated cost in US dollars
	CostUSD float64
}

// Capabilities describes provider features.
type Capabilities struct {
	// SupportsCaching indicates if prompt caching is available
	SupportsCaching bool

	// SupportsStreaming indicates if streaming responses are supported
	SupportsStreaming bool

	// MaxTokensPerRequest is the context window size
	MaxTokensPerRequest int

	// MaxConcurrentRequests is the rate limit
	MaxConcurrentRequests int

	// SupportedModels lists available model identifiers
	SupportedModels []string
}

// ProviderError wraps provider-specific errors with context.
type ProviderError struct {
	Provider  string // Provider name
	Operation string // Operation that failed (e.g., "generate", "authenticate")
	Err       error  // Underlying error
}

func (e *ProviderError) Error() string {
	return e.Provider + " provider " + e.Operation + " failed: " + e.Err.Error()
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// NewProviderError creates a ProviderError with context.
func NewProviderError(provider, operation string, err error) *ProviderError {
	return &ProviderError{
		Provider:  provider,
		Operation: operation,
		Err:       err,
	}
}
