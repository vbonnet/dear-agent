package costtrack

import (
	"context"
	"time"
)

// CostSink abstracts cost tracking destinations
type CostSink interface {
	// Record records a cost event with metadata
	Record(ctx context.Context, cost *CostInfo, meta *CostMetadata) error

	// Close flushes and closes the sink
	Close(ctx context.Context) error
}

// CostInfo contains cost and token usage information
type CostInfo struct {
	Provider  string // Provider name (e.g., "anthropic", "vertexai-gemini")
	Model     string // Model identifier
	Component string // Component name (e.g., "wayfinder-D1", "engram-search", "baseline")
	Tokens    Tokens // Token usage
	Cost      Cost   // Calculated costs
	Cache     *Cache // Cache metrics (optional)
}

// Tokens tracks token usage
type Tokens struct {
	Input      int // Input tokens
	Output     int // Output tokens
	CacheWrite int // Cache write tokens (if supported)
	CacheRead  int // Cache read tokens (if supported)
}

// Cost tracks monetary costs
type Cost struct {
	Input      float64 // Input cost ($)
	Output     float64 // Output cost ($)
	CacheWrite float64 // Cache write cost ($)
	CacheRead  float64 // Cache read cost ($)
	Total      float64 // Total cost ($)
}

// Cache tracks cache performance
type Cache struct {
	HitRate   float64 // Cache hit rate (0.0-1.0)
	Savings   float64 // Cost savings from caching ($)
	WriteCost float64 // Cost of writing to cache ($)
	ReadCost  float64 // Cost of reading from cache ($)
}

// CostMetadata contains context about the cost event
type CostMetadata struct {
	Operation string    // Operation name (e.g., "rank", "search")
	Timestamp time.Time // When the operation occurred
	Context   string    // Additional context (e.g., query, engram name)
	RequestID string    // Request identifier (optional)
}

// CalculateCost calculates costs based on tokens and pricing
func CalculateCost(tokens Tokens, pricing Pricing) Cost {
	cost := Cost{
		Input:      float64(tokens.Input) * pricing.Input,
		Output:     float64(tokens.Output) * pricing.Output,
		CacheWrite: float64(tokens.CacheWrite) * pricing.CacheWrite,
		CacheRead:  float64(tokens.CacheRead) * pricing.CacheRead,
	}

	cost.Total = cost.Input + cost.Output + cost.CacheWrite + cost.CacheRead

	return cost
}

// CalculateCacheMetrics calculates cache performance metrics
func CalculateCacheMetrics(tokens Tokens, cost Cost) *Cache {
	totalCacheTokens := tokens.CacheWrite + tokens.CacheRead
	if totalCacheTokens == 0 {
		return nil // No caching
	}

	// Cache hit rate = cache reads / (cache reads + cache writes)
	hitRate := 0.0
	if totalCacheTokens > 0 {
		hitRate = float64(tokens.CacheRead) / float64(totalCacheTokens)
	}

	// Calculate savings:
	// Baseline = what we would have paid if all tokens (input + cache write + cache read) were regular input
	// Actual = what we actually paid (input + cache write + cache read)
	// Savings = Baseline - Actual
	//
	// To calculate baseline, we need the per-token input price.
	// We can derive it from: inputPrice = cost.Input / tokens.Input (if tokens.Input > 0)
	savings := 0.0
	if tokens.Input > 0 {
		inputPricePerToken := cost.Input / float64(tokens.Input)
		totalInputTokens := tokens.Input + tokens.CacheWrite + tokens.CacheRead
		baselineCost := float64(totalInputTokens) * inputPricePerToken
		actualCost := cost.Input + cost.CacheWrite + cost.CacheRead
		savings = baselineCost - actualCost
	} else if tokens.CacheRead > 0 {
		// Special case: only cache reads, no regular input
		// Estimate input price from cache read price (cache read is typically ~10x cheaper)
		inputPricePerToken := (cost.CacheRead / float64(tokens.CacheRead)) * 10.0
		totalInputTokens := tokens.CacheWrite + tokens.CacheRead
		baselineCost := float64(totalInputTokens) * inputPricePerToken
		actualCost := cost.CacheWrite + cost.CacheRead
		savings = baselineCost - actualCost
	}

	return &Cache{
		HitRate:   hitRate,
		Savings:   savings,
		WriteCost: cost.CacheWrite,
		ReadCost:  cost.CacheRead,
	}
}
