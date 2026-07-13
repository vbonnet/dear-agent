package tokentracking

import (
	"encoding/json"
	"fmt"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// TokenUsage represents normalized token counts from a model response.
type TokenUsage struct {
	Provider            string
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
	TotalTokens         int
}

// APIResponse represents the legacy Anthropic-compatible response structure.
type APIResponse struct {
	Usage struct {
		InputTokens         int `json:"input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		CacheCreationTokens int `json:"cache_creation_input_tokens"`
		CacheReadTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// ExtractTokens parses a legacy Anthropic-compatible response for usage metadata.
//
// Parameters:
//
//	response: Anthropic-compatible response JSON (must contain "usage" field)
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
		Provider:            "anthropic",
		InputTokens:         response.Usage.InputTokens,
		OutputTokens:        response.Usage.OutputTokens,
		CacheCreationTokens: response.Usage.CacheCreationTokens,
		CacheReadTokens:     response.Usage.CacheReadTokens,
	}

	// Calculate total tokens
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens

	return validateUsage(usage)
}

// ExtractTokensFromJSON parses JSON response and extracts tokens.
// Convenience wrapper for ExtractTokens that handles JSON unmarshaling.
func ExtractTokensFromJSON(responseJSON []byte) (*TokenUsage, error) {
	return ExtractTokensFromJSONForFamily(responseJSON, "")
}

// ExtractTokensFromJSONForFamily parses provider usage metadata while
// preserving the configured model-family attribution. An empty family keeps
// schema-based legacy detection.
func ExtractTokensFromJSONForFamily(responseJSON []byte, family string) (*TokenUsage, error) {
	if family != "" && !supportedTokenFamily(family) {
		return nil, fmt.Errorf("unsupported model family %q", family)
	}
	var envelope usageEnvelope
	if err := json.Unmarshal(responseJSON, &envelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	if len(envelope.UsageMetadata) > 0 {
		return extractGeminiUsage(envelope.UsageMetadata, family)
	}

	if len(envelope.Usage) == 0 {
		provider := family
		if provider == "" {
			provider = "anthropic"
		}
		return validateUsage(&TokenUsage{Provider: provider})
	}
	var usage compatibleUsage
	if err := json.Unmarshal(envelope.Usage, &usage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal usage metadata: %w", err)
	}
	if usage.PromptTokens != nil || usage.CompletionTokens != nil {
		return extractOpenAICompatibleUsage(usage, family)
	}
	return extractAnthropicUsage(usage, family)
}

type usageEnvelope struct {
	Usage         json.RawMessage `json:"usage"`
	UsageMetadata json.RawMessage `json:"usageMetadata"`
}

type compatibleUsage struct {
	InputTokens         *int `json:"input_tokens"`
	OutputTokens        *int `json:"output_tokens"`
	CacheCreationTokens int  `json:"cache_creation_input_tokens"`
	CacheReadTokens     int  `json:"cache_read_input_tokens"`
	PromptTokens        *int `json:"prompt_tokens"`
	CompletionTokens    *int `json:"completion_tokens"`
	PromptDetails       struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func extractGeminiUsage(data json.RawMessage, family string) (*TokenUsage, error) {
	if family != "" && family != "gemini" {
		return nil, fmt.Errorf("gemini usage metadata does not match model family %q", family)
	}
	var usage struct {
		PromptTokens     int `json:"promptTokenCount"`
		CandidateTokens  int `json:"candidatesTokenCount"`
		CachedReadTokens int `json:"cachedContentTokenCount"`
	}
	if err := json.Unmarshal(data, &usage); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Gemini usage metadata: %w", err)
	}
	return validateUsage(&TokenUsage{
		Provider:        "gemini",
		InputTokens:     usage.PromptTokens,
		OutputTokens:    usage.CandidateTokens,
		CacheReadTokens: usage.CachedReadTokens,
	})
}

func extractOpenAICompatibleUsage(usage compatibleUsage, family string) (*TokenUsage, error) {
	provider := family
	if provider == "" {
		provider = "openai"
	}
	if provider == "anthropic" || provider == "gemini" {
		return nil, fmt.Errorf("openai-compatible usage metadata does not match model family %q", provider)
	}
	return validateUsage(&TokenUsage{
		Provider:        provider,
		InputTokens:     valueOrZero(usage.PromptTokens),
		OutputTokens:    valueOrZero(usage.CompletionTokens),
		CacheReadTokens: usage.PromptDetails.CachedTokens,
	})
}

func extractAnthropicUsage(usage compatibleUsage, family string) (*TokenUsage, error) {
	if usage.InputTokens == nil && usage.OutputTokens == nil {
		return nil, fmt.Errorf("usage metadata has no supported token fields")
	}
	if family != "" && family != "anthropic" {
		return nil, fmt.Errorf("anthropic usage metadata does not match model family %q", family)
	}
	return validateUsage(&TokenUsage{
		Provider:            "anthropic",
		InputTokens:         valueOrZero(usage.InputTokens),
		OutputTokens:        valueOrZero(usage.OutputTokens),
		CacheCreationTokens: usage.CacheCreationTokens,
		CacheReadTokens:     usage.CacheReadTokens,
	})
}

func supportedTokenFamily(family string) bool {
	switch family {
	case "anthropic", "openai", "gemini", "glm", "deepseek", "nemotron", "qwen":
		return true
	default:
		return false
	}
}

func validateUsage(usage *TokenUsage) (*TokenUsage, error) {
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens
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

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
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
