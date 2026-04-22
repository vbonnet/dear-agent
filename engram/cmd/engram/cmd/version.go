package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display engram version information",
	Long: `Display engram version information including version number,
git commit SHA, and build date.

This command shows the current version of engram that is installed.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Output format matches AC2: "engram version X.Y.Z (commit: <sha>, built: <date>)"
		fmt.Printf("engram version %s (commit: %s, built: %s)\n",
			version, commit, date)
	},
}

func init() {
	// Register with root command (auto-executed when package imported)
	rootCmd.AddCommand(versionCmd)
}
