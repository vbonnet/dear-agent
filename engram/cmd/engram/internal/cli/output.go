package cli

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

// Initialize color settings
func init() {
	// Respect NO_COLOR environment variable
	if os.Getenv("NO_COLOR") != "" {
		color.NoColor = true
	}
}

// DisableColor disables all color output (for --no-color flag)
func DisableColor() {
	color.NoColor = true
}

// Color helper functions
var (
	// Success - Green
	Success = color.New(color.FgGreen).SprintFunc()

	// Error - Red
	ErrorColor = color.New(color.FgRed).SprintFunc()

	// Warning - Yellow
	Warning = color.New(color.FgYellow).SprintFunc()

	// Info - Blue
	Info = color.New(color.FgBlue).SprintFunc()
)

// SuccessIcon returns colored success icon (✓)
func SuccessIcon() string {
	if color.NoColor {
		return "✓"
	}
	return Success("✓")
}

// ErrorIcon returns colored error icon (✗)
func ErrorIcon() string {
	if color.NoColor {
		return "✗"
	}
	return ErrorColor("✗")
}

// WarningIcon returns colored warning icon (⚠)
func WarningIcon() string {
	if color.NoColor {
		return "⚠"
	}
	return Warning("⚠")
}

// InfoIcon returns colored info icon (ℹ)
func InfoIcon() string {
	if color.NoColor {
		return "ℹ"
	}
	return Info("ℹ")
}

// PrintSuccess prints a success message with green ✓
func PrintSuccess(message string) {
	fmt.Printf("%s %s\n", SuccessIcon(), message)
}

// PrintError prints an error message with red ✗
func PrintError(message string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", ErrorIcon(), message)
}

// PrintWarning prints a warning message with yellow ⚠
func PrintWarning(message string) {
	fmt.Printf("%s %s\n", WarningIcon(), message)
}

// PrintInfo prints an info message with blue ℹ
func PrintInfo(message string) {
	fmt.Printf("%s %s\n", InfoIcon(), message)
}
