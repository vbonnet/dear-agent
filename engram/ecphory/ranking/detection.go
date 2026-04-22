package ranking

import (
	"os"
)

// detectEnv returns the first non-empty environment variable value.
// Variables are checked in the order provided (precedence).
// Returns empty string if all variables are unset or empty.
//
// Example:
//
//	projectID := detectEnv("GOOGLE_CLOUD_PROJECT", "ANTHROPIC_VERTEX_PROJECT_ID")
func detectEnv(vars ...string) string {
	for _, v := range vars {
		if val := os.Getenv(v); val != "" {
			return val
		}
	}
	return ""
}

// DetectionResult contains auto-detection results
type DetectionResult struct {
	Provider  string
	Reason    string
	Available []string // All available providers
}

// Detect performs environment-based provider detection
func (f *Factory) Detect() *DetectionResult {
	result := &DetectionResult{
		Available: f.ListProviders(),
	}

	// Check in precedence order
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		result.Provider = "anthropic"
		result.Reason = "ANTHROPIC_API_KEY environment variable set"
		return result
	}

	projectID := detectEnv("GOOGLE_CLOUD_PROJECT", "ANTHROPIC_VERTEX_PROJECT_ID")
	if projectID != "" {
		location := detectEnv("VERTEX_LOCATION", "CLOUD_ML_REGION")

		// Check for Claude in us-east5
		if location == "us-east5" {
			result.Provider = "vertexai-claude"
			result.Reason = "GOOGLE_CLOUD_PROJECT set with VERTEX_LOCATION=us-east5 (Claude region)"
			return result
		}

		// Check for explicit Gemini preference
		useGemini := detectEnv("USE_VERTEX_GEMINI", "GEMINI_API_KEY")
		if useGemini == "true" || (useGemini != "" && useGemini != "false") {
			result.Provider = "vertexai-gemini"
			result.Reason = "Vertex AI project detected with Gemini preference (USE_VERTEX_GEMINI or GEMINI_API_KEY)"
			return result
		}

		// Default to Gemini for Google Cloud (if location specified but not us-east5)
		if location != "" {
			result.Provider = "vertexai-gemini"
			result.Reason = "Vertex AI project detected (defaulting to Gemini)"
			return result
		}
	}

	// Fallback to local
	result.Provider = "local"
	result.Reason = "No API credentials found (using local fallback)"
	return result
}
