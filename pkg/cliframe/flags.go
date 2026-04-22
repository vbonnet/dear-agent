package cliframe

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

// CommonFlags represents standard CLI flags
type CommonFlags struct {
	// Output control
	Format  string // Output format (json, table, toon)
	JSON    bool   // Shorthand for --format=json
	NoColor bool   // Disable colored output
	Quiet   bool   // Minimal output (errors only)
	Verbose bool   // Verbose output

	// Configuration
	ConfigFile string // Config file path override
	Workspace  string // Workspace directory

	// Behavior
	DryRun bool // Preview without applying changes
	Force  bool // Skip confirmations

	// Debugging
	LogLevel string // Log level (debug, info, warn, error)
	Trace    bool   // Enable trace logging
}

// AddStandardFlags adds all common flags to a Cobra command
// Returns a CommonFlags struct bound to the command
func AddStandardFlags(cmd *cobra.Command) *CommonFlags {
	flags := &CommonFlags{}

	// Output control
	AddFormatFlag(cmd, &flags.Format)
	cmd.Flags().BoolVar(&flags.JSON, "json", false, "JSON output (shorthand for --format=json)")
	cmd.Flags().BoolVar(&flags.NoColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVarP(&flags.Quiet, "quiet", "q", false, "Minimal output (errors only)")
	AddVerboseFlag(cmd, &flags.Verbose)

	// Configuration
	cmd.Flags().StringVar(&flags.ConfigFile, "config", "", "Config file path override")
	cmd.Flags().StringVar(&flags.Workspace, "workspace", "", "Workspace directory")

	// Behavior
	AddDryRunFlag(cmd, &flags.DryRun)
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip confirmations")

	// Debugging
	cmd.Flags().StringVar(&flags.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	cmd.Flags().BoolVar(&flags.Trace, "trace", false, "Enable trace logging")

	return flags
}

// AddFormatFlag adds only --format flag (for minimal integration)
func AddFormatFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVarP(target, "format", "f", "table",
		"Output format: json, table, toon")
}

// AddVerboseFlag adds only --verbose flag
func AddVerboseFlag(cmd *cobra.Command, target *bool) {
	cmd.Flags().BoolVarP(target, "verbose", "v", false, "Verbose output")
}

// AddDryRunFlag adds only --dry-run flag
func AddDryRunFlag(cmd *cobra.Command, target *bool) {
	cmd.Flags().BoolVar(target, "dry-run", false, "Preview changes without applying")
}

// ResolveFormat resolves format from flags (handles --json shorthand)
func (f *CommonFlags) ResolveFormat() Format {
	if f.JSON {
		return FormatJSON
	}
	return Format(f.Format)
}

// IsInteractive returns true if output should be interactive (TTY)
func (f *CommonFlags) IsInteractive() bool {
	// Check if stdout is a terminal
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// OutputFromFlags outputs data using format from flags
// Convenience function combining Writer + Formatter + Flags
func OutputFromFlags(cmd *cobra.Command, v interface{}, flags *CommonFlags) error {
	format := flags.ResolveFormat()

	// Create formatter
	formatter, err := NewFormatter(format)
	if err != nil {
		return err
	}

	// Create writer
	writer := NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
	writer.SetColorEnabled(!flags.NoColor)
	writer = writer.WithFormatter(formatter)

	// Output data
	return writer.Output(v)
}

// ErrorFromFlags formats and displays error using flags
func ErrorFromFlags(cmd *cobra.Command, err error, flags *CommonFlags) error {
	writer := NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
	writer.SetColorEnabled(!flags.NoColor)

	// Check if it's a CLIError
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		// Display formatted error
		if flags.JSON {
			// Output as JSON
			jsonBytes, jsonErr := cliErr.JSON()
			if jsonErr == nil {
				// Ignore write errors - we're about to exit anyway
				//nolint:errcheck,gosec
				cmd.ErrOrStderr().Write(jsonBytes)
				//nolint:errcheck,gosec
				cmd.ErrOrStderr().Write([]byte("\n"))
			}
		} else {
			// Display human-readable error
			writer.Error(cliErr.Error())
		}

		// Exit with appropriate code
		os.Exit(cliErr.ExitCode)
	}

	// For non-CLIError, just display message
	writer.Error(err.Error())
	return err
}
