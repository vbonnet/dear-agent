// Package delegation provides strategies for LLM agent execution across different environments.
//
// This file implements the ExternalAPIStrategy for direct SDK calls to LLM providers.
package delegation

import (
	"context"
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// ExternalAPIStrategy implements direct SDK calls to LLM providers.
//
// This strategy makes direct API calls using either:
//   - Vertex AI SDK with Application Default Credentials (ADC)
//   - Provider-specific SDKs with API keys
//
// The strategy detects the appropriate authentication method using the auth
// package hierarchy and falls back to API keys when Vertex AI is not available.
//
// This is the fallback strategy (Available() always returns true) ensuring that
// LLM requests can always be executed even when harness Agent tools and CLI
// binaries are not available.
//
// Note: Actual SDK integration is implemented in Phase 3. This Phase 2
// implementation provides the structure and authentication layer.
type ExternalAPIStrategy struct {
	// provider is the LLM provider family (e.g., "anthropic", "gemini", "openrouter").
	provider string
}

// NewExternalAPIStrategy creates a new ExternalAPIStrategy for the specified provider.
//
// Parameters:
//   - provider: The LLM provider family (e.g., "anthropic", "gemini", "openrouter")
//
// Returns:
//   - *ExternalAPIStrategy: A new strategy instance
//
// Example:
//
//	strategy := delegation.NewExternalAPIStrategy("anthropic")
//	output, err := strategy.Execute(ctx, &delegation.AgentInput{
//	    Prompt: "What is 2+2?",
//	    Model: "claude-opus-4-6",
//	    Provider: "anthropic",
//	})
func NewExternalAPIStrategy(provider string) *ExternalAPIStrategy {
	return &ExternalAPIStrategy{provider: provider}
}

// Name returns the strategy name for logging and debugging.
//
// Returns:
//   - string: Always returns "ExternalAPI"
func (s *ExternalAPIStrategy) Name() string {
	return "ExternalAPI"
}

// Available checks if this strategy can be used in the current environment.
//
// The ExternalAPIStrategy is always available as it serves as the fallback
// strategy when other strategies (SubAgent, Headless) are not available.
//
// Returns:
//   - bool: Always returns true
func (s *ExternalAPIStrategy) Available() bool {
	// Always available as fallback strategy
	return true
}

// Execute runs an LLM agent request using direct SDK calls.
//
// The strategy:
//  1. Detects the appropriate authentication method using auth.DetectAuthMethod()
//  2. Routes to Vertex AI SDK if ADC is available (preferred)
//  3. Falls back to provider-specific API with API keys
//  4. Returns an error if no authentication is available
//
// Authentication flow:
//   - Vertex AI: Uses Application Default Credentials (no explicit key needed)
//   - API Key: Retrieves key from environment and validates format
//   - None: Returns error indicating missing credentials
//
// Parameters:
//   - ctx: Context for cancellation and deadlines
//   - input: Agent request containing prompt, model, and parameters
//
// Returns:
//   - *AgentOutput: The agent response with text, model, usage stats, and metadata
//   - error: StrategyError if authentication fails or SDK call fails
//
// Phase 2 Implementation:
// This is a placeholder implementation that validates authentication and returns
// a descriptive message indicating which authentication method would be used.
// Phase 3 will implement actual SDK integration with:
//   - Vertex AI Go SDK (cloud.google.com/go/vertexai)
//   - Anthropic Go SDK (github.com/anthropics/anthropic-sdk-go)
//   - Google Generative AI SDK (github.com/google/generative-ai-go)
//   - OpenRouter HTTP API
//
// Example:
//
//	strategy := delegation.NewExternalAPIStrategy("anthropic")
//	output, err := strategy.Execute(ctx, &delegation.AgentInput{
//	    Prompt: "What is the capital of France?",
//	    Model: "claude-opus-4-6",
//	    Provider: "anthropic",
//	    MaxTokens: 1024,
//	    Temperature: 0.7,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(output.Text)
func (s *ExternalAPIStrategy) Execute(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
	// Detect authentication method for this provider
	authMethod := auth.DetectAuthMethod(s.provider)

	switch authMethod {
	case auth.AuthVertexAI:
		// Vertex AI SDK call with Application Default Credentials
		// Phase 3: Implement actual Vertex AI SDK integration
		//   - For Anthropic: Use Vertex AI Claude API
		//   - For Google/Gemini: Use Vertex AI Gemini API
		//   - Uses cloud.google.com/go/vertexai
		return &AgentOutput{
			Text:     "External API (Vertex AI) not yet implemented - Phase 3",
			Model:    input.Model,
			Provider: input.Provider,
			Strategy: "ExternalAPI",
			Metadata: map[string]any{
				"auth":      "vertex-ai",
				"phase":     "2-placeholder",
				"next-step": "Phase 3: Integrate Vertex AI SDK",
			},
		}, nil

	case auth.AuthAPIKey:
		// Direct API call with provider-specific API key
		// Phase 3: Implement actual provider SDK integration
		//   - Anthropic: github.com/anthropics/anthropic-sdk-go
		//   - Gemini: github.com/google/generative-ai-go
		//   - OpenRouter: HTTP API (no official Go SDK)

		// Retrieve API key from environment
		key, err := auth.GetAPIKey(s.provider)
		if err != nil {
			return nil, NewStrategyError("ExternalAPI", "get-api-key", err)
		}

		// Validate API key format
		if err := auth.ValidateAPIKey(s.provider, key); err != nil {
			return nil, NewStrategyError("ExternalAPI", "validate-api-key", err)
		}

		// Phase 2: Return placeholder response with sanitized key
		return &AgentOutput{
			Text:     "External API (API Key) not yet implemented - Phase 3",
			Model:    input.Model,
			Provider: input.Provider,
			Strategy: "ExternalAPI",
			Metadata: map[string]any{
				"auth":      "api-key",
				"key":       auth.SanitizeKey(key),
				"phase":     "2-placeholder",
				"next-step": "Phase 3: Integrate provider SDKs",
			},
		}, nil

	case auth.AuthLocal:
		// Local providers (e.g., Ollama) require no authentication
		return &AgentOutput{
			Text:     "External API (Local) not yet implemented - Phase 3",
			Model:    input.Model,
			Provider: input.Provider,
			Strategy: "ExternalAPI",
			Metadata: map[string]any{
				"auth":      "local",
				"phase":     "2-placeholder",
				"next-step": "Phase 3: Integrate local provider",
			},
		}, nil

	case auth.AuthNone:
		// No authentication available for this provider
		return nil, NewStrategyError(
			"ExternalAPI",
			"auth",
			fmt.Errorf("no authentication available for %s", s.provider),
		)

	default:
		// Unknown authentication method (should never happen)
		return nil, NewStrategyError(
			"ExternalAPI",
			"auth",
			fmt.Errorf("unknown auth method: %v", authMethod),
		)
	}
}
