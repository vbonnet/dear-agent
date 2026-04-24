package costtrack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPricingConvertToPerToken(t *testing.T) {
	tests := []struct {
		name     string
		pricing  Pricing
		expected Pricing
	}{
		{
			name: "Claude 3.5 Sonnet",
			pricing: Pricing{
				Input:      3.00,
				Output:     15.00,
				CacheWrite: 3.75,
				CacheRead:  0.30,
			},
			expected: Pricing{
				Input:      0.000003,
				Output:     0.000015,
				CacheWrite: 0.00000375,
				CacheRead:  0.0000003,
			},
		},
		{
			name: "zero pricing",
			pricing: Pricing{
				Input:  0,
				Output: 0,
			},
			expected: Pricing{
				Input:  0,
				Output: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pricing.ConvertToPerToken()

			assert.InDelta(t, tt.expected.Input, result.Input, 0.000000001)
			assert.InDelta(t, tt.expected.Output, result.Output, 0.000000001)
			assert.InDelta(t, tt.expected.CacheWrite, result.CacheWrite, 0.000000001)
			assert.InDelta(t, tt.expected.CacheRead, result.CacheRead, 0.000000001)
		})
	}
}

func TestGetPricing(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantFound bool
		wantInput float64 // per-token price
	}{
		{
			name:      "Claude 3.5 Sonnet",
			model:     "claude-3-5-sonnet-20241022",
			wantFound: true,
			wantInput: 0.000003, // $3 per 1M = $0.000003 per token
		},
		{
			name:      "Claude 3.5 Haiku",
			model:     "claude-3-5-haiku-20241022",
			wantFound: true,
			wantInput: 0.000001, // $1 per 1M = $0.000001 per token
		},
		{
			name:      "Claude 3 Haiku (legacy)",
			model:     "claude-3-haiku-20240307",
			wantFound: true,
			wantInput: 0.00000025, // $0.25 per 1M
		},
		{
			name:      "Claude 4 Opus",
			model:     "claude-opus-4-6",
			wantFound: true,
			wantInput: 0.000015, // $15 per 1M = $0.000015 per token
		},
		{
			name:      "Vertex AI Claude naming",
			model:     "claude-sonnet-4-5@20250929",
			wantFound: true,
			wantInput: 0.000003, // Maps to Claude 3.5 Sonnet
		},
		{
			name:      "Gemini 2.0 Flash (free)",
			model:     "gemini-2.0-flash-exp",
			wantFound: true,
			wantInput: 0, // Free tier
		},
		{
			name:      "local provider",
			model:     "local-jaccard-v1",
			wantFound: true,
			wantInput: 0, // Local is free
		},
		{
			name:      "unknown model",
			model:     "unknown-model",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pricing, found := GetPricing(tt.model)

			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.InDelta(t, tt.wantInput, pricing.Input, 0.000000001)
			}
		})
	}
}

func TestGetPricingOrDefault(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantInput float64
	}{
		{
			name:      "known model",
			model:     "claude-3-5-haiku-20241022",
			wantInput: 0.000001,
		},
		{
			name:      "unknown model returns zero",
			model:     "unknown-model-xyz",
			wantInput: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pricing := GetPricingOrDefault(tt.model)
			assert.InDelta(t, tt.wantInput, pricing.Input, 0.000000001)
		})
	}
}

func TestPricingTableCompleteness(t *testing.T) {
	// Verify all expected models are in the table
	expectedModels := []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-haiku-20240307",
		"claude-3-opus-20240229",
		"claude-opus-4-6",
		"claude-sonnet-4-5@20250929",
		"gemini-2.0-flash-exp",
		"gemini-1.5-pro",
		"local-jaccard-v1",
	}

	for _, model := range expectedModels {
		t.Run(model, func(t *testing.T) {
			_, found := PricingTable[model]
			assert.True(t, found, "Model %s should be in PricingTable", model)
		})
	}

	// Verify all models have 4 dimensions
	for model, pricing := range PricingTable {
		t.Run(model+"/4-dimensions", func(t *testing.T) {
			// All models should have Input and Output defined (may be zero for free models)
			// CacheWrite and CacheRead exist by struct definition, so just verify they're
			// consistent (cache write >= cache read when both non-zero)
			if pricing.CacheWrite > 0 && pricing.CacheRead > 0 {
				assert.Greater(t, pricing.CacheWrite, pricing.CacheRead,
					"Model %s: CacheWrite should be > CacheRead", model)
			}
		})
	}
}

func TestResolveModelAlias(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "opus alias", input: "opus", want: "claude-opus-4-6"},
		{name: "sonnet alias", input: "sonnet", want: "claude-sonnet-4-5@20250929"},
		{name: "haiku alias", input: "haiku", want: "claude-3-5-haiku-20241022"},
		{name: "full model ID passthrough", input: "claude-opus-4-6", want: "claude-opus-4-6"},
		{name: "unknown passthrough", input: "gpt-4", want: "gpt-4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveModelAlias(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetPricingWithAlias(t *testing.T) {
	// GetPricing should resolve aliases before lookup
	pricing, found := GetPricing("opus")
	assert.True(t, found, "alias 'opus' should resolve")
	assert.InDelta(t, 0.000015, pricing.Input, 0.000000001)

	pricing, found = GetPricing("haiku")
	assert.True(t, found, "alias 'haiku' should resolve")
	assert.InDelta(t, 0.000001, pricing.Input, 0.000000001)
}

func TestAllAliasesResolveToKnownModels(t *testing.T) {
	for alias, canonical := range ModelAliases {
		t.Run(alias, func(t *testing.T) {
			_, found := PricingTable[canonical]
			assert.True(t, found, "alias %q maps to %q which is not in PricingTable", alias, canonical)
		})
	}
}
