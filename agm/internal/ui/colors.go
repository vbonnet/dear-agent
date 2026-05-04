package ui

import (
	"fmt"
	"os"
)

// Color represents ANSI color codes
type Color int

// ANSI color identifiers.
const (
	ColorRed Color = iota
	ColorGreen
	ColorYellow
	ColorBlue
	ColorReset
)

var colorCodes = map[Color]string{
	ColorRed:    "\033[31m",
	ColorGreen:  "\033[32m",
	ColorYellow: "\033[33m",
	ColorBlue:   "\033[34m",
	ColorReset:  "\033[0m",
}

// Colorize wraps text in ANSI color codes
func Colorize(text string, color Color) string {
	cfg := GetGlobalConfig()

	// Check --no-color flag first (WCAG AA requirement)
	if cfg.UI.NoColor {
		return text
	}
	// Also check NO_COLOR env var for compatibility
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	if !isTerminal() {
		return text
	}
	return fmt.Sprintf("%s%s%s", colorCodes[color], text, colorCodes[ColorReset])
}

// isTerminal checks if stdout is a terminal
func isTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// Red returns red-colored text
func Red(text string) string {
	return Colorize(text, ColorRed)
}

// Green returns green-colored text
func Green(text string) string {
	return Colorize(text, ColorGreen)
}

// Yellow returns yellow-colored text
func Yellow(text string) string {
	return Colorize(text, ColorYellow)
}

// Blue returns blue-colored text
func Blue(text string) string {
	return Colorize(text, ColorBlue)
}

// Bold returns bold text
func Bold(text string) string {
	cfg := GetGlobalConfig()

	// Check --no-color flag first (WCAG AA requirement)
	if cfg.UI.NoColor {
		return text
	}
	// Also check NO_COLOR env var for compatibility
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	if !isTerminal() {
		return text
	}
	return fmt.Sprintf("\033[1m%s\033[0m", text)
}
