package provider

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiterConfig configures provider-level rate limiting.
type RateLimiterConfig struct {
	RequestsPerMinute int           // Token bucket capacity (default 60)
	QueueTimeout      time.Duration // Max time to wait for a token (default 30s)
}

// RateLimitedProvider wraps a Provider with token-bucket rate limiting.
type RateLimitedProvider struct {
	inner Provider

	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	timeout    time.Duration
}

// NewRateLimitedProvider wraps a provider with rate limiting.
func NewRateLimitedProvider(inner Provider, cfg RateLimiterConfig) *RateLimitedProvider {
	if cfg.RequestsPerMinute <= 0 {
		cfg.RequestsPerMinute = 60
	}
	if cfg.QueueTimeout <= 0 {
		cfg.QueueTimeout = 30 * time.Second
	}

	rate := float64(cfg.RequestsPerMinute)
	return &RateLimitedProvider{
		inner:      inner,
		tokens:     rate,
		maxTokens:  rate,
		refillRate: rate / 60.0, // per second
		lastRefill: time.Now(),
		timeout:    cfg.QueueTimeout,
	}
}

func (r *RateLimitedProvider) Name() string {
	return r.inner.Name()
}

func (r *RateLimitedProvider) Capabilities() Capabilities {
	return r.inner.Capabilities()
}

// Generate waits for a rate limit token, then delegates to the inner provider.
func (r *RateLimitedProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	return r.inner.Generate(ctx, req)
}

// acquire blocks until a token is available or timeout/context is exceeded.
func (r *RateLimitedProvider) acquire(ctx context.Context) error {
	deadline := time.Now().Add(r.timeout)

	for {
		if r.tryConsume() {
			return nil
		}

		// Check timeout
		if time.Now().After(deadline) {
			return fmt.Errorf("rate limit queue timeout (%s) for provider %q", r.timeout, r.inner.Name())
		}

		// Check context
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for rate limit: %w", ctx.Err())
		default:
		}

		// Brief sleep before retry
		time.Sleep(50 * time.Millisecond)
	}
}

func (r *RateLimitedProvider) tryConsume() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Refill
	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()
	r.tokens += elapsed * r.refillRate
	if r.tokens > r.maxTokens {
		r.tokens = r.maxTokens
	}
	r.lastRefill = now

	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

// Reset clears rate limiter state (for testing).
func (r *RateLimitedProvider) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens = r.maxTokens
	r.lastRefill = time.Now()
}
