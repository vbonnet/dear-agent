package main

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/deadlock"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var forceKill bool
var hardKill bool

var killCmd = &cobra.Command{
	Use:   "kill [session-name]",
	Short: "Kill tmux session or deadlocked Claude process",
	Long: `Kill tmux session for a AGM session or force-kill deadlocked Claude process.

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
  # Soft kill with confirmation prompt
  agm session kill my-session

  # Soft kill without confirmation (for scripts)
  agm session kill my-session --force

  # Hard kill (detect deadlock and SIGKILL Claude process)
  agm session kill my-session --hard

  # Hard kill without confirmation
  agm session kill my-session --hard --force

  # Resume session after killing
  agm session resume my-session`,
	Args: cobra.ExactArgs(1),
	RunE: runKillCommand,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete first argument (session identifier)
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Defensive check: ensure cfg is initialized
		if cfg == nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// List manifests from configured sessions directory
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			// Fail gracefully - return empty list if can't read sessions
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// Get tmux mapping (session ID → tmux name)
		tmuxMapping, _ := discovery.GetTmuxMapping(cfg.SessionsDir)
		// Ignore error - worst case: empty mapping, no tmux names suggested

		// Build completion suggestions
		var suggestions []string
		for _, m := range manifests {
			// Skip archived sessions (can't kill archived)
			if m.Lifecycle == manifest.LifecycleArchived {
				continue
			}

			// Add tmux name (primary identifier)
			if tmuxName := tmuxMapping[m.SessionID]; tmuxName != "" {
				suggestions = append(suggestions, tmuxName)
			}

			// Add manifest name (secondary identifier, if different from tmux name)
			if m.Name != "" && m.Name != tmuxMapping[m.SessionID] {
				suggestions = append(suggestions, m.Name)
			}
		}

		// Return suggestions with NoFileComp directive (prevent file completion)
		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

func init() {
	killCmd.Flags().BoolVarP(&forceKill, "force", "f", false, "skip confirmation prompt")
	killCmd.Flags().BoolVar(&hardKill, "hard", false, "hard kill: detect deadlock and SIGKILL Claude process")
	sessionCmd.AddCommand(killCmd)
}

func runKillCommand(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Step 1: Resolve session identifier
	m, _, err := session.ResolveIdentifier(sessionName, cfg.SessionsDir)
	if err != nil {
		return renderSessionNotFoundError(sessionName)
	}

	// Step 2: Validate session is not archived
	if m.Lifecycle == manifest.LifecycleArchived {
		return renderSessionArchivedError(sessionName)
	}

	tmuxSessionName := m.Tmux.SessionName

	// Step 3: Branch based on hard/soft kill
	if hardKill {
		return runHardKill(sessionName, tmuxSessionName)
	}

	// Soft kill: just terminate tmux session
	// Step 4: Confirm action (unless --force)
	if !forceKill {
		confirmed, err := confirmKill(sessionName, tmuxSessionName)
		if err != nil || !confirmed {
			fmt.Println("Cancelled")
			return nil
		}
	}

	// Step 5: Kill tmux session (idempotent)
	err = killTmuxSession(tmuxSessionName)
	if err != nil {
		return renderKillError(sessionName, err)
	}

	// Step 6: Success message
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
	if !info.IsDeadlock && !forceKill {
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
			return nil
		}
	} else if !forceKill {
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
			return nil
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

func killTmuxSession(tmuxName string) error {
	socketPath := tmux.GetSocketPath()
	ctx := context.Background()

	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "kill-session", "-t", tmuxName)

	// Execute and ignore errors (idempotent behavior)
	// Session may already be dead, which is OK
	_ = cmd.Run()

	return nil
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

func renderKillError(sessionName string, err error) error {
	ui.PrintError(
		err,
		"Failed to kill tmux session",
		fmt.Sprintf(`Check if tmux is installed and accessible.

If session is stuck, try:
  • Manually kill: tmux kill-session -t %s
  • Check tmux socket: %s`, sessionName, tmux.GetSocketPath()),
	)
	return err
}

func renderSuccessMessage(sessionName string) {
	ui.PrintSuccess(fmt.Sprintf("Tmux session killed for '%s'", sessionName))
	fmt.Println()
	fmt.Printf("  The session can be resumed with:\n")
	fmt.Printf("    agm session resume %s\n", sessionName)
}
