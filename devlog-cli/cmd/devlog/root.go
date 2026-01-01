package devlog

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
	dryRun  bool
)

var rootCmd = &cobra.Command{
	Use:   "devlog",
	Short: "Manage devlog workspaces with bare repos and worktrees",
	Long: `devlog is a CLI tool for managing development workspaces that use
bare git repositories with multiple worktrees.

It simplifies syncing repos across machines and managing worktrees
for parallel development workflows.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set up verbose logging if requested
		if verbose {
			fmt.Fprintln(os.Stderr, "Verbose mode enabled")
			fmt.Fprintf(os.Stderr, "Config file: %s\n", cfgFile)
			fmt.Fprintf(os.Stderr, "Dry run: %v\n", dryRun)
		}
	},
}

// Execute runs the root command and returns any error encountered.
// The caller is responsible for handling the error and exiting with
// appropriate exit codes.
func Execute() error {
	return rootCmd.Execute()
}

// GetConfigFile returns the config file path specified via --config flag.
// Returns empty string if not specified (will use default .devlog/config.yaml).
func GetConfigFile() string {
	return cfgFile
}

// IsVerbose returns true if verbose output is enabled via -v/--verbose flag.
func IsVerbose() bool {
	return verbose
}

// IsDryRun returns true if dry-run mode is enabled via --dry-run flag.
// In dry-run mode, commands should show what would happen without executing.
func IsDryRun() bool {
	return dryRun
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is .devlog/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would happen without doing it")
}
