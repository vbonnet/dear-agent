package agent

import (
	"os"
	"testing"
)

func TestIsVertexAIConfigured(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name: "CLOUD_ML_REGION set",
			envVars: map[string]string{
				"CLOUD_ML_REGION": "us-east5",
			},
			expected: true,
		},
		{
			name: "GOOGLE_CLOUD_PROJECT set",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-project",
			},
			expected: true,
		},
		{
			name: "GCP_PROJECT set",
			envVars: map[string]string{
				"GCP_PROJECT": "my-project",
			},
			expected: true,
		},
		{
			name: "GOOGLE_APPLICATION_CREDENTIALS set",
			envVars: map[string]string{
				"GOOGLE_APPLICATION_CREDENTIALS": "/path/to/creds.json",
			},
			expected: true,
		},
		{
			name: "Multiple vars set",
			envVars: map[string]string{
				"CLOUD_ML_REGION":      "us-east5",
				"GOOGLE_CLOUD_PROJECT": "my-project",
			},
			expected: true,
		},
		{
			name:     "No vars set",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name: "Empty string values",
			envVars: map[string]string{
				"CLOUD_ML_REGION":      "",
				"GOOGLE_CLOUD_PROJECT": "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all Vertex AI related env vars first
			clearVertexEnvVars := []string{
				"CLOUD_ML_REGION",
				"GOOGLE_CLOUD_PROJECT",
				"GCP_PROJECT",
				"GOOGLE_APPLICATION_CREDENTIALS",
			}

			originalValues := make(map[string]string)
			for _, key := range clearVertexEnvVars {
				originalValues[key] = os.Getenv(key)
				os.Unsetenv(key)
			}

			// Restore original values after test
			defer func() {
				for key, val := range originalValues {
					if val != "" {
						os.Setenv(key, val)
					} else {
						os.Unsetenv(key)
					}
				}
			}()

			// Set test env vars
			for key, val := range tt.envVars {
				os.Setenv(key, val)
			}

			result := isVertexAIConfigured()
			if result != tt.expected {
				t.Errorf("isVertexAIConfigured() = %v, expected %v (env vars: %v)",
					result, tt.expected, tt.envVars)
			}
		})
	}
}

func TestValidateAgentAvailability_VertexAI(t *testing.T) {
	// Save original env vars
	origCloudML := os.Getenv("CLOUD_ML_REGION")
	origAnthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	origClaudeCode := os.Getenv("CLAUDECODE")

	defer func() {
		if origCloudML != "" {
			os.Setenv("CLOUD_ML_REGION", origCloudML)
		} else {
			os.Unsetenv("CLOUD_ML_REGION")
		}
		if origAnthropicKey != "" {
			os.Setenv("ANTHROPIC_API_KEY", origAnthropicKey)
		} else {
			os.Unsetenv("ANTHROPIC_API_KEY")
		}
		if origClaudeCode != "" {
			os.Setenv("CLAUDECODE", origClaudeCode)
		} else {
			os.Unsetenv("CLAUDECODE")
		}
	}()

	t.Run("Claude with Vertex AI configured", func(t *testing.T) {
		// Clear ANTHROPIC_API_KEY and CLAUDECODE, set Vertex AI env var
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("CLAUDECODE")
		os.Setenv("CLOUD_ML_REGION", "us-east5")

		err := ValidateAgentAvailability("claude")
		if err != nil {
			t.Errorf("Expected no error with Vertex AI configured, got: %v", err)
		}
	})

	t.Run("Claude with ANTHROPIC_API_KEY", func(t *testing.T) {
		// Set API key, unset Vertex AI and CLAUDECODE
		os.Setenv("ANTHROPIC_API_KEY", "test-key")
		os.Unsetenv("CLOUD_ML_REGION")
		os.Unsetenv("CLAUDECODE")

		err := ValidateAgentAvailability("claude")
		if err != nil {
			t.Errorf("Expected no error with API key, got: %v", err)
		}
	})

	t.Run("Claude with Claude Code CLI", func(t *testing.T) {
		// Set CLAUDECODE, unset others
		os.Setenv("CLAUDECODE", "1")
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("CLOUD_ML_REGION")

		err := ValidateAgentAvailability("claude")
		if err != nil {
			t.Errorf("Expected no error with Claude Code CLI, got: %v", err)
		}
	})

	t.Run("Claude without either", func(t *testing.T) {
		// Clear all auth methods
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("CLOUD_ML_REGION")
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("GCP_PROJECT")
		os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		os.Unsetenv("CLAUDECODE")

		err := ValidateAgentAvailability("claude")
		if err == nil {
			t.Error("Expected error without API key or Vertex AI, got nil")
		}

		// Check it's the right error type
		if _, ok := err.(*AgentUnavailableError); !ok {
			t.Errorf("Expected AgentUnavailableError, got: %T", err)
		}
	})
}
