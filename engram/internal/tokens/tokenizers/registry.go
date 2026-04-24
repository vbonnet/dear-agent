package tokenizers

import "sync"

var (
	// Global registry of all tokenizers.
	// Tokenizers are registered automatically via init() functions.
	registry = make(map[string]Tokenizer)
	mu       sync.RWMutex
)

// Register adds a tokenizer to the global registry.
//
// This is typically called from init() functions in tokenizer implementation
// files (simple.go, tiktoken.go). Panics if a tokenizer with the same name
// is already registered.
//
// Example:
//
//	func init() {
//	    Register(NewSimpleTokenizer())
//	}
//
// Thread-safe: Can be called concurrently (though typically only called from init()).
func Register(t Tokenizer) {
	mu.Lock()
	defer mu.Unlock()

	name := t.Name()
	if _, exists := registry[name]; exists {
		panic("tokenizer already registered: " + name)
	}
	registry[name] = t
}

// GetAll returns all registered tokenizers.
//
// Returns a copy of the registry slice to prevent concurrent modification.
// The returned slice is safe to iterate over even if Register() is called
// concurrently (though in practice, all tokenizers are registered during init()).
//
// Thread-safe: Safe to call concurrently with Register() and Get().
func GetAll() []Tokenizer {
	mu.RLock()
	defer mu.RUnlock()

	tokenizers := make([]Tokenizer, 0, len(registry))
	for _, t := range registry {
		tokenizers = append(tokenizers, t)
	}
	return tokenizers
}

// Get returns a tokenizer by name, or nil if not found.
//
// Example:
//
//	tok := Get("tiktoken")
//	if tok != nil && tok.Available() {
//	    count, _ := tok.Count(text)
//	}
//
// Thread-safe: Safe to call concurrently.
func Get(name string) Tokenizer {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}
