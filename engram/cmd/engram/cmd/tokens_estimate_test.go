package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/tokens"
)

func TestExpandGlobs(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()
	createTestFile(t, filepath.Join(tmpDir, "file1.md"), "test content 1")
	createTestFile(t, filepath.Join(tmpDir, "file2.md"), "test content 2")
	createTestFile(t, filepath.Join(tmpDir, "file3.txt"), "test content 3")
	createTestFile(t, filepath.Join(tmpDir, "nested", "file4.md"), "test content 4")

	tests := []struct {
		name     string
		patterns []string
		want     int // expected file count
		wantErr  bool
	}{
		{
			name:     "single file",
			patterns: []string{filepath.Join(tmpDir, "file1.md")},
			want:     1,
			wantErr:  false,
		},
		{
			name:     "multiple files",
			patterns: []string{filepath.Join(tmpDir, "file1.md"), filepath.Join(tmpDir, "file2.md")},
			want:     2,
			wantErr:  false,
		},
		{
			name:     "glob pattern",
			patterns: []string{filepath.Join(tmpDir, "*.md")},
			want:     2, // file1.md and file2.md, but not file3.txt
			wantErr:  false,
		},
		{
			name:     "deduplicate overlapping patterns",
			patterns: []string{filepath.Join(tmpDir, "file1.md"), filepath.Join(tmpDir, "*.md")},
			want:     2, // file1.md should only appear once
			wantErr:  false,
		},
		{
			name:     "invalid glob pattern",
			patterns: []string{"[invalid"},
			want:     0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandGlobs(tt.patterns)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandGlobs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.want {
				t.Errorf("expandGlobs() got %d files, want %d", len(got), tt.want)
			}
		})
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{123, "123"},
		{1234, "1,234"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{-1234, "-1,234"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatNumber(tt.input)
			if got != tt.want {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestFormatJSON removed - JSON formatting now handled by cliframe library
// Integration tests in tokens_estimate_integration_test.go verify end-to-end JSON output

func TestFormatText(t *testing.T) {
	estimate := &tokens.Estimate{
		CharCount:   12450,
		TokensChar4: 3112,
		Tokenizers: map[string]int{
			"tiktoken": 2847,
			"simple":   2923,
		},
	}

	files := []string{"file1.md", "file2.md"}
	output := formatTokensText(estimate, files, "")

	// Verify output contains expected elements
	if !strings.Contains(output, "Token estimate for 2 files:") {
		t.Errorf("Output missing file count header")
	}
	if !strings.Contains(output, "12,450 chars") {
		t.Errorf("Output missing formatted character count")
	}
	if !strings.Contains(output, "3,112 tokens") {
		t.Errorf("Output missing formatted char/4 tokens")
	}
	if !strings.Contains(output, "tiktoken") {
		t.Errorf("Output missing tiktoken tokenizer")
	}
	if !strings.Contains(output, "simple") {
		t.Errorf("Output missing simple tokenizer")
	}
	if !strings.Contains(output, "Estimated cost:") {
		t.Errorf("Output missing cost estimation section")
	}
	if !strings.Contains(output, "Sonnet 4.5") {
		t.Errorf("Output missing Sonnet 4.5 cost")
	}
}

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name         string
		tokens       int
		pricePerMTok float64
		want         float64
	}{
		{"zero tokens", 0, 3.0, 0.0},
		{"1000 tokens", 1000, 3.0, 0.003},
		{"1 million tokens", 1_000_000, 3.0, 3.0},
		{"haiku pricing", 500_000, 1.0, 0.5},
		{"opus pricing", 100_000, 15.0, 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateCost(tt.tokens, tt.pricePerMTok)
			if got != tt.want {
				t.Errorf("estimateCost(%d, %.1f) = %.4f, want %.4f", tt.tokens, tt.pricePerMTok, got, tt.want)
			}
		})
	}
}

func TestHasGlobChars(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"file.md", false},
		{"*.md", true},
		{"file?.md", true},
		{"file[0-9].md", true},
		{"path/to/file.md", false},
		{"**/*.md", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := hasGlobChars(tt.input)
			if got != tt.want {
				t.Errorf("hasGlobChars(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Helper function to create test files
func createTestFile(t *testing.T, path, content string) {
	t.Helper()

	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", dir, err)
	}

	// Write file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create file %s: %v", path, err)
	}
}
