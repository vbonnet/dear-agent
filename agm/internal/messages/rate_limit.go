package messages

import (
	"fmt"
	"sync"
	"time"
)

// RateLimiter implements token bucket algorithm for rate limiting
type RateLimiter struct {
	senderName     string
	messagesPerMin int
	burst          int
	tokens         int
	lastRefill     time.Time
	mu             sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(senderName string, messagesPerMin, burst int) *RateLimiter {
	return &RateLimiter{
		senderName:     senderName,
		messagesPerMin: messagesPerMin,
		burst:          burst,
		tokens:         burst, // Start with full bucket
		lastRefill:     time.Now(),
	}
}

// Allow checks if a message can be sent
// Returns (allowed, remaining tokens, error)
func (rl *RateLimiter) Allow() (bool, int, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := int(elapsed.Minutes() * float64(rl.messagesPerMin))

	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		if rl.tokens > rl.burst {
			rl.tokens = rl.burst
		}
		rl.lastRefill = now
	}

	// Check if we have tokens available
	if rl.tokens > 0 {
		rl.tokens--
		return true, rl.tokens, nil
	}

	return false, 0, fmt.Errorf("rate limit exceeded for sender '%s': %d messages per minute limit", rl.senderName, rl.messagesPerMin)
}

// Global rate limiters (one per sender)
var (
	rateLimiters   = make(map[string]*RateLimiter)
	rateLimitersMu sync.Mutex
)

// GetRateLimiter returns or creates a rate limiter for a sender
func GetRateLimiter(senderName string) *RateLimiter {
	rateLimitersMu.Lock()
	defer rateLimitersMu.Unlock()

	if rl, exists := rateLimiters[senderName]; exists {
		return rl
	}

	// Default: 10 messages per minute, burst 15
	rl := NewRateLimiter(senderName, 10, 15)
	rateLimiters[senderName] = rl
	return rl
}
