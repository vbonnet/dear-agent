package limiter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	// CumulativeTokenLimit is the total token budget across all hooks in a session.
	CumulativeTokenLimit = 5000

	// ReducedBudget is the per-hook budget when the cumulative limit is exceeded.
	ReducedBudget = 250

	// CircuitBreakerThreshold is the number of consecutive violations before a hook is disabled.
	CircuitBreakerThreshold = 3

	budgetFileName = "hook-token-budget.json"
)

// SessionBudget tracks per-session cumulative token usage and circuit breaker state.
type SessionBudget struct {
	TotalTokens           int             `json:"total_tokens"`
	ConsecutiveViolations map[string]int  `json:"consecutive_violations"`
	DisabledHooks         map[string]bool `json:"disabled_hooks"`
}

// SessionTracker manages per-session token budget persistence.
type SessionTracker struct {
	mu   sync.Mutex
	path string
}

// NewSessionTracker creates a tracker that persists to ~/.agm/hook-token-budget.json.
// If basePath is empty, it defaults to ~/.agm/.
func NewSessionTracker(basePath string) *SessionTracker {
	if basePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		basePath = filepath.Join(home, ".agm")
	}
	return &SessionTracker{
		path: filepath.Join(basePath, budgetFileName),
	}
}

// Load reads the session budget from disk. Returns a zero-value budget if the file doesn't exist.
func (t *SessionTracker) Load() (*SessionBudget, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.loadLocked()
}

func (t *SessionTracker) loadLocked() (*SessionBudget, error) {
	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return newBudget(), nil
		}
		return nil, fmt.Errorf("load session budget: %w", err)
	}

	var b SessionBudget
	if err := json.Unmarshal(data, &b); err != nil {
		return newBudget(), nil //nolint:nilerr // corrupt file: reset
	}
	if b.ConsecutiveViolations == nil {
		b.ConsecutiveViolations = make(map[string]int)
	}
	if b.DisabledHooks == nil {
		b.DisabledHooks = make(map[string]bool)
	}
	return &b, nil
}

// Save persists the session budget to disk.
func (t *SessionTracker) Save(b *SessionBudget) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.saveLocked(b)
}

func (t *SessionTracker) saveLocked(b *SessionBudget) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session budget: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(t.path), 0700); err != nil {
		return fmt.Errorf("create budget dir: %w", err)
	}
	return os.WriteFile(t.path, data, 0600)
}

// EffectiveBudget returns the token budget a hook should use, accounting for
// cumulative usage. Returns 0 if the hook is disabled by the circuit breaker.
func (t *SessionTracker) EffectiveBudget(hookName string, defaultBudget int) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	b, err := t.loadLocked()
	if err != nil {
		return defaultBudget, err
	}

	if b.DisabledHooks[hookName] {
		return 0, nil
	}

	if b.TotalTokens >= CumulativeTokenLimit {
		return ReducedBudget, nil
	}

	return defaultBudget, nil
}

// RecordUsage updates the session budget after a hook invocation.
// It tracks cumulative tokens and manages the circuit breaker.
func (t *SessionTracker) RecordUsage(hookName string, tokensUsed int, wasTruncated bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	b, err := t.loadLocked()
	if err != nil {
		return err
	}

	b.TotalTokens += tokensUsed

	if wasTruncated {
		b.ConsecutiveViolations[hookName]++
		if b.ConsecutiveViolations[hookName] >= CircuitBreakerThreshold {
			b.DisabledHooks[hookName] = true
		}
	} else {
		b.ConsecutiveViolations[hookName] = 0
	}

	return t.saveLocked(b)
}

// IsDisabled returns true if the hook has been disabled by the circuit breaker.
func (t *SessionTracker) IsDisabled(hookName string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	b, err := t.loadLocked()
	if err != nil {
		return false, err
	}
	return b.DisabledHooks[hookName], nil
}

// Reset clears all session tracking state (for use at session start).
func (t *SessionTracker) Reset() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.saveLocked(newBudget())
}

func newBudget() *SessionBudget {
	return &SessionBudget{
		ConsecutiveViolations: make(map[string]int),
		DisabledHooks:         make(map[string]bool),
	}
}
