// Package token provides token-related functionality.
package token

import (
	"fmt"
)

const (
	MinTokenBudget = 800
	MaxTokenBudget = 1000
	CharsPerToken  = 4
)

// Counter provides token counting functionality
type Counter struct{}

// NewCounter creates a new token counter
func NewCounter() *Counter {
	return &Counter{}
}

// Count returns approximate token count for the given text
func (c *Counter) Count(text string) int {
	runeCount := len([]rune(text))
	return runeCount / CharsPerToken
}

// ValidateBudget checks if text is within token budget
func (c *Counter) ValidateBudget(text string) error {
	tokens := c.Count(text)
	if tokens > MaxTokenBudget {
		return fmt.Errorf("token budget exceeded: %d tokens (max: %d)", tokens, MaxTokenBudget)
	}
	return nil
}

// IsWithinBudget returns true if text is within budget
func (c *Counter) IsWithinBudget(text string) bool {
	return c.Count(text) <= MaxTokenBudget
}

// IsOptimal returns true if text is within optimal range (800-1000 tokens)
func (c *Counter) IsOptimal(text string) bool {
	tokens := c.Count(text)
	return tokens >= MinTokenBudget && tokens <= MaxTokenBudget
}

// BudgetStatus returns a string describing the budget status
func (c *Counter) BudgetStatus(text string) string {
	tokens := c.Count(text)
	if tokens > MaxTokenBudget {
		return fmt.Sprintf("Over budget: %d tokens (max: %d)", tokens, MaxTokenBudget)
	}
	if tokens < MinTokenBudget {
		return fmt.Sprintf("Under budget: %d tokens (min: %d recommended)", tokens, MinTokenBudget)
	}
	return fmt.Sprintf("Within budget: %d tokens (optimal: %d-%d)", tokens, MinTokenBudget, MaxTokenBudget)
}

// RemainingBudget returns how many tokens remain in budget
func (c *Counter) RemainingBudget(text string) int {
	tokens := c.Count(text)
	remaining := MaxTokenBudget - tokens
	if remaining < 0 {
		return 0
	}
	return remaining
}
