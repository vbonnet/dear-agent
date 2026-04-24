package limiter

import (
	"bytes"
	"strings"
	"testing"
)

func TestApproxTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"abcdefgh", 2},
		{strings.Repeat("x", 100), 25},
		{strings.Repeat("x", 101), 26},
	}
	for _, tt := range tests {
		got := approxTokens([]byte(tt.input))
		if got != tt.want {
			t.Errorf("approxTokens(%d bytes) = %d, want %d", len(tt.input), got, tt.want)
		}
	}
}

func TestStderrLimiter_UnderBudget(t *testing.T) {
	var buf bytes.Buffer
	lim := Wrap(&buf, "test-hook", 100)

	msg := "Hello, world!\n" // 14 bytes = ~4 tokens
	n, err := lim.Write([]byte(msg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write returned %d, want %d", n, len(msg))
	}
	if buf.String() != msg {
		t.Errorf("output = %q, want %q", buf.String(), msg)
	}
	if lim.WasTruncated() {
		t.Error("should not be truncated")
	}
	if lim.TokensWritten() != 4 {
		t.Errorf("TokensWritten() = %d, want 4", lim.TokensWritten())
	}
}

func TestStderrLimiter_ExactBudget(t *testing.T) {
	var buf bytes.Buffer
	lim := Wrap(&buf, "test-hook", 10) // 10 tokens = 40 bytes

	msg := strings.Repeat("x", 40) // exactly 10 tokens
	lim.Write([]byte(msg))
	if lim.WasTruncated() {
		t.Error("should not be truncated at exact budget")
	}
	if buf.String() != msg {
		t.Errorf("output should be complete")
	}
}

func TestStderrLimiter_Truncation(t *testing.T) {
	var buf bytes.Buffer
	lim := Wrap(&buf, "my-hook", 5) // 5 tokens = 20 bytes

	// Write 100 bytes = 25 tokens, way over budget
	msg := strings.Repeat("A", 100)
	n, err := lim.Write([]byte(msg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 100 {
		t.Errorf("Write should report full len, got %d", n)
	}
	if !lim.WasTruncated() {
		t.Error("should be truncated")
	}

	output := buf.String()
	if !strings.Contains(output, "[my-hook] Output truncated at 5 tokens") {
		t.Errorf("missing truncation notice in output: %q", output)
	}
}

func TestStderrLimiter_MultipleWrites(t *testing.T) {
	var buf bytes.Buffer
	lim := Wrap(&buf, "hook", 10) // 10 tokens

	// First write: 8 bytes = 2 tokens
	lim.Write([]byte("12345678"))
	if lim.TokensWritten() != 2 {
		t.Errorf("after first write: tokens = %d, want 2", lim.TokensWritten())
	}

	// Second write: 32 bytes = 8 tokens, total = 10, fits
	lim.Write([]byte(strings.Repeat("b", 32)))
	if lim.WasTruncated() {
		t.Error("should not be truncated at exactly 10 tokens")
	}

	// Third write: 4 bytes = 1 token, over budget
	lim.Write([]byte("over"))
	if !lim.WasTruncated() {
		t.Error("should be truncated after exceeding budget")
	}
}

func TestStderrLimiter_DropsAfterTruncation(t *testing.T) {
	var buf bytes.Buffer
	lim := Wrap(&buf, "hook", 1) // 1 token = 4 bytes

	// First write exceeds budget
	lim.Write([]byte(strings.Repeat("x", 20)))

	outputAfterTrunc := buf.String()

	// Subsequent writes should be silently dropped
	lim.Write([]byte("more data"))
	lim.Write([]byte("even more"))

	if buf.String() != outputAfterTrunc {
		t.Error("writes after truncation should be dropped")
	}
}

func TestStderrLimiter_PartialWriteOnOverflow(t *testing.T) {
	var buf bytes.Buffer
	lim := Wrap(&buf, "hook", 5) // 5 tokens

	// Write 12 bytes = 3 tokens (under budget)
	lim.Write([]byte(strings.Repeat("a", 12)))

	// Write 40 bytes = 10 tokens (would push to 13, over 5)
	// Remaining budget: 2 tokens = 8 bytes
	lim.Write([]byte(strings.Repeat("b", 40)))

	if !lim.WasTruncated() {
		t.Error("should be truncated")
	}

	output := buf.String()
	// Should contain partial 'b' output (8 bytes worth)
	if !strings.Contains(output, "bbbbbbbb") {
		t.Errorf("should contain partial write, got: %q", output)
	}
	if !strings.Contains(output, "[hook] Output truncated at 5 tokens") {
		t.Errorf("should contain truncation notice, got: %q", output)
	}
}
