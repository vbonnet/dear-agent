package tokenizers

import (
	"strings"
	"sync"
	"testing"
)

// TestSimpleTokenizer_Count verifies basic tokenization behavior
func TestSimpleTokenizer_Count(t *testing.T) {
	tok := NewSimpleTokenizer()

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single word", "hello", 1},
		{"two words", "hello world", 2},
		{"with punctuation", "Hello, world!", 2},
		{"with multiple punctuation", "Mary*had,a%little_lamb", 5},
		{"unicode japanese", "こんにちは 世界", 2},          // Space-separated
		{"unicode japanese no spaces", "こんにちは世界", 1}, // No separators, counted as one token
		{"whitespace only", "   ", 0},
		{"multiple spaces", "word1   word2", 2},
		{"tabs and newlines", "word1\t\nword2", 2},
		{"leading/trailing spaces", "  hello  world  ", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tok.Count(tt.input)
			if err != nil {
				t.Fatalf("Count() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Count(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSimpleTokenizer_Available(t *testing.T) {
	tok := NewSimpleTokenizer()
	if !tok.Available() {
		t.Error("SimpleTokenizer should always be available")
	}
}

func TestSimpleTokenizer_Name(t *testing.T) {
	tok := NewSimpleTokenizer()
	if tok.Name() != "simple" {
		t.Errorf("Name() = %q, want \"simple\"", tok.Name())
	}
}

// TestTiktokenTokenizer_Basic verifies tiktoken initialization and usage
func TestTiktokenTokenizer_Basic(t *testing.T) {
	tok := NewTiktokenTokenizer()

	if tok.Name() != "tiktoken" {
		t.Errorf("Name() = %q, want \"tiktoken\"", tok.Name())
	}

	// Count() triggers lazy initialization
	count, err := tok.Count("Hello, world!")
	if err != nil {
		// Tiktoken may be unavailable (no network, etc.)
		// This is acceptable - skip test
		t.Skipf("Tiktoken unavailable: %v", err)
	}

	if count == 0 {
		t.Error("Count() should return non-zero for non-empty text")
	}

	// Verify available after successful initialization
	if !tok.Available() {
		t.Error("Available() should be true after successful Count()")
	}
}

func TestTiktokenTokenizer_EmptyString(t *testing.T) {
	tok := NewTiktokenTokenizer()
	count, err := tok.Count("")

	if !tok.Available() {
		t.Skip("Tiktoken unavailable")
	}

	if err != nil {
		t.Fatalf("Count(\"\") error = %v", err)
	}

	if count != 0 {
		t.Errorf("Count(\"\") = %d, want 0", count)
	}
}

// TestTiktokenTokenizer_Concurrent verifies thread-safety of lazy initialization
func TestTiktokenTokenizer_Concurrent(t *testing.T) {
	tok := NewTiktokenTokenizer()

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Trigger concurrent lazy initialization
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = tok.Count("test")
		}()
	}

	wg.Wait()

	// If tiktoken unavailable, this is fine - just skip verification
	if !tok.Available() {
		t.Skip("Tiktoken unavailable")
	}

	// Verify subsequent calls work
	count, err := tok.Count("hello")
	if err != nil {
		t.Errorf("Count() after concurrent init failed: %v", err)
	}
	if count == 0 {
		t.Error("Count() should return non-zero")
	}
}

// TestRegistry_Basic verifies registration and retrieval
func TestRegistry_Basic(t *testing.T) {
	// Registry should have auto-registered tokenizers from init()
	all := GetAll()

	if len(all) < 2 {
		t.Errorf("GetAll() returned %d tokenizers, want at least 2 (simple, tiktoken)", len(all))
	}

	// Verify we can get by name
	simple := Get("simple")
	if simple == nil {
		t.Error("Get(\"simple\") returned nil, want SimpleTokenizer")
	}
	if simple != nil && simple.Name() != "simple" {
		t.Errorf("Get(\"simple\").Name() = %q, want \"simple\"", simple.Name())
	}

	tiktoken := Get("tiktoken")
	if tiktoken == nil {
		t.Error("Get(\"tiktoken\") returned nil, want TiktokenTokenizer")
	}
	if tiktoken != nil && tiktoken.Name() != "tiktoken" {
		t.Errorf("Get(\"tiktoken\").Name() = %q, want \"tiktoken\"", tiktoken.Name())
	}
}

func TestRegistry_NonExistent(t *testing.T) {
	tok := Get("nonexistent")
	if tok != nil {
		t.Errorf("Get(\"nonexistent\") = %v, want nil", tok)
	}
}

// TestRegistry_DuplicateRegistration verifies error on duplicate
func TestRegistry_DuplicateRegistration(t *testing.T) {
	// Try to register duplicate "simple" tokenizer (should return error)
	err := Register(NewSimpleTokenizer())
	if err == nil {
		t.Error("Register() with duplicate name should return error")
	}
}

// Benchmark tokenizer performance
func BenchmarkSimpleTokenizer_Count(b *testing.B) {
	tok := NewSimpleTokenizer()
	text := strings.Repeat("Hello world ", 100) // ~1200 chars

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tok.Count(text)
	}
}

func BenchmarkTiktokenTokenizer_Count(b *testing.B) {
	tok := NewTiktokenTokenizer()
	text := strings.Repeat("Hello world ", 100) // ~1200 chars

	// Trigger initialization outside benchmark
	_, err := tok.Count("test")
	if err != nil {
		b.Skipf("Tiktoken unavailable: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tok.Count(text)
	}
}
