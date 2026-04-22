// Package tokens provides token estimation for engram files.
//
// This package calculates token counts using multiple tokenization methods:
//   - char/4 heuristic (legacy baseline, always present)
//   - tiktoken (OpenAI cl100k_base encoding, optional)
//   - simple word tokenizer (validation baseline, optional)
//
// Tokenizers are pluggable via the tokenizers subpackage and run concurrently
// for performance.
package tokens

// Estimate contains token count estimations using multiple methods.
//
// The char/4 heuristic is always present for backward compatibility.
// Additional tokenizers (tiktoken, simple) are included if available.
type Estimate struct {
	// CharCount is the total character count across all engram files.
	CharCount int `json:"char_count"`

	// TokensChar4 is the legacy char/4 heuristic estimate.
	// This field is always present for backward compatibility.
	// Formula: CharCount / 4
	TokensChar4 int `json:"tokens_char4"`

	// Tokenizers contains token counts from registered tokenizers.
	// Map keys are tokenizer names (e.g., "tiktoken", "simple").
	// Only includes tokenizers that are available and succeeded.
	// May be nil or empty if no optional tokenizers are available.
	Tokenizers map[string]int `json:"tokenizers,omitempty"`
}
