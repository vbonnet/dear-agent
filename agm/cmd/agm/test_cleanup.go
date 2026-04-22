package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	cleanupJSON        bool
	cleanupSessionsDir string
)

var testCleanupCmd = &cobra.Command{
	Use:   "cleanup [name]",
	Short: "Clean up test sessions",
	Long: `Clean up test sessions by killing the tmux session and removing state directories.

Cleanup is best-effort - if one step fails, the others will still be attempted.
Partial cleanup is considered successful if at least one step succeeds.

Examples:
  # Cleanup specific session
  agm test cleanup my-test

  # Cleanup with custom sessions directory
  agm test cleanup my-test --sessions-dir /tmp/my-tests

  # Get JSON output for automation
  agm test cleanup my-test --json`,
	Args: cobra.ExactArgs(1),
	RunE: runTestCleanup,
}

func init() {
	testCleanupCmd.Flags().BoolVar(
		&cleanupJSON,
		"json",
		false,
		"Output as JSON for automation",
	)
	testCleanupCmd.Flags().StringVar(
		&cleanupSessionsDir,
		"sessions-dir",
		"",
		"Sessions directory to clean (default: /tmp/agm-test-<name>)",
	)

	// Mark as hidden - use common commands with --test flag instead
	testCleanupCmd.Hidden = true

	testCmd.AddCommand(testCleanupCmd)
}

// CleanupStatus represents cleanup status
type CleanupStatus struct {
	Name           string `json:"name"`
	TmuxKilled     bool   `json:"tmux_killed"`
	DirectoryClean bool   `json:"directory_clean"`
	Status         string `json:"status"` // "success" or "partial"
}

func runTestCleanup(cmd *cobra.Command, args []string) error {
	name := args[0]
	tmuxName := fmt.Sprintf("agm-test-%s", name)

	// Set default sessions directory
	if cleanupSessionsDir == "" {
		cleanupSessionsDir = fmt.Sprintf("/tmp/agm-test-%s", name)
	}

	// Perform cleanup with or without spinner
	var status *CleanupStatus
	var cleanupErr error

	if !cleanupJSON {
		// Human mode - use spinner
		spinErr := spinner.New().
			Title(fmt.Sprintf("Cleaning up test session '%s'...", name)).
			Accessible(true).
			Action(func() {
				status, cleanupErr = performCleanup(name, tmuxName, cleanupSessionsDir)
			}).
			Run()
		if spinErr != nil {
			return fmt.Errorf("spinner error: %w", spinErr)
		}

		// Ensure clean line after spinner
		fmt.Println()

		if cleanupErr != nil {
			return cleanupErr
		}

		// Print human-readable output
		ui.PrintSuccess(fmt.Sprintf("Test session cleaned up: %s", name))
		if status.TmuxKilled {
			fmt.Printf("  Tmux session: killed\n")
		} else {
			fmt.Printf("  Tmux session: already gone\n")
		}
		if status.DirectoryClean {
			fmt.Printf("  Directory: removed\n")
		} else {
			fmt.Printf("  Directory: failed to remove (may not exist)\n")
		}
		if status.Status == "partial" {
			fmt.Println()
			ui.PrintWarning("Partial cleanup - some steps failed")
		}
	} else {
		// Automation mode - direct call
		status, cleanupErr = performCleanup(name, tmuxName, cleanupSessionsDir)
		if cleanupErr != nil {
			return cleanupErr
		}

		// Print JSON output
		jsonBytes, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	}

	return nil
}

// performCleanup performs best-effort cleanup
func performCleanup(name, tmuxName, sessionsDir string) (*CleanupStatus, error) {
	status := &CleanupStatus{
		Name:           name,
		TmuxKilled:     false,
		DirectoryClean: false,
		Status:         "success",
	}

	// Step 1: Kill tmux session (best effort)
	exists, err := tmux.HasSession(tmuxName)
	if err == nil && exists {
		killCmd := exec.Command("tmux", "kill-session", "-t", tmuxName)
		if killCmd.Run() == nil {
			status.TmuxKilled = true
		}
	} else {
		// Session doesn't exist or check failed - consider it killed
		status.TmuxKilled = true
	}

	// Step 2: Remove sessions directory (best effort)
	if err := os.RemoveAll(sessionsDir); err == nil {
		status.DirectoryClean = true
	}

	// Determine overall status
	if !status.TmuxKilled || !status.DirectoryClean {
		status.Status = "partial"
	}

	// Return error only if all steps failed
	if !status.TmuxKilled && !status.DirectoryClean {
		return status, fmt.Errorf("cleanup failed completely.\n\nManual cleanup:\n  • Kill session: tmux kill-session -t %s\n  • Remove directory: rm -rf %s", tmuxName, sessionsDir)
	}

	return status, nil
}
