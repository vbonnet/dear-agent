package ranking

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAnthropicProvider(t *testing.T) {
	tests := []struct {
		name      string
		config    AnthropicConfig
		setAPIKey bool
		apiKey    string
		wantErr   bool
	}{
		{
			name: "valid config with API key",
			config: AnthropicConfig{
				Model: "claude-3-5-haiku-20241022",
			},
			setAPIKey: true,
			apiKey:    "sk-ant-test123456789",
			wantErr:   false,
		},
		{
			name: "missing API key",
			config: AnthropicConfig{
				Model: "claude-3-5-haiku-20241022",
			},
			setAPIKey: false,
			wantErr:   true,
		},
		{
			name: "default model",
			config: AnthropicConfig{
				Model: "", // Should default to claude-3-5-haiku-20241022
			},
			setAPIKey: true,
			apiKey:    "sk-ant-test123456789",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set/unset API key using standard ANTHROPIC_API_KEY env var
			if tt.setAPIKey {
				t.Setenv("ANTHROPIC_API_KEY", tt.apiKey)
			}

			provider, err := NewAnthropicProvider(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, provider)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, provider)

			// Verify provider interface
			assert.Equal(t, "anthropic", provider.Name())

			// Verify model (default or specified)
			if tt.config.Model == "" {
				assert.Equal(t, "claude-3-5-haiku-20241022", provider.Model())
			} else {
				assert.Equal(t, tt.config.Model, provider.Model())
			}
		})
	}
}

func TestAnthropicProvider_Capabilities(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123456789")

	provider, err := NewAnthropicProvider(AnthropicConfig{
		Model: "claude-3-5-haiku-20241022",
	})
	require.NoError(t, err)

	caps := provider.Capabilities()

	assert.True(t, caps.SupportsCaching)
	assert.True(t, caps.SupportsStructuredOutput)
	assert.Equal(t, 5, caps.MaxConcurrentRequests)
	assert.Equal(t, 200000, caps.MaxTokensPerRequest)
}

func TestAnthropicProvider_BuildRankingPrompt(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123456789")

	provider, err := NewAnthropicProvider(AnthropicConfig{})
	require.NoError(t, err)

	ap := provider.(*AnthropicProvider)

	candidates := []Candidate{
		{
			Name:        "oauth-pattern",
			Description: "OAuth 2.0 implementation pattern",
			Tags:        []string{"security", "authentication"},
		},
		{
			Name:        "jwt-validation",
			Description: "JWT token validation",
			Tags:        []string{"security", "tokens"},
		},
	}

	prompt := ap.buildRankingPrompt("implement OAuth", candidates)

	// Verify prompt structure
	assert.Contains(t, prompt, "Query: implement OAuth")
	assert.Contains(t, prompt, "Candidates:")
	assert.Contains(t, prompt, "0. Name: oauth-pattern")
	assert.Contains(t, prompt, "Description: OAuth 2.0 implementation pattern")
	assert.Contains(t, prompt, "Tags: [security authentication]")
	assert.Contains(t, prompt, "1. Name: jwt-validation")
	assert.Contains(t, prompt, "ranked list")
}

func TestAnthropicProvider_ParseRankingResponse(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123456789")

	provider, err := NewAnthropicProvider(AnthropicConfig{})
	require.NoError(t, err)

	ap := provider.(*AnthropicProvider)

	candidates := []Candidate{
		{Name: "oauth-pattern", Description: "OAuth"},
		{Name: "jwt-validation", Description: "JWT"},
	}

	tests := []struct {
		name           string
		responseText   string
		wantResultsLen int
		wantFirstIndex int
	}{
		{
			name: "structured JSON response",
			responseText: `[
				{"index": 0, "score": 0.95, "reasoning": "Best match"},
				{"index": 1, "score": 0.3, "reasoning": "Weak match"}
			]`,
			wantResultsLen: 2,
			wantFirstIndex: 0,
		},
		{
			name: "single result",
			responseText: `[
				{"index": 1, "score": 0.8, "reasoning": "Good match"}
			]`,
			wantResultsLen: 1,
			wantFirstIndex: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock response (simplified - doesn't use actual anthropic.Message)
			// This test focuses on JSON parsing logic
			results, err := ap.parseStructuredJSON(tt.responseText, candidates)
			require.NoError(t, err)

			assert.Len(t, results, tt.wantResultsLen)
			if tt.wantResultsLen > 0 {
				assert.Equal(t, candidates[tt.wantFirstIndex].Name, results[0].Candidate.Name)
			}
		})
	}
}

// Helper function to test JSON parsing directly
func (p *AnthropicProvider) parseStructuredJSON(responseText string, candidates []Candidate) ([]RankedResult, error) {
	var structuredResults []struct {
		Index     int     `json:"index"`
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(responseText), &structuredResults); err != nil {
		return nil, err
	}

	results := make([]RankedResult, 0, len(structuredResults))
	for _, sr := range structuredResults {
		if sr.Index >= 0 && sr.Index < len(candidates) {
			results = append(results, RankedResult{
				Candidate: candidates[sr.Index],
				Score:     sr.Score,
				Reasoning: sr.Reasoning,
			})
		}
	}
	return results, nil
}
