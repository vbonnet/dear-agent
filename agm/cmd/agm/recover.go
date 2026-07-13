package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/recovery"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/state"
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
		defer func() { _ = adapter.Close() }()

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
		defer func() { _ = adapter.Close() }()
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

	attempts := []struct {
		label string
		keys  []string
	}{
		{label: "ESC", keys: []string{"Escape"}},
		{label: "Ctrl-C", keys: []string{"C-c"}},
		{label: "double Ctrl-C", keys: []string{"C-c", "C-c"}},
	}
	for i, attempt := range attempts {
		fmt.Printf("%d. Sending %s...\n", i+1, attempt.label)
		confirmed, attemptErr := attemptVerifiedRecovery(cmd.Context(), tmuxSessionName, attempt.keys)
		if attemptErr != nil {
			fmt.Printf("   Warning: %v\n", attemptErr)
			continue
		}
		if confirmed {
			fmt.Printf("   Recovery confirmed with %s\n", attempt.label)
			renderRecoverySuccess(sessionName)
			return nil
		}
		fmt.Println("   Recovery signal sent but child-process exit was not confirmed")
	}

	if recovery.FallbackForHarness(m.Harness) == recovery.FallbackLeafInterrupt {
		fmt.Println("4. AGY terminal keys were unconfirmed; interrupting session-scoped work leaves...")
		before, snapshotErr := recovery.SnapshotSession(cmd.Context(), tmuxSessionName)
		if snapshotErr != nil {
			fmt.Printf("   Warning: could not snapshot AGY process tree: %v\n", snapshotErr)
		} else {
			interrupted, interruptErr := recovery.InterruptWorkLeaves(before)
			if interruptErr != nil {
				fmt.Printf("   Warning: %v\n", interruptErr)
			}
			if interrupted > 0 {
				if err := recovery.WaitForConfirmation(cmd.Context(), 5*time.Second); err != nil {
					return err
				}
				after, afterErr := recovery.SnapshotSession(cmd.Context(), tmuxSessionName)
				if afterErr == nil && recovery.Confirmed(before, after, false) {
					fmt.Printf("   Recovery confirmed after interrupting %d AGY work process(es)\n", interrupted)
					renderRecoverySuccess(sessionName)
					return nil
				}
			}
		}
	}

	// All methods failed
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

func sendKey(tmuxSessionName, key string) error {
	socketPath := tmux.GetSocketPath()
	ctx := context.Background()

	// Verify session state via capture-pane before sending recovery keys.
	// Bug fix: must confirm session is reachable before injecting keys.
	checkCmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "capture-pane", "-p", "-t", tmuxSessionName)
	if err := checkCmd.Run(); err != nil {
		return fmt.Errorf("capture-pane failed before sending %s: %w (session may be down)", key, err)
	}

	// Use raw hex 0x0d for Enter to avoid paste coalescing
	if key == "Enter" || key == "C-m" {
		cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", tmuxSessionName, "-H", "0d")
		return cmd.Run()
	}
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", tmuxSessionName, key)
	return cmd.Run()
}

func attemptVerifiedRecovery(ctx context.Context, tmuxSessionName string, keys []string) (bool, error) {
	before, err := recovery.SnapshotSession(ctx, tmuxSessionName)
	if err != nil {
		return false, fmt.Errorf("snapshot before recovery: %w", err)
	}
	for i, key := range keys {
		if err := sendKey(tmuxSessionName, key); err != nil {
			return false, fmt.Errorf("send %s: %w", key, err)
		}
		if i+1 < len(keys) {
			if err := recovery.WaitForConfirmation(ctx, 500*time.Millisecond); err != nil {
				return false, err
			}
		}
	}
	fmt.Println("   Signal sent, waiting 5 seconds for process-state confirmation...")
	if err := recovery.WaitForConfirmation(ctx, 5*time.Second); err != nil {
		return false, err
	}
	after, err := recovery.SnapshotSession(ctx, tmuxSessionName)
	if err != nil {
		return false, fmt.Errorf("snapshot after recovery: %w", err)
	}
	promptReady := session.CheckSessionDelivery(tmuxSessionName) == state.CanReceiveYes
	return recovery.Confirmed(before, after, promptReady), nil
}

func renderRecoverySuccess(sessionName string) {
	fmt.Println()
	ui.PrintSuccess(fmt.Sprintf("Session '%s' recovered", sessionName))
	fmt.Println()
	fmt.Printf("  You can now:\n")
	fmt.Printf("    • Continue working in the session\n")
	fmt.Printf("    • Attach to verify: agm session resume %s\n", sessionName)
}
