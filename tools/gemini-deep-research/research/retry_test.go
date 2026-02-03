package research

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "HTTP 500 error",
			err:      &HTTPError{StatusCode: 500, Message: "Internal Server Error"},
			expected: true,
		},
		{
			name:     "HTTP 503 error",
			err:      &HTTPError{StatusCode: 503, Message: "Service Unavailable"},
			expected: true,
		},
		{
			name:     "HTTP 429 error",
			err:      &HTTPError{StatusCode: 429, Message: "Too Many Requests"},
			expected: true,
		},
		{
			name:     "HTTP 400 error",
			err:      &HTTPError{StatusCode: 400, Message: "Bad Request"},
			expected: false,
		},
		{
			name:     "HTTP 401 error",
			err:      &HTTPError{StatusCode: 401, Message: "Unauthorized"},
			expected: false,
		},
		{
			name:     "HTTP 404 error",
			err:      &HTTPError{StatusCode: 404, Message: "Not Found"},
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("network error"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRetryWithBackoff_Success(t *testing.T) {
	ctx := context.Background()
	config := &RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}

	attempts := 0
	operation := func() error {
		attempts++
		return nil // Success on first attempt
	}

	err := RetryWithBackoff(ctx, operation, config)
	if err != nil {
		t.Errorf("RetryWithBackoff() error = %v, want nil", err)
	}

	if attempts != 1 {
		t.Errorf("operation called %d times, want 1", attempts)
	}
}

func TestRetryWithBackoff_RetryableError(t *testing.T) {
	ctx := context.Background()
	config := &RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}

	attempts := 0
	operation := func() error {
		attempts++
		if attempts < 3 {
			return &HTTPError{StatusCode: 500, Message: "Server Error"}
		}
		return nil // Success on third attempt
	}

	err := RetryWithBackoff(ctx, operation, config)
	if err != nil {
		t.Errorf("RetryWithBackoff() error = %v, want nil", err)
	}

	if attempts != 3 {
		t.Errorf("operation called %d times, want 3", attempts)
	}
}

func TestRetryWithBackoff_PermanentError(t *testing.T) {
	ctx := context.Background()
	config := &RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}

	attempts := 0
	permanentErr := &HTTPError{StatusCode: 401, Message: "Unauthorized"}
	operation := func() error {
		attempts++
		return permanentErr
	}

	err := RetryWithBackoff(ctx, operation, config)
	if err != permanentErr {
		t.Errorf("RetryWithBackoff() error = %v, want %v", err, permanentErr)
	}

	if attempts != 1 {
		t.Errorf("operation called %d times, want 1 (no retry on permanent error)", attempts)
	}
}

func TestRetryWithBackoff_MaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()
	config := &RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}

	attempts := 0
	operation := func() error {
		attempts++
		return &HTTPError{StatusCode: 500, Message: "Server Error"}
	}

	err := RetryWithBackoff(ctx, operation, config)
	if err == nil {
		t.Error("RetryWithBackoff() error = nil, want error")
	}

	if attempts != 3 {
		t.Errorf("operation called %d times, want 3", attempts)
	}

	// Check error message contains retry count
	expectedMsg := "operation failed after 3 retries"
	if err != nil && err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("error message = %q, want prefix %q", err.Error(), expectedMsg)
	}
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := &RetryConfig{
		MaxRetries:     10,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}

	attempts := 0
	operation := func() error {
		attempts++
		if attempts == 2 {
			cancel() // Cancel after second attempt
		}
		return &HTTPError{StatusCode: 500, Message: "Server Error"}
	}

	err := RetryWithBackoff(ctx, operation, config)
	if err != context.Canceled {
		t.Errorf("RetryWithBackoff() error = %v, want %v", err, context.Canceled)
	}

	// Should stop retrying after context cancellation
	if attempts > 3 {
		t.Errorf("operation called %d times, expected to stop after context cancellation", attempts)
	}
}

func TestRetryWithBackoff_ExponentialBackoff(t *testing.T) {
	ctx := context.Background()
	config := &RetryConfig{
		MaxRetries:     4,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}

	attempts := 0
	backoffTimes := []time.Duration{}
	lastAttemptTime := time.Now()

	operation := func() error {
		currentTime := time.Now()
		if attempts > 0 {
			backoffTimes = append(backoffTimes, currentTime.Sub(lastAttemptTime))
		}
		lastAttemptTime = currentTime
		attempts++
		if attempts < 4 {
			return &HTTPError{StatusCode: 500, Message: "Server Error"}
		}
		return nil
	}

	err := RetryWithBackoff(ctx, operation, config)
	if err != nil {
		t.Errorf("RetryWithBackoff() error = %v, want nil", err)
	}

	// Verify exponential backoff pattern
	if len(backoffTimes) < 2 {
		t.Fatalf("expected at least 2 backoff times, got %d", len(backoffTimes))
	}

	// First backoff should be ~10ms
	if backoffTimes[0] < 10*time.Millisecond || backoffTimes[0] > 20*time.Millisecond {
		t.Errorf("first backoff = %v, want ~10ms", backoffTimes[0])
	}

	// Second backoff should be ~20ms (2x)
	if backoffTimes[1] < 20*time.Millisecond || backoffTimes[1] > 30*time.Millisecond {
		t.Errorf("second backoff = %v, want ~20ms", backoffTimes[1])
	}
}

func TestRetryWithBackoff_MaxBackoffCap(t *testing.T) {
	ctx := context.Background()
	config := &RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
	}

	attempts := 0
	backoffTimes := []time.Duration{}
	lastAttemptTime := time.Now()

	operation := func() error {
		currentTime := time.Now()
		if attempts > 0 {
			backoffTimes = append(backoffTimes, currentTime.Sub(lastAttemptTime))
		}
		lastAttemptTime = currentTime
		attempts++
		if attempts < 5 {
			return &HTTPError{StatusCode: 500, Message: "Server Error"}
		}
		return nil
	}

	err := RetryWithBackoff(ctx, operation, config)
	if err != nil {
		t.Errorf("RetryWithBackoff() error = %v, want nil", err)
	}

	// Later backoffs should be capped at MaxBackoff
	if len(backoffTimes) >= 3 {
		// Third backoff (200ms) should be capped at 100ms
		if backoffTimes[2] > 120*time.Millisecond {
			t.Errorf("third backoff = %v, want <=120ms (capped at MaxBackoff)", backoffTimes[2])
		}
	}
}

func TestRetryWithBackoff_DefaultConfig(t *testing.T) {
	ctx := context.Background()

	attempts := 0
	operation := func() error {
		attempts++
		return nil
	}

	// Pass nil config to use default
	err := RetryWithBackoff(ctx, operation, nil)
	if err != nil {
		t.Errorf("RetryWithBackoff() error = %v, want nil", err)
	}

	if attempts != 1 {
		t.Errorf("operation called %d times, want 1", attempts)
	}
}

func TestHTTPError_Error(t *testing.T) {
	err := &HTTPError{
		StatusCode: 500,
		Message:    "Internal Server Error",
	}

	expected := "HTTP 500: Internal Server Error"
	if err.Error() != expected {
		t.Errorf("HTTPError.Error() = %q, want %q", err.Error(), expected)
	}
}
