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
	Use:   "new [session-name]",
	Short: "Create a new Claude session with tmux",
	Long: `Create a new Claude session with tmux integration.

This command will:
1. Create or use an existing tmux session with the specified name
2. Start Claude CLI in the tmux session
3. Create a manifest linking the tmux session to the Claude session

Arguments:
  session-name - Name for the tmux/Claude session (optional)
                 If not provided and outside tmux, you'll be prompted
                 If not provided and inside tmux, uses current tmux session name

Behavior:
  • Outside tmux + no name → Prompts for name, creates tmux + claude
  • Outside tmux + name provided → Creates tmux session with that name + claude
  • Inside tmux + no name → Uses current tmux name, starts claude
  • Inside tmux + matching name → Uses current tmux, starts claude
  • Inside tmux + different name → Error (name mismatch)

Examples:
  csm new                       # Prompt for name or use current tmux session
  csm new my-project            # Create session named "my-project"
  csm new feature-branch        # Create session named "feature-branch"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		inTmux := os.Getenv("TMUX") != ""
		var sessionName string
		var err error

		// Determine session name based on context
		if len(args) > 0 {
			// User provided a session name
			sessionName = args[0]

			// If inside tmux, verify name matches current session
			if inTmux {
				currentTmuxName, err := tmux.GetCurrentSessionName()
				if err != nil {
					ui.PrintError(err, "Failed to get current tmux session name", "")
					return err
				}

				if sessionName != currentTmuxName {
					ui.PrintError(
						fmt.Errorf("session name mismatch: %s (provided) != %s (current tmux)", sessionName, currentTmuxName),
						"Cannot create session with different name while inside tmux",
						"  • Exit tmux first, or\n  • Use 'csm new' without arguments to use current tmux session",
					)
					return fmt.Errorf("session name mismatch")
				}
			}
		} else {
			// No name provided
			if inTmux {
				// Use current tmux session name
				sessionName, err = tmux.GetCurrentSessionName()
				if err != nil {
					ui.PrintError(err, "Failed to get current tmux session name", "")
					return err
				}
				fmt.Printf("Using current tmux session: %s\n", sessionName)
			} else {
				// Prompt for session name
				sessionName, err = ui.PromptForString("Enter session name")
				if err != nil {
					ui.PrintError(err, "Failed to read session name", "")
					return err
				}

				if sessionName == "" {
					ui.PrintError(
						fmt.Errorf("session name cannot be empty"),
						"Invalid session name",
						"  • Provide a non-empty session name",
					)
					return fmt.Errorf("empty session name")
				}
			}
		}

		// Now we have a session name. Handle the two scenarios:
		// 1. Inside tmux: start Claude in current session
		// 2. Outside tmux: create tmux session, start Claude, attach

		if inTmux {
			return startClaudeInCurrentTmux(sessionName)
		}

		return createTmuxSessionAndStartClaude(sessionName)
	},
}

// createTmuxSessionAndStartClaude creates a new tmux session and starts Claude in it
func createTmuxSessionAndStartClaude(sessionName string) error {
	// Get current working directory
	workDir, err := os.Getwd()
	if err != nil {
		ui.PrintError(err, "Failed to get current directory", "")
		return err
	}

	fmt.Printf("Creating new tmux session: %s (in %s)\n", sessionName, workDir)

	// Check if tmux session already exists
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		ui.PrintError(err, "Failed to check tmux session", "")
		return err
	}

	if exists {
		// Prompt user for action
		choice, err := ui.Prompt(
			fmt.Sprintf("Tmux session '%s' already exists. What would you like to do?", sessionName),
			[]string{
				"Reuse existing tmux session (start Claude in it)",
				"Choose a different name",
				"Cancel",
			},
		)
		if err != nil {
			ui.PrintError(err, "Failed to read choice", "")
			return err
		}

		switch choice {
		case 0:
			// Reuse existing session
			fmt.Printf("Reusing existing tmux session: %s\n", sessionName)
		case 1:
			// Prompt for new name
			newName, err := ui.PromptForString("Enter new session name")
			if err != nil {
				ui.PrintError(err, "Failed to read session name", "")
				return err
			}
			if newName == "" {
				ui.PrintError(
					fmt.Errorf("session name cannot be empty"),
					"Invalid session name",
					"",
				)
				return fmt.Errorf("empty session name")
			}
			sessionName = newName
			// Recursively handle the new name (might also conflict)
			return createTmuxSessionAndStartClaude(sessionName)
		case 2:
			// Cancel
			fmt.Println("Cancelled.")
			return nil
		}
	} else {
		// Create new tmux session
		if err := tmux.NewSession(sessionName, workDir); err != nil {
			ui.PrintError(err, "Failed to create tmux session", "")
			return err
		}
		ui.PrintSuccess(fmt.Sprintf("Created tmux session: %s", sessionName))
	}

	// Start Claude in the session
	claudeCmd := "claude; exit"
	if err := tmux.SendCommand(sessionName, claudeCmd); err != nil {
		ui.PrintError(err, "Failed to start Claude", "")
		// Try to kill the tmux session if we just created it and Claude failed
		if !exists {
			_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
		}
		return err
	}
	ui.PrintSuccess("Started Claude CLI in tmux session")

	// Give Claude a moment to initialize before we start polling
	time.Sleep(500 * time.Millisecond)

	// Wait for Claude to be ready before attaching
	// Increased timeout to 15s to account for MCP loading and SessionStart hooks
	spinner := ui.NewSpinner("Waiting for Claude to be ready...")
	spinner.Start()
	if err := tmux.WaitForProcessReady(sessionName, "claude", 15*time.Second); err != nil {
		spinner.Warning("Claude is taking longer than expected (still starting)")
		fmt.Println("  Attaching now - Claude should appear shortly")
	} else {
		spinner.Success("Claude is ready!")
	}

	// Create manifest
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, fmt.Sprintf("session-%s", sessionName))
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
	} else {
		// Create v2 manifest
		m := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     fmt.Sprintf("session-%s", sessionName),
			Name:          sessionName,
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
				SessionName: sessionName,
			},
		}

		if err := manifest.Write(manifestPath, m); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to write manifest: %v", err))
		} else {
			fmt.Printf("Created manifest: %s\n", manifestPath)
			fmt.Println("💡 Run 'csm sync' after sending first message to populate Claude UUID")
		}
	}

	// Update VS Code tab title if running in VS Code
	updateVSCodeTabTitle(sessionName)

	// Release lock before attaching (attachment can block for hours)
	// The lock should only protect the setup phase, not the tmux attachment
	if globalLock != nil {
		if err := globalLock.Unlock(); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to release lock: %v", err))
		}
		globalLock = nil // Prevent double-unlock in PersistentPostRunE
	}

	// Attach to session
	fmt.Printf("Attaching to tmux session: %s\n", sessionName)
	if err := tmux.AttachSession(sessionName); err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not attach to session: %v", err))
		fmt.Printf("Session created successfully. Attach manually with: tmux attach -t %s\n", sessionName)
	}

	return nil
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

// startClaudeInCurrentTmux starts a fresh Claude session in the current tmux session
func startClaudeInCurrentTmux(sessionName string) error {
	fmt.Printf("Starting new Claude session in current tmux: %s\n", sessionName)

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		ui.PrintError(err, "Failed to get current directory", "")
		return err
	}

	// Create or update manifest
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, fmt.Sprintf("session-%s", sessionName))
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	// Create manifest directory if it doesn't exist
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
	} else {
		// Create v2 manifest (will be updated with UUID by hooks)
		m := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     fmt.Sprintf("session-%s", sessionName),
			Name:          sessionName,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     "", // Empty = active
			Context: manifest.Context{
				Project: workDir,
				Purpose: "",
				Tags:    nil,
				Notes:   "",
			},
			Tmux: manifest.Tmux{
				SessionName: sessionName,
			},
		}

		if err := manifest.Write(manifestPath, m); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to write manifest: %v", err))
		} else {
			ui.PrintSuccess(fmt.Sprintf("Created/updated manifest: %s", manifestPath))
		}
	}

	// Start Claude in current pane
	fmt.Println("Starting Claude CLI...")
	claudeCmd := "claude; exit"
	if err := tmux.SendCommand(sessionName, claudeCmd); err != nil {
		ui.PrintError(err, "Failed to start Claude", "")
		return err
	}

	ui.PrintSuccess("Claude session started in current tmux!")
	fmt.Println("💡 The session will be automatically associated via SessionStart hook")

	// Update VS Code tab title if running in VS Code
	updateVSCodeTabTitle(sessionName)

	return nil
}

func init() {
	rootCmd.AddCommand(newCmd)
}
