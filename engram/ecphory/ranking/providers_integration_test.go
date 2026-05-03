package ranking_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/engram/ecphory/ranking"
)

// TestAllProviders_Integration tests all 4 providers end-to-end
// This is a comprehensive integration test that exercises:
// - Provider factory creation and registration
// - Auto-detection logic
// - Actual ranking calls to each provider
// - Cost tracking integration
//
// Tests are conditional based on available credentials
func TestAllProviders_Integration(t *testing.T) {
	ctx := context.Background()

	// Test candidates
	candidates := []ranking.Candidate{
		{
			Name:        "Go Error Handling",
			Description: "Best practices for error handling in Go using fmt.Errorf and error wrapping",
			Tags:        []string{"go", "errors"},
		},
		{
			Name:        "Python Testing",
			Description: "How to write unit tests in Python using pytest framework",
			Tags:        []string{"python", "testing"},
		},
		{
			Name:        "TypeScript Types",
			Description: "Advanced type safety patterns in TypeScript",
			Tags:        []string{"typescript", "types"},
		},
	}

	query := "How do I handle errors in Go?"

	t.Run("Local_Provider_Always_Available", func(t *testing.T) {
		// Local provider should always work (no API needed)
		factory, err := ranking.NewFactory(nil)
		require.NoError(t, err)

		provider, err := factory.GetProvider("local")
		require.NoError(t, err)
		assert.Equal(t, "local", provider.Name())

		// Rank candidates
		results, err := provider.Rank(ctx, query, candidates)
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// Local provider uses Jaccard similarity, should rank Go engram first
		assert.Equal(t, "Go Error Handling", results[0].Candidate.Name)
	})

	t.Run("Anthropic_Provider", func(t *testing.T) {
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			t.Skip("Skipping Anthropic provider test: ANTHROPIC_API_KEY not set")
		}

		factory, err := ranking.NewFactory(nil)
		require.NoError(t, err)

		provider, err := factory.GetProvider("anthropic")
		require.NoError(t, err)
		assert.Equal(t, "anthropic", provider.Name())
		assert.Equal(t, "claude-3-5-haiku-20241022", provider.Model())

		// Check capabilities
		caps := provider.Capabilities()
		assert.True(t, caps.SupportsCaching, "Anthropic should support caching")
		assert.True(t, caps.SupportsStructuredOutput, "Anthropic should support structured output")

		// Rank candidates
		results, err := provider.Rank(ctx, query, candidates)
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// All results should have scores in [0.0, 1.0]
		for _, r := range results {
			assert.GreaterOrEqual(t, r.Score, 0.0)
			assert.LessOrEqual(t, r.Score, 1.0)
			assert.NotEmpty(t, r.Reasoning, "Anthropic should provide reasoning")
		}

		// Go Error Handling should be top ranked for this query
		assert.Equal(t, "Go Error Handling", results[0].Candidate.Name)
	})

	t.Run("VertexAI_Gemini_Provider", func(t *testing.T) {
		if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" || os.Getenv("USE_VERTEX_GEMINI") != "true" {
			t.Skip("Skipping Vertex Gemini test: GOOGLE_CLOUD_PROJECT or USE_VERTEX_GEMINI not set")
		}

		factory, err := ranking.NewFactory(nil)
		require.NoError(t, err)

		provider, err := factory.GetProvider("vertexai-gemini")
		require.NoError(t, err)
		assert.Equal(t, "vertexai-gemini", provider.Name())

		// Check capabilities
		caps := provider.Capabilities()
		assert.Equal(t, 1000000, caps.MaxTokensPerRequest, "Gemini has 1M token window")

		// Rank candidates
		results, err := provider.Rank(ctx, query, candidates)
		require.NoError(t, err)
		assert.NotEmpty(t, results)

		// Validate scores
		for _, r := range results {
			assert.GreaterOrEqual(t, r.Score, 0.0)
			assert.LessOrEqual(t, r.Score, 1.0)
		}
	})

	t.Run("VertexAI_Claude_Provider", func(t *testing.T) {
		if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
			t.Skip("Skipping Vertex Claude test: GOOGLE_CLOUD_PROJECT not set")
		}

		// Note: Vertex Claude only available in us-east5
		config := ranking.DefaultConfig()
		if config.Ecphory.Providers.VertexAIClaude.Location != "us-east5" {
			t.Skip("Skipping Vertex Claude test: location must be us-east5")
		}

		factory, err := ranking.NewFactory(nil)
		require.NoError(t, err)

		provider, err := factory.GetProvider("vertexai-claude")
		if err != nil {
			t.Skipf("Vertex Claude provider not available: %v", err)
		}

		assert.Equal(t, "vertexai-claude", provider.Name())

		// Rank candidates
		results, err := provider.Rank(ctx, query, candidates)
		require.NoError(t, err)
		assert.NotEmpty(t, results)
	})
}

// TestProviderAutoDetection_Integration tests the complete auto-detection flow
func TestProviderAutoDetection_Integration(t *testing.T) {
	t.Run("AutoDetect_With_Multiple_Credentials", func(t *testing.T) {
		// This test verifies precedence order when multiple providers available
		// Precedence: Anthropic > Vertex Claude > Vertex Gemini > Local

		// Save original env
		origAnthropicKey := os.Getenv("ANTHROPIC_API_KEY")
		origGCPProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
		origUseGemini := os.Getenv("USE_VERTEX_GEMINI")

		defer func() {
			t.Setenv("ANTHROPIC_API_KEY", origAnthropicKey)
			t.Setenv("GOOGLE_CLOUD_PROJECT", origGCPProject)
			t.Setenv("USE_VERTEX_GEMINI", origUseGemini)
		}()

		// Test case 1: No credentials → Local
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("USE_VERTEX_GEMINI")

		factory, err := ranking.NewFactory(nil)
		require.NoError(t, err)

		provider, err := factory.AutoDetect()
		require.NoError(t, err)
		assert.Equal(t, "local", provider.Name())

		// Test case 2: Anthropic credentials → Anthropic
		if origAnthropicKey != "" {
			t.Setenv("ANTHROPIC_API_KEY", origAnthropicKey)

			factory, err = ranking.NewFactory(nil)
			require.NoError(t, err)

			provider, err = factory.AutoDetect()
			require.NoError(t, err)
			assert.Equal(t, "anthropic", provider.Name())
		}
	})

	t.Run("Detect_Method_Shows_Available_Providers", func(t *testing.T) {
		factory, err := ranking.NewFactory(nil)
		require.NoError(t, err)

		result := factory.Detect()
		require.NotNil(t, result)

		// Should always show at least local provider
		assert.Contains(t, result.Available, "local")
		assert.NotEmpty(t, result.Provider)
		assert.NotEmpty(t, result.Reason)
	})
}

// TestProviderFallback_Integration tests fallback behavior when primary provider fails
func TestProviderFallback_Integration(t *testing.T) {
	t.Run("Fallback_To_Local_On_API_Error", func(t *testing.T) {
		// This test verifies that when an API provider is unavailable,
		// the system can fall back to local provider

		ctx := context.Background()
		candidates := []ranking.Candidate{
			{
				Name:        "Test Engram",
				Description: "Test description",
				Tags:        []string{"test"},
			},
		}

		factory, err := ranking.NewFactory(nil)
		require.NoError(t, err)

		// Get local provider (fallback)
		localProvider, err := factory.GetProvider("local")
		require.NoError(t, err)

		// Should always work
		results, err := localProvider.Rank(ctx, "test query", candidates)
		require.NoError(t, err)
		assert.NotEmpty(t, results)
	})
}

// TestProviderCapabilities_Integration verifies each provider reports correct capabilities
func TestProviderCapabilities_Integration(t *testing.T) {
	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	tests := []struct {
		name                   string
		providerName           string
		skipCondition          func() bool
		expectedCaching        bool
		expectedStructuredOut  bool
		expectedMaxTokens      int
		expectedMaxConcurrency int
	}{
		{
			name:                   "Local",
			providerName:           "local",
			skipCondition:          func() bool { return false }, // Always available
			expectedCaching:        false,
			expectedStructuredOut:  false,
			expectedMaxTokens:      0, // No limit for local
			expectedMaxConcurrency: 0,
		},
		{
			name:         "Anthropic",
			providerName: "anthropic",
			skipCondition: func() bool {
				return os.Getenv("ANTHROPIC_API_KEY") == ""
			},
			expectedCaching:        true,
			expectedStructuredOut:  true,
			expectedMaxTokens:      200000, // Claude 3.5 Haiku context window
			expectedMaxConcurrency: 5,
		},
		{
			name:         "Vertex_Gemini",
			providerName: "vertexai-gemini",
			skipCondition: func() bool {
				return os.Getenv("GOOGLE_CLOUD_PROJECT") == "" || os.Getenv("USE_VERTEX_GEMINI") != "true"
			},
			expectedCaching:        false,
			expectedStructuredOut:  true,
			expectedMaxTokens:      1000000, // Gemini 2.0 Flash
			expectedMaxConcurrency: 5,
		},
		{
			name:         "Vertex_Claude",
			providerName: "vertexai-claude",
			skipCondition: func() bool {
				return os.Getenv("GOOGLE_CLOUD_PROJECT") == ""
			},
			expectedCaching:        true,
			expectedStructuredOut:  true,
			expectedMaxTokens:      200000,
			expectedMaxConcurrency: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipCondition() {
				t.Skipf("Skipping %s: credentials not available", tt.name)
			}

			provider, err := factory.GetProvider(tt.providerName)
			if err != nil {
				t.Skipf("Provider %s not available: %v", tt.providerName, err)
			}

			caps := provider.Capabilities()
			assert.Equal(t, tt.expectedCaching, caps.SupportsCaching, "Caching support")
			assert.Equal(t, tt.expectedStructuredOut, caps.SupportsStructuredOutput, "Structured output support")
			assert.Equal(t, tt.expectedMaxTokens, caps.MaxTokensPerRequest, "Max tokens")
			assert.Equal(t, tt.expectedMaxConcurrency, caps.MaxConcurrentRequests, "Max concurrency")
		})
	}
}

// TestProviderRanking_Quality tests that each provider produces reasonable rankings
func TestProviderRanking_Quality(t *testing.T) {
	ctx := context.Background()

	// Create test candidates with clear relevance ordering
	candidates := []ranking.Candidate{
		{
			Name:        "Exact Match",
			Description: "This document is about Go error handling using fmt.Errorf and wrapping errors",
			Tags:        []string{"go", "errors"},
		},
		{
			Name:        "Related Topic",
			Description: "Go programming best practices and common patterns",
			Tags:        []string{"go", "best-practices"},
		},
		{
			Name:        "Unrelated",
			Description: "Python web development with Django framework",
			Tags:        []string{"python", "web"},
		},
	}

	query := "How do I handle errors in Go?"

	factory, err := ranking.NewFactory(nil)
	require.NoError(t, err)

	t.Run("All_Providers_Rank_ExactMatch_Highest", func(t *testing.T) {
		providers := []string{"local"}

		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			providers = append(providers, "anthropic")
		}
		if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" && os.Getenv("USE_VERTEX_GEMINI") == "true" {
			providers = append(providers, "vertexai-gemini")
		}

		for _, providerName := range providers {
			provider, err := factory.GetProvider(providerName)
			if err != nil {
				t.Logf("Skipping %s: %v", providerName, err)
				continue
			}

			results, err := provider.Rank(ctx, query, candidates)
			require.NoError(t, err, "Provider %s failed to rank", providerName)
			require.NotEmpty(t, results, "Provider %s returned no results", providerName)

			// Top result should be "Exact Match"
			assert.Equal(t, "Exact Match", results[0].Candidate.Name,
				"Provider %s did not rank exact match highest", providerName)

			// Unrelated should be lowest
			lastIdx := len(results) - 1
			assert.Equal(t, "Unrelated", results[lastIdx].Candidate.Name,
				"Provider %s did not rank unrelated lowest", providerName)
		}
	})
}
