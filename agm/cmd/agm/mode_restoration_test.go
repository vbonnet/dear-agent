package main

import (
	"testing"
)

// TestCalculateShiftTabCount verifies correct Shift+Tab count for each mode
func TestCalculateShiftTabCount(t *testing.T) {
	tests := []struct {
		name       string
		targetMode string
		want       int
	}{
		{
			name:       "default mode requires 0 tabs",
			targetMode: "default",
			want:       0,
		},
		{
			name:       "auto mode requires 1 tab",
			targetMode: "auto",
			want:       1,
		},
		{
			name:       "plan mode requires 2 tabs",
			targetMode: "plan",
			want:       2,
		},
		{
			name:       "invalid mode returns 0",
			targetMode: "invalid",
			want:       0,
		},
		{
			name:       "empty mode returns 0",
			targetMode: "",
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateShiftTabCount(tt.targetMode)
			if got != tt.want {
				t.Errorf("calculateShiftTabCount(%q) = %d, want %d", tt.targetMode, got, tt.want)
			}
		})
	}
}

// TestSupportsPermissionMode verifies agent support detection
func TestSupportsPermissionMode(t *testing.T) {
	tests := []struct {
		name  string
		agent string
		want  bool
	}{
		{
			name:  "claude-code supports permission modes",
			agent: "claude-code",
			want:  true,
		},
		{
			name:  "gemini-cli does not support permission modes",
			agent: "gemini-cli",
			want:  false,
		},
		{
			name:  "codex-cli does not support permission modes",
			agent: "codex-cli",
			want:  false,
		},
		{
			name:  "opencode-cli does not support permission modes",
			agent: "opencode-cli",
			want:  false,
		},
		{
			name:  "empty agent does not support permission modes",
			agent: "",
			want:  false,
		},
		{
			name:  "unknown agent does not support permission modes",
			agent: "unknown",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := supportsPermissionMode(tt.agent)
			if got != tt.want {
				t.Errorf("supportsPermissionMode(%q) = %v, want %v", tt.agent, got, tt.want)
			}
		})
	}
}
