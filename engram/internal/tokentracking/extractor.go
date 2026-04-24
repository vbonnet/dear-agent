package tokentracking

import (
	"encoding/json"
	"fmt"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// TokenUsage represents extracted token counts from Claude API response.
type TokenUsage struct {
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	TotalTokens         int
}

// APIResponse represents the structure of a Claude API response.
// The actual response from Claude API includes a usage field with token counts.
type APIResponse struct {
	Usage struct {
		InputTokens         int `json:"input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		CacheCreationTokens int `json:"cache_creation_input_tokens"`
		CacheReadTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// ExtractTokens parses Claude API response for usage metadata.
//
// Parameters:
//
//	response: Claude API response JSON (must contain "usage" field)
//
// Returns:
//
//	usage: Extracted token counts
//	error: Parsing failure (missing field, invalid format)
func ExtractTokens(response *APIResponse) (*TokenUsage, error) {
	if response == nil {
		return nil, fmt.Errorf("response is nil")
	}

	// Extract token counts from usage field
	usage := &TokenUsage{
		InputTokens:         response.Usage.InputTokens,
		OutputTokens:        response.Usage.OutputTokens,
		CacheCreationTokens: response.Usage.CacheCreationTokens,
		CacheReadTokens:     response.Usage.CacheReadTokens,
	}

	// Calculate total tokens
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens

	// Validate token counts (sanity checks)
	if usage.InputTokens < 0 {
		return nil, fmt.Errorf("invalid input_tokens: %d (must be >= 0)", usage.InputTokens)
	}
	if usage.OutputTokens < 0 {
		return nil, fmt.Errorf("invalid output_tokens: %d (must be >= 0)", usage.OutputTokens)
	}
	if usage.TotalTokens > 1000000 {
		return nil, fmt.Errorf("unrealistic total_tokens: %d (exceeds 1M sanity limit)", usage.TotalTokens)
	}

	return usage, nil
}

// ExtractTokensFromJSON parses JSON response and extracts tokens.
// Convenience wrapper for ExtractTokens that handles JSON unmarshaling.
func ExtractTokensFromJSON(responseJSON []byte) (*TokenUsage, error) {
	var response APIResponse
	if err := json.Unmarshal(responseJSON, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return ExtractTokens(&response)
}

// DetermineSeverityLevel maps token counts to telemetry levels.
//
// Rules (from D4 FR1):
//
//	LevelInfo (0): Normal operation (<50,000 tokens)
//	LevelWarn (4): High usage (≥50,000 tokens)
//	LevelError (8): Extremely high (≥100,000 tokens)
func DetermineSeverityLevel(totalTokens int) telemetry.Level {
	if totalTokens >= 100000 {
		return telemetry.LevelError // Extremely high
	} else if totalTokens >= 50000 {
		return telemetry.LevelWarn // High usage
	}
	return telemetry.LevelInfo // Normal
}
