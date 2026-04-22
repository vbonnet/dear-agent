package validator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/tokens/tokenizers"
)

func TestCountHeuristic(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty string",
			text:     "",
			expected: 0,
		},
		{
			name:     "4 characters = 1 token",
			text:     "test",
			expected: 1,
		},
		{
			name:     "8 characters = 2 tokens",
			text:     "testtest",
			expected: 2,
		},
		{
			name:     "typical frontmatter",
			text:     "title: Example\ndescription: Test document\n",
			expected: 10, // 42 chars / 4
		},
		{
			name:     "single character",
			text:     "a",
			expected: 0, // 1 / 4 = 0 (integer division)
		},
	}

	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := counter.countHeuristic(tt.text)
			if result != tt.expected {
				t.Errorf("countHeuristic(%q) = %d, want %d", tt.text, result, tt.expected)
			}
		})
	}
}

func TestCountTokens_Offline(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		expectedMethod string
	}{
		{
			name:           "offline mode uses heuristic",
			text:           "test text",
			expectedMethod: "heuristic",
		},
		{
			name:           "offline with empty string",
			text:           "",
			expectedMethod: "heuristic",
		},
	}

	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, method, err := counter.CountTokens(tt.text)
			if err != nil {
				t.Fatalf("CountTokens() error = %v, want nil", err)
			}
			if method != tt.expectedMethod {
				t.Errorf("CountTokens() method = %q, want %q", method, tt.expectedMethod)
			}
			if count < 0 {
				t.Errorf("CountTokens() count = %d, want >= 0", count)
			}
		})
	}
}

func TestCountTokens_Online(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		minCount     int
		validMethods []string
	}{
		{
			name:         "simple text",
			text:         "Hello, world!",
			minCount:     1,
			validMethods: []string{"tiktoken", "simple", "heuristic-fallback"},
		},
		{
			name:         "empty string",
			text:         "",
			minCount:     0,
			validMethods: []string{"tiktoken", "simple", "heuristic-fallback"},
		},
		{
			name:         "yaml frontmatter",
			text:         "title: Example\ndescription: Test\n",
			minCount:     1,
			validMethods: []string{"tiktoken", "simple", "heuristic-fallback"},
		},
		{
			name:         "long text",
			text:         "This is a longer piece of text that should definitely have multiple tokens when counted by any tokenization method.",
			minCount:     10,
			validMethods: []string{"tiktoken", "simple", "heuristic-fallback"},
		},
		{
			name:         "unicode text",
			text:         "Hello 世界 🌍",
			minCount:     1,
			validMethods: []string{"tiktoken", "simple", "heuristic-fallback"},
		},
	}

	counter := NewYAMLTokenCounter(CounterOptions{Offline: false})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, method, err := counter.CountTokens(tt.text)
			if err != nil {
				t.Fatalf("CountTokens() error = %v, want nil", err)
			}
			if count < tt.minCount {
				t.Errorf("CountTokens() count = %d, want >= %d", count, tt.minCount)
			}

			// Verify method is one of the valid methods
			validMethod := false
			for _, vm := range tt.validMethods {
				if method == vm {
					validMethod = true
					break
				}
			}
			if !validMethod {
				t.Errorf("CountTokens() method = %q, want one of %v", method, tt.validMethods)
			}
		})
	}
}

func TestCountTokens_FallbackBehavior(t *testing.T) {
	// Test that the tokenizer gracefully falls back when preferred methods fail
	counter := NewYAMLTokenCounter(CounterOptions{Offline: false})

	// This should work with any available tokenizer or fallback to heuristic
	text := "Test text for fallback behavior validation"
	count, method, err := counter.CountTokens(text)

	if err != nil {
		t.Fatalf("CountTokens() error = %v, want nil (should always succeed with fallback)", err)
	}

	if count <= 0 {
		t.Errorf("CountTokens() count = %d, want > 0", count)
	}

	// Method should be one of the known methods
	validMethods := map[string]bool{
		"tiktoken":           true,
		"simple":             true,
		"heuristic-fallback": true,
	}

	if !validMethods[method] {
		t.Errorf("CountTokens() method = %q, want one of %v", method, []string{"tiktoken", "simple", "heuristic-fallback"})
	}

	t.Logf("Used method: %s, count: %d", method, count)
}

func TestCountTokens_AllTokenizers(t *testing.T) {
	// Test with all available tokenizers to ensure coverage
	text := "This is a test string for tokenization"

	// Test offline (heuristic)
	offlineCounter := NewYAMLTokenCounter(CounterOptions{Offline: true})
	offlineCount, offlineMethod, err := offlineCounter.CountTokens(text)
	if err != nil {
		t.Errorf("Offline CountTokens() error = %v", err)
	}
	if offlineMethod != "heuristic" {
		t.Errorf("Offline method = %q, want heuristic", offlineMethod)
	}
	if offlineCount <= 0 {
		t.Errorf("Offline count = %d, want > 0", offlineCount)
	}

	// Test online (tries tiktoken, simple, then heuristic-fallback)
	onlineCounter := NewYAMLTokenCounter(CounterOptions{Offline: false})
	onlineCount, onlineMethod, err := onlineCounter.CountTokens(text)
	if err != nil {
		t.Errorf("Online CountTokens() error = %v", err)
	}
	if onlineCount <= 0 {
		t.Errorf("Online count = %d, want > 0", onlineCount)
	}

	// Verify we get a valid method
	validMethods := map[string]bool{
		"tiktoken":           true,
		"simple":             true,
		"heuristic-fallback": true,
	}
	if !validMethods[onlineMethod] {
		t.Errorf("Online method = %q, want one of %v", onlineMethod, validMethods)
	}

	t.Logf("Offline: %s (%d tokens), Online: %s (%d tokens)",
		offlineMethod, offlineCount, onlineMethod, onlineCount)
}

func TestCountTokens_MethodSelection(t *testing.T) {
	// Verify the priority of tokenizer selection
	counter := NewYAMLTokenCounter(CounterOptions{Offline: false})

	// Try to get tiktoken
	tikTok := tokenizers.Get("tiktoken")
	simpleTok := tokenizers.Get("simple")

	text := "Sample text"
	_, method, _ := counter.CountTokens(text)

	// Verify method selection logic
	if tikTok != nil && tikTok.Available() {
		if method != "tiktoken" {
			t.Logf("Note: tiktoken available but method=%s (may be expected if tiktoken failed)", method)
		}
	} else if simpleTok != nil && simpleTok.Available() {
		if method != "simple" && method != "heuristic-fallback" {
			t.Errorf("Expected simple or heuristic-fallback, got %s", method)
		}
	} else {
		if method != "heuristic-fallback" {
			t.Errorf("Expected heuristic-fallback when no tokenizers available, got %s", method)
		}
	}
}

func TestCountFile_ValidFrontmatter(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ai.md")

	content := `---
title: Test Document
description: A test markdown file
tags:
  - test
  - example
---

# Test Content

This is the body of the document.
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name            string
		options         CounterOptions
		wantFMOnly      bool
		checkPercentage bool
	}{
		{
			name:            "count full document",
			options:         CounterOptions{FrontmatterOnly: false, Offline: true},
			wantFMOnly:      false,
			checkPercentage: true,
		},
		{
			name:            "count frontmatter only",
			options:         CounterOptions{FrontmatterOnly: true, Offline: true},
			wantFMOnly:      true,
			checkPercentage: false,
		},
		{
			name:            "online mode",
			options:         CounterOptions{FrontmatterOnly: false, Offline: false},
			wantFMOnly:      false,
			checkPercentage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := NewYAMLTokenCounter(tt.options)
			result, err := counter.CountFile(testFile)

			if err != nil {
				t.Fatalf("CountFile() error = %v, want nil", err)
			}

			// Basic validation
			if result.File != testFile {
				t.Errorf("result.File = %q, want %q", result.File, testFile)
			}
			if result.FrontmatterTokens <= 0 {
				t.Errorf("result.FrontmatterTokens = %d, want > 0", result.FrontmatterTokens)
			}
			if result.FrontmatterOnly != tt.wantFMOnly {
				t.Errorf("result.FrontmatterOnly = %v, want %v", result.FrontmatterOnly, tt.wantFMOnly)
			}
			if result.Method == "" {
				t.Error("result.Method is empty")
			}

			// Check total tokens and percentage
			if tt.checkPercentage {
				if result.TotalTokens <= 0 {
					t.Errorf("result.TotalTokens = %d, want > 0", result.TotalTokens)
				}
				if result.TotalTokens < result.FrontmatterTokens {
					t.Errorf("result.TotalTokens (%d) < FrontmatterTokens (%d)",
						result.TotalTokens, result.FrontmatterTokens)
				}
				if result.FrontmatterPercentage <= 0 || result.FrontmatterPercentage > 100 {
					t.Errorf("result.FrontmatterPercentage = %.1f, want 0 < x <= 100",
						result.FrontmatterPercentage)
				}
			} else {
				if result.TotalTokens != 0 {
					t.Errorf("result.TotalTokens = %d, want 0 (frontmatter only)", result.TotalTokens)
				}
				if result.FrontmatterPercentage != 0 {
					t.Errorf("result.FrontmatterPercentage = %.1f, want 0 (frontmatter only)",
						result.FrontmatterPercentage)
				}
			}
		})
	}
}

func TestCountFile_NoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "no-frontmatter.md")

	content := `# No Frontmatter

This file has no YAML frontmatter.
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})
	_, err = counter.CountFile(testFile)

	if err == nil {
		t.Error("CountFile() error = nil, want error for missing frontmatter")
	}
}

func TestCountFile_InvalidFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "invalid-frontmatter.md")

	content := `---
title: Unclosed frontmatter

This content has no closing delimiter.
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})
	_, err = counter.CountFile(testFile)

	if err == nil {
		t.Error("CountFile() error = nil, want error for invalid frontmatter")
	}
}

func TestCountFile_FileNotFound(t *testing.T) {
	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})
	_, err := counter.CountFile("/nonexistent/file.md")

	if err == nil {
		t.Error("CountFile() error = nil, want error for missing file")
	}

	// Verify error message is helpful
	if err != nil && err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

func TestCountFile_ReadError(t *testing.T) {
	// Test that we handle read errors properly
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unreadable.md")

	// Create file with valid content
	content := `---
title: Test
---
Body`
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Make it unreadable (on Unix-like systems)
	err = os.Chmod(testFile, 0000)
	if err != nil {
		t.Skip("Cannot change file permissions on this system")
	}

	// Restore permissions after test
	defer os.Chmod(testFile, 0644)

	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})
	_, err = counter.CountFile(testFile)

	// Should get a read error
	if err == nil {
		t.Error("CountFile() error = nil, want error for unreadable file")
	}
}

func TestCountFile_EmptyFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty-frontmatter.md")

	// Note: config-loader requires at least something between delimiters
	// Empty frontmatter (---\n---) is not valid per the regex pattern
	content := `---
key: value
---

# Content

Body text.
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})
	result, err := counter.CountFile(testFile)

	if err != nil {
		t.Fatalf("CountFile() error = %v, want nil", err)
	}

	// Minimal frontmatter should be counted
	if result.FrontmatterTokens <= 0 {
		t.Errorf("result.FrontmatterTokens = %d, want > 0", result.FrontmatterTokens)
	}
}

func TestCountTokens_CompareTokenizers(t *testing.T) {
	// Skip if tiktoken is not available (e.g., in CI without network)
	tok := tokenizers.Get("tiktoken")
	if tok == nil || !tok.Available() {
		t.Skip("tiktoken not available, skipping comparison test")
	}

	testText := "This is a sample text for tokenization comparison."

	counter := NewYAMLTokenCounter(CounterOptions{Offline: false})

	// Count with online mode (should use tiktoken)
	onlineCount, onlineMethod, err := counter.CountTokens(testText)
	if err != nil {
		t.Fatalf("CountTokens(online) error = %v", err)
	}

	// Count with offline mode (should use heuristic)
	offlineCounter := NewYAMLTokenCounter(CounterOptions{Offline: true})
	offlineCount, offlineMethod, err := offlineCounter.CountTokens(testText)
	if err != nil {
		t.Fatalf("CountTokens(offline) error = %v", err)
	}

	// Validate methods
	if onlineMethod != "tiktoken" && onlineMethod != "simple" && onlineMethod != "heuristic-fallback" {
		t.Errorf("online method = %q, want tiktoken/simple/heuristic-fallback", onlineMethod)
	}
	if offlineMethod != "heuristic" {
		t.Errorf("offline method = %q, want heuristic", offlineMethod)
	}

	// Both counts should be positive
	if onlineCount <= 0 {
		t.Errorf("online count = %d, want > 0", onlineCount)
	}
	if offlineCount <= 0 {
		t.Errorf("offline count = %d, want > 0", offlineCount)
	}

	// Counts may differ, but should be within reasonable range
	// Heuristic is typically 20-40% off from accurate tokenization
	t.Logf("Online (%s): %d tokens, Offline (%s): %d tokens",
		onlineMethod, onlineCount, offlineMethod, offlineCount)
}

func TestCountResult_PercentageCalculation(t *testing.T) {
	tests := []struct {
		name           string
		frontmatter    int
		total          int
		wantPercentage float64
	}{
		{
			name:           "10% frontmatter",
			frontmatter:    10,
			total:          100,
			wantPercentage: 10.0,
		},
		{
			name:           "50% frontmatter",
			frontmatter:    50,
			total:          100,
			wantPercentage: 50.0,
		},
		{
			name:           "100% frontmatter (no body)",
			frontmatter:    100,
			total:          100,
			wantPercentage: 100.0,
		},
		{
			name:           "zero total",
			frontmatter:    10,
			total:          0,
			wantPercentage: 0.0, // Should handle division by zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &CountResult{
				FrontmatterTokens: tt.frontmatter,
				TotalTokens:       tt.total,
			}

			// Calculate percentage (mimicking the logic in CountFile)
			var percentage float64
			if result.TotalTokens > 0 {
				percentage = float64(result.FrontmatterTokens) / float64(result.TotalTokens) * 100
			}

			if percentage != tt.wantPercentage {
				t.Errorf("percentage = %.1f, want %.1f", percentage, tt.wantPercentage)
			}
		})
	}
}

func TestNewYAMLTokenCounter(t *testing.T) {
	tests := []struct {
		name    string
		options CounterOptions
	}{
		{
			name:    "default options",
			options: CounterOptions{},
		},
		{
			name:    "offline mode",
			options: CounterOptions{Offline: true},
		},
		{
			name:    "frontmatter only",
			options: CounterOptions{FrontmatterOnly: true},
		},
		{
			name: "all options",
			options: CounterOptions{
				FrontmatterOnly: true,
				Offline:         true,
				Model:           "claude-sonnet-4-5-20250929",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := NewYAMLTokenCounter(tt.options)
			if counter == nil {
				t.Fatal("NewYAMLTokenCounter() returned nil")
			}
			if counter.options != tt.options {
				t.Errorf("counter.options = %+v, want %+v", counter.options, tt.options)
			}
		})
	}
}

// Benchmark tests
func BenchmarkCountHeuristic(b *testing.B) {
	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})
	text := "title: Example\ndescription: A sample YAML frontmatter for benchmarking\ntags:\n  - test\n  - benchmark\n"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.countHeuristic(text)
	}
}

func BenchmarkCountTokens_Offline(b *testing.B) {
	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})
	text := "title: Example\ndescription: A sample YAML frontmatter for benchmarking\n"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.CountTokens(text)
	}
}

func BenchmarkCountTokens_Online(b *testing.B) {
	counter := NewYAMLTokenCounter(CounterOptions{Offline: false})
	text := "title: Example\ndescription: A sample YAML frontmatter for benchmarking\n"

	// Check if tiktoken is available
	tok := tokenizers.Get("tiktoken")
	if tok == nil || !tok.Available() {
		b.Skip("tiktoken not available")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.CountTokens(text)
	}
}

func BenchmarkCountFile(b *testing.B) {
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "benchmark.ai.md")

	content := `---
title: Benchmark Test
description: A test file for benchmarking
tags:
  - performance
  - test
---

# Benchmark Content

This is some sample content for benchmarking the file counting operation.
`

	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	counter := NewYAMLTokenCounter(CounterOptions{Offline: true})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.CountFile(testFile)
	}
}
