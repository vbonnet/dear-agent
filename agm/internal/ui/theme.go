package ui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// AGMTheme returns a custom high-contrast theme optimized for AGM
// Addresses accessibility concerns and selection visibility issues
func AGMTheme() *huh.Theme {
	theme := huh.ThemeBase()
	palette := DefaultPalette()
	const cursorSymbol = "❯"

	// === Focused (active) form styling ===

	// Selected option: Bold + bright + cursor for maximum visibility
	theme.Focused.SelectedOption = lipgloss.NewStyle().
		Bold(true).
		Foreground(palette.Selection).
		PaddingLeft(0).
		MarginLeft(0)

	// Unselected option: Dim + indented to show it's not selected
	theme.Focused.UnselectedOption = lipgloss.NewStyle().
		Bold(false).
		Foreground(palette.Unselected).
		PaddingLeft(2). // Indent unselected options
		MarginLeft(0)

	// Focused button (in multi-select)
	theme.Focused.FocusedButton = lipgloss.NewStyle().
		Bold(true).
		Foreground(palette.Selection).
		Background(lipgloss.Color("0")). // Black background for contrast
		Padding(0, 1)

	// Blurred button (not focused)
	theme.Focused.BlurredButton = lipgloss.NewStyle().
		Foreground(palette.Unselected).
		Background(lipgloss.Color("0")).
		Padding(0, 1)

	// === Blurred (inactive) form styling ===

	theme.Blurred.SelectedOption = lipgloss.NewStyle().
		Foreground(palette.Unselected)

	theme.Blurred.UnselectedOption = lipgloss.NewStyle().
		Foreground(palette.Unselected).
		PaddingLeft(2)

	theme.Blurred.FocusedButton = theme.Focused.BlurredButton
	theme.Blurred.BlurredButton = theme.Focused.BlurredButton

	// === Text input styling ===

	theme.Focused.TextInput.Cursor = lipgloss.NewStyle().
		Foreground(palette.Cursor)

	theme.Focused.TextInput.Placeholder = lipgloss.NewStyle().
		Foreground(palette.Unselected)

	theme.Focused.TextInput.Prompt = lipgloss.NewStyle().
		Foreground(palette.Cursor).
		Bold(true).
		SetString(cursorSymbol + " ")

	// === Title and description styling ===

	theme.Focused.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(palette.Header).
		MarginBottom(1)

	theme.Focused.Description = lipgloss.NewStyle().
		Foreground(palette.Unselected).
		MarginBottom(1)

	theme.Blurred.Title = lipgloss.NewStyle().
		Foreground(palette.Unselected).
		MarginBottom(1)

	theme.Blurred.Description = lipgloss.NewStyle().
		Foreground(palette.Unselected).
		MarginBottom(1)

	// === Error styling ===

	theme.Focused.ErrorMessage = lipgloss.NewStyle().
		Foreground(palette.Error).
		Bold(true)

	theme.Focused.ErrorIndicator = lipgloss.NewStyle().
		Foreground(palette.Error).
		SetString("✗ ")

	return theme
}

// AGMThemeLight returns a high-contrast theme for light terminal backgrounds
func AGMThemeLight() *huh.Theme {
	theme := huh.ThemeBase()
	palette := LightPalette()
	const cursorSymbol = "❯"

	// Similar structure to dark theme, but with colors for light background

	theme.Focused.SelectedOption = lipgloss.NewStyle().
		Bold(true).
		Foreground(palette.Selection).
		PaddingLeft(0).
		MarginLeft(0)

	theme.Focused.UnselectedOption = lipgloss.NewStyle().
		Bold(false).
		Foreground(palette.Unselected).
		PaddingLeft(2).
		MarginLeft(0)

	theme.Focused.FocusedButton = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")). // White text
		Background(palette.Selection).
		Padding(0, 1)

	theme.Focused.BlurredButton = lipgloss.NewStyle().
		Foreground(palette.Header).
		Background(lipgloss.Color("7")). // Light gray background
		Padding(0, 1)

	theme.Blurred.SelectedOption = lipgloss.NewStyle().
		Foreground(palette.Unselected)

	theme.Blurred.UnselectedOption = lipgloss.NewStyle().
		Foreground(palette.Unselected).
		PaddingLeft(2)

	theme.Focused.TextInput.Cursor = lipgloss.NewStyle().
		Foreground(palette.Cursor)

	theme.Focused.TextInput.Placeholder = lipgloss.NewStyle().
		Foreground(palette.Unselected)

	theme.Focused.TextInput.Prompt = lipgloss.NewStyle().
		Foreground(palette.Cursor).
		Bold(true).
		SetString(cursorSymbol + " ")

	theme.Focused.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(palette.Header).
		MarginBottom(1)

	theme.Focused.Description = lipgloss.NewStyle().
		Foreground(palette.Unselected).
		MarginBottom(1)

	theme.Focused.ErrorMessage = lipgloss.NewStyle().
		Foreground(palette.Error).
		Bold(true)

	theme.Focused.ErrorIndicator = lipgloss.NewStyle().
		Foreground(palette.Error).
		SetString("✗ ")

	return theme
}
