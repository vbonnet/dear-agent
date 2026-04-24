package ui

import (
	"os"
	"testing"
)

func TestNoColorFlag(t *testing.T) {
	tests := []struct {
		name        string
		noColor     bool
		envVar      string
		expectPlain bool // Whether we expect plain output
	}{
		{
			name:        "NoColor flag true",
			noColor:     true,
			envVar:      "",
			expectPlain: true,
		},
		{
			name:        "NoColor flag false (but not TTY)",
			noColor:     false,
			envVar:      "",
			expectPlain: true, // Tests run without TTY, so expect plain
		},
		{
			name:        "NO_COLOR env var set",
			noColor:     false,
			envVar:      "1",
			expectPlain: true,
		},
		{
			name:        "NoColor flag takes precedence",
			noColor:     true,
			envVar:      "",
			expectPlain: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up config
			cfg := DefaultConfig()
			cfg.UI.NoColor = tt.noColor
			SetGlobalConfig(cfg)

			// Set env var if specified
			if tt.envVar != "" {
				os.Setenv("NO_COLOR", tt.envVar)
				defer os.Unsetenv("NO_COLOR")
			}

			// Test color output
			result := Red("test")

			isPlain := result == "test"
			if isPlain != tt.expectPlain {
				t.Errorf("Plain output = %v, want %v (result: %q)", isPlain, tt.expectPlain, result)
			}

			// Verify NoColor flag is being checked
			if tt.noColor && !isPlain {
				t.Error("NoColor flag should disable color output")
			}
		})
	}
}

func TestScreenReaderFlag(t *testing.T) {
	// Note: ScreenReaderText is a pure conversion function.
	// The flag/env checks happen in Print* functions.
	// This test verifies the config is properly accessible.

	tests := []struct {
		name         string
		screenReader bool
		envVar       string
	}{
		{
			name:         "ScreenReader flag true",
			screenReader: true,
			envVar:       "",
		},
		{
			name:         "ScreenReader flag false",
			screenReader: false,
			envVar:       "",
		},
		{
			name:         "AGM_SCREEN_READER env var set",
			screenReader: false,
			envVar:       "1",
		},
		{
			name:         "ScreenReader flag takes precedence",
			screenReader: true,
			envVar:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up config
			cfg := DefaultConfig()
			cfg.UI.ScreenReader = tt.screenReader
			SetGlobalConfig(cfg)

			// Set env var if specified
			if tt.envVar != "" {
				os.Setenv("AGM_SCREEN_READER", tt.envVar)
				defer os.Unsetenv("AGM_SCREEN_READER")
			}

			// Verify config is accessible
			retrieved := GetGlobalConfig()
			if retrieved.UI.ScreenReader != tt.screenReader {
				t.Errorf("Config ScreenReader = %v, want %v", retrieved.UI.ScreenReader, tt.screenReader)
			}

			// Verify ScreenReaderText conversion works (it's stateless)
			if result := ScreenReaderText("✓"); result != "[SUCCESS]" {
				t.Errorf("ScreenReaderText conversion failed: got %q", result)
			}
		})
	}
}

func TestGlobalConfig(t *testing.T) {
	// Test nil config returns default
	SetGlobalConfig(nil)
	cfg := GetGlobalConfig()
	if cfg == nil {
		t.Fatal("GetGlobalConfig() returned nil, expected default config")
	}

	// Test setting and getting config
	customCfg := DefaultConfig()
	customCfg.UI.NoColor = true
	customCfg.UI.ScreenReader = true
	SetGlobalConfig(customCfg)

	retrieved := GetGlobalConfig()
	if !retrieved.UI.NoColor {
		t.Error("NoColor should be true")
	}
	if !retrieved.UI.ScreenReader {
		t.Error("ScreenReader should be true")
	}
}

func TestScreenReaderTextAllSymbols(t *testing.T) {
	tests := []struct {
		symbol string
		want   string
	}{
		{"✓", "[SUCCESS]"},
		{"❌", "[ERROR]"},
		{"⚠", "[WARNING]"},
		{"⚠️", "[WARNING]"}, // Emoji variant
		{"○", "[INFO]"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			result := ScreenReaderText(tt.symbol)
			if result != tt.want {
				t.Errorf("ScreenReaderText(%q) = %q, want %q", tt.symbol, result, tt.want)
			}
		})
	}
}

func TestBoldWithNoColor(t *testing.T) {
	tests := []struct {
		name        string
		noColor     bool
		envVar      string
		expectPlain bool
	}{
		{
			name:        "NoColor flag true",
			noColor:     true,
			envVar:      "",
			expectPlain: true,
		},
		{
			name:        "NoColor flag false (but not TTY)",
			noColor:     false,
			envVar:      "",
			expectPlain: true, // Tests run without TTY
		},
		{
			name:        "NO_COLOR env var set",
			noColor:     false,
			envVar:      "1",
			expectPlain: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.UI.NoColor = tt.noColor
			SetGlobalConfig(cfg)

			if tt.envVar != "" {
				os.Setenv("NO_COLOR", tt.envVar)
				defer os.Unsetenv("NO_COLOR")
			}

			result := Bold("test")
			isPlain := result == "test"

			if isPlain != tt.expectPlain {
				t.Errorf("Plain output = %v, want %v", isPlain, tt.expectPlain)
			}

			// Verify NoColor flag is being checked
			if tt.noColor && !isPlain {
				t.Error("NoColor flag should disable bold output")
			}
		})
	}
}
