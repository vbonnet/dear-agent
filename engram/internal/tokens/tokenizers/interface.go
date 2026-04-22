// Package tokenizers provides pluggable tokenization implementations for token estimation.
//
// This package defines the Tokenizer interface and provides built-in implementations
// (tiktoken, simple) that can be registered and used concurrently for token counting.
package tokenizers

// Tokenizer defines the interface for all tokenization methods.
//
// Implementations must be thread-safe as Count() may be called concurrently
// from multiple goroutines.
type Tokenizer interface {
	// Name returns the unique identifier for this tokenizer (e.g., "tiktoken", "simple").
	// The name is used as the key in the Estimate.Tokenizers map.
	Name() string

	// Count returns the token count for the given text.
	// Returns error if tokenization fails (e.g., encoding unavailable, initialization error).
	//
	// Implementations should handle empty strings gracefully (return 0, nil).
	Count(text string) (int, error)

	// Available reports whether this tokenizer is ready for use.
	//
	// Returns false if required dependencies are missing (e.g., tiktoken dictionary
	// not downloaded) or initialization failed. Unavailable tokenizers are skipped
	// during token estimation.
	Available() bool
}
