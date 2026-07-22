package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version information - set via ldflags at build time
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
	BuiltBy   = "unknown"
)

var (
	// projectDirectory is the working directory for wayfinder commands
	projectDirectory string

	// workspaceFlag allows explicit workspace selection
	workspaceFlag string
)

var rootCmd = &cobra.Command{
	Use:   "wayfinder",
	Short: "Navigate the canonical 9-phase Wayfinder SDLC",
	Long: `Wayfinder - Structured Development Lifecycle Navigation

The canonical Wayfinder model is 9 phases:
  CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO

Commands:
  session                 Manage the session lifecycle and tasks

Examples:
	wayfinder session start my-project
	wayfinder session next-phase
	wayfinder session start-phase PROBLEM
	wayfinder session complete-phase PROBLEM --outcome success`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Print header to stderr for all commands except version
		if cmd.Name() != "version" {
			executable, err := os.Executable()
			if err != nil {
				executable = "unknown"
			}
			fmt.Fprintf(os.Stderr, "wayfinder %s (%s)\n", Version, executable)
		}
		return nil
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&projectDirectory, "directory", "C", "",
		"Run as if started in <path> instead of current directory")
	rootCmd.PersistentFlags().StringVar(&workspaceFlag, "workspace", "",
		"Explicit workspace name (overrides auto-detection)")
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

// GetProjectDirectory returns the project directory, defaulting to current directory
func GetProjectDirectory() string {
	if projectDirectory != "" {
		return projectDirectory
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get current directory: %v\n", err)
		return "."
	}
	return cwd
}

// SetProjectDirectory sets the project directory (used by subcommands)
func SetProjectDirectory(dir string) {
	projectDirectory = dir
}
