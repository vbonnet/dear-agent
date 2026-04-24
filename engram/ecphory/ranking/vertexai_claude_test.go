package ranking

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVertexAIClaudeProvider(t *testing.T) {
	tests := []struct {
		name          string
		config        VertexAIClaudeConfig
		setProjectID  bool
		wantErr       bool
		errorContains string
	}{
		{
			name: "valid config with project ID",
			config: VertexAIClaudeConfig{
				ProjectIDEnv: "TEST_GCP_PROJECT_CLAUDE",
				Location:     "us-east5",
				Model:        "claude-sonnet-4-5@20250929",
			},
			setProjectID: true,
			wantErr:      false,
		},
		{
			name: "missing project ID",
			config: VertexAIClaudeConfig{
				ProjectIDEnv: "NONEXISTENT_PROJECT",
				Location:     "us-east5",
				Model:        "claude-sonnet-4-5@20250929",
			},
			setProjectID:  false,
			wantErr:       true,
			errorContains: "not set",
		},
		{
			name: "default model",
			config: VertexAIClaudeConfig{
				ProjectIDEnv: "TEST_GCP_PROJECT_CLAUDE",
				Location:     "us-east5",
				Model:        "", // Should default to claude-sonnet-4-5@20250929
			},
			setProjectID: true,
			wantErr:      false,
		},
		{
			name: "default location",
			config: VertexAIClaudeConfig{
				ProjectIDEnv: "TEST_GCP_PROJECT_CLAUDE",
				Location:     "", // Should default to us-east5
				Model:        "claude-sonnet-4-5@20250929",
			},
			setProjectID: true,
			wantErr:      false,
		},
		{
			name: "invalid location",
			config: VertexAIClaudeConfig{
				ProjectIDEnv: "TEST_GCP_PROJECT_CLAUDE",
				Location:     "us-central1", // Claude only in us-east5
				Model:        "claude-sonnet-4-5@20250929",
			},
			setProjectID:  true,
			wantErr:       true,
			errorContains: "only available in us-east5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set/unset project ID
			if tt.setProjectID {
				t.Setenv(tt.config.ProjectIDEnv, "test-project-123")
			}

			provider, err := NewVertexAIClaudeProvider(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, provider)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, provider)

			// Verify provider interface
			assert.Equal(t, "vertexai-claude", provider.Name())

			// Verify model (default or specified)
			expectedModel := tt.config.Model
			if expectedModel == "" {
				expectedModel = "claude-sonnet-4-5@20250929"
			}
			assert.Equal(t, expectedModel, provider.Model())
		})
	}
}

func TestVertexAIClaudeProvider_Capabilities(t *testing.T) {
	t.Setenv("TEST_GCP_PROJECT_CLAUDE", "test-project")

	provider, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{
		ProjectIDEnv: "TEST_GCP_PROJECT_CLAUDE",
		Location:     "us-east5",
		Model:        "claude-sonnet-4-5@20250929",
	})
	require.NoError(t, err)

	caps := provider.Capabilities()

	assert.True(t, caps.SupportsCaching)
	assert.True(t, caps.SupportsStructuredOutput)
	assert.Equal(t, 5, caps.MaxConcurrentRequests)
	assert.Equal(t, 200000, caps.MaxTokensPerRequest) // Claude context window
}

func TestVertexAIClaudeProvider_BuildRankingPrompt(t *testing.T) {
	t.Setenv("TEST_GCP_PROJECT_CLAUDE", "test-project")

	provider, err := NewVertexAIClaudeProvider(VertexAIClaudeConfig{
		ProjectIDEnv: "TEST_GCP_PROJECT_CLAUDE",
	})
	require.NoError(t, err)

	vcp := provider.(*VertexAIClaudeProvider)

	candidates := []Candidate{
		{
			Name:        "oauth-pattern",
			Description: "OAuth 2.0 implementation",
			Tags:        []string{"security"},
		},
		{
			Name:        "jwt-validation",
			Description: "JWT token validation",
			Tags:        []string{"security", "tokens"},
		},
	}

	prompt := vcp.buildRankingPrompt("implement OAuth", candidates)

	// Verify prompt structure (same as Anthropic provider)
	assert.Contains(t, prompt, "Query: implement OAuth")
	assert.Contains(t, prompt, "Candidates:")
	assert.Contains(t, prompt, "0. Name: oauth-pattern")
	assert.Contains(t, prompt, "Description: OAuth 2.0 implementation")
	assert.Contains(t, prompt, "1. Name: jwt-validation")
	assert.Contains(t, prompt, "ranked list")
}
