package ranking

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNoADC skips when the failure stems from missing Google Application
// Default Credentials. NewVertexAIGeminiProvider needs ADC to construct a
// real client; on developer machines without `gcloud auth
// application-default login`, the constructor fails and these tests are
// not actually exercising our provider logic — they're just probing GCP
// auth state, which is environmental noise.
func skipIfNoADC(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "could not find default credentials") ||
		strings.Contains(msg, "credentials: ") {
		t.Skipf("Google Application Default Credentials not available — skipping: %v", err)
	}
}

func TestNewVertexAIGeminiProvider(t *testing.T) {
	tests := []struct {
		name          string
		config        VertexAIConfig
		setProjectID  bool
		wantErr       bool
		expectedModel string
	}{
		{
			name: "valid config with project ID",
			config: VertexAIConfig{
				ProjectIDEnv: "TEST_GCP_PROJECT",
				Location:     "us-central1",
				Model:        "gemini-2.0-flash-exp",
			},
			setProjectID:  true,
			wantErr:       false,
			expectedModel: "gemini-2.0-flash-exp",
		},
		{
			name: "missing project ID",
			config: VertexAIConfig{
				ProjectIDEnv: "NONEXISTENT_PROJECT",
				Location:     "us-central1",
				Model:        "gemini-2.0-flash-exp",
			},
			setProjectID: false,
			wantErr:      true,
		},
		{
			name: "default model",
			config: VertexAIConfig{
				ProjectIDEnv: "TEST_GCP_PROJECT",
				Location:     "us-central1",
				Model:        "", // Should default to gemini-2.0-flash-exp
			},
			setProjectID:  true,
			wantErr:       false,
			expectedModel: "gemini-2.0-flash-exp",
		},
		{
			name: "default location",
			config: VertexAIConfig{
				ProjectIDEnv: "TEST_GCP_PROJECT",
				Location:     "", // Should default to us-central1
				Model:        "gemini-1.5-pro",
			},
			setProjectID:  true,
			wantErr:       false,
			expectedModel: "gemini-1.5-pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set/unset project ID
			if tt.setProjectID {
				t.Setenv(tt.config.ProjectIDEnv, "test-project-123")
			}

			provider, err := NewVertexAIGeminiProvider(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, provider)
				return
			}

			skipIfNoADC(t, err)
			require.NoError(t, err)
			require.NotNil(t, provider)

			// Verify provider interface
			assert.Equal(t, "vertexai-gemini", provider.Name())
			assert.Equal(t, tt.expectedModel, provider.Model())
		})
	}
}

func TestVertexAIGeminiProvider_Capabilities(t *testing.T) {
	t.Setenv("TEST_GCP_PROJECT", "test-project")

	provider, err := NewVertexAIGeminiProvider(VertexAIConfig{
		ProjectIDEnv: "TEST_GCP_PROJECT",
		Location:     "us-central1",
		Model:        "gemini-2.0-flash-exp",
	})
	skipIfNoADC(t, err)
	require.NoError(t, err)

	caps := provider.Capabilities()

	assert.False(t, caps.SupportsCaching) // Gemini doesn't support caching yet
	assert.True(t, caps.SupportsStructuredOutput)
	assert.Equal(t, 10, caps.MaxConcurrentRequests)
	assert.Equal(t, 1000000, caps.MaxTokensPerRequest) // 1M token context
}

func TestVertexAIGeminiProvider_BuildRankingPrompt(t *testing.T) {
	t.Setenv("TEST_GCP_PROJECT", "test-project")

	provider, err := NewVertexAIGeminiProvider(VertexAIConfig{
		ProjectIDEnv: "TEST_GCP_PROJECT",
	})
	skipIfNoADC(t, err)
	require.NoError(t, err)

	gp := provider.(*VertexAIGeminiProvider)

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

	prompt := gp.buildRankingPrompt("implement OAuth", candidates)

	// Verify prompt structure
	assert.Contains(t, prompt, "Query: implement OAuth")
	assert.Contains(t, prompt, "Candidates:")
	assert.Contains(t, prompt, "0. Name: oauth-pattern")
	assert.Contains(t, prompt, "Description: OAuth 2.0 implementation")
	assert.Contains(t, prompt, "1. Name: jwt-validation")
	assert.Contains(t, prompt, "semantic ranking")
	assert.Contains(t, prompt, "JSON array")
}
