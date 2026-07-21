package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	sessioncmd "github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/commands"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage Wayfinder session lifecycle",
	Long: `Manage Wayfinder session lifecycle for analytics and tracking.

Publishes session and phase events to EventBus for analytics collection.

Commands:
  start <project>           Start new session
  next-phase                Get next phase in sequence
  start-phase <phase>       Mark phase as started
  complete-phase <phase>    Mark phase as completed
  end                       End current session

Examples:
  wayfinder session start myproject
  wayfinder session next-phase
  wayfinder session start-phase PROBLEM
  wayfinder session complete-phase PROBLEM --outcome success
  wayfinder session end --status completed`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		dir := GetProjectDirectory()

		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("directory does not exist: %s", dir)
			}
			return fmt.Errorf("failed to access directory %s: %w", dir, err)
		}

		// If STATUS file is not in dir, scan wf/*/ to auto-discover the project.
		// This fixes the common case where `wayfinder start` is run from the repo
		// root and places STATUS under wf/<id>/, but session commands are invoked
		// from the same repo root without an explicit -C flag.
		statusFile := filepath.Join(dir, "WAYFINDER-STATUS.md")
		if _, err := os.Stat(statusFile); os.IsNotExist(err) {
			discovered, discErr := discoverProjectDir(dir)
			if discErr == nil {
				fmt.Fprintf(os.Stderr, "wayfinder: using project at %s (use -C to be explicit)\n", discovered)
				dir = discovered
			}
		}

		sessioncmd.SetProjectDirectory(dir)
		return nil
	},
}

// discoverProjectDir scans <root>/wf/*/ for exactly one WAYFINDER-STATUS.md.
// Returns the matching subdirectory, or an error if zero or multiple are found.
func discoverProjectDir(root string) (string, error) {
	wfDir := filepath.Join(root, "wf")
	entries, err := os.ReadDir(wfDir)
	if err != nil {
		return "", fmt.Errorf("no wf/ directory under %s", root)
	}

	var found []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(wfDir, e.Name(), "WAYFINDER-STATUS.md")
		if _, err := os.Stat(candidate); err == nil {
			found = append(found, filepath.Dir(candidate))
		}
	}

	switch len(found) {
	case 0:
		return "", fmt.Errorf("no project found under %s/wf/", root)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("multiple projects found under %s/wf/; use -C <project-dir> to specify", root)
	}
}

func init() {
	// Add session subcommands from existing package
	sessionCmd.AddCommand(sessioncmd.StartCmd)
	sessionCmd.AddCommand(sessioncmd.StatusCmd)
	sessionCmd.AddCommand(sessioncmd.NextPhaseCmd)
	sessionCmd.AddCommand(sessioncmd.StartPhaseCmd)
	sessionCmd.AddCommand(sessioncmd.CompletePhaseCmd)
	sessionCmd.AddCommand(sessioncmd.EndCmd)

	// Add sandbox commands
	sessionCmd.AddCommand(sessioncmd.CreateSandboxCmd)
	sessionCmd.AddCommand(sessioncmd.ListSandboxesCmd)
	sessionCmd.AddCommand(sessioncmd.CleanupSandboxesCmd)

	// Add task commands
	sessionCmd.AddCommand(sessioncmd.TaskCmd)

	sessionCmd.AddCommand(sessioncmd.RewindCmd)
	sessionCmd.AddCommand(sessioncmd.GetLifecycleStateCmd())
	sessionCmd.AddCommand(sessioncmd.GetCoordCmd())

	// Add session to root
	rootCmd.AddCommand(sessionCmd)
}
