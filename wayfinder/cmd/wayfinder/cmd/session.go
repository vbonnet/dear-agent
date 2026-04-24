package cmd

import (
	"fmt"
	"os"

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
  wayfinder session start-phase D1
  wayfinder session complete-phase D1 --outcome success
  wayfinder session end --status completed`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Get project directory from root command flag or current directory
		dir := GetProjectDirectory()

		// Validate directory exists
		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("directory does not exist: %s", dir)
			}
			return fmt.Errorf("failed to access directory %s: %w", dir, err)
		}

		// Set project directory for session commands package
		sessioncmd.SetProjectDirectory(dir)
		return nil
	},
}

func init() {
	// Add session subcommands from existing package
	sessionCmd.AddCommand(sessioncmd.StartCmd)
	sessionCmd.AddCommand(sessioncmd.StatusCmd)
	sessionCmd.AddCommand(sessioncmd.VerifyCmd)
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

	// Add migration commands
	sessionCmd.AddCommand(sessioncmd.MigrateCmd)
	sessionCmd.AddCommand(sessioncmd.MigrateAllCmd)

	// Add session to root
	rootCmd.AddCommand(sessionCmd)
}
