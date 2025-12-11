package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var newCmd = &cobra.Command{
	Use:   "new [directory]",
	Short: "Create a new Claude session with tmux",
	Long: `Create a new Claude session with tmux integration in one command.

This command will:
1. Generate a unique tmux session name (claude-<project>)
2. Create a detached tmux session in the specified directory
3. Start Claude CLI in the tmux session
4. Create a manifest placeholder for future tracking

Arguments:
  directory - Working directory for the session (default: current directory)

Examples:
  csm new                          # Create session in current directory
  csm new ~/projects/my-app        # Create session in specific directory
  csm new /tmp/experiment          # Create session in tmp directory`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine working directory
		var workDir string
		if len(args) > 0 {
			workDir = args[0]
		} else {
			var err error
			workDir, err = os.Getwd()
			if err != nil {
				ui.PrintError(err, "Failed to get current directory", "")
				return err
			}
		}

		// Convert to absolute path
		workDir, err := filepath.Abs(workDir)
		if err != nil {
			ui.PrintError(err, "Failed to resolve absolute path", "")
			return err
		}

		// Verify directory exists
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			ui.PrintError(
				fmt.Errorf("directory does not exist: %s", workDir),
				"Working directory not found",
				"  • Create the directory first, or\n  • Specify an existing directory",
			)
			return err
		}

		fmt.Printf("Creating new Claude session in: %s\n", workDir)

		// Get existing tmux sessions for name conflict detection
		existingSessions, err := tmux.ListSessions()
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to list tmux sessions: %v", err))
			existingSessions = []string{}
		}

		// Generate unique tmux name
		tmuxName := generateTmuxName(workDir, existingSessions)
		fmt.Printf("Generated tmux session name: %s\n", tmuxName)

		// Create tmux session
		if err := tmux.NewSession(tmuxName, workDir); err != nil {
			ui.PrintError(err, "Failed to create tmux session", "")
			return err
		}
		ui.PrintSuccess(fmt.Sprintf("Created tmux session: %s", tmuxName))

		// Start Claude in the session
		claudeCmd := "claude"
		if err := tmux.SendCommand(tmuxName, claudeCmd); err != nil {
			ui.PrintError(err, "Failed to start Claude", "")
			// Try to kill the tmux session since Claude failed
			_ = tmux.SendCommand(tmuxName, "tmux kill-session -t "+tmuxName)
			return err
		}
		ui.PrintSuccess("Started Claude CLI in tmux session")

		// Wait for Claude to be ready before attaching
		fmt.Println("⏳ Waiting for Claude to be ready...")
		if err := tmux.WaitForProcessReady(tmuxName, "claude", 5*time.Second); err != nil {
			ui.PrintWarning("Claude is taking longer than expected (still starting)")
			fmt.Println("  Attaching now - Claude should appear shortly")
		} else {
			ui.PrintSuccess("Claude is ready!")
		}

		// Create manifest placeholder
		// NOTE: We can't know the Claude UUID yet since the session just started
		// The UUID will be populated later when user runs `csm sync`
		sessionsDir := getSessionsDir()
		manifestDir := filepath.Join(sessionsDir, fmt.Sprintf("session-%s", tmuxName))
		manifestPath := filepath.Join(manifestDir, "manifest.yaml")

		if err := os.MkdirAll(manifestDir, 0700); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
		} else {
			// Create v2 manifest
			m := &manifest.Manifest{
				SchemaVersion: manifest.SchemaVersion,
				SessionID:     "", // Will be populated by sync
				Name:          tmuxName,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Lifecycle:     "", // Empty = active/stopped
				Context: manifest.Context{
					Project: workDir,
					Purpose: "", // Can be set later
					Tags:    nil,
					Notes:   "",
				},
				Tmux: manifest.Tmux{
					SessionName: tmuxName,
				},
			}

			if err := manifest.Write(manifestPath, m); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to write manifest: %v", err))
			} else {
				fmt.Printf("Created manifest placeholder: %s\n", manifestPath)
				fmt.Println("💡 Run 'csm sync' after sending first message to populate Claude UUID")
			}
		}

		// Attach to session (or switch if already in tmux)
		fmt.Printf("Attaching to tmux session: %s\n", tmuxName)
		if err := tmux.AttachSession(tmuxName); err != nil {
			ui.PrintWarning(fmt.Sprintf("Could not attach to session: %v", err))
			fmt.Printf("Session created successfully. Attach manually with: tmux attach -t %s\n", tmuxName)
		}

		return nil
	},
}

// getSessionsDir returns the sessions directory (respects --sessions-dir flag)
func getSessionsDir() string {
	if cfg != nil && cfg.SessionsDir != "" {
		return cfg.SessionsDir
	}
	// Default to ~/sessions
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "sessions")
}

func init() {
	rootCmd.AddCommand(newCmd)
}
