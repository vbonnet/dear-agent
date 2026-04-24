package tokens

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCalculate_WithTokenizers(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ai.md")
	testContent := "Hello, world! This is a test."
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	estimate, err := Calculate([]string{testFile})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}

	// Verify char/4 fields (always present)
	if estimate.CharCount != len(testContent) {
		t.Errorf("CharCount = %d, want %d", estimate.CharCount, len(testContent))
	}

	expectedChar4 := len(testContent) / 4
	if estimate.TokensChar4 != expectedChar4 {
		t.Errorf("TokensChar4 = %d, want %d", estimate.TokensChar4, expectedChar4)
	}

	// Verify tokenizers field
	if estimate.Tokenizers == nil {
		t.Error("Tokenizers map should not be nil")
	}

	// Simple tokenizer should always be present
	if _, ok := estimate.Tokenizers["simple"]; !ok {
		t.Error("Tokenizers map missing 'simple' entry")
	}

	// Tiktoken may or may not be present (depends on availability)
	// Don't fail if missing - just log
	if _, ok := estimate.Tokenizers["tiktoken"]; !ok {
		t.Log("Tiktoken not available (expected if no network or first run)")
	}
}

func TestCalculate_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 3 test files
	files := []string{
		filepath.Join(tmpDir, "file1.ai.md"),
		filepath.Join(tmpDir, "file2.ai.md"),
		filepath.Join(tmpDir, "file3.ai.md"),
	}

	contents := []string{
		"First file content",
		"Second file content",
		"Third file content",
	}

	for i, file := range files {
		if err := os.WriteFile(file, []byte(contents[i]), 0644); err != nil {
			t.Fatal(err)
		}
	}

	estimate, err := Calculate(files)
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}

	// Verify total character count
	expectedChars := len(strings.Join(contents, ""))
	if estimate.CharCount != expectedChars {
		t.Errorf("CharCount = %d, want %d", estimate.CharCount, expectedChars)
	}

	// Verify tokenizers ran
	if len(estimate.Tokenizers) == 0 {
		t.Error("Tokenizers map should have at least one entry (simple)")
	}
}

func TestCalculate_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.ai.md")
	if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	estimate, err := Calculate([]string{testFile})
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}

	if estimate.CharCount != 0 {
		t.Errorf("CharCount = %d, want 0", estimate.CharCount)
	}

	if estimate.TokensChar4 != 0 {
		t.Errorf("TokensChar4 = %d, want 0", estimate.TokensChar4)
	}
}

func TestCalculate_NoFiles(t *testing.T) {
	_, err := Calculate([]string{})
	if err == nil {
		t.Error("Calculate([]) should return error, got nil")
	}

	expectedErr := "no engrams provided"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("Error = %q, want to contain %q", err.Error(), expectedErr)
	}
}

func TestCalculate_NonExistentFile(t *testing.T) {
	_, err := Calculate([]string{"/nonexistent/file.ai.md"})
	if err == nil {
		t.Error("Calculate() with nonexistent file should return error")
	}

	if !strings.Contains(err.Error(), "failed to read") {
		t.Errorf("Error = %q, want to contain 'failed to read'", err.Error())
	}
}

func TestCalculate_BackwardCompatibility(t *testing.T) {
	// Verify structure is backward compatible with P2
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ai.md")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	estimate, err := Calculate([]string{testFile})
	if err != nil {
		t.Fatal(err)
	}

	// Old code accessing CharCount and TokensChar4 should work
	_ = estimate.CharCount
	_ = estimate.TokensChar4

	// JSON marshaling should include new field
	jsonData, err := json.Marshal(estimate)
	if err != nil {
		t.Fatal(err)
	}

	// Should contain "tokenizers" key
	if !strings.Contains(string(jsonData), "tokenizers") {
		t.Error("JSON should contain 'tokenizers' field")
	}

	// Verify JSON structure
	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatal(err)
	}

	// Check required fields
	if _, ok := decoded["char_count"]; !ok {
		t.Error("JSON missing 'char_count' field")
	}
	if _, ok := decoded["tokens_char4"]; !ok {
		t.Error("JSON missing 'tokens_char4' field")
	}
}

// TestCalculate_Concurrent verifies thread-safety
func TestCalculate_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ai.md")
	testContent := strings.Repeat("Hello world ", 1000)
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run Calculate() from multiple goroutines concurrently
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := Calculate([]string{testFile})
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent Calculate() failed: %v", err)
	}
}

// Benchmark Calculate performance
func BenchmarkCalculate(b *testing.B) {
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "bench.ai.md")
	testContent := strings.Repeat("This is benchmark text. ", 1000) // ~24KB
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Calculate([]string{testFile})
	}
}
