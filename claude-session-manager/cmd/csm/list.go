package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	listJSON   bool
	listAll    bool
	listLegacy bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Claude session manifests",
	Long: `List Claude session manifests from ~/sessions/ directory.

By default, shows only non-archived sessions.
Use --all to show all sessions including archived.

Displays session status based on tmux state:
- active:   tmux session is running
- stopped:  tmux session not running
- archived: session marked as archived

Examples:
  csm list              # List active/stopped sessions
  csm list --all        # List all sessions (including archived)
  csm list --json       # Output as JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// List all manifests
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			if os.IsNotExist(err) {
				ui.PrintWarning(fmt.Sprintf("No sessions directory found: %s", cfg.SessionsDir))
				fmt.Printf("\nCreate your first session with: csm new\n")
				return nil
			}
			ui.PrintError(err,
			"Failed to list manifests",
			"  • Check sessions directory permissions: ls -ld "+cfg.SessionsDir+"\n"+
				"  • Verify directory structure: ls -la "+cfg.SessionsDir+"\n"+
				"  • Try creating sessions directory: mkdir -p "+cfg.SessionsDir)
			return err
		}

		if len(manifests) == 0 {
			ui.PrintWarning("No sessions found")
			fmt.Printf("\nCreate your first session with: csm new\n")
			return nil
		}

		// Filter out archived sessions unless --all is set
		if !listAll {
			filtered := make([]*manifest.Manifest, 0, len(manifests))
			for _, m := range manifests {
				if m.Lifecycle != manifest.LifecycleArchived {
					filtered = append(filtered, m)
				}
			}
			manifests = filtered

			if len(manifests) == 0 {
				ui.PrintWarning("No active/stopped sessions found")
				fmt.Println("\nUse --all to see archived sessions")
				return nil
			}
		}

		// Output
		if listJSON {
			output, err := ui.FormatJSON(manifests)
			if err != nil {
				return err
			}
			fmt.Println(output)
		} else {
			tmux := session.NewRealTmux()
			var output string
			if listLegacy {
				output = ui.FormatTableLegacy(manifests, tmux)
			} else {
				output = ui.FormatTable(manifests, tmux)
			}
			fmt.Print(output)
		}

		return nil
	},
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	listCmd.Flags().BoolVar(&listAll, "all", false, "Show all sessions including archived")
	listCmd.Flags().BoolVar(&listLegacy, "legacy", false, "Use legacy table format (v2.x style)")

	rootCmd.AddCommand(listCmd)
}
