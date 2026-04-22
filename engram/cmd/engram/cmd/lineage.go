package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/hippocampus"
)

var lineageCmd = &cobra.Command{
	Use:   "lineage [session-id]",
	Short: "Show session lineage chain",
	Long:  "Display the parent-child chain for a session, tracing back to the root session.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store := hippocampus.DefaultLineageStore()
		chain, err := store.GetLineageChain(args[0])
		if err != nil {
			return fmt.Errorf("get lineage chain: %w", err)
		}

		if len(chain) == 0 {
			fmt.Fprintf(os.Stderr, "No lineage found for session %s\n", args[0])
			return nil
		}

		for i, entry := range chain {
			prefix := "  "
			if i == 0 {
				prefix = "→ "
			}
			teleport := ""
			if entry.Teleported {
				teleport = " [teleported]"
			}
			parent := entry.ParentSessionID
			if parent == "" {
				parent = "(root)"
			}
			fmt.Printf("%s%s ← %s (%s, %s)%s\n",
				prefix, entry.SessionID, parent,
				entry.TransitionType, entry.CreatedAt.Format("2006-01-02 15:04"),
				teleport)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(lineageCmd)
}
