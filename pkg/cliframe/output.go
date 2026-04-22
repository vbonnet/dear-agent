// Package cliframe provides shared CLI framework components for output formatting,
// error handling, configuration loading, and standard flags.
//
// This package implements a hybrid API design with:
//   - Framework-agnostic core interfaces (OutputFormatter, Writer, CLIError)
//   - Optional Cobra integration helpers for convenience
//
// Example usage (standalone):
//
//	formatter := cliframe.NewJSONFormatter(true)
//	data, _ := formatter.Format(myData)
//	fmt.Println(string(data))
//
// Example usage (Cobra integration):
//
//	flags := cliframe.AddStandardFlags(cmd)
//	return cliframe.OutputFromFlags(cmd, myData, flags)
package cliframe

import (
	"fmt"
	"io"
)

// Version is the cliframe library version
const Version = "0.1.0"

// Format represents an output format type
type Format string

const (
	// FormatJSON outputs data as JSON
	FormatJSON Format = "json"

	// FormatTable outputs data as human-readable tables
	FormatTable Format = "table"

	// FormatTOON outputs data in Token-Oriented Object Notation
	FormatTOON Format = "toon"

	// FormatCSV outputs data as CSV (future enhancement)
	FormatCSV Format = "csv"
)

// OutputFormatter formats arbitrary data for CLI output
type OutputFormatter interface {
	// Format converts data to bytes in the formatter's encoding.
	// Returns error if data type is incompatible with format.
	Format(v interface{}) ([]byte, error)

	// Name returns the format name (json, table, toon)
	Name() string

	// MIMEType returns the MIME type for HTTP responses (optional)
	MIMEType() string
}

// FormatterOption configures formatters
type FormatterOption func(*formatterConfig)

// formatterConfig holds configuration for formatters
type formatterConfig struct {
	prettyPrint  bool
	colorEnabled bool
	maxWidth     int
	compact      bool
}

// WithPrettyPrint enables pretty-printing for JSON
func WithPrettyPrint(enable bool) FormatterOption {
	return func(c *formatterConfig) {
		c.prettyPrint = enable
	}
}

// WithColor enables colored output for tables
func WithColor(enable bool) FormatterOption {
	return func(c *formatterConfig) {
		c.colorEnabled = enable
	}
}

// WithMaxWidth sets maximum width for tables (0 = auto-detect terminal)
func WithMaxWidth(width int) FormatterOption {
	return func(c *formatterConfig) {
		c.maxWidth = width
	}
}

// WithCompact enables compact output (minimize whitespace)
func WithCompact(enable bool) FormatterOption {
	return func(c *formatterConfig) {
		c.compact = enable
	}
}

// NewFormatter creates a formatter by name.
// Returns error if format is unknown.
func NewFormatter(format Format, opts ...FormatterOption) (OutputFormatter, error) {
	config := &formatterConfig{
		prettyPrint:  false,
		colorEnabled: true,
		maxWidth:     0,
		compact:      false,
	}

	for _, opt := range opts {
		opt(config)
	}

	switch format {
	case FormatJSON:
		return NewJSONFormatter(config.prettyPrint), nil
	case FormatTable:
		return NewTableFormatter(opts...), nil
	case FormatTOON:
		return NewTOONFormatter(), nil
	case FormatCSV:
		return nil, fmt.Errorf("CSV format not yet implemented")
	default:
		return nil, fmt.Errorf("unknown format: %s (supported: json, table, toon)", format)
	}
}

// Writer writes formatted output to stdout/stderr
type Writer struct {
	out       io.Writer
	errOut    io.Writer
	formatter OutputFormatter
	noColor   bool
}

// NewWriter creates a Writer with default formatter (JSON)
func NewWriter(out, errOut io.Writer) *Writer {
	return &Writer{
		out:       out,
		errOut:    errOut,
		formatter: NewJSONFormatter(false),
		noColor:   false,
	}
}

// WithFormatter sets the output formatter
func (w *Writer) WithFormatter(f OutputFormatter) *Writer {
	w.formatter = f
	return w
}

// Output writes data using the configured formatter
func (w *Writer) Output(v interface{}) error {
	data, err := w.formatter.Format(v)
	if err != nil {
		return err
	}
	_, err = w.out.Write(data)
	if err != nil {
		return err
	}
	// Add newline if not present
	if len(data) > 0 && data[len(data)-1] != '\n' {
		_, err = w.out.Write([]byte("\n"))
	}
	return err
}

// OutputFormat writes data using specified format
func (w *Writer) OutputFormat(v interface{}, format Format) error {
	formatter, err := NewFormatter(format)
	if err != nil {
		return err
	}
	oldFormatter := w.formatter
	w.formatter = formatter
	defer func() { w.formatter = oldFormatter }()

	return w.Output(v)
}

// Success writes a success message to stdout (green if color enabled)
func (w *Writer) Success(msg string) {
	if w.noColor {
		fmt.Fprintln(w.out, msg)
		return
	}
	fmt.Fprintf(w.out, "\x1b[32m%s\x1b[0m\n", msg)
}

// Info writes an info message to stdout (blue if color enabled)
func (w *Writer) Info(msg string) {
	if w.noColor {
		fmt.Fprintln(w.out, msg)
		return
	}
	fmt.Fprintf(w.out, "\x1b[34m%s\x1b[0m\n", msg)
}

// Warning writes a warning to stderr (yellow if color enabled)
func (w *Writer) Warning(msg string) {
	if w.noColor {
		fmt.Fprintln(w.errOut, msg)
		return
	}
	fmt.Fprintf(w.errOut, "\x1b[33m%s\x1b[0m\n", msg)
}

// Error writes an error to stderr (red if color enabled)
func (w *Writer) Error(msg string) {
	if w.noColor {
		fmt.Fprintln(w.errOut, msg)
		return
	}
	fmt.Fprintf(w.errOut, "\x1b[31m%s\x1b[0m\n", msg)
}

// SetColorEnabled enables/disables colored output
func (w *Writer) SetColorEnabled(enabled bool) {
	w.noColor = !enabled
}
