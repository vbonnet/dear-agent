package ranking

import (
	"fmt"
	"os"
	"sync"

	llmauth "github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// Factory creates and manages providers
type Factory struct {
	providers map[string]Provider
	config    *Config
	mu        sync.RWMutex // Thread-safe provider registration
}

// NewFactory initializes provider factory
func NewFactory(config *Config) (*Factory, error) {
	if config == nil {
		config = DefaultConfig()
	}

	f := &Factory{
		providers: make(map[string]Provider),
		config:    config,
	}

	if err := f.registerProviders(); err != nil {
		return nil, fmt.Errorf("failed to register providers: %w", err)
	}

	return f, nil
}

// registerProviders registers all available providers using pkg/llm/auth for detection
func (f *Factory) registerProviders() error {
	// Register in reverse precedence order (last registered = highest priority in auto-detect)

	// Local provider (always available)
	if err := f.Register(NewLocalProvider()); err != nil {
		return fmt.Errorf("failed to register local provider: %w", err)
	}

	// Detect auth method for Gemini
	geminiAuth := llmauth.DetectAuthMethod("gemini")
	if geminiAuth == llmauth.AuthVertexAI {
		// Vertex AI Gemini (requires Google Cloud credentials)
		provider, err := NewVertexAIGeminiProvider(f.config.Ecphory.Providers.VertexAI)
		if err == nil {
			// Best-effort registration: provider already validated by NewVertexAIGeminiProvider
			if regErr := f.Register(provider); regErr != nil {
				// Provider creation succeeded but registration failed (unexpected)
				return fmt.Errorf("failed to register Vertex AI Gemini provider: %w", regErr)
			}
		}
	}

	// Detect auth method for Anthropic/Claude
	anthropicAuth := llmauth.DetectAuthMethod("anthropic")
	switch anthropicAuth {
	case llmauth.AuthVertexAI:
		// Vertex AI Claude (requires Google Cloud credentials + us-east5)
		projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
		if projectID != "" {
			location := f.config.Ecphory.Providers.VertexAIClaude.Location
			if location == "us-east5" { // Claude only in us-east5
				provider, err := NewVertexAIClaudeProvider(f.config.Ecphory.Providers.VertexAIClaude)
				if err == nil {
					if regErr := f.Register(provider); regErr != nil {
						return fmt.Errorf("failed to register Vertex AI Claude provider: %w", regErr)
					}
				}
			}
		}
	case llmauth.AuthAPIKey:
		// Anthropic API Key
		provider, err := NewAnthropicProvider(f.config.Ecphory.Providers.Anthropic)
		if err == nil {
			if regErr := f.Register(provider); regErr != nil {
				return fmt.Errorf("failed to register Anthropic provider: %w", regErr)
			}
		}
	case llmauth.AuthNone:
		// No Anthropic auth configured; skip provider registration
	}

	return nil
}

// Register adds a provider to the factory
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

// GetProvider returns provider by name
func (f *Factory) GetProvider(name string) (Provider, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	p, ok := f.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", name)
	}
	return p, nil
}

// AutoDetect selects provider based on environment variables
// Precedence: Anthropic → Vertex Claude → Vertex Gemini → Local
func (f *Factory) AutoDetect() (Provider, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Check precedence order
	precedence := []string{
		"anthropic",
		"vertexai-claude",
		"vertexai-gemini",
		"local",
	}

	for _, name := range precedence {
		if p, ok := f.providers[name]; ok {
			return p, nil
		}
	}

	// Should never happen (local always available)
	return nil, fmt.Errorf("no providers available")
}

// ListProviders returns all registered provider names
func (f *Factory) ListProviders() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	names := make([]string, 0, len(f.providers))
	for name := range f.providers {
		names = append(names, name)
	}
	return names
}

// MustAutoDetect is like AutoDetect but panics on error
func (f *Factory) MustAutoDetect() Provider {
	p, err := f.AutoDetect()
	if err != nil {
		panic(err)
	}
	return p
}
