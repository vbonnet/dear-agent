// Package limiter provides a token-budget stderr wrapper for hooks.
//
// Each hook gets a per-invocation token budget. When output exceeds the budget,
// further writes are silently dropped and a truncation notice is emitted once.
package limiter

import (
	"fmt"
	"io"
	"sync"
)

// DefaultMaxTokens is the default per-hook token budget.
const DefaultMaxTokens = 500

// approxTokens estimates token count as len/4 (good enough for English text).
func approxTokens(text []byte) int {
	return (len(text) + 3) / 4
}

// StderrLimiter wraps an io.Writer with a token budget.
// Once the budget is exhausted, it emits a truncation notice and drops further writes.
type StderrLimiter struct {
	w         io.Writer
	hookName  string
	maxTokens int

	mu            sync.Mutex
	tokensWritten int
	truncated     bool
}

// Wrap creates a new StderrLimiter around w with the given budget.
func Wrap(w io.Writer, hookName string, maxTokens int) *StderrLimiter {
	return &StderrLimiter{
		w:         w,
		hookName:  hookName,
		maxTokens: maxTokens,
	}
}

// Write implements io.Writer. It passes data through until the token budget
// is exhausted, then emits a single truncation notice and drops the rest.
func (l *StderrLimiter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.truncated {
		return len(p), nil // silently drop
	}

	tokens := approxTokens(p)
	if l.tokensWritten+tokens <= l.maxTokens {
		l.tokensWritten += tokens
		return l.w.Write(p)
	}

	// Budget exceeded: write what fits, then truncate.
	remaining := l.maxTokens - l.tokensWritten
	if remaining > 0 {
		// Approximate bytes for remaining tokens.
		byteLimit := remaining * 4
		if byteLimit > len(p) {
			byteLimit = len(p)
		}
		l.w.Write(p[:byteLimit]) //nolint:errcheck // best-effort partial write
		l.tokensWritten = l.maxTokens
	}

	l.truncated = true
	notice := fmt.Sprintf("\n[%s] Output truncated at %d tokens\n", l.hookName, l.maxTokens)
	l.w.Write([]byte(notice)) //nolint:errcheck // best-effort notice

	return len(p), nil
}

// TokensWritten returns the number of tokens written so far.
func (l *StderrLimiter) TokensWritten() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tokensWritten
}

// WasTruncated returns true if output was truncated.
func (l *StderrLimiter) WasTruncated() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.truncated
}
