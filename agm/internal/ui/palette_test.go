package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDefaultPalette(t *testing.T) {
	p := DefaultPalette()

	// Test selection colors
	if p.Selection != lipgloss.Color("14") {
		t.Errorf("Expected Selection to be 14 (Bright Cyan), got %s", p.Selection)
	}
	if p.Unselected != lipgloss.Color("8") {
		t.Errorf("Expected Unselected to be 8 (Gray), got %s", p.Unselected)
	}
	if p.Cursor != lipgloss.Color("14") {
		t.Errorf("Expected Cursor to be 14 (Bright Cyan), got %s", p.Cursor)
	}

	// Test status colors
	if p.Active != lipgloss.Color("10") {
		t.Errorf("Expected Active to be 10 (Bright Green), got %s", p.Active)
	}
	if p.Stopped != lipgloss.Color("11") {
		t.Errorf("Expected Stopped to be 11 (Bright Yellow), got %s", p.Stopped)
	}
	if p.Header != lipgloss.Color("15") {
		t.Errorf("Expected Header to be 15 (Bright White), got %s", p.Header)
	}

	// Test message colors
	if p.Success != lipgloss.Color("10") {
		t.Errorf("Expected Success to be 10 (Bright Green), got %s", p.Success)
	}
	if p.Warning != lipgloss.Color("11") {
		t.Errorf("Expected Warning to be 11 (Bright Yellow), got %s", p.Warning)
	}
	if p.Error != lipgloss.Color("9") {
		t.Errorf("Expected Error to be 9 (Bright Red), got %s", p.Error)
	}
	if p.Info != lipgloss.Color("12") {
		t.Errorf("Expected Info to be 12 (Bright Blue), got %s", p.Info)
	}
}

func TestLightPalette(t *testing.T) {
	p := LightPalette()

	// Test selection colors (should use darker colors for light background)
	if p.Selection != lipgloss.Color("6") {
		t.Errorf("Expected Selection to be 6 (Cyan), got %s", p.Selection)
	}
	if p.Unselected != lipgloss.Color("8") {
		t.Errorf("Expected Unselected to be 8 (Gray), got %s", p.Unselected)
	}

	// Test status colors
	if p.Active != lipgloss.Color("2") {
		t.Errorf("Expected Active to be 2 (Green), got %s", p.Active)
	}
	if p.Stopped != lipgloss.Color("3") {
		t.Errorf("Expected Stopped to be 3 (Yellow), got %s", p.Stopped)
	}
	if p.Header != lipgloss.Color("0") {
		t.Errorf("Expected Header to be 0 (Black), got %s", p.Header)
	}
}

func TestGetPalette(t *testing.T) {
	tests := []struct {
		name      string
		themeName string
		expected  Palette
	}{
		{
			name:      "agm theme returns DefaultPalette",
			themeName: "agm",
			expected:  DefaultPalette(),
		},
		{
			name:      "agm-light theme returns LightPalette",
			themeName: "agm-light",
			expected:  LightPalette(),
		},
		{
			name:      "dracula theme returns DefaultPalette (fallback)",
			themeName: "dracula",
			expected:  DefaultPalette(),
		},
		{
			name:      "unknown theme returns DefaultPalette (fallback)",
			themeName: "unknown",
			expected:  DefaultPalette(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPalette(tt.themeName)
			// Compare a few key colors to verify correct palette
			if got.Selection != tt.expected.Selection {
				t.Errorf("GetPalette(%s).Selection = %s, want %s",
					tt.themeName, got.Selection, tt.expected.Selection)
			}
			if got.Active != tt.expected.Active {
				t.Errorf("GetPalette(%s).Active = %s, want %s",
					tt.themeName, got.Active, tt.expected.Active)
			}
		})
	}
}

func TestSemanticConsistency(t *testing.T) {
	p := DefaultPalette()

	// Success should match Active (both indicate positive state)
	if p.Success != p.Active {
		t.Errorf("Success (%s) should match Active (%s) for consistency", p.Success, p.Active)
	}

	// Warning and Error use bright colors for high visibility (not matched to Stopped/Stale)
	// Stopped/Stale use neutral colors as they represent default/subdued states
	// This is intentional design as per commit 3304236

	// Cursor should match Selection (both indicate current choice)
	if p.Cursor != p.Selection {
		t.Errorf("Cursor (%s) should match Selection (%s) for consistency", p.Cursor, p.Selection)
	}
}

func TestSemanticConsistencyLight(t *testing.T) {
	p := LightPalette()

	// Same semantic consistency checks for light palette
	if p.Success != p.Active {
		t.Errorf("Success (%s) should match Active (%s) for consistency", p.Success, p.Active)
	}
	if p.Warning != p.Stopped {
		t.Errorf("Warning (%s) should match Stopped (%s) for consistency", p.Warning, p.Stopped)
	}
	if p.Cursor != p.Selection {
		t.Errorf("Cursor (%s) should match Selection (%s) for consistency", p.Cursor, p.Selection)
	}
}
