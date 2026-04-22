package auth

import (
	"os"
	"testing"
)

// TestAuthMethod_String verifies the string representation of AuthMethod values.
func TestAuthMethod_String(t *testing.T) {
	tests := []struct {
		name     string
		method   AuthMethod
		expected string
	}{
		{
			name:     "VertexAI",
			method:   AuthVertexAI,
			expected: "VertexAI",
		},
		{
			name:     "APIKey",
			method:   AuthAPIKey,
			expected: "APIKey",
		},
		{
			name:     "Local",
			method:   AuthLocal,
			expected: "Local",
		},
		{
			name:     "None",
			method:   AuthNone,
			expected: "None",
		},
		{
			name:     "Unknown value",
			method:   AuthMethod(999),
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.method.String()
			if result != tt.expected {
				t.Errorf("AuthMethod.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestDetectAuthMethod_Anthropic tests authentication detection for Anthropic/Claude providers.
func TestDetectAuthMethod_Anthropic(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		providerName string
		expected     AuthMethod
	}{
		{
			name: "Vertex AI with GOOGLE_CLOUD_PROJECT",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
			},
			providerName: "anthropic",
			expected:     AuthVertexAI,
		},
		{
			name: "Vertex AI with both GCP and API key - GCP takes precedence",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
				"ANTHROPIC_API_KEY":    "sk-ant-test",
			},
			providerName: "anthropic",
			expected:     AuthVertexAI,
		},
		{
			name: "API Key only",
			envVars: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-test",
			},
			providerName: "anthropic",
			expected:     AuthAPIKey,
		},
		{
			name:         "No authentication",
			envVars:      map[string]string{},
			providerName: "anthropic",
			expected:     AuthNone,
		},
		{
			name: "Claude provider name with Vertex AI",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
			},
			providerName: "claude",
			expected:     AuthVertexAI,
		},
		{
			name: "Claude provider name with API key",
			envVars: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-test",
			},
			providerName: "claude",
			expected:     AuthAPIKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before each test
			clearAuthEnv(t)

			// Set test environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			result := DetectAuthMethod(tt.providerName)
			if result != tt.expected {
				t.Errorf("DetectAuthMethod(%q) = %v, want %v", tt.providerName, result, tt.expected)
			}
		})
	}
}

// TestDetectAuthMethod_Gemini tests authentication detection for Gemini/Google providers.
func TestDetectAuthMethod_Gemini(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		providerName string
		expected     AuthMethod
	}{
		{
			name: "Vertex AI with GOOGLE_CLOUD_PROJECT",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
			},
			providerName: "gemini",
			expected:     AuthVertexAI,
		},
		{
			name: "Vertex AI with all credentials - GCP takes precedence",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
				"GEMINI_API_KEY":       "ai-key-test",
				"GOOGLE_API_KEY":       "goog-key-test",
			},
			providerName: "gemini",
			expected:     AuthVertexAI,
		},
		{
			name: "GEMINI_API_KEY only",
			envVars: map[string]string{
				"GEMINI_API_KEY": "ai-key-test",
			},
			providerName: "gemini",
			expected:     AuthAPIKey,
		},
		{
			name: "GOOGLE_API_KEY only (legacy)",
			envVars: map[string]string{
				"GOOGLE_API_KEY": "goog-key-test",
			},
			providerName: "gemini",
			expected:     AuthAPIKey,
		},
		{
			name: "Both API keys - either is sufficient",
			envVars: map[string]string{
				"GEMINI_API_KEY": "ai-key-test",
				"GOOGLE_API_KEY": "goog-key-test",
			},
			providerName: "gemini",
			expected:     AuthAPIKey,
		},
		{
			name:         "No authentication",
			envVars:      map[string]string{},
			providerName: "gemini",
			expected:     AuthNone,
		},
		{
			name: "Google provider name with Vertex AI",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
			},
			providerName: "google",
			expected:     AuthVertexAI,
		},
		{
			name: "Google provider name with API key",
			envVars: map[string]string{
				"GEMINI_API_KEY": "ai-key-test",
			},
			providerName: "google",
			expected:     AuthAPIKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before each test
			clearAuthEnv(t)

			// Set test environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			result := DetectAuthMethod(tt.providerName)
			if result != tt.expected {
				t.Errorf("DetectAuthMethod(%q) = %v, want %v", tt.providerName, result, tt.expected)
			}
		})
	}
}

// TestDetectAuthMethod_OpenRouter tests authentication detection for OpenRouter.
func TestDetectAuthMethod_OpenRouter(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected AuthMethod
	}{
		{
			name: "OpenRouter API key",
			envVars: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
			expected: AuthAPIKey,
		},
		{
			name: "OpenRouter with GOOGLE_CLOUD_PROJECT - ignores GCP",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
				"OPENROUTER_API_KEY":   "sk-or-test",
			},
			expected: AuthAPIKey,
		},
		{
			name: "OpenRouter with only GOOGLE_CLOUD_PROJECT - no Vertex AI support",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
			},
			expected: AuthNone,
		},
		{
			name:     "No authentication",
			envVars:  map[string]string{},
			expected: AuthNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before each test
			clearAuthEnv(t)

			// Set test environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			result := DetectAuthMethod("openrouter")
			if result != tt.expected {
				t.Errorf("DetectAuthMethod(\"openrouter\") = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestDetectAuthMethod_UnknownProvider tests behavior for unknown provider names.
func TestDetectAuthMethod_UnknownProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		envVars      map[string]string
		expected     AuthMethod
	}{
		{
			name:         "Unknown provider with no credentials",
			providerName: "unknown-provider",
			envVars:      map[string]string{},
			expected:     AuthNone,
		},
		{
			name:         "Unknown provider with GCP credentials",
			providerName: "unknown-provider",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
			},
			expected: AuthNone,
		},
		{
			name:         "Unknown provider with API keys",
			providerName: "unknown-provider",
			envVars: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-test",
				"GEMINI_API_KEY":    "ai-key-test",
			},
			expected: AuthNone,
		},
		{
			name:         "Empty provider name",
			providerName: "",
			envVars: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-test",
			},
			expected: AuthNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before each test
			clearAuthEnv(t)

			// Set test environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			result := DetectAuthMethod(tt.providerName)
			if result != tt.expected {
				t.Errorf("DetectAuthMethod(%q) = %v, want %v", tt.providerName, result, tt.expected)
			}
		})
	}
}

// TestDetectAuthMethod_EnvironmentIsolation verifies that environment changes
// in one test don't affect subsequent tests.
func TestDetectAuthMethod_EnvironmentIsolation(t *testing.T) {
	// First test: set ANTHROPIC_API_KEY
	t.Run("Set Anthropic key", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test1")
		result := DetectAuthMethod("anthropic")
		if result != AuthAPIKey {
			t.Errorf("Expected AuthAPIKey, got %v", result)
		}
	})

	// Second test: should not see ANTHROPIC_API_KEY from first test
	t.Run("Should not see previous key", func(t *testing.T) {
		result := DetectAuthMethod("anthropic")
		if result != AuthNone {
			t.Errorf("Expected AuthNone due to environment isolation, got %v", result)
		}
	})
}

// TestDetectAuthMethod_Ollama tests authentication detection for Ollama/local providers.
func TestDetectAuthMethod_Ollama(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		envVars      map[string]string
		expected     AuthMethod
	}{
		{
			name:         "ollama always returns AuthLocal",
			providerName: "ollama",
			envVars:      map[string]string{},
			expected:     AuthLocal,
		},
		{
			name:         "local always returns AuthLocal",
			providerName: "local",
			envVars:      map[string]string{},
			expected:     AuthLocal,
		},
		{
			name:         "ollama ignores GCP credentials",
			providerName: "ollama",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-gcp-project",
			},
			expected: AuthLocal,
		},
		{
			name:         "ollama ignores API keys",
			providerName: "ollama",
			envVars: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-test",
				"GEMINI_API_KEY":    "AIzatest",
			},
			expected: AuthLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearAuthEnv(t)

			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			result := DetectAuthMethod(tt.providerName)
			if result != tt.expected {
				t.Errorf("DetectAuthMethod(%q) = %v, want %v", tt.providerName, result, tt.expected)
			}
		})
	}
}

// clearAuthEnv clears all authentication-related environment variables.
// This helper ensures tests start with a clean environment state.
func clearAuthEnv(t *testing.T) {
	t.Helper()

	// Get current environment values to restore if needed
	// (though t.Setenv handles cleanup automatically)
	authEnvVars := []string{
		"GOOGLE_CLOUD_PROJECT",
		"ANTHROPIC_API_KEY",
		"GEMINI_API_KEY",
		"GOOGLE_API_KEY",
		"OPENROUTER_API_KEY",
		"OLLAMA_HOST",
	}

	for _, key := range authEnvVars {
		os.Unsetenv(key)
	}
}
