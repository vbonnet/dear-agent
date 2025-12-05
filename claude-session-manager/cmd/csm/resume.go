package main

import (
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <identifier>",
	Short: "Resume a Claude session",
	Long: `Resume a Claude session by tmux name, workspace ID, or Claude UUID.

Examples:
  csm resume claude-1                                 # By tmux name
  csm resume github.com-user-repo-main                # By workspace ID
  csm resume c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2     # By Claude UUID`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		if err := session.Resume(identifier, cfg); err != nil {
			ui.PrintError(
				err,
				"Failed to resume session.",
				"  • Run 'csm sync' to discover existing Claude sessions\n"+
					"  • Check available sessions with 'csm list'\n"+
					"  • Verify session health with 'csm doctor'",
			)
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}
