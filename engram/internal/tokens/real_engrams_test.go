package tokens

import (
	"encoding/json"
	"os"
	"testing"
)

// TestCalculate_RealEngrams tests with actual engram files from the repository
func TestCalculate_RealEngrams(t *testing.T) {
	// Find real engram files
	homeDir := os.Getenv("HOME")
	engramFiles := []string{
		homeDir + "/src/engram/engrams/references/performance-optimization.ai.md",
		homeDir + "/src/engram/engrams/references/effective-questioning.ai.md",
		homeDir + "/src/engram/engrams/references/claude-code/ask-user-question-usage.ai.md",
	}

	// Verify files exist
	for _, f := range engramFiles {
		if _, err := os.Stat(f); err != nil {
			t.Skipf("Skipping test: engram file not found: %s", f)
		}
	}

	estimate, err := Calculate(engramFiles)
	if err != nil {
		t.Fatalf("Calculate() failed: %v", err)
	}

	t.Logf("Character count: %d", estimate.CharCount)
	t.Logf("Tokens (char/4): %d", estimate.TokensChar4)
	t.Logf("Tokenizer results:")
	for name, count := range estimate.Tokenizers {
		t.Logf("  %-12s: %d tokens", name, count)
	}

	// Verify CharCount is non-zero
	if estimate.CharCount == 0 {
		t.Error("CharCount should not be zero for real engram files")
	}

	// Verify TokensChar4 is calculated
	expectedChar4 := estimate.CharCount / 4
	if estimate.TokensChar4 != expectedChar4 {
		t.Errorf("TokensChar4 = %d, want %d", estimate.TokensChar4, expectedChar4)
	}

	// Verify tokenizers ran
	if estimate.Tokenizers == nil {
		t.Error("Tokenizers map should not be nil")
	}

	if len(estimate.Tokenizers) == 0 {
		t.Error("At least one tokenizer (simple) should have run")
	}

	// Verify simple tokenizer present
	if _, ok := estimate.Tokenizers["simple"]; !ok {
		t.Error("Simple tokenizer should always be present")
	}

	// Verify JSON serialization
	jsonData, err := json.MarshalIndent(estimate, "", "  ")
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	t.Logf("JSON output:\n%s", string(jsonData))

	// Verify JSON structure
	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// Check required fields
	if _, ok := decoded["char_count"]; !ok {
		t.Error("JSON missing 'char_count' field")
	}
	if _, ok := decoded["tokens_char4"]; !ok {
		t.Error("JSON missing 'tokens_char4' field")
	}
	if _, ok := decoded["tokenizers"]; !ok {
		t.Error("JSON missing 'tokenizers' field")
	}

	t.Log("✓ All validation checks passed!")
}
