package token

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCounter_Count(t *testing.T) {
	c := NewCounter()
	// 400 chars / 4 = 100 tokens
	text := strings.Repeat("a", 400)
	assert.Equal(t, 100, c.Count(text))

	// Empty string
	assert.Equal(t, 0, c.Count(""))
}

func TestCounter_ValidateBudget(t *testing.T) {
	c := NewCounter()

	// Within budget (1000 tokens = 4000 chars)
	text := strings.Repeat("a", 3000)
	assert.NoError(t, c.ValidateBudget(text))

	// Over budget
	text = strings.Repeat("a", 5000)
	assert.Error(t, c.ValidateBudget(text))
}

func TestCounter_IsOptimal(t *testing.T) {
	c := NewCounter()

	// 800 tokens = 3200 chars (lower bound)
	assert.True(t, c.IsOptimal(strings.Repeat("a", 3200)))

	// 1000 tokens = 4000 chars (upper bound)
	assert.True(t, c.IsOptimal(strings.Repeat("a", 4000)))

	// Too short
	assert.False(t, c.IsOptimal(strings.Repeat("a", 100)))

	// Too long
	assert.False(t, c.IsOptimal(strings.Repeat("a", 5000)))
}

func TestCounter_RemainingBudget(t *testing.T) {
	c := NewCounter()
	text := strings.Repeat("a", 2000) // 500 tokens
	assert.Equal(t, 500, c.RemainingBudget(text))

	// Over budget
	text = strings.Repeat("a", 5000) // 1250 tokens
	assert.Equal(t, 0, c.RemainingBudget(text))
}

func TestCounter_BudgetStatus(t *testing.T) {
	c := NewCounter()
	assert.Contains(t, c.BudgetStatus(strings.Repeat("a", 3500)), "Within budget")
	assert.Contains(t, c.BudgetStatus(strings.Repeat("a", 100)), "Under budget")
	assert.Contains(t, c.BudgetStatus(strings.Repeat("a", 5000)), "Over budget")
}
