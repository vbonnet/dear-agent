package delegation

import (
	"context"
	"os"
	"testing"
)

// TestStrategyInterface verifies all strategies implement the interface correctly.
func TestStrategyInterface(t *testing.T) {
	t.Run("HeadlessStrategy implements DelegationStrategy", func(t *testing.T) {
		var _ DelegationStrategy = (*HeadlessStrategy)(nil)
	})

	t.Run("ExternalAPIStrategy implements DelegationStrategy", func(t *testing.T) {
		var _ DelegationStrategy = (*ExternalAPIStrategy)(nil)
	})
}

// TestDetectHarnessProvider tests harness environment detection.
func TestDetectHarnessProvider(t *testing.T) {
	tests := []struct {
		name             string
		claudeSessionID  string
		geminiSessionID  string
		expectedProvider string
	}{
		{
			name:             "No harness",
			claudeSessionID:  "",
			geminiSessionID:  "",
			expectedProvider: "",
		},
		{
			name:             "Claude Code harness",
			claudeSessionID:  "test-session-123",
			geminiSessionID:  "",
			expectedProvider: "anthropic",
		},
		{
			name:             "Gemini CLI harness",
			claudeSessionID:  "",
			geminiSessionID:  "test-session-456",
			expectedProvider: "gemini",
		},
		{
			name:             "Both harnesses (Claude takes precedence)",
			claudeSessionID:  "test-session-123",
			geminiSessionID:  "test-session-456",
			expectedProvider: "anthropic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			t.Setenv("CLAUDE_SESSION_ID", tt.claudeSessionID)
			t.Setenv("GEMINI_SESSION_ID", tt.geminiSessionID)

			provider := detectHarnessProvider()
			if provider != tt.expectedProvider {
				t.Errorf("detectHarnessProvider() = %q, want %q", provider, tt.expectedProvider)
			}
		})
	}
}

// TestNormalizeProvider tests provider name normalization.
func TestNormalizeProvider(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"anthropic", "anthropic"},
		{"claude", "anthropic"},
		{"gemini", "gemini"},
		{"google", "gemini"},
		{"codex", "codex"},
		{"openrouter", "openrouter"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeProvider(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeProvider(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNewDelegationStrategy tests factory strategy selection.
func TestNewDelegationStrategy(t *testing.T) {
	t.Run("No harness, no override - returns ExternalAPI", func(t *testing.T) {
		t.Setenv("CLAUDE_SESSION_ID", "")
		t.Setenv("GEMINI_SESSION_ID", "")

		strategy, err := NewDelegationStrategy("")
		if err != nil {
			t.Fatalf("NewDelegationStrategy() error = %v", err)
		}

		if strategy.Name() != "ExternalAPI" {
			t.Errorf("Expected ExternalAPI strategy, got %s", strategy.Name())
		}
	})

}

// TestGetAvailableStrategies tests available strategy enumeration.
func TestGetAvailableStrategies(t *testing.T) {
	t.Run("No harness - only ExternalAPI available", func(t *testing.T) {
		t.Setenv("CLAUDE_SESSION_ID", "")
		t.Setenv("GEMINI_SESSION_ID", "")

		strategies := GetAvailableStrategies("anthropic")

		// At minimum, ExternalAPI should always be available
		found := false
		for _, s := range strategies {
			if s == "ExternalAPI" {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("ExternalAPI not found in available strategies: %v", strategies)
		}
	})

}

// TestSelectStrategyWithFallback tests fallback behavior.
func TestSelectStrategyWithFallback(t *testing.T) {
	t.Run("With fallback allowed", func(t *testing.T) {
		t.Setenv("CLAUDE_SESSION_ID", "")
		t.Setenv("GEMINI_SESSION_ID", "")

		strategy, err := SelectStrategyWithFallback("anthropic", true)
		if err != nil {
			t.Fatalf("SelectStrategyWithFallback() error = %v", err)
		}

		if strategy == nil {
			t.Fatal("Expected non-nil strategy")
		}

		if strategy.Name() != "ExternalAPI" && strategy.Name() != "Headless" {
			t.Errorf("Expected ExternalAPI or Headless, got %s", strategy.Name())
		}
	})

	t.Run("With fallback disabled and no preferred strategy", func(t *testing.T) {
		t.Setenv("CLAUDE_SESSION_ID", "")
		t.Setenv("GEMINI_SESSION_ID", "")

		// Disable fallback - should error if no sub-agent or headless available
		strategy, err := SelectStrategyWithFallback("nonexistent-provider", false)

		// If headless is available, we get a strategy
		// If not, we get an error
		if strategy == nil && err == nil {
			t.Fatal("Expected either strategy or error, got neither")
		}

		if err != nil && strategy != nil {
			t.Fatal("Expected either strategy or error, got both")
		}
	})
}

// TestStrategyError tests error type.
func TestStrategyError(t *testing.T) {
	t.Run("Error() returns formatted message", func(t *testing.T) {
		err := NewStrategyError("TestStrategy", "test-operation", os.ErrNotExist)
		expected := "TestStrategy strategy test-operation failed: file does not exist"
		if err.Error() != expected {
			t.Errorf("Error() = %q, want %q", err.Error(), expected)
		}
	})

	t.Run("Unwrap() returns underlying error", func(t *testing.T) {
		underlying := os.ErrNotExist
		err := NewStrategyError("TestStrategy", "test-operation", underlying)

		unwrapped := err.Unwrap()
		if unwrapped != underlying {
			t.Errorf("Unwrap() = %v, want %v", unwrapped, underlying)
		}
	})
}

// TestAgentInputOutput tests data structures.
func TestAgentInputOutput(t *testing.T) {
	t.Run("AgentInput basic fields", func(t *testing.T) {
		input := &AgentInput{
			Prompt:       "Test prompt",
			Model:        "test-model",
			Provider:     "anthropic",
			MaxTokens:    1000,
			Temperature:  0.7,
			SystemPrompt: "System instruction",
			ToolName:     "test-tool",
		}

		if input.Prompt != "Test prompt" {
			t.Errorf("Prompt = %q, want %q", input.Prompt, "Test prompt")
		}
		if input.MaxTokens != 1000 {
			t.Errorf("MaxTokens = %d, want %d", input.MaxTokens, 1000)
		}
	})

	t.Run("AgentOutput basic fields", func(t *testing.T) {
		output := &AgentOutput{
			Text:     "Response text",
			Model:    "test-model",
			Provider: "anthropic",
			Usage: &UsageStats{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
				CostUSD:      0.001,
			},
			Metadata: map[string]any{"key": "value"},
			Strategy: "TestStrategy",
		}

		if output.Text != "Response text" {
			t.Errorf("Text = %q, want %q", output.Text, "Response text")
		}
		if output.Usage.TotalTokens != 150 {
			t.Errorf("TotalTokens = %d, want %d", output.Usage.TotalTokens, 150)
		}
	})
}

// TestHeadlessStrategy tests headless strategy (basic tests).
func TestHeadlessStrategy(t *testing.T) {
	t.Run("NewHeadlessStrategy for gemini", func(t *testing.T) {
		strategy := NewHeadlessStrategy("gemini")
		if strategy == nil {
			t.Fatal("Expected non-nil strategy")
		}

		if strategy.Name() != "Headless" {
			t.Errorf("Name() = %q, want %q", strategy.Name(), "Headless")
		}

		// Available() depends on whether gemini CLI is installed
		// Just verify it returns a bool without error
		_ = strategy.Available()
	})

	t.Run("NewHeadlessStrategy for anthropic", func(t *testing.T) {
		strategy := NewHeadlessStrategy("anthropic")
		if strategy == nil {
			t.Fatal("Expected non-nil strategy")
		}

		if strategy.Name() != "Headless" {
			t.Errorf("Name() = %q, want %q", strategy.Name(), "Headless")
		}
	})
}

// TestExternalAPIStrategy tests external API strategy (basic tests).
func TestExternalAPIStrategy(t *testing.T) {
	t.Run("NewExternalAPIStrategy", func(t *testing.T) {
		strategy := NewExternalAPIStrategy("anthropic")
		if strategy == nil {
			t.Fatal("Expected non-nil strategy")
		}

		if strategy.Name() != "ExternalAPI" {
			t.Errorf("Name() = %q, want %q", strategy.Name(), "ExternalAPI")
		}

		if !strategy.Available() {
			t.Error("ExternalAPI should always be available")
		}
	})
}

// TestStrategyExecution tests actual strategy execution (integration-level).
func TestStrategyExecution(t *testing.T) {
	ctx := context.Background()

	input := &AgentInput{
		Prompt:   "Test prompt",
		Model:    "test-model",
		Provider: "anthropic",
	}

	t.Run("ExternalAPIStrategy execution", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

		strategy := NewExternalAPIStrategy("anthropic")
		output, err := strategy.Execute(ctx, input)

		// Phase 2: Placeholder returns success with message
		// Phase 3: Will implement actual API calls
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if output == nil {
			t.Fatal("Expected non-nil output")
		}

		if output.Strategy != "ExternalAPI" {
			t.Errorf("Strategy = %q, want %q", output.Strategy, "ExternalAPI")
		}
	})
}

// --- Delegation cleanup verification tests (sandbox changes) ---

// TestSubAgentStrategy_NotRegistered verifies that SubAgentStrategy is NOT
// registered in the factory after the sandbox cleanup (deleted subagent code).
func TestSubAgentStrategy_NotRegistered(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_ID", "")
	t.Setenv("GEMINI_SESSION_ID", "")

	strategies := GetAvailableStrategies("anthropic")

	for _, s := range strategies {
		if s == "SubAgent" {
			t.Error("SubAgent strategy should NOT be registered after sandbox cleanup")
		}
	}
}

// TestFactory_OnlyHeadlessAndExternalAPI verifies the factory only produces
// Headless or ExternalAPI strategies (SubAgent was removed).
func TestFactory_OnlyHeadlessAndExternalAPI(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_ID", "")
	t.Setenv("GEMINI_SESSION_ID", "")

	providers := []string{"anthropic", "gemini", "codex", "openrouter", ""}

	for _, provider := range providers {
		t.Run("provider="+provider, func(t *testing.T) {
			strategy, err := NewDelegationStrategy(provider)
			if err != nil {
				t.Fatalf("NewDelegationStrategy(%q) error = %v", provider, err)
			}

			name := strategy.Name()
			if name != "Headless" && name != "ExternalAPI" {
				t.Errorf("NewDelegationStrategy(%q) returned strategy %q, "+
					"want Headless or ExternalAPI (SubAgent deleted)", provider, name)
			}
		})
	}
}

// TestFactory_FallbackChain verifies the strategy selection fallback chain
// after SubAgent removal: Headless -> ExternalAPI.
func TestFactory_FallbackChain(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_ID", "")
	t.Setenv("GEMINI_SESSION_ID", "")

	// Test with a provider that definitely has no headless CLI
	strategy, err := SelectStrategyWithFallback("nonexistent-provider-xyz", true)
	if err != nil {
		t.Fatalf("SelectStrategyWithFallback() error = %v", err)
	}

	// With fallback enabled, should always get ExternalAPI for unknown providers
	if strategy.Name() != "ExternalAPI" {
		t.Errorf("Strategy = %q, want ExternalAPI for unknown provider with fallback",
			strategy.Name())
	}
}

// TestAvailableStrategies_NoSubAgent verifies SubAgent is absent from available
// strategies for all providers.
func TestAvailableStrategies_NoSubAgent(t *testing.T) {
	providers := []string{"anthropic", "gemini", "codex", "openrouter"}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			strategies := GetAvailableStrategies(provider)

			for _, s := range strategies {
				if s == "SubAgent" {
					t.Errorf("SubAgent found in available strategies for %s, "+
						"should have been removed", provider)
				}
			}
		})
	}
}

// TestDelegationStrategy_OnlyTwoImplementations verifies that exactly two
// strategy types implement DelegationStrategy (SubAgent was removed).
func TestDelegationStrategy_OnlyTwoImplementations(t *testing.T) {
	// HeadlessStrategy and ExternalAPIStrategy are the only two
	var _ DelegationStrategy = (*HeadlessStrategy)(nil)
	var _ DelegationStrategy = (*ExternalAPIStrategy)(nil)

	// Verify names
	headless := NewHeadlessStrategy("anthropic")
	external := NewExternalAPIStrategy("anthropic")

	if headless.Name() != "Headless" {
		t.Errorf("HeadlessStrategy.Name() = %q, want Headless", headless.Name())
	}
	if external.Name() != "ExternalAPI" {
		t.Errorf("ExternalAPIStrategy.Name() = %q, want ExternalAPI", external.Name())
	}
}

// TestExternalAPI_AlwaysAvailable verifies ExternalAPI remains always-available
// as the fallback strategy after SubAgent removal.
func TestExternalAPI_AlwaysAvailable(t *testing.T) {
	strategy := NewExternalAPIStrategy("anthropic")
	if !strategy.Available() {
		t.Error("ExternalAPI.Available() = false, should always be true (fallback)")
	}

	strategy2 := NewExternalAPIStrategy("unknown-provider")
	if !strategy2.Available() {
		t.Error("ExternalAPI.Available() = false even for unknown provider")
	}
}
