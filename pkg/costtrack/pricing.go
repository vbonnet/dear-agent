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

	// Claude 4.6 Opus
	Claude4Opus4_6 = Pricing{
		Input:      15.00, // $15 per 1M tokens
		Output:     75.00, // $75 per 1M tokens
		CacheWrite: 18.75, // $18.75 per 1M tokens
		CacheRead:  1.50,  // $1.50 per 1M tokens
	}

	// Claude 4.8 Opus (newest, most capable)
	Claude4Opus4_8 = Pricing{
		Input:      15.00, // $15 per 1M tokens
		Output:     75.00, // $75 per 1M tokens
		CacheWrite: 18.75, // $18.75 per 1M tokens
		CacheRead:  1.50,  // $1.50 per 1M tokens
	}

	// Claude Opus 5 (2026-07-24) — $5/$25 per Mtok, unchanged from Opus 4.8's
	// real rate per Anthropic's own docs (platform.claude.com/docs/en/about-claude/models/whats-new-opus-5).
	// Note: Claude4Opus4_8 above is stale (looks like carried-over Claude-3-
	// Opus-era pricing); not corrected here since it's pre-existing and out
	// of this change's scope, but it means the two entries don't currently
	// agree despite both nominally being the same $5/$25 rate.
	Claude5Opus5 = Pricing{
		Input:      5.00,  // $5 per 1M tokens
		Output:     25.00, // $25 per 1M tokens
		CacheWrite: 6.25,  // $6.25 per 1M tokens (1.25x input, this file's standard cache-write ratio)
		CacheRead:  0.50,  // $0.50 per 1M tokens (0.1x input, this file's standard cache-read ratio)
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

	// Gemini 3.5 Flash (GA 2026-05-19 at I/O 2026)
	// Source: cloud.google.com/vertex-ai/generative-ai/pricing
	// Standard tier; non-global regions priced higher ($1.65 / $9.90).
	Gemini35Flash = Pricing{
		Input:      1.50, // $1.50 per 1M tokens
		Output:     9.00, // $9 per 1M tokens
		CacheWrite: 0.00, // Context caching priced separately; sink not yet tracking it
		CacheRead:  0.15, // $0.15 per 1M tokens (cached input)
	}
)

// ModelAliases maps short aliases to canonical model IDs.
var ModelAliases = map[string]string{
	"opus":   "claude-opus-4-8",
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
	"claude-opus-4-8":            Claude4Opus4_8,
	"claude-opus-5":              Claude5Opus5,
	"anthropic/claude-opus-5":    Claude5Opus5,           // OpenRouter and Pi provider-qualified naming
	"claude-sonnet-4-5@20250929": Claude35Sonnet20241022, // Vertex AI naming
	"gemini-2.0-flash-exp":       Gemini20FlashExp,
	"gemini-1.5-pro":             Gemini15Pro,
	"gemini-3.5-flash":           Gemini35Flash,
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
