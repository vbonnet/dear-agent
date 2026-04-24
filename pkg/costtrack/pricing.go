package costtrack

// Pricing defines token pricing for a model (per million tokens)
type Pricing struct {
	Input      float64 // Input tokens price ($ per 1M tokens)
	Output     float64 // Output tokens price ($ per 1M tokens)
	CacheWrite float64 // Cache write price ($ per 1M tokens)
	CacheRead  float64 // Cache read price ($ per 1M tokens)
}

// ConvertToPerToken converts per-million pricing to per-token
func (p Pricing) ConvertToPerToken() Pricing {
	return Pricing{
		Input:      p.Input / 1_000_000,
		Output:     p.Output / 1_000_000,
		CacheWrite: p.CacheWrite / 1_000_000,
		CacheRead:  p.CacheRead / 1_000_000,
	}
}

// Anthropic pricing (as of April 2025)
// Source: https://www.anthropic.com/pricing
var (
	// Claude 3.5 Sonnet (latest)
	Claude35Sonnet20241022 = Pricing{
		Input:      3.00,  // $3 per 1M tokens
		Output:     15.00, // $15 per 1M tokens
		CacheWrite: 3.75,  // $3.75 per 1M tokens
		CacheRead:  0.30,  // $0.30 per 1M tokens
	}

	// Claude 3.5 Haiku (fast, cheap)
	Claude35Haiku20241022 = Pricing{
		Input:      1.00, // $1 per 1M tokens
		Output:     5.00, // $5 per 1M tokens
		CacheWrite: 1.25, // $1.25 per 1M tokens
		CacheRead:  0.10, // $0.10 per 1M tokens
	}

	// Claude 3 Haiku (legacy)
	Claude3Haiku20240307 = Pricing{
		Input:      0.25, // $0.25 per 1M tokens
		Output:     1.25, // $1.25 per 1M tokens
		CacheWrite: 0.30, // $0.30 per 1M tokens
		CacheRead:  0.03, // $0.03 per 1M tokens
	}

	// Claude 3 Opus (most capable, expensive)
	Claude3Opus20240229 = Pricing{
		Input:      15.00, // $15 per 1M tokens
		Output:     75.00, // $75 per 1M tokens
		CacheWrite: 18.75, // $18.75 per 1M tokens
		CacheRead:  1.50,  // $1.50 per 1M tokens
	}

	// Claude 4.6 Opus (newest, most capable)
	Claude4Opus4_6 = Pricing{
		Input:      15.00, // $15 per 1M tokens
		Output:     75.00, // $75 per 1M tokens
		CacheWrite: 18.75, // $18.75 per 1M tokens
		CacheRead:  1.50,  // $1.50 per 1M tokens
	}

	// Gemini 2.0 Flash (free tier available)
	Gemini20FlashExp = Pricing{
		Input:      0.00, // Free tier (up to limits)
		Output:     0.00, // Free tier (up to limits)
		CacheWrite: 0.00, // No caching
		CacheRead:  0.00, // No caching
	}

	// Gemini 1.5 Pro
	Gemini15Pro = Pricing{
		Input:      1.25, // $1.25 per 1M tokens (estimate)
		Output:     5.00, // $5 per 1M tokens (estimate)
		CacheWrite: 0.00, // No caching
		CacheRead:  0.00, // No caching
	}
)

// ModelAliases maps short aliases to canonical model IDs.
var ModelAliases = map[string]string{
	"opus":   "claude-opus-4-6",
	"sonnet": "claude-sonnet-4-5@20250929",
	"haiku":  "claude-3-5-haiku-20241022",
}

// PricingTable maps model IDs to pricing
var PricingTable = map[string]Pricing{
	"claude-3-5-sonnet-20241022": Claude35Sonnet20241022,
	"claude-3-5-haiku-20241022":  Claude35Haiku20241022,
	"claude-3-haiku-20240307":    Claude3Haiku20240307,
	"claude-3-opus-20240229":     Claude3Opus20240229,
	"claude-opus-4-6":            Claude4Opus4_6,
	"claude-sonnet-4-5@20250929": Claude35Sonnet20241022, // Vertex AI naming
	"gemini-2.0-flash-exp":       Gemini20FlashExp,
	"gemini-1.5-pro":             Gemini15Pro,
	"local-jaccard-v1":           {}, // Local provider is free
}

// ResolveModelAlias resolves a short alias to its canonical model ID.
// If the input is not an alias, it is returned unchanged.
func ResolveModelAlias(model string) string {
	if canonical, ok := ModelAliases[model]; ok {
		return canonical
	}
	return model
}

// GetPricing returns pricing for a model, resolving aliases first.
func GetPricing(model string) (Pricing, bool) {
	model = ResolveModelAlias(model)
	pricing, ok := PricingTable[model]
	if !ok {
		return Pricing{}, false
	}
	return pricing.ConvertToPerToken(), true
}

// GetPricingOrDefault returns pricing for a model or zero pricing
func GetPricingOrDefault(model string) Pricing {
	pricing, ok := GetPricing(model)
	if !ok {
		return Pricing{} // Zero cost for unknown models
	}
	return pricing
}
