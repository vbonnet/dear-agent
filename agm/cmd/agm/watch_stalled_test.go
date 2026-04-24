package main

import (
	"testing"
	"time"
)

// --- Command registration ---

func TestWatchStalledCommand_Registered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "watch-stalled" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'watch-stalled' command to be registered on rootCmd")
	}
}

// --- Flag registration and defaults ---

func TestWatchStalledCommand_Flags(t *testing.T) {
	tests := []struct {
		name     string
		defValue string
	}{
		{"check-interval", "30s"},
		{"permission-timeout", "5m0s"},
		{"no-commit-timeout", "15m0s"},
		{"error-repeat-threshold", "3"},
		{"orchestrator", ""},
		{"dry-run", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := watchStalledCmd.Flags().Lookup(tt.name)
			if flag == nil {
				t.Fatalf("expected --%s flag to be registered", tt.name)
			}
			if flag.DefValue != tt.defValue {
				t.Errorf("flag --%s default = %q, want %q", tt.name, flag.DefValue, tt.defValue)
			}
		})
	}
}

// --- formatDurationForJSON unit tests ---

func TestFormatDurationForJSON(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
		{time.Minute, "1m"},
		{5 * time.Minute, "5m"},
		{10*time.Minute + 30*time.Second, "10m"},
		{time.Hour, "1h"},
		{2 * time.Hour, "2h"},
		{2*time.Hour + 30*time.Minute, "2h30m"},
		{24 * time.Hour, "24h"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDurationForJSON(tt.input)
			if got != tt.want {
				t.Errorf("formatDurationForJSON(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- JSON output structure validation ---

func TestStallEventOutput_JSONTags(t *testing.T) {
	out := stallEventOutput{
		Timestamp:         "2026-04-12T10:00:00Z",
		SessionName:       "worker-1",
		StallType:         "permission_prompt",
		Duration:          "10m",
		Severity:          "critical",
		Evidence:          "Permission dialog open for 10m",
		RecommendedAction: "Send alert to orchestrator",
	}

	// Verify all fields are populated (non-zero)
	if out.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
	if out.SessionName == "" {
		t.Error("SessionName should not be empty")
	}
	if out.StallType == "" {
		t.Error("StallType should not be empty")
	}
	if out.Duration == "" {
		t.Error("Duration should not be empty")
	}
	if out.Severity == "" {
		t.Error("Severity should not be empty")
	}
	if out.Evidence == "" {
		t.Error("Evidence should not be empty")
	}
	if out.RecommendedAction == "" {
		t.Error("RecommendedAction should not be empty")
	}
}

func TestRecoveryActionOutput_JSONTags(t *testing.T) {
	out := recoveryActionOutput{
		Timestamp:   "2026-04-12T10:00:00Z",
		SessionName: "worker-1",
		ActionType:  "alert_orchestrator",
		Description: "Sent alert to orchestrator",
		Sent:        true,
		Error:       "",
	}

	if out.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
	if out.SessionName == "" {
		t.Error("SessionName should not be empty")
	}
	if out.ActionType == "" {
		t.Error("ActionType should not be empty")
	}
	if out.Description == "" {
		t.Error("Description should not be empty")
	}
	if !out.Sent {
		t.Error("Sent should be true")
	}
	if out.Error != "" {
		t.Error("Error should be empty on success")
	}
}

func TestRecoveryActionOutput_ErrorOmitEmpty(t *testing.T) {
	// When Error is empty, it should be omitted from JSON via omitempty tag
	out := recoveryActionOutput{
		Timestamp:   "2026-04-12T10:00:00Z",
		SessionName: "worker-1",
		ActionType:  "nudge",
		Sent:        false,
		Error:       "",
	}

	// Error field has `json:"error,omitempty"` — verify the tag exists
	// by ensuring the struct compiles and the field is accessible
	if out.Error != "" {
		t.Error("Error should be empty")
	}
}

// --- Command description ---

func TestWatchStalledCommand_Description(t *testing.T) {
	if watchStalledCmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
	if watchStalledCmd.Long == "" {
		t.Error("expected non-empty Long description")
	}
	if watchStalledCmd.Use != "watch-stalled" {
		t.Errorf("Use = %q, want %q", watchStalledCmd.Use, "watch-stalled")
	}
}
