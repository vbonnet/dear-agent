package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/deadlock"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var hardKill bool
var forceKill bool
var confirmedStuck bool

var killCmd = &cobra.Command{
	Use:   "kill [session-name]",
	Short: "Kill tmux session or deadlocked Claude process",
	Long: `Kill tmux session for a AGM session or force-kill deadlocked Claude process.

SAFETY: Active sessions require --confirmed-stuck flag to prevent accidental
termination. Stopped sessions can be killed without the flag.

This command has two modes:

SOFT KILL (default):
  Terminates the tmux session immediately while preserving session metadata.
  The session can be resumed later with 'agm session resume'.

HARD KILL (--hard flag):
  Detects deadlocked Claude processes and sends SIGKILL.
  - Checks for deadlock criteria (RNl+ state, CPU >25%, runtime >5min)
  - Confirms with user before killing
  - Sends SIGKILL to Claude process
  - Logs incident to ~/deadlock-log.txt
  - Verifies session recovered to prompt

Use soft kill when:
  • Tmux session is stuck or unresponsive
  • Terminal crashed but session still running
  • Need to force-stop without archiving

Use hard kill when:
  • ESC/Ctrl-C don't work (tried 'agm session recover' first)
  • Process shows high CPU usage
  • Session stuck in deadlock state

Note: This does NOT archive the session. Use 'agm exit' for graceful
shutdown with archiving.

Examples:
  # Kill a stopped session (no flag needed)
  agm session kill my-session

  # Kill an active session (requires --confirmed-stuck)
  agm session kill my-session --confirmed-stuck

  # Hard kill (detect deadlock and SIGKILL Claude process)
  agm session kill my-session --hard

  # Resume session after killing
  agm session resume my-session`,
	Args: cobra.ExactArgs(1),
	RunE: runKillCommand,
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

		// Return suggestions with NoFileComp directive (prevent file completion)
		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

func init() {
	killCmd.Flags().BoolVar(&hardKill, "hard", false, "hard kill: detect deadlock and SIGKILL Claude process")
	killCmd.Flags().BoolVarP(&forceKill, "force", "f", false, "skip confirmation prompt (for automation)")
	killCmd.Flags().BoolVar(&confirmedStuck, "confirmed-stuck", false, "required to kill an active (running) session")
	sessionCmd.AddCommand(killCmd)
}

func runKillCommand(cmd *cobra.Command, args []string) (retErr error) {
	sessionName := args[0]

	// Audit trail: log who killed what session and mode
	defer func() {
		mode := "soft"
		if hardKill {
			mode = "hard"
		}
		logCommandAudit("session.kill", sessionName, map[string]string{
			"mode":  mode,
			"force": fmt.Sprintf("%v", forceKill),
		}, retErr)
	}()

	// Construct OpContext with storage
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer cleanup()

	// Use ops.KillSession for session resolution and validation
	killResult, killErr := ops.KillSession(opCtx, &ops.KillSessionRequest{
		Identifier:     sessionName,
		Force:          forceKill,
		ConfirmedStuck: confirmedStuck,
	})
	if killErr != nil {
		var opErr *ops.OpError
		if errors.As(killErr, &opErr) {
			switch opErr.Code {
			case ops.ErrCodeSessionNotFound:
				return renderSessionNotFoundError(sessionName)
			case ops.ErrCodeSessionArchived:
				return renderSessionArchivedError(sessionName)
			case ops.ErrCodeActiveSessionKill:
				return renderActiveSessionError(sessionName)
			case ops.ErrCodeKillProtected:
				// Session is recently active — prompt for confirmation
				ago := "recently"
				if killResult != nil && killResult.LastActivity != nil {
					ago = fmt.Sprintf("%s ago", time.Since(*killResult.LastActivity).Truncate(time.Second))
				}
				ui.PrintWarning(fmt.Sprintf("Session '%s' was active %s", sessionName, ago))
				var confirmed bool
				confirmErr := huh.NewConfirm().
					Title("Kill recently active session?").
					Description("This session has recent activity. Are you sure you want to kill it?").
					Affirmative("Yes, kill it").
					Negative("Cancel").
					Value(&confirmed).
					WithTheme(ui.GetTheme()).
					Run()
				if confirmErr != nil || !confirmed {
					fmt.Println("Cancelled")
					return nil
				}
				// Re-issue with force
				killResult, killErr = ops.KillSession(opCtx, &ops.KillSessionRequest{
					Identifier: sessionName,
					Force:      true,
				})
				if killErr != nil {
					return killErr
				}
			}
		}
		if killErr != nil {
			return killErr
		}
	}

	// For hard kill, use the tmux session name from ops result
	if hardKill {
		return runHardKill(sessionName, killResult.Name)
	}

	// Soft kill: confirm (unless --force) and terminate tmux session
	if !forceKill {
		confirmed, confirmErr := confirmKill(sessionName, killResult.Name)
		if confirmErr != nil || !confirmed {
			fmt.Println("Cancelled")
			return nil
		}
	}

	// Kill tmux session (idempotent)
	killTmuxSession(killResult.Name)

	// Success message
	renderSuccessMessage(sessionName)
	return nil
}

func runHardKill(sessionName, tmuxSessionName string) error {
	fmt.Printf("Detecting deadlock for session '%s'...\n\n", sessionName)

	// Step 1: Detect deadlock
	info, err := deadlock.DetectClaudeDeadlock(tmuxSessionName)
	if err != nil {
		ui.PrintError(
			err,
			"Failed to detect deadlock",
			fmt.Sprintf(`Could not find Claude process or check deadlock status.

Possible causes:
  • Session may not be running
  • Claude process already terminated
  • Permission issues accessing process info

Try:
  • Check session status: agm session list
  • Resume session: agm session resume %s
  • Use soft kill: agm session kill %s`, sessionName, sessionName),
		)
		return err
	}

	// Step 2: Display process information
	fmt.Println(deadlock.FormatProcessInfo(info))
	fmt.Println()

	// Step 3: Confirm if deadlock detected, warn if not
	if !info.IsDeadlock {
		ui.PrintWarning(fmt.Sprintf(`Process does not appear to be deadlocked.

Deadlock criteria (from ROADMAP-STAGE-1.md):
  • State: R (running/runnable)
  • CPU: > 25%%
  • Runtime: > 5 minutes

Current process does not meet all criteria.

Recommendations:
  1. Try soft recovery first: agm session recover %s
  2. If that fails, try soft kill: agm session kill %s
  3. Only use hard kill if process is truly deadlocked

Do you still want to proceed with hard kill?`, sessionName, sessionName))
		fmt.Println()

		var confirmed bool
		err := huh.NewConfirm().
			Title("Proceed with hard kill anyway?").
			Description("This will send SIGKILL to the Claude process.").
			Affirmative("Yes, kill process").
			Negative("Cancel").
			Value(&confirmed).
			WithTheme(ui.GetTheme()).
			Run()

		if err != nil || !confirmed {
			fmt.Println("Cancelled")
			return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
		}
	} else {
		// Deadlock detected, confirm kill
		var confirmed bool
		description := fmt.Sprintf(`DEADLOCK DETECTED

This will:
  1. Send SIGKILL to Claude process (PID %d)
  2. Log incident to ~/deadlock-log.txt
  3. Verify session recovery

This is an irreversible action.`, info.PID)

		err := huh.NewConfirm().
			Title(fmt.Sprintf("Kill deadlocked Claude process for '%s'?", sessionName)).
			Description(description).
			Affirmative("Yes, kill process").
			Negative("Cancel").
			Value(&confirmed).
			WithTheme(ui.GetTheme()).
			Run()

		if err != nil || !confirmed {
			fmt.Println("Cancelled")
			return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
		}
	}

	// Step 4: Send SIGKILL to Claude process
	fmt.Printf("\nSending SIGKILL to process %d...\n", info.PID)
	if err := syscall.Kill(info.PID, syscall.SIGKILL); err != nil {
		ui.PrintError(
			err,
			"Failed to kill Claude process",
			fmt.Sprintf(`Could not send SIGKILL to process %d.

Try manual kill:
  kill -9 %d`, info.PID, info.PID),
		)
		return err
	}

	// Step 5: Wait for process to die
	time.Sleep(2 * time.Second)

	// Step 6: Verify session recovered
	fmt.Println("Verifying session recovery...")

	// Check if process is still alive
	if processExists(info.PID) {
		ui.PrintWarning(fmt.Sprintf("Process %d may still be alive. Check manually with: ps -p %d", info.PID, info.PID))
	} else {
		fmt.Println("✓ Claude process terminated")
	}

	// Step 7: Log incident
	fmt.Println("Logging incident to ~/deadlock-log.txt...")
	if err := deadlock.LogDeadlockIncident(sessionName, info); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to log incident: %v", err))
	} else {
		fmt.Println("✓ Incident logged")
	}

	// Step 8: Success message
	fmt.Println()
	ui.PrintSuccess(fmt.Sprintf("Hard kill complete for session '%s'", sessionName))
	fmt.Println()
	fmt.Printf("  Next steps:\n")
	fmt.Printf("    • Resume session: agm session resume %s\n", sessionName)
	fmt.Printf("    • Review incident log: cat ~/deadlock-log.txt\n")

	return nil
}

func processExists(pid int) bool {
	// Check if process exists by sending signal 0
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil
}

func confirmKill(sessionName, tmuxName string) (bool, error) {
	var confirmed bool

	description := fmt.Sprintf(`Session: %s
Tmux session: %s

This will terminate the tmux process immediately.
Session data will be preserved and can be resumed later.

Resume with: agm session resume %s`, sessionName, tmuxName, sessionName)

	err := huh.NewConfirm().
		Title(fmt.Sprintf("Kill tmux session for '%s'?", sessionName)).
		Description(description).
		Affirmative("Yes, kill session").
		Negative("Cancel").
		Value(&confirmed).
		WithTheme(ui.GetTheme()).
		Run()

	return confirmed, err
}

func killTmuxSession(tmuxName string) {
	socketPath := tmux.GetSocketPath()
	ctx := context.Background()

	// Normalize session name (dots/colons → dashes)
	// This matches tmux's actual normalization behavior
	normalizedName := tmux.NormalizeTmuxSessionName(tmuxName)

	// Use exact matching (= prefix) to prevent prefix-matching bugs
	// This is critical: without =prefix, "astrocyte" could match "astrocyte-improvements"
	// See ADR-0002 for details on tmux exact matching behavior
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "kill-session", "-t", tmux.FormatSessionTarget(normalizedName))

	// Execute and ignore errors (idempotent behavior)
	// Session may already be dead, which is OK
	_ = cmd.Run()
}

func renderSessionNotFoundError(sessionName string) error {
	ui.PrintError(
		fmt.Errorf("session not found"),
		fmt.Sprintf("Session '%s' not found", sessionName),
		`• List all sessions: agm session list
• Create new session: agm session new <name>`,
	)
	return fmt.Errorf("session not found")
}

func renderActiveSessionError(sessionName string) error {
	ui.PrintWarning(fmt.Sprintf(`Session '%s' is actively running.

Killing an active session can cause data loss. If the session is truly
stuck, re-run with --confirmed-stuck:

  agm session kill --confirmed-stuck %s

If the session is healthy, use graceful shutdown instead:

  agm exit  (from inside the session)`, sessionName, sessionName))
	return fmt.Errorf("session is active — use --confirmed-stuck to force kill")
}

func renderSessionArchivedError(sessionName string) error {
	ui.PrintError(
		fmt.Errorf("session is archived"),
		fmt.Sprintf("Cannot kill archived session '%s'", sessionName),
		fmt.Sprintf(`Archived sessions don't have active tmux processes.

To work with this session:
  1. Resume it: agm session resume %s
  2. Then kill if needed: agm session kill %s`, sessionName, sessionName),
	)
	return fmt.Errorf("session is archived")
}

func renderSuccessMessage(sessionName string) {
	ui.PrintSuccess(fmt.Sprintf("Tmux session killed for '%s'", sessionName))
	fmt.Println()
	fmt.Printf("  The session can be resumed with:\n")
	fmt.Printf("    agm session resume %s\n", sessionName)
}
