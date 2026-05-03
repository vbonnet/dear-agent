package tokens

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vbonnet/dear-agent/internal/tokens/tokenizers"
)

// Calculate computes token estimates for the given engram files.
//
// Uses multiple tokenization methods:
//   - char/4 heuristic (always present, baseline)
//   - All registered tokenizers (tiktoken, simple, etc.) running concurrently
//
// Tokenizers are registered automatically via init() functions in the
// tokenizers subpackage. Only available tokenizers are used; unavailable
// tokenizers are skipped gracefully.
//
// Example:
//
//	estimate, err := Calculate([]string{"~/engrams/example.ai.md"})
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("char/4: %d tokens\n", estimate.TokensChar4)
//	if count, ok := estimate.Tokenizers["tiktoken"]; ok {
//	    fmt.Printf("tiktoken: %d tokens\n", count)
//	}
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func Calculate(engramPaths []string) (*Estimate, error) {
	if len(engramPaths) == 0 {
		return nil, fmt.Errorf("no engrams provided")
	}

	// 1. Read all engram files, concatenate text
	var fullText strings.Builder
	totalChars := 0

	for _, path := range engramPaths {
		// Clean path to prevent directory traversal
		cleanPath := filepath.Clean(path)

		content, err := os.ReadFile(cleanPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}

		fullText.Write(content)
		totalChars += len(content)
	}

	text := fullText.String()

	// 2. Calculate char/4 baseline (always present)
	tokensChar4 := totalChars / 4

	// 3. Run all registered tokenizers in parallel
	tokenizerResults := runTokenizers(text)

	// 4. Return aggregated estimate
	return &Estimate{
		CharCount:   totalChars,
		TokensChar4: tokensChar4,
		Tokenizers:  tokenizerResults,
	}, nil
}

// runTokenizers executes all registered tokenizers concurrently.
//
// Tokenizers are run in parallel using goroutines for performance.
// Each tokenizer that is unavailable or fails is skipped gracefully
// (no error returned, just not included in results).
//
// Returns map[tokenizerName]count for successful tokenizers only.
func runTokenizers(text string) map[string]int {
	allTokenizers := tokenizers.GetAll()

	results := make(map[string]int)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, tok := range allTokenizers {
		// Skip if tokenizer not available
		if !tok.Available() {
			continue
		}

		wg.Add(1)
		go func(t tokenizers.Tokenizer) {
			defer wg.Done()

			// Recover from panics (defensive — silently skip panicked tokenizers
			// until structured logging is available; tracked separately).
			defer func() { _ = recover() }()

			count, err := t.Count(text)
			if err != nil {
				// TODO: Log error when structured logging available
				return // Skip failed tokenizer
			}

			// Add result to map (thread-safe via mutex)
			mu.Lock()
			results[t.Name()] = count
			mu.Unlock()
		}(tok) // Pass tok as parameter to avoid closure capture bug
	}

	wg.Wait()
	return results
}
