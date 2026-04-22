package costtrack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name     string
		tokens   Tokens
		pricing  Pricing
		expected Cost
	}{
		{
			name: "basic calculation",
			tokens: Tokens{
				Input:  1000,
				Output: 500,
			},
			pricing: Pricing{
				Input:  0.000003, // $3 per 1M = $0.000003 per token
				Output: 0.000015, // $15 per 1M = $0.000015 per token
			},
			expected: Cost{
				Input:  0.003,  // 1000 * 0.000003
				Output: 0.0075, // 500 * 0.000015
				Total:  0.0105, // 0.003 + 0.0075
			},
		},
		{
			name: "with caching",
			tokens: Tokens{
				Input:      1000,
				Output:     500,
				CacheWrite: 200,
				CacheRead:  800,
			},
			pricing: Pricing{
				Input:      0.000003,
				Output:     0.000015,
				CacheWrite: 0.00000375, // $3.75 per 1M
				CacheRead:  0.0000003,  // $0.30 per 1M
			},
			expected: Cost{
				Input:      0.003,
				Output:     0.0075,
				CacheWrite: 0.00075, // 200 * 0.00000375
				CacheRead:  0.00024, // 800 * 0.0000003
				Total:      0.01149, // sum of all
			},
		},
		{
			name: "zero tokens",
			tokens: Tokens{
				Input:  0,
				Output: 0,
			},
			pricing: Pricing{
				Input:  0.000003,
				Output: 0.000015,
			},
			expected: Cost{
				Input:  0,
				Output: 0,
				Total:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateCost(tt.tokens, tt.pricing)

			assert.InDelta(t, tt.expected.Input, result.Input, 0.000001)
			assert.InDelta(t, tt.expected.Output, result.Output, 0.000001)
			assert.InDelta(t, tt.expected.CacheWrite, result.CacheWrite, 0.000001)
			assert.InDelta(t, tt.expected.CacheRead, result.CacheRead, 0.000001)
			assert.InDelta(t, tt.expected.Total, result.Total, 0.000001)
		})
	}
}

func TestCalculateCacheMetrics(t *testing.T) {
	tests := []struct {
		name     string
		tokens   Tokens
		cost     Cost
		expected *Cache
	}{
		{
			name: "with caching",
			tokens: Tokens{
				Input:      1000, // Regular input tokens
				CacheWrite: 200,  // Cache write tokens
				CacheRead:  800,  // Cache read tokens
			},
			cost: Cost{
				Input:      0.003,   // 1000 * 0.000003
				CacheWrite: 0.00075, // 200 * 0.00000375
				CacheRead:  0.00024, // 800 * 0.0000003
			},
			expected: &Cache{
				HitRate:   0.8,     // 800 / (200 + 800)
				Savings:   0.00201, // Baseline: 2000*0.000003=0.006, Actual: 0.00399, Savings: 0.00201
				WriteCost: 0.00075,
				ReadCost:  0.00024,
			},
		},
		{
			name: "no caching",
			tokens: Tokens{
				Input:  1000,
				Output: 500,
			},
			cost: Cost{
				Input:  0.003,
				Output: 0.0075,
			},
			expected: nil,
		},
		{
			name: "only cache writes",
			tokens: Tokens{
				CacheWrite: 500,
				CacheRead:  0,
			},
			cost: Cost{
				CacheWrite: 0.001,
			},
			expected: &Cache{
				HitRate:   0.0,
				Savings:   0.0,
				WriteCost: 0.001,
				ReadCost:  0.0,
			},
		},
		{
			name: "only cache reads",
			tokens: Tokens{
				CacheRead: 1000,
			},
			cost: Cost{
				CacheRead: 0.0003, // 1000 * 0.0000003
			},
			expected: &Cache{
				HitRate:   1.0,    // 1000 / 1000
				Savings:   0.0027, // Baseline: 1000*0.000003=0.003, Actual: 0.0003, Savings: 0.0027
				WriteCost: 0.0,
				ReadCost:  0.0003,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateCacheMetrics(tt.tokens, tt.cost)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}

			assert.NotNil(t, result)
			assert.InDelta(t, tt.expected.HitRate, result.HitRate, 0.001)
			assert.InDelta(t, tt.expected.Savings, result.Savings, 0.001)
			assert.InDelta(t, tt.expected.WriteCost, result.WriteCost, 0.000001)
			assert.InDelta(t, tt.expected.ReadCost, result.ReadCost, 0.000001)
		})
	}
}
