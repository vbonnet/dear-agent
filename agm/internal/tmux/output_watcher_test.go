package tmux

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOutputWatcher_WaitForPattern_Simple(t *testing.T) {
	input := `%output %0 Starting Claude...
%output %0 Welcome to Claude Code
%output %0 Rename successful
%output %0 Ready to help`

	reader := strings.NewReader(input)
	watcher := NewOutputWatcher(reader)

	err := watcher.WaitForPattern("Rename successful", 1*time.Second)
	assert.NoError(t, err, "should find pattern in simple text")
}

func TestOutputWatcher_WaitForPattern_WithOctalEscapes(t *testing.T) {
	// Simulates tmux output with octal escapes
	// \040 = space, \012 = newline
	input := `%output %0 Rename\040successful\012`

	reader := strings.NewReader(input)
	watcher := NewOutputWatcher(reader)

	err := watcher.WaitForPattern("Rename successful", 1*time.Second)
	assert.NoError(t, err, "should decode octal escapes and find pattern")
}

func TestOutputWatcher_WaitForPattern_Timeout(t *testing.T) {
	// Use a slow reader that blocks longer than the timeout
	reader := &slowReader{delay: 200 * time.Millisecond}
	watcher := NewOutputWatcher(reader)

	err := watcher.WaitForPattern("Pattern not here", 100*time.Millisecond)
	assert.Error(t, err, "should timeout when pattern not found")
	// Can be either timeout or EOF depending on timing
}

func TestOutputWatcher_WaitForPattern_NonOutputLine(t *testing.T) {
	// Test matching patterns in non-%output lines (like %end, %error)
	input := `%output %0 Some content
%end 1234
%output %0 More content`

	reader := strings.NewReader(input)
	watcher := NewOutputWatcher(reader)

	err := watcher.WaitForPattern("%end", 1*time.Second)
	assert.NoError(t, err, "should find pattern in non-%output lines")
}

func TestUnescapeOctal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "space escape",
			input:    "hello\\040world",
			expected: "hello world",
		},
		{
			name:     "newline escape",
			input:    "line1\\012line2",
			expected: "line1\nline2",
		},
		{
			name:     "tab escape",
			input:    "tab\\011here",
			expected: "tab\there",
		},
		{
			name:     "invalid escape (not 3 digits)",
			input:    "no\\12escape",
			expected: "no\\12escape",
		},
		{
			name:     "multiple spaces",
			input:    "\\040\\040spaces",
			expected: "  spaces",
		},
		{
			name:     "mixed content",
			input:    "Hello\\040World\\012Next\\040Line",
			expected: "Hello World\nNext Line",
		},
		{
			name:     "no escapes",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "backslash but not octal",
			input:    "not\\999octal",
			expected: "not\\999octal",
		},
		{
			name:     "carriage return",
			input:    "with\\015return",
			expected: "with\rreturn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := unescapeOctal(tt.input)
			assert.NoError(t, err, "should not error on valid input")
			assert.Equal(t, tt.expected, result, "unescaped string should match expected")
		})
	}
}

func TestIsOctal(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123", true},
		{"777", true},
		{"000", true},
		{"012", true},
		{"789", false}, // 8 and 9 are not octal
		{"abc", false},
		{"12a", false},
		{"", false},
		{"040", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isOctal(tt.input)
			assert.Equal(t, tt.expected, result, "isOctal(%q) should be %v", tt.input, tt.expected)
		})
	}
}

func TestExtractOutputContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple output",
			input:    "%output %0 Hello World",
			expected: "Hello World",
		},
		{
			name:     "with octal escapes",
			input:    "%output %0 Hello\\040World",
			expected: "Hello World",
		},
		{
			name:     "empty content",
			input:    "%output %0",
			expected: "",
		},
		{
			name:     "multi-word pane id",
			input:    "%output %0 Content here",
			expected: "Content here",
		},
		{
			name:     "with newlines",
			input:    "%output %0 Line1\\012Line2",
			expected: "Line1\nLine2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractOutputContent(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOutputWatcher_GetRecentOutput(t *testing.T) {
	input := `%output %0 Line 1
%output %0 Line 2
%output %0 Line 3
%output %0 Line 4
%output %0 Line 5`

	reader := strings.NewReader(input)
	watcher := NewOutputWatcher(reader)

	// Read all lines
	watcher.WaitForPattern("Line 5", 1*time.Second)

	// Get last 3 lines
	recent := watcher.GetRecentOutput(3)
	assert.Len(t, recent, 3, "should return requested number of lines")
	assert.Contains(t, recent[2], "Line 5", "should contain most recent line")

	// Request more lines than available
	allLines := watcher.GetRecentOutput(100)
	assert.Len(t, allLines, 5, "should return all available lines when requested more than exist")

	// Request 0 lines
	empty := watcher.GetRecentOutput(0)
	assert.Len(t, empty, 0, "should return empty slice for 0 request")
}

func TestOutputWatcher_WaitForAnyPattern(t *testing.T) {
	input := `%output %0 Starting process
%output %0 Processing data
%output %0 Operation complete
%output %0 All done`

	reader := strings.NewReader(input)
	watcher := NewOutputWatcher(reader)

	patterns := []string{"Operation complete", "Failed", "Error"}
	matched, err := watcher.WaitForAnyPattern(patterns, 1*time.Second)

	assert.NoError(t, err, "should find one of the patterns")
	assert.Equal(t, "Operation complete", matched, "should return the matched pattern")
}

func TestOutputWatcher_WaitForAnyPattern_Timeout(t *testing.T) {
	// Use a slow reader that never returns data
	reader := &slowReader{delay: 200 * time.Millisecond}
	watcher := NewOutputWatcher(reader)

	patterns := []string{"Never", "Gonna", "Match"}
	matched, err := watcher.WaitForAnyPattern(patterns, 100*time.Millisecond)

	assert.Error(t, err, "should timeout or EOF")
	assert.Empty(t, matched, "should not return a matched pattern")
}

// slowReader simulates a slow/blocking reader for timeout testing
type slowReader struct {
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (n int, err error) {
	time.Sleep(r.delay)
	return 0, fmt.Errorf("EOF")
}

func TestOutputWatcher_RealWorldRenameOutput(t *testing.T) {
	// Simulates actual tmux control mode output from a /rename command
	input := `%begin 1234 5678 0
%output %0 \033[2J\033[H
%output %0 \033[1;32m✓\033[0m\040Rename\040successful\012
%output %0 \033[0m
%end 1234 5678 0`

	reader := strings.NewReader(input)
	watcher := NewOutputWatcher(reader)

	// Should find "Rename successful" despite octal escapes and ANSI codes
	err := watcher.WaitForPattern("Rename successful", 1*time.Second)
	assert.NoError(t, err, "should find pattern in real-world tmux output")

	// Should also detect %end
	reader2 := strings.NewReader(input)
	watcher2 := NewOutputWatcher(reader2)
	err = watcher2.WaitForPattern("%end", 1*time.Second)
	assert.NoError(t, err, "should find %end marker")
}

func TestOutputWatcher_BufferSizeLimit(t *testing.T) {
	// Create watcher with custom small buffer size
	watcher := NewOutputWatcher(strings.NewReader(""))
	watcher.maxSize = 5

	// Add more lines than buffer size
	for i := 0; i < 10; i++ {
		watcher.addToBuffer(fmt.Sprintf("Line %d", i))
	}

	// Buffer should only have last 5 lines
	assert.Len(t, watcher.buffer, 5, "buffer should be limited to maxSize")
	assert.Contains(t, watcher.buffer[0], "Line 5", "oldest line should be Line 5")
	assert.Contains(t, watcher.buffer[4], "Line 9", "newest line should be Line 9")
}

func TestOutputWatcher_EOF(t *testing.T) {
	input := `%output %0 Only one line`

	reader := strings.NewReader(input)
	watcher := NewOutputWatcher(reader)

	// First pattern should work
	err := watcher.WaitForPattern("Only one line", 1*time.Second)
	assert.NoError(t, err)

	// Second wait should hit EOF
	err = watcher.WaitForPattern("Another pattern", 1*time.Second)
	assert.Error(t, err, "should error on EOF")
	assert.Contains(t, err.Error(), "EOF", "error should mention EOF")
}

// Benchmark tests
func BenchmarkUnescapeOctal(b *testing.B) {
	input := "Hello\\040World\\012This\\040is\\040a\\040test\\012"
	for i := 0; i < b.N; i++ {
		unescapeOctal(input)
	}
}

func BenchmarkWaitForPattern(b *testing.B) {
	// Create a long input with pattern at the end
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(fmt.Sprintf("%%output %%0 Line %d\n", i))
	}
	sb.WriteString("%output %0 Target pattern\n")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := strings.NewReader(sb.String())
		watcher := NewOutputWatcher(reader)
		watcher.WaitForPattern("Target pattern", 5*time.Second)
	}
}
