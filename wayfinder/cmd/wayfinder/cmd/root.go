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
	Short: "Navigate SDLC journey with 9 canonical phases (V1 12-ID legacy orchestrator)",
	Long: `Wayfinder - Structured Development Lifecycle Navigation

The canonical Wayfinder model is 9 phases:
  CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO

This (` + "`wayfinder`" + `) is the V1 legacy orchestrator. It walks 12 detailed
phase IDs that map to the 9 canonical V2 phases via the migration bridge:
  D1-D4 → PROBLEM, RESEARCH, DESIGN, SPEC          (discovery)
  S4    → SPEC                                     (merged: stakeholder alignment)
  S5    → PLAN                                     (merged: research notes)
  S6-S7 → PLAN, SETUP                              (design + plan)
  S8-S10 → BUILD                                   (merged: implement + validate + deploy)
  S11   → RETRO                                    (retrospective)

For the V2 9-phase CLI, see ` + "`wayfinder-session`" + `. See wayfinder/PHASES.md for the
full V1↔V2 truth table.

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
