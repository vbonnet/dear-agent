package auth

import (
	"os"
	"strings"
	"testing"
)

// TestGetAPIKey tests the GetAPIKey function for all providers.
func TestGetAPIKey(t *testing.T) {
	tests := []struct {
		name          string
		provider      string
		envVars       map[string]string
		expectedKey   string
		expectedError string
	}{
		{
			name:        "Anthropic - valid key",
			provider:    "anthropic",
			envVars:     map[string]string{"ANTHROPIC_API_KEY": "sk-ant-test123"},
			expectedKey: "sk-ant-test123",
		},
		{
			name:        "Claude - valid key (alias)",
			provider:    "claude",
			envVars:     map[string]string{"ANTHROPIC_API_KEY": "sk-ant-test456"},
			expectedKey: "sk-ant-test456",
		},
		{
			name:          "Anthropic - missing key",
			provider:      "anthropic",
			envVars:       map[string]string{},
			expectedError: "ANTHROPIC_API_KEY environment variable not set",
		},
		{
			name:        "Gemini - GEMINI_API_KEY set",
			provider:    "gemini",
			envVars:     map[string]string{"GEMINI_API_KEY": "AIzaTest123"},
			expectedKey: "AIzaTest123",
		},
		{
			name:        "Gemini - GOOGLE_API_KEY fallback",
			provider:    "gemini",
			envVars:     map[string]string{"GOOGLE_API_KEY": "AIzaTest456"},
			expectedKey: "AIzaTest456",
		},
		{
			name:     "Gemini - GEMINI_API_KEY preferred over GOOGLE_API_KEY",
			provider: "gemini",
			envVars: map[string]string{
				"GEMINI_API_KEY": "AIzaPreferred",
				"GOOGLE_API_KEY": "AIzaFallback",
			},
			expectedKey: "AIzaPreferred",
		},
		{
			name:        "Google - valid key (alias)",
			provider:    "google",
			envVars:     map[string]string{"GOOGLE_API_KEY": "AIzaGoogle123"},
			expectedKey: "AIzaGoogle123",
		},
		{
			name:          "Gemini - missing key",
			provider:      "gemini",
			envVars:       map[string]string{},
			expectedError: "GEMINI_API_KEY or GOOGLE_API_KEY environment variable not set",
		},
		{
			name:        "OpenRouter - valid key",
			provider:    "openrouter",
			envVars:     map[string]string{"OPENROUTER_API_KEY": "sk-or-test123"},
			expectedKey: "sk-or-test123",
		},
		{
			name:          "OpenRouter - missing key",
			provider:      "openrouter",
			envVars:       map[string]string{},
			expectedError: "OPENROUTER_API_KEY environment variable not set",
		},
		{
			name:          "Unsupported provider",
			provider:      "unsupported",
			envVars:       map[string]string{},
			expectedError: "unsupported provider: unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant environment variables
			clearEnvVars := []string{
				"ANTHROPIC_API_KEY",
				"GEMINI_API_KEY",
				"GOOGLE_API_KEY",
				"OPENROUTER_API_KEY",
			}
			originalEnv := make(map[string]string)
			for _, key := range clearEnvVars {
				originalEnv[key] = os.Getenv(key)
				os.Unsetenv(key)
			}
			defer func() {
				for key, val := range originalEnv {
					if val != "" {
						t.Setenv(key, val)
					}
				}
			}()

			// Set test environment variables
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			// Execute test
			key, err := GetAPIKey(tt.provider)

			// Verify results
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if key != tt.expectedKey {
					t.Errorf("expected key %q, got %q", tt.expectedKey, key)
				}
			}
		})
	}
}

// TestValidateAPIKey tests the ValidateAPIKey function for all providers.
func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		name          string
		provider      string
		key           string
		expectedError string
	}{
		{
			name:     "Anthropic - valid key",
			provider: "anthropic",
			key:      "sk-ant-api03-1234567890abcdef",
		},
		{
			name:     "Claude - valid key (alias)",
			provider: "claude",
			key:      "sk-ant-test-xyz",
		},
		{
			name:          "Anthropic - invalid prefix",
			provider:      "anthropic",
			key:           "sk-test-invalid",
			expectedError: "invalid Anthropic API key format: must start with 'sk-ant-'",
		},
		{
			name:          "Anthropic - empty key",
			provider:      "anthropic",
			key:           "",
			expectedError: "API key cannot be empty",
		},
		{
			name:     "Gemini - valid key",
			provider: "gemini",
			key:      "AIzaSyDtest1234567890",
		},
		{
			name:     "Google - valid key (alias)",
			provider: "google",
			key:      "AIzaTestKey",
		},
		{
			name:          "Gemini - invalid prefix",
			provider:      "gemini",
			key:           "invalid-key",
			expectedError: "invalid Gemini API key format: must start with 'AIza'",
		},
		{
			name:          "Gemini - empty key",
			provider:      "gemini",
			key:           "",
			expectedError: "API key cannot be empty",
		},
		{
			name:     "OpenRouter - valid key",
			provider: "openrouter",
			key:      "sk-or-v1-test123",
		},
		{
			name:          "OpenRouter - invalid prefix",
			provider:      "openrouter",
			key:           "sk-ant-invalid",
			expectedError: "invalid OpenRouter API key format: must start with 'sk-or-'",
		},
		{
			name:          "OpenRouter - empty key",
			provider:      "openrouter",
			key:           "",
			expectedError: "API key cannot be empty",
		},
		{
			name:          "Unsupported provider",
			provider:      "unsupported",
			key:           "any-key",
			expectedError: "unsupported provider: unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAPIKey(tt.provider, tt.key)

			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestSanitizeKey tests the SanitizeKey function.
func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "Long Anthropic key",
			key:      "sk-ant-api03-1234567890abcdefghijklmnopqrstuvwxyz",
			expected: "sk-ant-a***...***wxyz",
		},
		{
			name:     "Long Gemini key",
			key:      "AIzaSyDtest1234567890abcdefghijklmnopqrstuvwxyz",
			expected: "AIzaSyDt***...***wxyz",
		},
		{
			name:     "Long OpenRouter key",
			key:      "sk-or-v1-1234567890abcdefghijklmnopqrstuvwxyz1234",
			expected: "sk-or-v1***...***1234",
		},
		{
			name:     "Exact 13 character key",
			key:      "1234567890abc",
			expected: "12345678***...***0abc",
		},
		{
			name:     "Short key (less than 13 chars)",
			key:      "short",
			expected: "***...***",
		},
		{
			name:     "Empty key",
			key:      "",
			expected: "***...***",
		},
		{
			name:     "12 character key",
			key:      "123456789012",
			expected: "***...***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeKey(tt.key)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestSanitizeKeyDoesNotLeakSecrets tests that SanitizeKey properly masks sensitive data.
func TestSanitizeKeyDoesNotLeakSecrets(t *testing.T) {
	testKey := "sk-ant-api03-SECRETMIDDLEPART-end4"
	sanitized := SanitizeKey(testKey)

	if strings.Contains(sanitized, "SECRETMIDDLEPART") {
		t.Errorf("sanitized key should not contain secret middle part, got: %s", sanitized)
	}

	if !strings.Contains(sanitized, "sk-ant-a") {
		t.Errorf("sanitized key should contain prefix, got: %s", sanitized)
	}

	if !strings.Contains(sanitized, "end4") {
		t.Errorf("sanitized key should contain suffix, got: %s", sanitized)
	}

	if !strings.Contains(sanitized, "***...***") {
		t.Errorf("sanitized key should contain masking pattern, got: %s", sanitized)
	}
}

// TestGetAPIKeyWithValidation tests the integration of GetAPIKey and ValidateAPIKey.
func TestGetAPIKeyWithValidation(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		envVar         string
		envValue       string
		shouldGetKey   bool
		shouldValidate bool
	}{
		{
			name:           "Anthropic - valid flow",
			provider:       "anthropic",
			envVar:         "ANTHROPIC_API_KEY",
			envValue:       "sk-ant-test123",
			shouldGetKey:   true,
			shouldValidate: true,
		},
		{
			name:           "Anthropic - invalid key format in env",
			provider:       "anthropic",
			envVar:         "ANTHROPIC_API_KEY",
			envValue:       "invalid-key",
			shouldGetKey:   true,
			shouldValidate: false,
		},
		{
			name:           "Gemini - valid flow",
			provider:       "gemini",
			envVar:         "GEMINI_API_KEY",
			envValue:       "AIzaTest123",
			shouldGetKey:   true,
			shouldValidate: true,
		},
		{
			name:           "OpenRouter - valid flow",
			provider:       "openrouter",
			envVar:         "OPENROUTER_API_KEY",
			envValue:       "sk-or-test123",
			shouldGetKey:   true,
			shouldValidate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set environment variable
			originalVal := os.Getenv(tt.envVar)
			defer func() {
				if originalVal != "" {
					t.Setenv(tt.envVar, originalVal)
				} else {
					os.Unsetenv(tt.envVar)
				}
			}()
			t.Setenv(tt.envVar, tt.envValue)

			// Test GetAPIKey
			key, err := GetAPIKey(tt.provider)
			if tt.shouldGetKey && err != nil {
				t.Errorf("GetAPIKey failed: %v", err)
			}
			if !tt.shouldGetKey && err == nil {
				t.Error("GetAPIKey should have failed but succeeded")
			}

			// Test ValidateAPIKey if we got a key
			if tt.shouldGetKey && key != "" {
				err = ValidateAPIKey(tt.provider, key)
				if tt.shouldValidate && err != nil {
					t.Errorf("ValidateAPIKey failed: %v", err)
				}
				if !tt.shouldValidate && err == nil {
					t.Error("ValidateAPIKey should have failed but succeeded")
				}
			}
		})
	}
}
