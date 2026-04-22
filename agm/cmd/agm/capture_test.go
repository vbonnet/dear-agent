package main

import (
	"testing"
)

func TestCaptureCommand(t *testing.T) {
	if captureCmd == nil {
		t.Fatal("captureCmd is nil")
	}

	if captureCmd.Use != "capture <session-name>" {
		t.Errorf("unexpected Use: got %q", captureCmd.Use)
	}

	// Test flags
	linesFlag := captureCmd.Flags().Lookup("lines")
	if linesFlag == nil {
		t.Error("--lines flag not registered")
	}

	historyFlag := captureCmd.Flags().Lookup("history")
	if historyFlag == nil {
		t.Error("--history flag not registered")
	}

	tailFlag := captureCmd.Flags().Lookup("tail")
	if tailFlag == nil {
		t.Error("--tail flag not registered")
	}

	jsonFlag := captureCmd.Flags().Lookup("json")
	if jsonFlag == nil {
		t.Error("--json flag not registered")
	}

	yamlFlag := captureCmd.Flags().Lookup("yaml")
	if yamlFlag == nil {
		t.Error("--yaml flag not registered")
	}

	filterFlag := captureCmd.Flags().Lookup("filter")
	if filterFlag == nil {
		t.Error("--filter flag not registered")
	}
}

func TestFilterLines(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		pattern string
		wantLen int
		wantErr bool
	}{
		{
			name:    "filter errors",
			lines:   []string{"INFO: started", "ERROR: failed", "DEBUG: done"},
			pattern: "ERROR",
			wantLen: 1,
			wantErr: false,
		},
		{
			name:    "filter with regex",
			lines:   []string{"test1", "test2", "prod1"},
			pattern: "test.*",
			wantLen: 2,
			wantErr: false,
		},
		{
			name:    "invalid regex",
			lines:   []string{"test"},
			pattern: "[invalid",
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterLines(tt.lines, tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("filterLines() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantLen {
				t.Errorf("filterLines() got %d lines, want %d", len(got), tt.wantLen)
			}
		})
	}
}
