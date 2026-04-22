package main

import (
	"strings"
	"testing"
)

func TestResolveModelAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		valid    bool
	}{
		{"sonnet", "sonnet", true},
		{"default", "sonnet", true},
		{"sonnet-1m", "sonnet[1m]", true},
		{"opus", "opus", true},
		{"opus-1m", "opus[1m]", true},
		{"haiku", "haiku", true},
		// Case insensitive
		{"Opus", "opus", true},
		{"HAIKU", "haiku", true},
		{"Sonnet-1m", "sonnet[1m]", true},
		// Invalid
		{"gpt-4", "", false},
		{"", "", false},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, valid := resolveModelAlias(tt.input)
			if valid != tt.valid {
				t.Errorf("resolveModelAlias(%q) valid = %v, want %v", tt.input, valid, tt.valid)
			}
			if got != tt.expected {
				t.Errorf("resolveModelAlias(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestModelAliasesCompleteness(t *testing.T) {
	// Ensure all expected aliases exist
	expected := []string{"default", "sonnet", "sonnet-1m", "opus", "opus-1m", "haiku"}
	for _, alias := range expected {
		if _, ok := modelAliases[alias]; !ok {
			t.Errorf("missing model alias %q", alias)
		}
	}

	// Verify count matches
	if len(modelAliases) != len(expected) {
		t.Errorf("expected %d aliases, got %d", len(expected), len(modelAliases))
	}
}

func TestVerifyModelSetParsing(t *testing.T) {
	// Test the line-matching logic used by verifyModelSet
	tests := []struct {
		name    string
		line    string
		matches bool
	}{
		{"confirmation line", "Set model to claude-sonnet-4-6-20250514", true},
		{"with whitespace", "  Set model to claude-opus-4-6  ", true},
		{"unrelated line", "Some other output", false},
		{"empty line", "", false},
		{"partial match", "Set model", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := strings.TrimSpace(tt.line)
			got := strings.HasPrefix(trimmed, "Set model to")
			if got != tt.matches {
				t.Errorf("line %q: got match=%v, want %v", tt.line, got, tt.matches)
			}
		})
	}
}

func TestRunSendSetModelInvalidModel(t *testing.T) {
	// Calling with invalid model should return descriptive error
	err := runSendSetModel(nil, []string{"test-session", "gpt-4"})
	if err == nil {
		t.Fatal("expected error for invalid model")
	}
	if !strings.Contains(err.Error(), "unknown model") {
		t.Errorf("error should mention 'unknown model', got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "gpt-4") {
		t.Errorf("error should include the invalid model name, got: %s", err.Error())
	}
}
