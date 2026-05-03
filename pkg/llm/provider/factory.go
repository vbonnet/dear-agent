package provider

import (
	"fmt"
	"sync"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// Factory creates and manages LLM providers with authentication hierarchy.
//
// The factory implements provider auto-detection based on available authentication
// credentials, following the hierarchy defined in pkg/llm/auth:
//  1. Vertex AI (ADC) - Preferred for cloud environments
//  2. API Keys - Direct provider authentication
//
// Example usage:
//
//	factory := NewFactory()
//
//	// Create provider with auto-detection
//	provider, err := factory.NewProvider("anthropic", "")
//	if err != nil {
//	    // Handle error
//	}
//
//	// Or get specific provider by name
//	provider, err := factory.GetProvider("vertexai-claude")
type Factory struct {
	providers map[string]Provider
	mu        sync.RWMutex // Thread-safe provider access
}

// NewFactory initializes the provider factory.
func NewFactory() *Factory {
	return &Factory{
		providers: make(map[string]Provider),
	}
}

// NewProvider creates a provider for the given family and model.
//
// The function uses auth.DetectAuthMethod() to determine the appropriate
// authentication method and returns the corresponding provider implementation.
//
// Parameters:
//   - providerFamily: The LLM provider family ("anthropic", "gemini", "openrouter")
//   - model: Optional model identifier. If empty, uses provider default.
//
// Returns:
//   - Provider implementation based on authentication hierarchy
//   - Error if no authentication is available or provider creation fails
//
// Provider selection logic:
//
// For "anthropic" or "claude":
//   - Vertex AI available (us-east5) → VertexAIClaudeProvider
//   - Anthropic API key available → AnthropicProvider
//   - Neither available → error
//
// For "gemini" or "google":
//   - Vertex AI available → VertexAIGeminiProvider (future)
//   - Gemini API key available → GeminiProvider (future)
//   - Neither available → error
//
// For "openrouter":
//   - OpenRouter API key available → OpenRouterProvider (future)
//   - No key → error
//
// For "ollama" or "local":
//   - Always → OllamaProvider (no authentication required)
//   - Endpoint defaults to http://localhost:11434 (override via OLLAMA_HOST)
//
// Example:
//
//	// In GCP with ADC configured
//	provider, err := factory.NewProvider("anthropic", "claude-3-5-sonnet-20241022")
//	// Returns: VertexAIClaudeProvider
//
//	// With ANTHROPIC_API_KEY set
//	provider, err := factory.NewProvider("anthropic", "")
//	// Returns: AnthropicProvider with default model
func (f *Factory) NewProvider(providerFamily, model string) (Provider, error) {
	// Detect authentication method
	authMethod := auth.DetectAuthMethod(providerFamily)

	switch providerFamily {
	case "anthropic", "claude":
		return f.newAnthropicProvider(authMethod, model)

	case "gemini", "google":
		return f.newGeminiProvider(authMethod, model)

	case "openrouter":
		return f.newOpenRouterProvider(authMethod, model)

	case "ollama", "local":
		return f.newOllamaProvider(model)

	default:
		return nil, fmt.Errorf("unsupported provider family: %s", providerFamily)
	}
}

// newAnthropicProvider creates Anthropic/Claude provider based on auth hierarchy.
func (f *Factory) newAnthropicProvider(authMethod auth.AuthMethod, model string) (Provider, error) {
	switch authMethod {
	case auth.AuthVertexAI:
		// Try Vertex AI Claude (only in us-east5)
		config := VertexAIClaudeConfig{
			Location: "us-east5", // Claude only available in us-east5
			Model:    model,
		}
		provider, err := NewVertexAIClaudeProvider(config)
		if err == nil {
			return provider, nil
		}
		// If Vertex AI fails, fall through to try API key
		// This handles cases where GOOGLE_CLOUD_PROJECT is set but ADC fails

	case auth.AuthAPIKey:
		// Use Anthropic API with API key
		config := AnthropicConfig{
			Model: model,
		}
		return NewAnthropicProvider(config)

	case auth.AuthLocal:
		return nil, fmt.Errorf("anthropic does not support local authentication")

	case auth.AuthNone:
		return nil, fmt.Errorf("no authentication available for Anthropic (need GOOGLE_CLOUD_PROJECT or ANTHROPIC_API_KEY)")
	}

	return nil, fmt.Errorf("failed to create Anthropic provider")
}

// newGeminiProvider creates Gemini/Google provider based on auth hierarchy.
func (f *Factory) newGeminiProvider(authMethod auth.AuthMethod, model string) (Provider, error) {
	switch authMethod {
	case auth.AuthVertexAI:
		// TODO: Implement VertexAIGeminiProvider
		return nil, fmt.Errorf("Vertex AI Gemini provider not yet implemented")

	case auth.AuthAPIKey:
		// TODO: Implement GeminiProvider
		return nil, fmt.Errorf("gemini API provider not yet implemented")

	case auth.AuthLocal:
		return nil, fmt.Errorf("gemini does not support local authentication")

	case auth.AuthNone:
		return nil, fmt.Errorf("no authentication available for Gemini (need GOOGLE_CLOUD_PROJECT or GEMINI_API_KEY)")
	}

	return nil, fmt.Errorf("failed to create Gemini provider")
}

// newOpenRouterProvider creates OpenRouter provider based on auth hierarchy.
func (f *Factory) newOpenRouterProvider(authMethod auth.AuthMethod, model string) (Provider, error) {
	switch authMethod {
	case auth.AuthAPIKey:
		// TODO: Implement OpenRouterProvider
		return nil, fmt.Errorf("OpenRouter provider not yet implemented")

	case auth.AuthNone:
		return nil, fmt.Errorf("no authentication available for OpenRouter (need OPENROUTER_API_KEY)")

	case auth.AuthVertexAI:
		return nil, fmt.Errorf("OpenRouter only supports API key authentication")

	case auth.AuthLocal:
		return nil, fmt.Errorf("openrouter does not support local authentication")
	}

	return nil, fmt.Errorf("unsupported auth method for OpenRouter")
}

// newOllamaProvider creates an Ollama local provider.
//
// No authentication is required. The endpoint is resolved from OllamaConfig
// or OLLAMA_HOST environment variable (default: http://localhost:11434).
func (f *Factory) newOllamaProvider(model string) (Provider, error) {
	config := OllamaConfig{
		Model: model,
	}
	return NewOllamaProvider(config)
}

// Register adds a provider to the factory registry.
//
// This allows manual registration of provider instances for testing
// or custom provider implementations.
//
// Parameters:
//   - provider: Provider implementation to register
//
// Returns:
//   - Error if provider is nil or has empty name
func (f *Factory) Register(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	name := provider.Name()
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	f.providers[name] = provider
	return nil
}

// GetProvider returns a registered provider by name.
//
// This is useful for retrieving manually registered providers or
// testing with mock providers.
//
// Parameters:
//   - name: Provider name (e.g., "anthropic", "vertexai-claude")
//
// Returns:
//   - Provider instance
//   - Error if provider not found
func (f *Factory) GetProvider(name string) (Provider, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	p, ok := f.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", name)
	}
	return p, nil
}

// ListProviders returns all registered provider names.
//
// Returns:
//   - Slice of provider names currently in the registry
func (f *Factory) ListProviders() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	names := make([]string, 0, len(f.providers))
	for name := range f.providers {
		names = append(names, name)
	}
	return names
}

// NewProviderWithGuardrails creates a provider wrapped with circuit breaker and rate limiting.
func (f *Factory) NewProviderWithGuardrails(providerFamily, model string, cbCfg CircuitBreakerConfig, rlCfg RateLimiterConfig) (Provider, error) {
	p, err := f.NewProvider(providerFamily, model)
	if err != nil {
		return nil, err
	}
	return WrapWithGuardrails(p, cbCfg, rlCfg), nil
}

// WrapWithGuardrails wraps a provider with rate limiting and circuit breaker.
// Order: caller → RateLimiter → CircuitBreaker → provider
func WrapWithGuardrails(p Provider, cbCfg CircuitBreakerConfig, rlCfg RateLimiterConfig) Provider {
	cb := NewCircuitBreaker(p, cbCfg)
	rl := NewRateLimitedProvider(cb, rlCfg)
	return rl
}
