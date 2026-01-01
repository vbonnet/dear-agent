// Package output provides interfaces and implementations for formatted CLI output.
//
// It supports different output modes (verbose, quiet) and formatting styles
// (success, error, info, progress, tables).
package output

import (
	"fmt"
	"os"
	"strings"
)

// Writer handles formatted output for CLI commands.
// Implementations can write to stdout, files, or buffers for testing.
type Writer interface {
	// Success writes a success message (typically green with checkmark)
	Success(msg string)

	// Error writes an error message (typically red with X)
	Error(msg string)

	// Info writes an informational message
	Info(msg string)

	// Progress writes a progress message (typically for ongoing operations)
	Progress(msg string)

	// Table writes a formatted table with headers and rows
	Table(headers []string, rows [][]string)
}

// StdoutWriter writes formatted output to standard output.
type StdoutWriter struct {
	Verbose bool // Whether to show verbose output
}

// NewStdoutWriter creates a new stdout writer.
func NewStdoutWriter(verbose bool) *StdoutWriter {
	return &StdoutWriter{Verbose: verbose}
}

// Success writes a success message to stdout.
func (w *StdoutWriter) Success(msg string) {
	fmt.Printf("✓ %s\n", msg)
}

// Error writes an error message to stderr.
func (w *StdoutWriter) Error(msg string) {
	fmt.Fprintf(os.Stderr, "✗ %s\n", msg)
}

// Info writes an informational message to stdout.
func (w *StdoutWriter) Info(msg string) {
	fmt.Printf("  %s\n", msg)
}

// Progress writes a progress message to stdout (only in verbose mode).
func (w *StdoutWriter) Progress(msg string) {
	if w.Verbose {
		fmt.Printf("→ %s\n", msg)
	}
}

// Table writes a formatted table with headers and rows.
// Uses simple ASCII table formatting.
func (w *StdoutWriter) Table(headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print headers
	for i, h := range headers {
		fmt.Printf("%-*s  ", widths[i], h)
	}
	fmt.Println()

	// Print separator
	for _, width := range widths {
		fmt.Print(strings.Repeat("-", width) + "  ")
	}
	fmt.Println()

	// Print rows
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				fmt.Printf("%-*s  ", widths[i], cell)
			}
		}
		fmt.Println()
	}
}
