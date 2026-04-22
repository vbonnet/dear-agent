package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RateLimiter implements token-bucket rate limiting per tool.
type RateLimiter struct {
	mu            sync.Mutex
	buckets       map[string]*tokenBucket
	defaultRate   int           // tokens per interval
	defaultWindow time.Duration // refill interval
	toolRates     map[string]int
	logger        *slog.Logger
}

type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// RateLimiterConfig holds rate limiter configuration.
type RateLimiterConfig struct {
	DefaultRate   int            // requests per minute (default 60)
	DefaultWindow time.Duration  // window duration (default 1 minute)
	ToolRates     map[string]int // per-tool rate overrides
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(cfg RateLimiterConfig, logger *slog.Logger) *RateLimiter {
	if cfg.DefaultRate <= 0 {
		cfg.DefaultRate = 60
	}
	if cfg.DefaultWindow <= 0 {
		cfg.DefaultWindow = time.Minute
	}
	if cfg.ToolRates == nil {
		cfg.ToolRates = make(map[string]int)
	}
	return &RateLimiter{
		buckets:       make(map[string]*tokenBucket),
		defaultRate:   cfg.DefaultRate,
		defaultWindow: cfg.DefaultWindow,
		toolRates:     cfg.ToolRates,
		logger:        logger,
	}
}

// Middleware returns an MCP middleware that rate-limits tool calls.
func (r *RateLimiter) Middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}

			toolName := extractToolName(req)
			if toolName == "" {
				return next(ctx, method, req)
			}

			if !r.allow(toolName) {
				r.logger.WarnContext(ctx, "gateway.ratelimit: rate limit exceeded", "tool", toolName)
				return nil, fmt.Errorf("rate limit exceeded for tool %q", toolName)
			}

			return next(ctx, method, req)
		}
	}
}

// allow checks if a request is allowed under the rate limit.
func (r *RateLimiter) allow(tool string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	bucket, ok := r.buckets[tool]
	if !ok {
		rate := r.defaultRate
		if toolRate, exists := r.toolRates[tool]; exists {
			rate = toolRate
		}
		refillRate := float64(rate) / r.defaultWindow.Seconds()
		bucket = &tokenBucket{
			tokens:     float64(rate),
			maxTokens:  float64(rate),
			refillRate: refillRate,
			lastRefill: time.Now(),
		}
		r.buckets[tool] = bucket
	}

	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill).Seconds()
	bucket.tokens += elapsed * bucket.refillRate
	if bucket.tokens > bucket.maxTokens {
		bucket.tokens = bucket.maxTokens
	}
	bucket.lastRefill = now

	// Try to consume a token
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

// Reset clears all rate limit state (for testing).
func (r *RateLimiter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buckets = make(map[string]*tokenBucket)
}
