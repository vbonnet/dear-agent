package tokenizers

import (
	"strings"
	"unicode"
)

// SimpleTokenizer implements whitespace + punctuation tokenization.
//
// This is a validation baseline tokenizer, not intended for production use.
// It splits text on any character that is not a letter or number, providing
// a simple word-level token count for comparison with more sophisticated
// tokenizers like tiktoken.
//
// Algorithm:
//   - Split on whitespace, punctuation, and symbols
//   - Unicode-aware (handles international characters)
//   - No external dependencies (stdlib only)
type SimpleTokenizer struct {
	name string
}

// NewSimpleTokenizer creates a new simple tokenizer.
func NewSimpleTokenizer() *SimpleTokenizer {
	return &SimpleTokenizer{
		name: "simple",
	}
}

// Name returns "simple".
func (s *SimpleTokenizer) Name() string {
	return s.name
}

// Available always returns true (no dependencies).
func (s *SimpleTokenizer) Available() bool {
	return true // Always available (stdlib-only, no dependencies)
}

// Count returns the token count by splitting on non-alphanumeric characters.
//
// Examples:
//   - "Hello, world!" → 2 tokens (Hello, world)
//   - "Mary*had,a%little_lamb" → 5 tokens (Mary, had, a, little, lamb)
//   - "  multiple   spaces  " → splits correctly on whitespace
//   - Empty string → 0 tokens
func (s *SimpleTokenizer) Count(text string) (int, error) {
	if text == "" {
		return 0, nil
	}

	// Split on any character that is not a letter or number
	// This handles:
	// - Whitespace (spaces, tabs, newlines)
	// - Punctuation (.,!?;:)
	// - Symbols (*%$#@)
	// - Unicode punctuation and symbols
	tokens := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	return len(tokens), nil
}

func init() {
	Register(NewSimpleTokenizer())
}
