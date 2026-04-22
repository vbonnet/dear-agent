// Package app provides app functionality.
package app

import (
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/terminal"
)

// FilesystemInterface abstracts filesystem operations for testing
type FilesystemInterface interface {
	// Add filesystem methods as needed for testing
	// Examples: ReadFile, WriteFile, Stat, etc.
}

// App holds all dependencies for the AGM application
type App struct {
	PTY     terminal.PTYProvider
	Tmux    session.TmuxInterface
	Harness agent.Agent
	FS      FilesystemInterface
}

// NewApp creates a new App with the given dependencies
func NewApp(pty terminal.PTYProvider, tmux session.TmuxInterface, agent agent.Agent, fs FilesystemInterface) *App {
	return &App{
		PTY:     pty,
		Tmux:    tmux,
		Harness: agent,
		FS:      fs,
	}
}

// BuildRootCommand creates the root Cobra command with all subcommands
// This allows the command structure to be tested with mocked dependencies
func (a *App) BuildRootCommand() *cobra.Command {
	// This will be populated with actual command implementations in future phases
	// For now, return a basic root command
	rootCmd := &cobra.Command{
		Use:   "agm",
		Short: "Agent Gateway Manager - Multi-AI session management",
		Long:  "agm (Agent Gateway Manager) helps you manage AI agent sessions",
	}

	// Subcommands will be added here in future phases
	// Examples: session, admin, etc.

	return rootCmd
}

// Run executes the main application logic with the given arguments
func (a *App) Run(args []string) error {
	rootCmd := a.BuildRootCommand()
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}
