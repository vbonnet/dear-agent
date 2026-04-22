package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/delegation"
)

var delegationCmd = &cobra.Command{
	Use:   "delegation",
	Short: "Manage delegation tracking",
	Long:  `Track and resolve outbound task delegations between sessions.`,
}

var delegationListCmd = &cobra.Command{
	Use:   "list [session-name]",
	Short: "List pending delegations for a session",
	Args:  cobra.ExactArgs(1),
	RunE:  runDelegationList,
}

var delegationResolveCmd = &cobra.Command{
	Use:   "resolve [session-name] [message-id]",
	Short: "Mark a delegation as completed",
	Args:  cobra.ExactArgs(2),
	RunE:  runDelegationResolve,
}

var delegationResolveAllCmd = &cobra.Command{
	Use:   "resolve-all [session-name]",
	Short: "Mark all pending delegations for a session as completed",
	Args:  cobra.ExactArgs(1),
	RunE:  runDelegationResolveAll,
}

func init() {
	delegationCmd.AddCommand(delegationListCmd)
	delegationCmd.AddCommand(delegationResolveCmd)
	delegationCmd.AddCommand(delegationResolveAllCmd)
	rootCmd.AddCommand(delegationCmd)
}

func runDelegationList(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	dir, err := delegation.DefaultDir()
	if err != nil {
		return err
	}
	tracker, err := delegation.NewTracker(dir)
	if err != nil {
		return err
	}

	pending, err := tracker.Pending(sessionName)
	if err != nil {
		return fmt.Errorf("failed to read delegations: %w", err)
	}

	if len(pending) == 0 {
		fmt.Printf("No pending delegations for '%s'\n", sessionName)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "MESSAGE ID\tTO\tSUMMARY\tCREATED\n")
	for _, d := range pending {
		summary := d.TaskSummary
		if len(summary) > 60 {
			summary = summary[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			d.MessageID, d.To, summary, d.CreatedAt.Format("2006-01-02 15:04"))
	}
	w.Flush()

	fmt.Printf("\n%d pending delegation(s)\n", len(pending))
	return nil
}

func runDelegationResolve(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	messageID := args[1]

	dir, err := delegation.DefaultDir()
	if err != nil {
		return err
	}
	tracker, err := delegation.NewTracker(dir)
	if err != nil {
		return err
	}

	if err := tracker.Resolve(sessionName, messageID, delegation.StatusCompleted); err != nil {
		return err
	}

	fmt.Printf("✓ Delegation %s resolved for '%s'\n", messageID, sessionName)
	return nil
}

func runDelegationResolveAll(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	dir, err := delegation.DefaultDir()
	if err != nil {
		return err
	}
	tracker, err := delegation.NewTracker(dir)
	if err != nil {
		return err
	}

	pending, err := tracker.Pending(sessionName)
	if err != nil {
		return fmt.Errorf("failed to read delegations: %w", err)
	}

	if len(pending) == 0 {
		fmt.Printf("No pending delegations for '%s'\n", sessionName)
		return nil
	}

	for _, d := range pending {
		if err := tracker.Resolve(sessionName, d.MessageID, delegation.StatusCompleted); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to resolve %s: %v\n", d.MessageID, err)
		}
	}

	fmt.Printf("✓ Resolved %d delegation(s) for '%s'\n", len(pending), sessionName)
	return nil
}
