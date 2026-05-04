// Package auth provides API key management and validation for LLM providers.
//
// This file implements API key retrieval, validation, and sanitization functions
// for secure handling of provider-specific authentication credentials.
package auth

import (
	"fmt"
	"os"
	"strings"
)

// GetAPIKey retrieves the API key for the specified provider from environment variables.
//
// The function checks provider-specific environment variables in order of preference.
// If no API key is found, it returns an error indicating which environment variable
// should be set.
//
// Parameters:
//   - provider: The LLM provider name (e.g., "anthropic", "gemini", "openrouter")
//
// Returns:
//   - string: The API key value if found
//   - error: An error if no API key is found for the provider
//
// Environment variables checked by provider:
//
// Anthropic:
//   - ANTHROPIC_API_KEY
//
// Gemini/Google:
//   - GEMINI_API_KEY (preferred)
//   - GOOGLE_API_KEY (fallback)
//
// OpenRouter:
//   - OPENROUTER_API_KEY
//
// Example:
//
//	key, err := GetAPIKey("anthropic")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// Use key for API authentication
func GetAPIKey(provider string) (string, error) {
	switch provider {
	case "anthropic", "claude":
		if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
			return key, nil
		}
		return "", fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")

	case "gemini", "google":
		// Prefer GEMINI_API_KEY, fall back to GOOGLE_API_KEY
		if key := os.Getenv("GEMINI_API_KEY"); key != "" {
			return key, nil
		}
		if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
			return key, nil
		}
		return "", fmt.Errorf("GEMINI_API_KEY or GOOGLE_API_KEY environment variable not set")

	case "openrouter":
		if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
			return key, nil
		}
		return "", fmt.Errorf("OPENROUTER_API_KEY environment variable not set")

	case "openai":
		if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			return key, nil
		}
		return "", fmt.Errorf("OPENAI_API_KEY environment variable not set")

	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

// ValidateAPIKey validates the format of an API key for the specified provider.
//
// Each provider has different API key format requirements. This function checks
// that the key matches the expected prefix pattern for the given provider.
//
// Parameters:
//   - provider: The LLM provider name (e.g., "anthropic", "gemini", "openrouter")
//   - key: The API key to validate
//
// Returns:
//   - error: An error if the key format is invalid, nil if valid
//
// Validation rules by provider:
//
// Anthropic:
//   - Must start with "sk-ant-"
//
// Gemini/Google:
//   - Must start with "AIza"
//
// OpenRouter:
//   - Must start with "sk-or-"
//
// Example:
//
//	err := ValidateAPIKey("anthropic", "sk-ant-api03-...")
//	if err != nil {
//	    log.Fatal("Invalid API key format:", err)
//	}
func ValidateAPIKey(provider, key string) error {
	if key == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	switch provider {
	case "anthropic", "claude":
		if !strings.HasPrefix(key, "sk-ant-") {
			return fmt.Errorf("invalid Anthropic API key format: must start with 'sk-ant-'")
		}

	case "gemini", "google":
		if !strings.HasPrefix(key, "AIza") {
			return fmt.Errorf("invalid Gemini API key format: must start with 'AIza'")
		}

	case "openrouter":
		if !strings.HasPrefix(key, "sk-or-") {
			return fmt.Errorf("invalid OpenRouter API key format: must start with 'sk-or-'")
		}

	case "openai":
		// OpenAI keys start with "sk-" but project-scoped keys use "sk-proj-".
		// Both are valid; reject anything that doesn't begin with "sk-".
		if !strings.HasPrefix(key, "sk-") {
			return fmt.Errorf("invalid OpenAI API key format: must start with 'sk-'")
		}

	default:
		return fmt.Errorf("unsupported provider: %s", provider)
	}

	return nil
}

// SanitizeKey sanitizes an API key for safe logging by masking the middle portion.
//
// The function preserves the first 8 characters and last 4 characters of the key,
// replacing the middle section with "***...***" to prevent accidental exposure of
// sensitive credentials in logs while maintaining some identifiability.
//
// Parameters:
//   - key: The API key to sanitize
//
// Returns:
//   - string: The sanitized key suitable for logging
//
// Behavior:
//   - Keys shorter than 13 characters are fully masked as "***...***"
//   - Keys 13+ characters show first 8 chars, "***...***", and last 4 chars
//
// Example:
//
//	sanitized := SanitizeKey("sk-ant-api03-1234567890abcdefghijklmnopqrstuvwxyz")
//	// Returns: "sk-ant-a***...***wxyz"
func SanitizeKey(key string) string {
	if len(key) < 13 {
		// For very short keys, just mask everything
		return "***...***"
	}

	// Show first 8 characters and last 4 characters
	prefix := key[:8]
	suffix := key[len(key)-4:]
	return fmt.Sprintf("%s***...***%s", prefix, suffix)
}
