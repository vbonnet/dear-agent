package delegation

import (
	"context"
	"errors"
	"testing"
)

// ---------------------------------------------------------------------------
// HeadlessStrategy.Execute — input‐validation paths (no real CLI needed)
// ---------------------------------------------------------------------------

func TestHeadlessExecute_NilInput(t *testing.T) {
	s := NewHeadlessStrategy("anthropic")
	_, err := s.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
	se := &StrategyError{}
	ok := errors.As(err, &se)
	if !ok {
		t.Fatalf("expected *StrategyError, got %T", err)
	}
	if se.Strategy != "Headless" {
		t.Errorf("Strategy = %q, want Headless", se.Strategy)
	}
	if se.Operation != "execute" {
		t.Errorf("Operation = %q, want execute", se.Operation)
	}
}

func TestHeadlessExecute_EmptyPrompt(t *testing.T) {
	s := NewHeadlessStrategy("gemini")
	_, err := s.Execute(context.Background(), &AgentInput{Prompt: ""})
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
	se := &StrategyError{}
	ok := errors.As(err, &se)
	if !ok {
		t.Fatalf("expected *StrategyError, got %T", err)
	}
	if se.Strategy != "Headless" {
		t.Errorf("Strategy = %q, want Headless", se.Strategy)
	}
}

func TestHeadlessExecute_UnsupportedProvider(t *testing.T) {
	s := NewHeadlessStrategy("unknown-provider")
	_, err := s.Execute(context.Background(), &AgentInput{Prompt: "hello"})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
	se := &StrategyError{}
	ok := errors.As(err, &se)
	if !ok {
		t.Fatalf("expected *StrategyError, got %T", err)
	}
	if se.Operation != "execute" {
		t.Errorf("Operation = %q, want execute", se.Operation)
	}
}

// ---------------------------------------------------------------------------
// HeadlessStrategy.parseOutput — direct unit tests
// ---------------------------------------------------------------------------

func TestParseOutput_PlainText(t *testing.T) {
	s := NewHeadlessStrategy("anthropic")
	input := &AgentInput{Prompt: "hi", Provider: "anthropic"}

	out := s.parseOutput("some plain text", input)
	if out.Text != "some plain text" {
		t.Errorf("Text = %q, want %q", out.Text, "some plain text")
	}
	if out.Strategy != "Headless" {
		t.Errorf("Strategy = %q, want Headless", out.Strategy)
	}
	if out.Metadata["format"] != "plaintext" {
		t.Errorf("Metadata[format] = %v, want plaintext", out.Metadata["format"])
	}
}

func TestParseOutput_PlainTextDefaultModels(t *testing.T) {
	tests := []struct {
		provider      string
		expectedModel string
	}{
		{"gemini", "gemini-2.0-flash-exp"},
		{"google", "gemini-2.0-flash-exp"},
		{"anthropic", "claude-sonnet-4"},
		{"claude", "claude-sonnet-4"},
		{"codex", "codex-default"},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			s := NewHeadlessStrategy(tt.provider)
			input := &AgentInput{Prompt: "hi", Provider: tt.provider} // no Model set
			out := s.parseOutput("response", input)
			if out.Model != tt.expectedModel {
				t.Errorf("Model = %q, want %q", out.Model, tt.expectedModel)
			}
		})
	}
}

func TestParseOutput_PlainTextWithModelFromInput(t *testing.T) {
	s := NewHeadlessStrategy("anthropic")
	input := &AgentInput{Prompt: "hi", Provider: "anthropic", Model: "my-model"}
	out := s.parseOutput("txt", input)
	if out.Model != "my-model" {
		t.Errorf("Model = %q, want my-model", out.Model)
	}
}

func TestParseOutput_JSONWithText(t *testing.T) {
	s := NewHeadlessStrategy("gemini")
	input := &AgentInput{Prompt: "hi", Provider: "gemini", Model: "fallback"}
	jsonStr := `{"text":"hello world","model":"gemini-pro"}`

	out := s.parseOutput(jsonStr, input)
	if out.Text != "hello world" {
		t.Errorf("Text = %q, want hello world", out.Text)
	}
	if out.Model != "gemini-pro" {
		t.Errorf("Model = %q, want gemini-pro", out.Model)
	}
	if out.Metadata["cli"] != "gemini" {
		t.Errorf("Metadata[cli] = %v, want gemini", out.Metadata["cli"])
	}
}

func TestParseOutput_JSONWithResponse(t *testing.T) {
	s := NewHeadlessStrategy("anthropic")
	input := &AgentInput{Prompt: "hi", Provider: "anthropic"}
	jsonStr := `{"response":"resp text"}`

	out := s.parseOutput(jsonStr, input)
	if out.Text != "resp text" {
		t.Errorf("Text = %q, want resp text", out.Text)
	}
}

func TestParseOutput_JSONWithContent(t *testing.T) {
	s := NewHeadlessStrategy("anthropic")
	input := &AgentInput{Prompt: "hi", Provider: "anthropic"}
	jsonStr := `{"content":"content text"}`

	out := s.parseOutput(jsonStr, input)
	if out.Text != "content text" {
		t.Errorf("Text = %q, want content text", out.Text)
	}
}

func TestParseOutput_JSONModelFallback(t *testing.T) {
	s := NewHeadlessStrategy("anthropic")
	input := &AgentInput{Prompt: "hi", Provider: "anthropic", Model: "input-model"}
	// JSON with no "model" key
	jsonStr := `{"text":"ok"}`

	out := s.parseOutput(jsonStr, input)
	if out.Model != "input-model" {
		t.Errorf("Model = %q, want input-model", out.Model)
	}
}

func TestParseOutput_JSONWithUsage(t *testing.T) {
	s := NewHeadlessStrategy("anthropic")
	input := &AgentInput{Prompt: "hi", Provider: "anthropic"}
	jsonStr := `{"text":"ok","usage":{"InputTokens":10,"OutputTokens":20,"TotalTokens":30}}`

	out := s.parseOutput(jsonStr, input)
	if out.Usage == nil {
		t.Fatal("Usage should not be nil")
	}
	if out.Usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", out.Usage.InputTokens)
	}
}

func TestParseOutput_JSONWithMetadata(t *testing.T) {
	s := NewHeadlessStrategy("gemini")
	input := &AgentInput{Prompt: "hi", Provider: "gemini"}
	jsonStr := `{"text":"ok","metadata":{"key":"val"}}`

	out := s.parseOutput(jsonStr, input)
	if out.Metadata["key"] != "val" {
		t.Errorf("Metadata[key] = %v, want val", out.Metadata["key"])
	}
	// cli should be injected
	if out.Metadata["cli"] != "gemini" {
		t.Errorf("Metadata[cli] = %v, want gemini", out.Metadata["cli"])
	}
}

// ---------------------------------------------------------------------------
// ExternalAPIStrategy.Execute — auth branch coverage
// ---------------------------------------------------------------------------

func TestExternalExecute_VertexAI(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")
	t.Setenv("ANTHROPIC_API_KEY", "")

	s := NewExternalAPIStrategy("anthropic")
	out, err := s.Execute(context.Background(), &AgentInput{
		Prompt:   "hi",
		Model:    "m",
		Provider: "anthropic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Metadata["auth"] != "vertex-ai" {
		t.Errorf("auth = %v, want vertex-ai", out.Metadata["auth"])
	}
	if out.Strategy != "ExternalAPI" {
		t.Errorf("Strategy = %q, want ExternalAPI", out.Strategy)
	}
}

func TestExternalExecute_AuthNone(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	s := NewExternalAPIStrategy("anthropic")
	_, err := s.Execute(context.Background(), &AgentInput{
		Prompt:   "hi",
		Provider: "anthropic",
	})
	if err == nil {
		t.Fatal("expected error when no auth available")
	}
	se := &StrategyError{}
	ok := errors.As(err, &se)
	if !ok {
		t.Fatalf("expected *StrategyError, got %T", err)
	}
	if se.Operation != "auth" {
		t.Errorf("Operation = %q, want auth", se.Operation)
	}
}

func TestExternalExecute_APIKeyInvalid(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_API_KEY", "bad-key-format")

	s := NewExternalAPIStrategy("anthropic")
	_, err := s.Execute(context.Background(), &AgentInput{
		Prompt:   "hi",
		Provider: "anthropic",
	})
	if err == nil {
		t.Fatal("expected error for invalid API key format")
	}
	se := &StrategyError{}
	ok := errors.As(err, &se)
	if !ok {
		t.Fatalf("expected *StrategyError, got %T", err)
	}
	if se.Operation != "validate-api-key" {
		t.Errorf("Operation = %q, want validate-api-key", se.Operation)
	}
}

func TestExternalExecute_GeminiVertexAI(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "proj")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	s := NewExternalAPIStrategy("gemini")
	out, err := s.Execute(context.Background(), &AgentInput{
		Prompt:   "hi",
		Model:    "gemini-pro",
		Provider: "gemini",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Metadata["auth"] != "vertex-ai" {
		t.Errorf("auth = %v, want vertex-ai", out.Metadata["auth"])
	}
}

func TestExternalExecute_GeminiAPIKey(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GEMINI_API_KEY", "AIzaSyTestKey1234567890")

	s := NewExternalAPIStrategy("gemini")
	out, err := s.Execute(context.Background(), &AgentInput{
		Prompt:   "hi",
		Model:    "gemini-pro",
		Provider: "gemini",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Metadata["auth"] != "api-key" {
		t.Errorf("auth = %v, want api-key", out.Metadata["auth"])
	}
}

func TestExternalExecute_OpenRouterAuthNone(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	s := NewExternalAPIStrategy("openrouter")
	_, err := s.Execute(context.Background(), &AgentInput{
		Prompt:   "hi",
		Provider: "openrouter",
	})
	if err == nil {
		t.Fatal("expected error for openrouter with no key")
	}
}

// ---------------------------------------------------------------------------
// SelectStrategyWithFallback — remaining branches
// ---------------------------------------------------------------------------

func TestSelectStrategyWithFallback_EmptyProviderFallback(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_ID", "")
	t.Setenv("GEMINI_SESSION_ID", "")

	strategy, err := SelectStrategyWithFallback("", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strategy == nil {
		t.Fatal("expected non-nil strategy")
	}
	// Empty provider with fallback → ExternalAPI with default "anthropic"
	if strategy.Name() != "ExternalAPI" {
		t.Errorf("Name() = %q, want ExternalAPI", strategy.Name())
	}
}

func TestSelectStrategyWithFallback_NoFallbackError(t *testing.T) {
	// Use a provider that definitely has no headless CLI
	_, err := SelectStrategyWithFallback("totally-fake-provider", false)
	if err == nil {
		t.Fatal("expected error when fallback disabled and no strategy available")
	}
}

// ---------------------------------------------------------------------------
// HeadlessStrategy.Available — unsupported provider
// ---------------------------------------------------------------------------

func TestHeadlessAvailable_UnsupportedProvider(t *testing.T) {
	s := NewHeadlessStrategy("unknown-provider-xyz")
	if s.Available() {
		t.Error("Available() should be false for unsupported provider")
	}
}

// ---------------------------------------------------------------------------
// CanUseHeadless — codex and unknown providers
// ---------------------------------------------------------------------------

func TestCanUseHeadless_UnknownProvider(t *testing.T) {
	if CanUseHeadless("random-thing") {
		t.Error("CanUseHeadless should return false for unknown provider")
	}
}

// ---------------------------------------------------------------------------
// NewDelegationStrategy — provider override that resolves to ExternalAPI
// ---------------------------------------------------------------------------

func TestNewDelegationStrategy_ProviderOverrideNoHeadless(t *testing.T) {
	// Use a normalized alias that maps to a real provider but lacks CLI
	strategy, err := NewDelegationStrategy("openrouter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// openrouter has no headless CLI, so should fall back to ExternalAPI
	if strategy.Name() != "ExternalAPI" {
		t.Errorf("Name() = %q, want ExternalAPI", strategy.Name())
	}
}

func TestNewDelegationStrategy_AliasNormalization(t *testing.T) {
	// "claude" should normalize to "anthropic"
	strategy, err := NewDelegationStrategy("claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Whether headless or external, it should succeed
	if strategy == nil {
		t.Fatal("expected non-nil strategy")
	}
}

func TestNewDelegationStrategy_GoogleAlias(t *testing.T) {
	strategy, err := NewDelegationStrategy("google")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strategy == nil {
		t.Fatal("expected non-nil strategy")
	}
}
