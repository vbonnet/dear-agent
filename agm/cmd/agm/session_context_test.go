package main

import (
	"encoding/json"
	"testing"
)

func TestSessionContextCommandMetadata(t *testing.T) {
	if sessionContextCmd.Use != "context <identifier>" {
		t.Errorf("Use = %q, want %q", sessionContextCmd.Use, "context <identifier>")
	}
	if sessionContextCmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if sessionContextCmd.RunE == nil {
		t.Error("RunE should be set")
	}
	if sessionContextCmd.Args == nil {
		t.Error("Args validator should be set")
	}
}

func TestSessionContextRegistered(t *testing.T) {
	found := false
	for _, cmd := range sessionCmd.Commands() {
		if cmd.Name() == "context" {
			found = true
			break
		}
	}
	if !found {
		t.Error("context should be registered as a subcommand of session")
	}
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected string
	}{
		{"zero", 0, "0"},
		{"small", 999, "999"},
		{"one thousand", 1000, "1,000"},
		{"mid range", 150000, "150,000"},
		{"200k", 200000, "200,000"},
		{"one million", 1000000, "1,000,000"},
		{"large", 1234567, "1,234,567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTokenCount(tt.input)
			if got != tt.expected {
				t.Errorf("formatTokenCount(%d) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestContextBar(t *testing.T) {
	tests := []struct {
		name       string
		percentage float64
	}{
		{"zero", 0.0},
		{"half", 50.0},
		{"full", 100.0},
		{"over", 110.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := contextBar(tt.percentage)
			if bar == "" {
				t.Error("contextBar should return non-empty string")
			}
			// Bar should contain brackets
			if bar[0] != '[' {
				t.Errorf("bar should start with '[', got %q", bar)
			}
		})
	}
}

func TestSessionContextResultJSON(t *testing.T) {
	result := &SessionContextResult{
		Operation: "session_context",
		SessionID: "abc-123",
		Name:      "test-session",
		Context: &SessionContextUsage{
			TotalTokens:    200000,
			UsedTokens:     150000,
			PercentageUsed: 75.0,
			ModelID:        "claude-sonnet-4-20250929",
			Source:         "statusline",
			LastUpdated:    "2026-03-29T14:30:00Z",
			EstimatedCost:  1.23,
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded SessionContextResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Operation != "session_context" {
		t.Errorf("Operation = %q, want %q", decoded.Operation, "session_context")
	}
	if decoded.Context == nil {
		t.Fatal("Context should not be nil")
	}
	if decoded.Context.PercentageUsed != 75.0 {
		t.Errorf("PercentageUsed = %f, want 75.0", decoded.Context.PercentageUsed)
	}
	if decoded.Context.EstimatedCost != 1.23 {
		t.Errorf("EstimatedCost = %f, want 1.23", decoded.Context.EstimatedCost)
	}
}
