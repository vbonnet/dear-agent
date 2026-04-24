// Package auth provides authentication hierarchy detection for LLM providers.
//
// This package implements a multi-tiered authentication strategy that detects
// the appropriate authentication method for each LLM provider based on available
// credentials and environment configuration. The hierarchy prioritizes managed
// authentication services (like Vertex AI ADC) over API keys for better security
// and integration with cloud platforms.
//
// Authentication Hierarchy:
//  1. Vertex AI ADC (Application Default Credentials) - Preferred for GCP environments
//  2. API Keys - Direct provider authentication
//  3. Local - No authentication required (e.g., Ollama)
//  4. None - No authentication available
//
// Provider Support:
//   - Anthropic/Claude: Vertex AI Claude, Anthropic API Key
//   - Gemini/Google: Vertex AI Gemini, Gemini API Key, Google API Key
//   - OpenRouter: API Key only
//   - Ollama/Local: No authentication (local endpoint)
//
// Example usage:
//
//	authMethod := auth.DetectAuthMethod("anthropic")
//	switch authMethod {
//	case auth.AuthVertexAI:
//	    // Use Vertex AI Claude with ADC
//	case auth.AuthAPIKey:
//	    // Use Anthropic API with key from ANTHROPIC_API_KEY
//	case auth.AuthNone:
//	    // No authentication available
//	}
package auth

import (
	"os"
)

// AuthMethod represents the type of authentication to use for LLM provider access.
type AuthMethod int

const (
	// AuthVertexAI indicates authentication via Google Cloud Vertex AI
	// using Application Default Credentials (ADC). This is the preferred
	// method when running in GCP environments as it provides automatic
	// credential rotation and integration with IAM.
	AuthVertexAI AuthMethod = iota

	// AuthAPIKey indicates authentication via provider-specific API keys.
	// This is used for direct provider access (e.g., Anthropic API, Gemini API).
	AuthAPIKey

	// AuthLocal indicates no authentication is required.
	// This is used for local providers (e.g., Ollama, llama.cpp) that run
	// on the local machine and are accessed via a local HTTP endpoint.
	AuthLocal

	// AuthNone indicates no authentication is available for the requested provider.
	// This typically means the necessary environment variables are not set.
	AuthNone
)

// String returns a human-readable string representation of the AuthMethod.
func (a AuthMethod) String() string {
	switch a {
	case AuthVertexAI:
		return "VertexAI"
	case AuthAPIKey:
		return "APIKey"
	case AuthLocal:
		return "Local"
	case AuthNone:
		return "None"
	default:
		return "Unknown"
	}
}

// DetectAuthMethod determines the appropriate authentication method for the given
// provider family based on available environment variables.
//
// The function checks for authentication credentials in order of preference:
//  1. Vertex AI (Google Cloud) - checked via GOOGLE_CLOUD_PROJECT
//  2. Provider-specific API keys
//
// Parameters:
//   - providerFamily: The LLM provider family (e.g., "anthropic", "claude", "gemini", "google", "openrouter")
//
// Returns:
//   - AuthMethod indicating the highest-priority authentication method available
//
// Provider-specific behavior:
//
// Anthropic/Claude:
//   - Checks GOOGLE_CLOUD_PROJECT for Vertex AI Claude
//   - Falls back to ANTHROPIC_API_KEY
//
// Gemini/Google:
//   - Checks GOOGLE_CLOUD_PROJECT for Vertex AI Gemini
//   - Falls back to GEMINI_API_KEY or GOOGLE_API_KEY
//
// OpenRouter:
//   - Only supports API key via OPENROUTER_API_KEY
//
// Ollama/Local:
//   - Always returns AuthLocal (no credentials required)
//
// Example:
//
//	// In GCP with ADC configured
//	method := DetectAuthMethod("gemini")
//	// Returns: AuthVertexAI
//
//	// With API key only
//	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-...")
//	method := DetectAuthMethod("anthropic")
//	// Returns: AuthAPIKey
func DetectAuthMethod(providerFamily string) AuthMethod {
	switch providerFamily {
	case "anthropic", "claude":
		// Priority 1: Vertex AI Claude (GCP with Claude on Vertex)
		// Vertex AI requires a GCP project to be configured
		if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" {
			return AuthVertexAI
		}

		// Priority 2: Anthropic API Key
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			return AuthAPIKey
		}

	case "gemini", "google":
		// Priority 1: Vertex AI Gemini (GCP ADC)
		// Vertex AI requires a GCP project to be configured
		if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" {
			return AuthVertexAI
		}

		// Priority 2: Gemini API Key
		// Support both GEMINI_API_KEY (preferred) and GOOGLE_API_KEY (legacy)
		if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != "" {
			return AuthAPIKey
		}

	case "openrouter":
		// OpenRouter only supports API key authentication
		// No Vertex AI or OAuth support
		if os.Getenv("OPENROUTER_API_KEY") != "" {
			return AuthAPIKey
		}

	case "ollama", "local":
		// Local providers require no authentication
		return AuthLocal
	}

	// No authentication available for this provider
	return AuthNone
}
