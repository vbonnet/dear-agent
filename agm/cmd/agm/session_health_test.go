package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestSessionHealthCommandMetadata(t *testing.T) {
	if sessionHealthCmd.Use != "health [name]" {
		t.Errorf("Use = %q, want %q", sessionHealthCmd.Use, "health [name]")
	}
	if sessionHealthCmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if sessionHealthCmd.RunE == nil {
		t.Error("RunE should be set")
	}
}

func TestSessionHealthRegistered(t *testing.T) {
	found := false
	for _, cmd := range sessionCmd.Commands() {
		if cmd.Name() == "health" {
			found = true
			break
		}
	}
	if !found {
		t.Error("health should be registered as a subcommand of session")
	}
}

func TestSessionHealthAllFlag(t *testing.T) {
	flag := sessionHealthCmd.Flags().Lookup("all")
	if flag == nil {
		t.Fatal("--all flag should be registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("--all default = %q, want %q", flag.DefValue, "false")
	}
}

func TestColorizeHealth(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{"healthy", "HEALTHY"},
		{"warning", "WARNING"},
		{"critical", "CRITICAL"},
		{"stopped", "STOPPED"},
		{"archived", "ARCHIVED"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := colorizeHealth(tt.input)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("colorizeHealth(%q) = %q, does not contain %q", tt.input, got, tt.contains)
			}
		})
	}
}

func TestPrintSessionHealthText_Empty(t *testing.T) {
	// Should not panic on empty result
	r := &ops.SessionHealthResult{
		Operation: "session_health",
		Sessions:  []ops.SessionHealthDetail{},
		Total:     0,
	}
	// Capture would require redirecting stdout; just ensure no panic
	printSessionHealthText(r)
}

func TestPrintSessionHealthText_Single(t *testing.T) {
	score := 80
	r := &ops.SessionHealthResult{
		Operation: "session_health",
		Sessions: []ops.SessionHealthDetail{
			{
				Name:                "my-session",
				ID:                  "abc123",
				Status:              "active",
				State:               "WORKING",
				Health:              "healthy",
				Duration:            "2h30m",
				StartedAt:           "2026-04-13T06:00:00Z",
				TimeSinceLastUpdate: "5m",
				TrustScore:          &score,
				CommitCount:         3,
				CPUPct:              15.2,
				MemoryMB:            512.0,
				MemoryPct:           3.1,
				PanePID:             12345,
			},
		},
		Total: 1,
	}
	// Ensure no panic; output goes to stdout
	printSessionHealthText(r)
}

func TestPrintSessionHealthText_MultipleWithWarnings(t *testing.T) {
	r := &ops.SessionHealthResult{
		Operation: "session_health",
		Sessions: []ops.SessionHealthDetail{
			{
				Name:    "session-a",
				Health:  "healthy",
				Status:  "active",
				State:   "WORKING",
				PanePID: 0, // No resource data
			},
			{
				Name:    "session-b",
				Health:  "warning",
				Status:  "active",
				State:   "USER_PROMPT",
				Warnings: []string{
					"No manifest update in 15m",
					"Session waiting on permission prompt",
				},
			},
			{
				Name:   "session-c",
				Health: "critical",
				Status: "active",
				State:  "WORKING",
				Warnings: []string{
					"High error rate: 12 error lines in recent output",
				},
			},
		},
		Total: 3,
	}
	// Ensure no panic
	printSessionHealthText(r)
}
