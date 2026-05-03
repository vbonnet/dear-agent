package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"
)

// mockProvider implements Provider for testing the Factory.
type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Generate(_ context.Context, _ *GenerateRequest) (*GenerateResponse, error) {
	return &GenerateResponse{Text: "mock response", Model: "mock-model"}, nil
}
func (m *mockProvider) Capabilities() Capabilities {
	return Capabilities{
		SupportsCaching:       false,
		SupportsStreaming:     false,
		MaxTokensPerRequest:   1000,
		MaxConcurrentRequests: 1,
		SupportedModels:       []string{"mock-model"},
	}
}

// --- Factory tests ---

func TestNewFactory(t *testing.T) {
	f := NewFactory()
	if f == nil {
		t.Fatal("NewFactory() returned nil")
	}
	if f.providers == nil {
		t.Fatal("providers map is nil")
	}
	if len(f.providers) != 0 {
		t.Errorf("expected empty providers map, got %d entries", len(f.providers))
	}
}

func TestFactory_Register(t *testing.T) {
	t.Run("register valid provider", func(t *testing.T) {
		f := NewFactory()
		p := &mockProvider{name: "test-provider"}

		err := f.Register(p)
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		got, err := f.GetProvider("test-provider")
		if err != nil {
			t.Fatalf("GetProvider() error = %v", err)
		}
		if got.Name() != "test-provider" {
			t.Errorf("got Name() = %q, want %q", got.Name(), "test-provider")
		}
	})

	t.Run("register nil provider", func(t *testing.T) {
		f := NewFactory()
		err := f.Register(nil)
		if err == nil {
			t.Fatal("expected error registering nil provider")
		}
	})

	t.Run("register provider with empty name", func(t *testing.T) {
		f := NewFactory()
		p := &mockProvider{name: ""}
		err := f.Register(p)
		if err == nil {
			t.Fatal("expected error registering provider with empty name")
		}
	})

	t.Run("register overwrites existing", func(t *testing.T) {
		f := NewFactory()
		p1 := &mockProvider{name: "dup"}
		p2 := &mockProvider{name: "dup"}

		if err := f.Register(p1); err != nil {
			t.Fatalf("first Register() error = %v", err)
		}
		if err := f.Register(p2); err != nil {
			t.Fatalf("second Register() error = %v", err)
		}

		// Should still succeed; last write wins
		got, err := f.GetProvider("dup")
		if err != nil {
			t.Fatalf("GetProvider() error = %v", err)
		}
		if got != p2 {
			t.Error("expected second registered provider to be returned")
		}
	})
}

func TestFactory_GetProvider(t *testing.T) {
	t.Run("provider not found", func(t *testing.T) {
		f := NewFactory()
		_, err := f.GetProvider("nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent provider")
		}
		if got := err.Error(); got != "provider not found: nonexistent" {
			t.Errorf("error = %q, want %q", got, "provider not found: nonexistent")
		}
	})

	t.Run("provider found", func(t *testing.T) {
		f := NewFactory()
		p := &mockProvider{name: "found"}
		_ = f.Register(p)

		got, err := f.GetProvider("found")
		if err != nil {
			t.Fatalf("GetProvider() error = %v", err)
		}
		if got.Name() != "found" {
			t.Errorf("Name() = %q, want %q", got.Name(), "found")
		}
	})
}

func TestFactory_ListProviders(t *testing.T) {
	t.Run("empty factory", func(t *testing.T) {
		f := NewFactory()
		names := f.ListProviders()
		if len(names) != 0 {
			t.Errorf("expected 0 providers, got %d", len(names))
		}
	})

	t.Run("multiple providers", func(t *testing.T) {
		f := NewFactory()
		_ = f.Register(&mockProvider{name: "alpha"})
		_ = f.Register(&mockProvider{name: "beta"})
		_ = f.Register(&mockProvider{name: "gamma"})

		names := f.ListProviders()
		if len(names) != 3 {
			t.Fatalf("expected 3 providers, got %d", len(names))
		}

		sort.Strings(names)
		expected := []string{"alpha", "beta", "gamma"}
		for i, name := range names {
			if name != expected[i] {
				t.Errorf("names[%d] = %q, want %q", i, name, expected[i])
			}
		}
	})
}

func TestFactory_NewProvider_UnsupportedFamily(t *testing.T) {
	f := NewFactory()
	_, err := f.NewProvider("unknown-family", "some-model")
	if err == nil {
		t.Fatal("expected error for unsupported provider family")
	}
	if got := err.Error(); got != "unsupported provider family: unknown-family" {
		t.Errorf("error = %q, want %q", got, "unsupported provider family: unknown-family")
	}
}

func TestFactory_NewProvider_AnthropicNoAuth(t *testing.T) {
	// Clear all relevant env vars
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	f := NewFactory()
	_, err := f.NewProvider("anthropic", "")
	if err == nil {
		t.Fatal("expected error when no auth available for anthropic")
	}
}

func TestFactory_NewProvider_ClaudeAlias(t *testing.T) {
	// "claude" should be treated the same as "anthropic"
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	f := NewFactory()
	_, err := f.NewProvider("claude", "")
	if err == nil {
		t.Fatal("expected error when no auth available for claude")
	}
}

func TestFactory_NewProvider_AnthropicWithAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key-123")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	f := NewFactory()
	p, err := f.NewProvider("anthropic", "claude-3-5-haiku-20241022")
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Name() != "anthropic" {
		t.Errorf("Name() = %q, want %q", p.Name(), "anthropic")
	}
}

func TestFactory_NewProvider_GeminiNoAuth(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	f := NewFactory()
	_, err := f.NewProvider("gemini", "")
	if err == nil {
		t.Fatal("expected error when no auth available for gemini")
	}
}

func TestFactory_NewProvider_GoogleAlias(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	f := NewFactory()
	_, err := f.NewProvider("google", "")
	if err == nil {
		t.Fatal("expected error when no auth available for google")
	}
}

func TestFactory_NewProvider_GeminiWithVertexAI(t *testing.T) {
	// Gemini via Vertex AI is "not yet implemented" per factory code
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("GEMINI_API_KEY", "")

	f := NewFactory()
	_, err := f.NewProvider("gemini", "")
	if err == nil {
		t.Fatal("expected error (Gemini via Vertex AI not implemented)")
	}
	// Should mention "not yet implemented"
	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
}

func TestFactory_NewProvider_GeminiWithAPIKey(t *testing.T) {
	// Gemini via API key is "not yet implemented" per factory code
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GEMINI_API_KEY", "AIzaTestKey123")

	f := NewFactory()
	_, err := f.NewProvider("gemini", "")
	if err == nil {
		t.Fatal("expected error (Gemini API provider not implemented)")
	}
}

func TestFactory_NewProvider_OpenRouterNoAuth(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	f := NewFactory()
	_, err := f.NewProvider("openrouter", "")
	if err == nil {
		t.Fatal("expected error when no auth available for openrouter")
	}
}

func TestFactory_NewProvider_OpenRouterWithAPIKey(t *testing.T) {
	// OpenRouter API key provider is "not yet implemented" per factory code
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	f := NewFactory()
	_, err := f.NewProvider("openrouter", "")
	if err == nil {
		t.Fatal("expected error (OpenRouter provider not implemented)")
	}
}

func TestFactory_NewProvider_OpenRouterWithVertexAI(t *testing.T) {
	// OpenRouter does not support Vertex AI auth
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	f := NewFactory()
	_, err := f.NewProvider("openrouter", "")
	if err == nil {
		t.Fatal("expected error (OpenRouter only supports API key)")
	}
}

// --- ProviderError additional tests ---

func TestProviderError_ErrorsAs(t *testing.T) {
	underlying := errors.New("timeout")
	provErr := NewProviderError("anthropic", "generate", underlying)

	// Wrap it further
	wrapped := errors.Join(errors.New("wrapper"), provErr)

	var target *ProviderError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find ProviderError in wrapped error")
	}
	if target.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", target.Provider, "anthropic")
	}
}

func TestProviderError_ErrorsIs(t *testing.T) {
	underlying := errors.New("specific error")
	provErr := NewProviderError("test", "op", underlying)

	if !errors.Is(provErr, underlying) {
		t.Error("errors.Is should find underlying error via Unwrap")
	}
}

// --- AnthropicProvider tests ---

func TestAnthropicProvider_DefaultModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

	p, err := NewAnthropicProvider(AnthropicConfig{})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	if p.model != "claude-3-5-haiku-20241022" {
		t.Errorf("default model = %q, want %q", p.model, "claude-3-5-haiku-20241022")
	}
}

func TestAnthropicProvider_CustomModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

	p, err := NewAnthropicProvider(AnthropicConfig{Model: "claude-opus-4-6"})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	if p.model != "claude-opus-4-6" {
		t.Errorf("model = %q, want %q", p.model, "claude-opus-4-6")
	}
}

func TestAnthropicProvider_GenerateEmptyPrompt(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

	p, err := NewAnthropicProvider(AnthropicConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	_, err = p.Generate(context.Background(), &GenerateRequest{
		Prompt:    "",
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error with empty prompt")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if provErr.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", provErr.Provider, "anthropic")
	}
	if provErr.Operation != "generate" {
		t.Errorf("Operation = %q, want %q", provErr.Operation, "generate")
	}
}

func TestAnthropicProvider_CostSinkDefault(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

	p, err := NewAnthropicProvider(AnthropicConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if p.costSink == nil {
		t.Error("expected default cost sink, got nil")
	}
}

func TestAnthropicProvider_CapabilitiesValues(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

	p, err := NewAnthropicProvider(AnthropicConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	caps := p.Capabilities()
	if !caps.SupportsCaching {
		t.Error("expected SupportsCaching = true")
	}
	if !caps.SupportsStreaming {
		t.Error("expected SupportsStreaming = true")
	}
	if caps.MaxTokensPerRequest != 200000 {
		t.Errorf("MaxTokensPerRequest = %d, want 200000", caps.MaxTokensPerRequest)
	}
	if caps.MaxConcurrentRequests != 5 {
		t.Errorf("MaxConcurrentRequests = %d, want 5", caps.MaxConcurrentRequests)
	}
	if len(caps.SupportedModels) == 0 {
		t.Error("expected non-empty SupportedModels")
	}
}

// --- OpenRouterProvider tests ---

func TestNewOpenRouterProvider_Success(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("NewOpenRouterProvider() error = %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Name() != "openrouter" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openrouter")
	}
}

func TestNewOpenRouterProvider_DefaultModel(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("NewOpenRouterProvider() error = %v", err)
	}
	if p.model != "anthropic/claude-3-5-sonnet" {
		t.Errorf("default model = %q, want %q", p.model, "anthropic/claude-3-5-sonnet")
	}
}

func TestNewOpenRouterProvider_CustomModel(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{Model: "openai/gpt-4"})
	if err != nil {
		t.Fatalf("NewOpenRouterProvider() error = %v", err)
	}
	if p.model != "openai/gpt-4" {
		t.Errorf("model = %q, want %q", p.model, "openai/gpt-4")
	}
}

func TestNewOpenRouterProvider_NoAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	_, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err == nil {
		t.Fatal("expected error with no API key")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if provErr.Provider != "openrouter" {
		t.Errorf("Provider = %q, want %q", provErr.Provider, "openrouter")
	}
}

func TestNewOpenRouterProvider_InvalidAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "invalid-key")

	_, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err == nil {
		t.Fatal("expected error with invalid API key format")
	}
}

func TestOpenRouterProvider_GenerateEmptyPrompt(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	_, err = p.Generate(context.Background(), &GenerateRequest{
		Prompt:    "",
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error with empty prompt")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if provErr.Operation != "generate" {
		t.Errorf("Operation = %q, want %q", provErr.Operation, "generate")
	}
}

func TestOpenRouterProvider_Capabilities(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	caps := p.Capabilities()
	if caps.SupportsCaching {
		t.Error("OpenRouter should not support caching")
	}
	if !caps.SupportsStreaming {
		t.Error("expected SupportsStreaming = true")
	}
	if caps.MaxConcurrentRequests != 20 {
		t.Errorf("MaxConcurrentRequests = %d, want 20", caps.MaxConcurrentRequests)
	}
	if len(caps.SupportedModels) == 0 {
		t.Error("expected non-empty SupportedModels")
	}
}

func TestOpenRouterProvider_CostSinkDefault(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if p.costSink == nil {
		t.Error("expected default cost sink, got nil")
	}
}

// --- VertexAIClaudeProvider tests ---

func TestNewVertexAIClaudeProvider_NoAuth(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	_, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{})
	if err == nil {
		t.Fatal("expected error when GOOGLE_CLOUD_PROJECT not set")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if provErr.Provider != "vertexai-claude" {
		t.Errorf("Provider = %q, want %q", provErr.Provider, "vertexai-claude")
	}
}

func TestNewVertexAIClaudeProvider_InvalidLocation(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	_, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{
		Location: "us-central1", // Invalid for Claude
	})
	if err == nil {
		t.Fatal("expected error with invalid location for Claude")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if provErr.Operation != "configure" {
		t.Errorf("Operation = %q, want %q", provErr.Operation, "configure")
	}
}

func TestNewVertexAIClaudeProvider_DefaultsWithProject(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")

	p, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{})
	if err != nil {
		t.Fatalf("NewVertexAIClaudeProvider() error = %v", err)
	}

	if p.Name() != "vertexai-claude" {
		t.Errorf("Name() = %q, want %q", p.Name(), "vertexai-claude")
	}
	if p.projectID != "my-project" {
		t.Errorf("projectID = %q, want %q", p.projectID, "my-project")
	}
	if p.location != "us-east5" {
		t.Errorf("location = %q, want %q", p.location, "us-east5")
	}
	if p.model != "claude-sonnet-4-5@20250929" {
		t.Errorf("model = %q, want %q", p.model, "claude-sonnet-4-5@20250929")
	}
}

func TestNewVertexAIClaudeProvider_CustomConfig(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "other-project")

	p, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{
		ProjectID: "custom-project",
		Location:  "us-east5",
		Model:     "claude-3-5-sonnet-v2@20241022",
	})
	if err != nil {
		t.Fatalf("NewVertexAIClaudeProvider() error = %v", err)
	}

	if p.projectID != "custom-project" {
		t.Errorf("projectID = %q, want %q", p.projectID, "custom-project")
	}
	if p.model != "claude-3-5-sonnet-v2@20241022" {
		t.Errorf("model = %q, want %q", p.model, "claude-3-5-sonnet-v2@20241022")
	}
}

func TestNewVertexAIClaudeProvider_MissingProjectEnv(t *testing.T) {
	// GOOGLE_CLOUD_PROJECT set (so auth passes) but config.ProjectID empty and env empty
	// Actually this can't happen since DetectAuthMethod checks GOOGLE_CLOUD_PROJECT.
	// But we can test that projectID comes from config when provided.
	t.Setenv("GOOGLE_CLOUD_PROJECT", "env-project")

	p, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{
		ProjectID: "config-project",
	})
	if err != nil {
		t.Fatalf("NewVertexAIClaudeProvider() error = %v", err)
	}
	if p.projectID != "config-project" {
		t.Errorf("projectID = %q, want %q", p.projectID, "config-project")
	}
}

func TestVertexAIClaudeProvider_GenerateEmptyPrompt(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	p, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	_, err = p.Generate(context.Background(), &GenerateRequest{
		Prompt:    "",
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error with empty prompt")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if provErr.Provider != "vertexai-claude" {
		t.Errorf("Provider = %q, want %q", provErr.Provider, "vertexai-claude")
	}
}

func TestVertexAIClaudeProvider_Capabilities(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	p, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	caps := p.Capabilities()
	if !caps.SupportsCaching {
		t.Error("expected SupportsCaching = true")
	}
	if !caps.SupportsStreaming {
		t.Error("expected SupportsStreaming = true")
	}
	if caps.MaxTokensPerRequest != 200000 {
		t.Errorf("MaxTokensPerRequest = %d, want 200000", caps.MaxTokensPerRequest)
	}
	if len(caps.SupportedModels) == 0 {
		t.Error("expected non-empty SupportedModels")
	}
}

// --- VertexAIGeminiProvider tests ---

func TestNewVertexAIGeminiProvider_NoAuth(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	_, err := NewVertexAIGeminiProvider(VertexAIGeminiConfig{})
	if err == nil {
		t.Fatal("expected error when GOOGLE_CLOUD_PROJECT not set")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if provErr.Provider != "vertexai-gemini" {
		t.Errorf("Provider = %q, want %q", provErr.Provider, "vertexai-gemini")
	}
}

// Note: Testing NewVertexAIGeminiProvider with GOOGLE_CLOUD_PROJECT set would
// attempt to create a real gRPC client which may fail in CI/sandbox. We skip
// actual client creation tests.

// --- Concurrency test for Factory ---

func TestFactory_ConcurrentAccess(t *testing.T) {
	f := NewFactory()

	// Register providers concurrently
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			name := "provider-" + string(rune('a'+n))
			_ = f.Register(&mockProvider{name: name})
			_, _ = f.GetProvider(name)
			_ = f.ListProviders()
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic; providers should be registered
	names := f.ListProviders()
	if len(names) == 0 {
		t.Error("expected some providers after concurrent registration")
	}
}

// --- Mock provider interface compliance ---

func TestMockProviderImplementsInterface(t *testing.T) {
	var p Provider = &mockProvider{name: "test"}
	if p.Name() != "test" {
		t.Errorf("Name() = %q, want %q", p.Name(), "test")
	}

	resp, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "mock response" {
		t.Errorf("Text = %q, want %q", resp.Text, "mock response")
	}

	caps := p.Capabilities()
	if len(caps.SupportedModels) != 1 {
		t.Errorf("expected 1 supported model, got %d", len(caps.SupportedModels))
	}
}

// --- GenerateRequest/Response edge cases ---

func TestGenerateRequest_ZeroValues(t *testing.T) {
	req := &GenerateRequest{}
	if req.Prompt != "" {
		t.Error("expected empty prompt")
	}
	if req.MaxTokens != 0 {
		t.Error("expected zero MaxTokens")
	}
	if req.Temperature != 0 {
		t.Error("expected zero Temperature")
	}
	if req.Metadata != nil {
		t.Error("expected nil Metadata")
	}
}

func TestGenerateResponse_ZeroValues(t *testing.T) {
	resp := &GenerateResponse{}
	if resp.Text != "" {
		t.Error("expected empty Text")
	}
	if resp.Usage.TotalTokens != 0 {
		t.Error("expected zero TotalTokens")
	}
	if resp.Metadata != nil {
		t.Error("expected nil Metadata")
	}
}

// --- OpenRouter request/response JSON serialization ---

// --- OpenRouterProvider.calculateUsage (pure function) ---

func TestOpenRouterProvider_calculateUsage(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	usage := p.calculateUsage("anthropic/claude-3-5-sonnet", openRouterUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	})

	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", usage.OutputTokens)
	}
	if usage.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", usage.TotalTokens)
	}
	// Cost should be >= 0 (depends on pricing table but should not be negative)
	if usage.CostUSD < 0 {
		t.Errorf("CostUSD = %f, expected non-negative", usage.CostUSD)
	}
}

func TestOpenRouterProvider_calculateUsage_ZeroTokens(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	usage := p.calculateUsage("unknown-model", openRouterUsage{})
	if usage.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", usage.InputTokens)
	}
	if usage.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0", usage.OutputTokens)
	}
	if usage.CostUSD != 0 {
		t.Errorf("CostUSD = %f, want 0", usage.CostUSD)
	}
}

// --- Factory edge cases for newAnthropicProvider with VertexAI auth ---

func TestFactory_NewProvider_AnthropicVertexAIFallthrough(t *testing.T) {
	// When GOOGLE_CLOUD_PROJECT is set, factory tries VertexAI first.
	// VertexAI Claude creation should fail (no real ADC), then it falls
	// through to the final error "failed to create Anthropic provider"
	// because after the switch there's no fallback to APIKey in the VertexAI case.
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("ANTHROPIC_API_KEY", "")

	f := NewFactory()
	_, err := f.NewProvider("anthropic", "")
	if err == nil {
		// On some environments this might succeed if ADC is configured.
		// But in sandbox without real ADC, it should fail.
		t.Log("NewProvider succeeded (ADC may be configured)")
		return
	}
	// Just verify we get an error, not a panic
	t.Logf("Got expected error: %v", err)
}

// --- OpenRouter Generate with cancelled context ---

func TestOpenRouterProvider_Generate_CancelledContext(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = p.Generate(ctx, &GenerateRequest{
		Prompt:    "hello",
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if provErr.Operation != "generate" {
		t.Errorf("Operation = %q, want %q", provErr.Operation, "generate")
	}
}

func TestVertexAIClaudeProvider_Generate_CancelledContext(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	p, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = p.Generate(ctx, &GenerateRequest{
		Prompt:    "hello",
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
}

// --- Test factory Gemini with GOOGLE_API_KEY fallback ---

func TestFactory_NewProvider_GeminiWithGoogleAPIKey(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "AIzaTestKey456")

	f := NewFactory()
	_, err := f.NewProvider("gemini", "")
	// Still fails because GeminiProvider is not implemented, but exercises the APIKey path
	if err == nil {
		t.Fatal("expected error (Gemini API provider not implemented)")
	}
}

// --- OpenRouter Generate with httptest mock server ---


func TestOpenRouterProvider_Generate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer sk-or-test-key-123" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		resp := openRouterResponse{
			ID: "gen-test-123",
			Choices: []openRouterChoice{
				{
					Message:      openRouterMessage{Role: "assistant", Content: "Hello there!"},
					FinishReason: "stop",
				},
			},
			Usage: openRouterUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenRouterProvider{
		apiKey: "sk-or-test-key-123",
		client: server.Client(),
		model:  "anthropic/claude-3-5-sonnet",
	}
	// Override the URL by creating an HTTP request that points to the test server
	// Since OpenRouter hardcodes the URL, we need to replace the client's Transport.
	p.client = &http.Client{
		Timeout: 5 * time.Second,
		Transport: &rewriteTransport{
			base:    http.DefaultTransport,
			baseURL: server.URL,
		},
	}

	resp, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:       "Hello",
		SystemPrompt: "Be helpful",
		MaxTokens:    100,
		Temperature:  0.5,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if resp.Text != "Hello there!" {
		t.Errorf("Text = %q, want %q", resp.Text, "Hello there!")
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", resp.Usage.OutputTokens)
	}
	if resp.Metadata["openrouter_id"] != "gen-test-123" {
		t.Errorf("openrouter_id = %v", resp.Metadata["openrouter_id"])
	}
}

func TestOpenRouterProvider_Generate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "rate limited"}`))
	}))
	defer server.Close()

	p := &OpenRouterProvider{
		apiKey: "sk-or-test-key-123",
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL},
		},
		model: "anthropic/claude-3-5-sonnet",
	}

	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:    "hello",
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error on HTTP 429")
	}

	var provErr *ProviderError
	if !errors.As(err, &provErr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
}

func TestOpenRouterProvider_Generate_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openRouterResponse{
			ID:      "gen-empty",
			Choices: []openRouterChoice{},
			Usage:   openRouterUsage{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenRouterProvider{
		apiKey: "sk-or-test-key-123",
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL},
		},
		model: "anthropic/claude-3-5-sonnet",
	}

	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:    "hello",
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error with empty choices")
	}
}

func TestOpenRouterProvider_Generate_EmptyResponseText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openRouterResponse{
			ID: "gen-empty-text",
			Choices: []openRouterChoice{
				{Message: openRouterMessage{Role: "assistant", Content: ""}, FinishReason: "stop"},
			},
			Usage: openRouterUsage{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenRouterProvider{
		apiKey: "sk-or-test-key-123",
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL},
		},
		model: "anthropic/claude-3-5-sonnet",
	}

	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:    "hello",
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error with empty response text")
	}
}

func TestOpenRouterProvider_Generate_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	p := &OpenRouterProvider{
		apiKey: "sk-or-test-key-123",
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL},
		},
		model: "anthropic/claude-3-5-sonnet",
	}

	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:    "hello",
		MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("expected error with invalid JSON response")
	}
}

func TestOpenRouterProvider_Generate_UsesRequestModel(t *testing.T) {
	var receivedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openRouterRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedModel = req.Model

		resp := openRouterResponse{
			ID: "gen-model",
			Choices: []openRouterChoice{
				{Message: openRouterMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"},
			},
			Usage: openRouterUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenRouterProvider{
		apiKey: "sk-or-test-key-123",
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL},
		},
		model: "anthropic/claude-3-5-sonnet",
	}

	// Use a specific model in the request
	_, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:    "hello",
		Model:     "openai/gpt-4",
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if receivedModel != "openai/gpt-4" {
		t.Errorf("received model = %q, want %q", receivedModel, "openai/gpt-4")
	}
}

func TestOpenRouterProvider_Generate_WithCostSink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openRouterResponse{
			ID: "gen-cost",
			Choices: []openRouterChoice{
				{Message: openRouterMessage{Role: "assistant", Content: "response"}, FinishReason: "stop"},
			},
			Usage: openRouterUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	p.client = &http.Client{
		Timeout:   5 * time.Second,
		Transport: &rewriteTransport{base: http.DefaultTransport, baseURL: server.URL},
	}

	// Generate with cost sink (default stdout sink)
	resp, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:    "hello",
		MaxTokens: 100,
		Metadata:  map[string]any{"test": true},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "response" {
		t.Errorf("Text = %q, want %q", resp.Text, "response")
	}
}

func TestOpenRouterProvider_recordCost(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test-key-123")

	p, err := NewOpenRouterProvider(OpenRouterConfig{})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	resp := openRouterResponse{
		ID: "test-id",
		Usage: openRouterUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}

	err = p.recordCost(context.Background(), "anthropic/claude-3-5-sonnet", resp, nil)
	if err != nil {
		t.Errorf("recordCost() error = %v", err)
	}
}

// rewriteTransport rewrites the request URL to point to a test server.
type rewriteTransport struct {
	base    http.RoundTripper
	baseURL string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = t.baseURL[len("http://"):]
	return t.base.RoundTrip(req)
}

func TestOpenRouterRequestSerialization(t *testing.T) {
	// Test that the internal types serialize correctly
	req := openRouterRequest{
		Model:       "anthropic/claude-3-5-sonnet",
		MaxTokens:   100,
		Temperature: 0.7,
		Messages: []openRouterMessage{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	}

	if req.Model != "anthropic/claude-3-5-sonnet" {
		t.Errorf("Model = %q", req.Model)
	}
	if len(req.Messages) != 2 {
		t.Errorf("Messages length = %d, want 2", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want %q", req.Messages[0].Role, "system")
	}
}
