package tokenizers

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// TiktokenTokenizer wraps pkoukk/tiktoken-go for OpenAI cl100k_base encoding.
//
// This tokenizer provides accurate token counts compatible with Claude and GPT-4
// models, which use the cl100k_base byte-pair encoding (BPE).
//
// Features:
//   - Lazy initialization (dictionary downloaded on first use)
//   - Thread-safe (safe for concurrent Count() calls)
//   - Graceful degradation (Available() returns false if initialization fails)
//   - Automatic caching (dictionaries cached to ~/.engram/cache/tiktoken)
//
// The tokenizer requires downloading a ~1MB dictionary on first use. If the
// download fails (network unavailable, permissions), the tokenizer will be
// marked as unavailable and skipped during token estimation.
type TiktokenTokenizer struct {
	name      string
	encoding  *tiktoken.Tiktoken
	available bool
	initOnce  sync.Once
	mu        sync.RWMutex
}

// NewTiktokenTokenizer creates a new tiktoken tokenizer.
//
// Initialization is lazy - the cl100k_base encoding is not loaded until the
// first Count() call. This avoids blocking startup if the dictionary needs
// to be downloaded.
func NewTiktokenTokenizer() *TiktokenTokenizer {
	return &TiktokenTokenizer{
		name: "tiktoken",
	}
}

// Name returns "tiktoken".
func (t *TiktokenTokenizer) Name() string {
	return t.name
}

// Available reports whether tiktoken initialization succeeded.
//
// Returns false if:
//   - Dictionary download failed
//   - Cache directory unwritable
//   - tiktoken.GetEncoding() returned an error
//
// Available() is thread-safe and can be called before or after Count().
func (t *TiktokenTokenizer) Available() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.available
}

// Count returns the token count using cl100k_base encoding.
//
// On the first call, this triggers lazy initialization:
//  1. Set TIKTOKEN_CACHE_DIR if not already set
//  2. Create cache directory (~/.engram/cache/tiktoken)
//  3. Download cl100k_base dictionary (if not cached)
//  4. Initialize encoding
//
// If initialization fails, subsequent calls will return an error.
//
// Thread-safe: Multiple goroutines can call Count() concurrently.
func (t *TiktokenTokenizer) Count(text string) (int, error) {
	// Lazy initialization (thread-safe via sync.Once)
	t.initOnce.Do(func() {
		t.initialize()
	})

	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.available {
		return 0, fmt.Errorf("tiktoken not available (initialization failed)")
	}

	// Encode text to token IDs
	// Parameters:
	//   - text: input text
	//   - nil: allowedSpecial (no special tokens)
	//   - nil: disallowedSpecial (no special tokens)
	tokens := t.encoding.Encode(text, nil, nil)

	return len(tokens), nil
}

// initialize attempts to load the cl100k_base encoding.
//
// Sets available=true on success, available=false on failure.
// Handles cache directory setup and provides graceful fallback.
func (t *TiktokenTokenizer) initialize() {
	// Set cache directory if not already set
	if os.Getenv("TIKTOKEN_CACHE_DIR") == "" {
		// Expand $HOME to actual home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// Fallback to ~ expansion (may not work on all systems)
			homeDir = os.ExpandEnv("$HOME")
		}

		cacheDir := filepath.Join(homeDir, ".engram", "cache", "tiktoken")
		os.Setenv("TIKTOKEN_CACHE_DIR", cacheDir)

		// Ensure cache directory exists; ignore failure — tiktoken falls back
		// to its own tmpdir if the cache dir can't be created.
		_ = os.MkdirAll(cacheDir, 0o700)
	}

	// Load cl100k_base encoding (used by Claude/GPT-4)
	encoding, err := tiktoken.GetEncoding("cl100k_base")

	t.mu.Lock()
	defer t.mu.Unlock()

	if err != nil {
		t.available = false
		// TODO: Log error when structured logging available
		// Expected errors: network unavailable, permission denied, corrupted cache
		return
	}

	t.encoding = encoding
	t.available = true
}

func init() {
	Register(NewTiktokenTokenizer())
}
