package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	listJSON   bool
	listStatus string
	listSort   string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Claude sessions",
	Long: `List all Claude sessions with their status, last activity, and location.

Examples:
  csm list                    # List all sessions
  csm list --status=active    # List only active sessions
  csm list --json             # Output as JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			ui.PrintError(
				err,
				"Failed to list sessions.",
				"  • Verify sessions directory exists: "+cfg.SessionsDir+"\n"+
					"  • Run 'csm sync' to discover sessions",
			)
			return err
		}

		// Filter by status
		if listStatus != "" {
			filtered := []*manifest.Manifest{}
			for _, m := range manifests {
				if m.Status == listStatus {
					filtered = append(filtered, m)
				}
			}
			manifests = filtered
		}

		// Sort
		switch listSort {
		case "id":
			sort.Slice(manifests, func(i, j int) bool {
				return manifests[i].SessionID < manifests[j].SessionID
			})
		case "tmux":
			sort.Slice(manifests, func(i, j int) bool {
				return manifests[i].Tmux.SessionName < manifests[j].Tmux.SessionName
			})
		default: // last_activity (default)
			sort.Slice(manifests, func(i, j int) bool {
				return manifests[i].LastActivity.After(manifests[j].LastActivity)
			})
		}

		// Output
		if listJSON {
			output, err := ui.FormatJSON(manifests)
			if err != nil {
				return fmt.Errorf("failed to format JSON: %w", err)
			}
			fmt.Println(output)
		} else {
			output := ui.FormatTable(manifests)
			fmt.Print(output)
		}

		return nil
	},
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status (active, discovered, stale, archived)")
	listCmd.Flags().StringVar(&listSort, "sort", "activity", "Sort by (activity, id, tmux)")

	rootCmd.AddCommand(listCmd)
}
