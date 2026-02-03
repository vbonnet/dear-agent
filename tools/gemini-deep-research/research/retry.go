package research

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

// HTTPError represents an HTTP error with status code
type HTTPError struct {
	StatusCode int
	Message    string
}

// Error returns the error message
func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
	}
}

// IsRetryable determines if an error should be retried
func IsRetryable(err error) bool {
	// Network timeouts are retryable
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// HTTP errors
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		// Retry on 500-level errors (server errors)
		if httpErr.StatusCode >= 500 && httpErr.StatusCode < 600 {
			return true
		}
		// Retry on 429 (rate limit)
		if httpErr.StatusCode == http.StatusTooManyRequests {
			return true
		}
		// Don't retry on 4xx errors (client errors) except 429
		if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
			return false
		}
	}

	// Default to retryable for unknown errors
	return true
}

// RetryWithBackoff executes an operation with exponential backoff retry
func RetryWithBackoff(ctx context.Context, operation func() error, config *RetryConfig) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	backoff := config.InitialBackoff
	var lastErr error

	for attempt := 1; attempt <= config.MaxRetries; attempt++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute operation
		err := operation()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		if !IsRetryable(err) {
			return err // Permanent error, don't retry
		}

		// Last attempt failed
		if attempt == config.MaxRetries {
			return fmt.Errorf("operation failed after %d retries: %w", config.MaxRetries, err)
		}

		// Log retry attempt
		log.Printf("Retry %d/%d after %v: %v", attempt, config.MaxRetries, backoff, err)

		// Wait before retry
		select {
		case <-time.After(backoff):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}

		// Exponential backoff with cap
		backoff *= 2
		if backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", config.MaxRetries, lastErr)
}
