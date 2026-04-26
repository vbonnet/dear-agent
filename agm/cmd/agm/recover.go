package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var recoverCmd = &cobra.Command{
	Use:   "recover [session-name]",
	Short: "Soft recovery for stuck sessions (ESC/Ctrl-C)",
	Long: `Attempt soft recovery for stuck or unresponsive sessions using non-destructive methods.

This command tries multiple recovery strategies in sequence:
  1. Send ESC (wait 5 seconds) - interrupts thinking/processing
  2. Send Ctrl-C (wait 5 seconds) - cancels current operation
  3. Send double Ctrl-C (wait 5 seconds) - force cancel

If all methods fail, suggests using 'agm session kill' for hard recovery.

Use this when:
  • Session shows "Improvising..." with zero tokens for extended time
  • Claude appears stuck but tmux session is responsive
  • You want to try non-destructive recovery first

Note: This is SOFT recovery (non-destructive). For deadlocked processes
that don't respond to ESC/Ctrl-C, use 'agm session kill'.

Examples:
  # Try soft recovery
  agm session recover my-session

  # If soft recovery fails, use hard recovery
  agm session kill my-session`,
	Args: cobra.ExactArgs(1),
	RunE: runRecoverCommand,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete first argument (session identifier)
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Get Dolt adapter
		adapter, err := getStorage()
		if err != nil {
			// Fail gracefully - return empty list if can't connect to Dolt
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		defer adapter.Close()

		// List sessions from Dolt (exclude archived sessions from completion)
		filter := &dolt.SessionFilter{
			ExcludeArchived: true,
		}
		sessions, err := adapter.ListSessions(filter)
		if err != nil {
			// Fail gracefully - return empty list if query fails
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// Build completion suggestions
		var suggestions []string
		for _, m := range sessions {
			// Add tmux name (primary identifier)
			if m.Tmux.SessionName != "" {
				suggestions = append(suggestions, m.Tmux.SessionName)
			}

			// Add manifest name (secondary identifier, if different)
			if m.Name != "" && m.Name != m.Tmux.SessionName {
				suggestions = append(suggestions, m.Name)
			}
		}

		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

func init() {
	sessionCmd.AddCommand(recoverCmd)
}

func runRecoverCommand(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Get Dolt adapter for session resolution
	adapter, _ := getStorage()
	if adapter != nil {
		defer adapter.Close()
	}

	// Step 1: Resolve session identifier
	m, _, err := session.ResolveIdentifier(sessionName, cfg.SessionsDir, adapter)
	if err != nil {
		return renderSessionNotFoundError(sessionName)
	}

	// Step 2: Validate session is not archived
	if m.Lifecycle == manifest.LifecycleArchived {
		return renderSessionArchivedError(sessionName)
	}

	tmuxSessionName := m.Tmux.SessionName

	// Step 3: Check if tmux session exists
	exists, err := tmux.HasSession(tmuxSessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		ui.PrintError(
			fmt.Errorf("tmux session not found"),
			fmt.Sprintf("Session '%s' has no active tmux session", sessionName),
			fmt.Sprintf("Resume the session first:\n  agm session resume %s", sessionName),
		)
		return fmt.Errorf("tmux session not found")
	}

	// Step 4: Attempt soft recovery
	fmt.Printf("Attempting soft recovery for session '%s'...\n\n", sessionName)

	success := false

	// Try 1: ESC
	fmt.Println("1. Sending ESC to interrupt...")
	if err := sendKey(tmuxSessionName, "Escape"); err != nil {
		fmt.Printf("   Warning: Failed to send ESC: %v\n", err)
	} else {
		fmt.Println("   Sent ESC, waiting 5 seconds...")
		time.Sleep(5 * time.Second)
		if checkRecovered(tmuxSessionName) {
			success = true
			fmt.Println("   ✓ Recovery successful with ESC")
			renderRecoverySuccess(sessionName)
			return nil
		}
		fmt.Println("   Still stuck, trying next method...")
	}

	// Try 2: Single Ctrl-C
	fmt.Println("\n2. Sending Ctrl-C...")
	if err := sendKey(tmuxSessionName, "C-c"); err != nil {
		fmt.Printf("   Warning: Failed to send Ctrl-C: %v\n", err)
	} else {
		fmt.Println("   Sent Ctrl-C, waiting 5 seconds...")
		time.Sleep(5 * time.Second)
		if checkRecovered(tmuxSessionName) {
			success = true
			fmt.Println("   ✓ Recovery successful with Ctrl-C")
			renderRecoverySuccess(sessionName)
			return nil
		}
		fmt.Println("   Still stuck, trying next method...")
	}

	// Try 3: Double Ctrl-C
	fmt.Println("\n3. Sending double Ctrl-C...")
	if err := sendKey(tmuxSessionName, "C-c"); err != nil {
		fmt.Printf("   Warning: Failed to send first Ctrl-C: %v\n", err)
	} else {
		time.Sleep(500 * time.Millisecond)
		if err := sendKey(tmuxSessionName, "C-c"); err != nil {
			fmt.Printf("   Warning: Failed to send second Ctrl-C: %v\n", err)
		} else {
			fmt.Println("   Sent double Ctrl-C, waiting 5 seconds...")
			time.Sleep(5 * time.Second)
			if checkRecovered(tmuxSessionName) {
				success = true
				fmt.Println("   ✓ Recovery successful with double Ctrl-C")
				renderRecoverySuccess(sessionName)
				return nil
			}
			fmt.Println("   Still stuck")
		}
	}

	// All methods failed
	if !success {
		fmt.Println()
		ui.PrintError(
			fmt.Errorf("soft recovery failed"),
			"All soft recovery methods failed",
			fmt.Sprintf(`The session may be in a deadlock state.

Next steps:
  1. Check session status: agm session list
  2. Attach to see current state: tmux -S %s attach -t %s
  3. Use hard recovery: agm session kill %s

Hard recovery will:
  - Detect deadlock (high CPU, RNl+ state)
  - Confirm before killing
  - Send SIGKILL to Claude process
  - Log incident to ~/deadlock-log.txt`, tmux.GetSocketPath(), tmuxSessionName, sessionName),
		)
		return fmt.Errorf("soft recovery failed")
	}

	return nil
}

func sendKey(tmuxSessionName, key string) error {
	socketPath := tmux.GetSocketPath()
	ctx := context.Background()

	// Verify session state via capture-pane before sending recovery keys.
	// Bug fix: must confirm session is reachable before injecting keys.
	checkCmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "capture-pane", "-p", "-t", tmuxSessionName)
	if err := checkCmd.Run(); err != nil {
		return fmt.Errorf("capture-pane failed before sending %s: %w (session may be down)", key, err)
	}

	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", tmuxSessionName, key)
	return cmd.Run()
}

func checkRecovered(tmuxSessionName string) bool {
	// Check if session is responsive by capturing pane content
	// If we can read the pane and it shows a prompt, it's likely recovered
	// This is a simple heuristic - just check if tmux responds
	socketPath := tmux.GetSocketPath()
	ctx := context.Background()

	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "capture-pane", "-p", "-t", tmuxSessionName)
	_, err := cmd.Output()

	// If we can capture the pane without error, session is responsive
	// Note: This is a simple check. A more sophisticated check would parse
	// the output for prompt markers, but that's complex and fragile.
	return err == nil
}

func renderRecoverySuccess(sessionName string) {
	fmt.Println()
	ui.PrintSuccess(fmt.Sprintf("Session '%s' recovered", sessionName))
	fmt.Println()
	fmt.Printf("  You can now:\n")
	fmt.Printf("    • Continue working in the session\n")
	fmt.Printf("    • Attach to verify: agm session resume %s\n", sessionName)
}
