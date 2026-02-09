package helpers

import "sync"

// APIQuota manages API call limits for contract tests.
//
// Provides thread-safe quota tracking to prevent exceeding API rate limits
// during contract testing. Default quota is 20 calls.
//
// Example:
//
//	quota := GetAPIQuota()
//	if !quota.Consume() {
//	    t.Skip("API quota exhausted")
//	}
//	// ... make API call ...
type APIQuota struct {
	max  int
	used int
	mu   sync.Mutex
}

// Global quota instance (initialized to 20 calls)
var globalQuota = &APIQuota{max: 20, used: 0}

// GetAPIQuota returns the global API quota instance.
//
// Returns a shared quota tracker used across all contract tests to prevent
// exceeding API rate limits.
//
// Example:
//
//	func TestClaudeAPI(t *testing.T) {
//	    quota := helpers.GetAPIQuota()
//	    if !quota.Consume() {
//	        t.Skip("API quota exhausted")
//	    }
//	    // ... test code ...
//	}
func GetAPIQuota() *APIQuota {
	return globalQuota
}

// Consume attempts to use one API call from the quota.
//
// Returns true if quota was available and consumed, false if quota is exhausted.
// Thread-safe for concurrent test execution.
//
// Example:
//
//	quota := helpers.GetAPIQuota()
//	if !quota.Consume() {
//	    t.Skip("API quota exhausted")
//	}
func (q *APIQuota) Consume() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.used >= q.max {
		return false
	}
	q.used++
	return true
}

// Remaining returns the number of API calls left in the quota.
//
// Thread-safe for concurrent access.
//
// Example:
//
//	quota := helpers.GetAPIQuota()
//	fmt.Printf("Calls remaining: %d\n", quota.Remaining())
func (q *APIQuota) Remaining() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.max - q.used
}

// Reset resets the quota back to maximum (for testing).
//
// WARNING: Only use in test cleanup or setup. Not thread-safe with ongoing
// Consume() calls.
//
// Example:
//
//	func TestQuota(t *testing.T) {
//	    quota := helpers.GetAPIQuota()
//	    defer quota.Reset() // Cleanup after test
//	    // ... test code ...
//	}
func (q *APIQuota) Reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.used = 0
}
