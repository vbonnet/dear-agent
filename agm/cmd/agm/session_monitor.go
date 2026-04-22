package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var addMonitorCmd = &cobra.Command{
	Use:   "add-monitor <session> <monitor-session>",
	Short: "Register a session as a loop monitor for another session",
	Long: `Register a monitor session that watches another session's loop heartbeat.
When the monitored session's loop heartbeat goes stale, the daemon will
attempt to wake the monitor session's loop.

Examples:
  agm session add-monitor my-worker orchestrator-v2
  agm session add-monitor research-session meta-orchestrator`,
	Args: cobra.ExactArgs(2),
	RunE: runAddMonitor,
}

var removeMonitorCmd = &cobra.Command{
	Use:   "remove-monitor <session> <monitor-session>",
	Short: "Remove a loop monitor from a session",
	Long: `Remove a previously registered monitor session.

Examples:
  agm session remove-monitor my-worker orchestrator-v2`,
	Args: cobra.ExactArgs(2),
	RunE: runRemoveMonitor,
}

var listMonitorsCmd = &cobra.Command{
	Use:   "list-monitors <session>",
	Short: "List loop monitors registered for a session",
	Long: `Show all sessions registered as loop monitors for the given session.

Examples:
  agm session list-monitors my-worker`,
	Args: cobra.ExactArgs(1),
	RunE: runListMonitors,
}

func init() {
	sessionCmd.AddCommand(addMonitorCmd)
	sessionCmd.AddCommand(removeMonitorCmd)
	sessionCmd.AddCommand(listMonitorsCmd)
}

func runAddMonitor(_ *cobra.Command, args []string) error {
	sessionIdentifier := args[0]
	monitorSession := args[1]

	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()

	// Resolve the target session
	getResult, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: sessionIdentifier,
	})
	if opErr != nil {
		return handleError(opErr)
	}

	m, err := opCtx.Storage.GetSession(getResult.Session.ID)
	if err != nil {
		return fmt.Errorf("failed to read session: %w", err)
	}

	// Check for duplicates
	for _, mon := range m.Monitors {
		if mon == monitorSession {
			ui.PrintWarning(fmt.Sprintf("Monitor %q already registered on session %s", monitorSession, m.Name))
			return nil
		}
	}

	m.Monitors = append(m.Monitors, monitorSession)
	if err := opCtx.Storage.UpdateSession(m); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	fmt.Printf("Added monitor %s to session %s\n", ui.Bold(monitorSession), ui.Bold(m.Name))
	fmt.Printf("Monitors: %s\n", strings.Join(m.Monitors, ", "))
	return nil
}

func runRemoveMonitor(_ *cobra.Command, args []string) error {
	sessionIdentifier := args[0]
	monitorSession := args[1]

	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()

	getResult, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: sessionIdentifier,
	})
	if opErr != nil {
		return handleError(opErr)
	}

	m, err := opCtx.Storage.GetSession(getResult.Session.ID)
	if err != nil {
		return fmt.Errorf("failed to read session: %w", err)
	}

	found := false
	monitors := make([]string, 0, len(m.Monitors))
	for _, mon := range m.Monitors {
		if mon == monitorSession {
			found = true
			continue
		}
		monitors = append(monitors, mon)
	}
	if !found {
		return fmt.Errorf("monitor %q not found on session %s", monitorSession, m.Name)
	}

	m.Monitors = monitors
	if err := opCtx.Storage.UpdateSession(m); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	fmt.Printf("Removed monitor %s from session %s\n", ui.Bold(monitorSession), ui.Bold(m.Name))
	if len(m.Monitors) > 0 {
		fmt.Printf("Monitors: %s\n", strings.Join(m.Monitors, ", "))
	} else {
		fmt.Println("Monitors: (none)")
	}
	return nil
}

func runListMonitors(_ *cobra.Command, args []string) error {
	sessionIdentifier := args[0]

	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()

	getResult, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: sessionIdentifier,
	})
	if opErr != nil {
		return handleError(opErr)
	}

	m, err := opCtx.Storage.GetSession(getResult.Session.ID)
	if err != nil {
		return fmt.Errorf("failed to read session: %w", err)
	}

	fmt.Printf("Session: %s\n", ui.Bold(m.Name))
	if len(m.Monitors) > 0 {
		fmt.Printf("Monitors (%d):\n", len(m.Monitors))
		for _, mon := range m.Monitors {
			fmt.Printf("  - %s\n", mon)
		}
	} else {
		fmt.Println("Monitors: (none)")
	}
	return nil
}
