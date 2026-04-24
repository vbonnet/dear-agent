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
	Short: "Navigate SDLC journey with 12 sequential waypoints",
	Long: `Wayfinder - Structured Development Lifecycle Navigation

Wayfinder guides you through 12 sequential phases:
  D1-D4: Discovery phases (problem validation, solutions, approach, requirements)
  S4-S11: SDLC phases (alignment, research, design, plan, implement, validate, deploy, retrospective)

Commands:
  start <description>     Create new Wayfinder project
  session                 Manage session lifecycle
  features                Track feature-level progress
  waypoints               Manage waypoint summaries
  autopilot               Execute all phases automatically
  abort                   Abort and archive project

Examples:
  wayfinder start "Implement OAuth authentication"
  wayfinder session next-phase
  wayfinder features status
  wayfinder autopilot --isolated`,
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
