package validator

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/internal/tokens/tokenizers"
	configloader "github.com/vbonnet/dear-agent/pkg/config-loader"
)

// CountResult holds token counting results for a file.
type CountResult struct {
	File                  string  `json:"file"`
	FrontmatterTokens     int     `json:"frontmatter_tokens"`
	TotalTokens           int     `json:"total_tokens,omitempty"`
	FrontmatterPercentage float64 `json:"frontmatter_percentage,omitempty"`
	Method                string  `json:"method"`
	FrontmatterOnly       bool    `json:"frontmatter_only,omitempty"`
}

// CounterOptions configures the token counter behavior.
type CounterOptions struct {
	// FrontmatterOnly counts only frontmatter tokens (exclude body)
	FrontmatterOnly bool

	// Offline uses heuristic estimation (no tiktoken dependency)
	Offline bool

	// Model is the model name (currently unused, for future API integration)
	Model string
}

// YAMLTokenCounter counts tokens in YAML frontmatter.
type YAMLTokenCounter struct {
	options CounterOptions
}

// NewYAMLTokenCounter creates a new YAML token counter with the given options.
func NewYAMLTokenCounter(options CounterOptions) *YAMLTokenCounter {
	return &YAMLTokenCounter{
		options: options,
	}
}

// CountTokens counts tokens in the given text using available tokenizers.
//
// Returns:
//   - token count
//   - method used ("tiktoken", "simple", or "heuristic")
//   - error if all tokenization methods fail
func (c *YAMLTokenCounter) CountTokens(text string) (int, string, error) {
	// Offline mode: use character-based heuristic
	if c.options.Offline {
		return c.countHeuristic(text), "heuristic", nil
	}

	// Try tiktoken first (most accurate)
	if tok := tokenizers.Get("tiktoken"); tok != nil && tok.Available() {
		count, err := tok.Count(text)
		if err == nil {
			return count, "tiktoken", nil
		}
		// If tiktoken fails, fall through to simple tokenizer
	}

	// Try simple tokenizer (word-based, always available)
	if tok := tokenizers.Get("simple"); tok != nil && tok.Available() {
		count, err := tok.Count(text)
		if err == nil {
			return count, "simple", nil
		}
		// If simple fails, fall through to heuristic
	}

	// Fallback: character-based heuristic
	return c.countHeuristic(text), "heuristic-fallback", nil
}

// countHeuristic estimates tokens using character count.
// Approximation: 1 token ≈ 4 characters for English text.
func (c *YAMLTokenCounter) countHeuristic(text string) int {
	return len(text) / 4
}

// CountFile counts tokens in a file's frontmatter and optionally the full content.
//
// Returns:
//   - CountResult with token counts and metadata
//   - error if file cannot be read or has no frontmatter
func (c *YAMLTokenCounter) CountFile(filepath string) (*CountResult, error) {
	// Read file
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", filepath, err)
	}

	// Extract frontmatter
	frontmatter, _, err := configloader.ParseFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	// Count frontmatter tokens
	fmTokens, method, err := c.CountTokens(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("count frontmatter tokens: %w", err)
	}

	result := &CountResult{
		File:              filepath,
		FrontmatterTokens: fmTokens,
		Method:            method,
		FrontmatterOnly:   c.options.FrontmatterOnly,
	}

	// Count total tokens if requested
	if !c.options.FrontmatterOnly {
		totalTokens, _, err := c.CountTokens(string(content))
		if err != nil {
			return nil, fmt.Errorf("count total tokens: %w", err)
		}

		result.TotalTokens = totalTokens

		// Calculate percentage
		if totalTokens > 0 {
			result.FrontmatterPercentage = float64(fmTokens) / float64(totalTokens) * 100
		}
	}

	return result, nil
}
