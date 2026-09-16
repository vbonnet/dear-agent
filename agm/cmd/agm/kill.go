package main

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/deadlock"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/internal/override"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var hardKill bool
var forceKill bool
var confirmedStuck bool
var killReason string

var killCmd = &cobra.Command{
	Use:   "kill [session-name]",
	Short: "Kill tmux session or deadlocked Claude process",
	Long: `Kill tmux session for a AGM session or force-kill deadlocked Claude process.

SAFETY: Active sessions require --confirmed-stuck flag to prevent accidental
termination. Stopped sessions can be killed without the flag. "Active" means
a harness process is actually running in the session's pane tree — a tmux
session whose harness has died (zombie pane) does not require the flag, and
either --confirmed-stuck or --force alone is sufficient confirmation for a
recently-active session (no flag combination is ever required).

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

		// Return suggestions with NoFileComp directive (prevent file completion)
		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

func init() {
	killCmd.Flags().BoolVar(&hardKill, "hard", false, "hard kill: detect deadlock and SIGKILL Claude process")
	killCmd.Flags().BoolVarP(&forceKill, "force", "f", false, "skip confirmation prompt — requires --reason")
	killCmd.Flags().StringVar(&killReason, "reason", "", "justification for --force, recorded in the override audit log")
	killCmd.Flags().BoolVar(&confirmedStuck, "confirmed-stuck", false, "required to kill an active (running) session")
	sessionCmd.AddCommand(killCmd)
}

func runKillCommand(cmd *cobra.Command, args []string) (retErr error) {
	sessionName := args[0]

	// Override guard: --force skips interactive confirmation — require a reason.
	if forceKill {
		if gerr := override.Require(context.Background(), override.Guard{
			Tool: "agm session kill",
			Flag: "--force",
			Gate: "interactive kill confirmation",
			Risk: override.RiskP2,
		}, killReason); gerr != nil {
			return gerr
		}
	}

	// In JSON mode, suppress Cobra's text error/usage dump so the only thing
	// emitted on the error path is our structured JSON object.
	if isJSONOutput() {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
	}

	ctx, span := startKillSpan(cmd.Context(), sessionName, hardKill, forceKill)
	defer func() {
		if retErr != nil {
			span.RecordError(retErr)
			span.SetStatus(codes.Error, retErr.Error())
		}
		span.End()
	}()

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
	opCtx.Context = ctx
	interactiveSoftKill := !hardKill && !forceKill && !isJSONOutput()
	// Interactive soft kill and hard-kill detection need a side-effect-free
	// shared preflight. All non-interactive soft-kill paths execute the shared
	// mutation immediately so no surface can report success without it.
	opCtx.DryRun = hardKill || interactiveSoftKill

	killResult, killErr := ops.KillSession(opCtx, &ops.KillSessionRequest{
		Identifier:     sessionName,
		Force:          forceKill,
		ConfirmedStuck: confirmedStuck,
	})
	if killErr != nil {
		var done bool
		killResult, done, killErr = handleKillError(opCtx, sessionName, killResult, killErr)
		if done {
			return killErr
		}
	}

	// Hard kill is a process-level recovery flow, so the shared kill operation
	// remains a dry-run preflight and supplies the exact tmux target.
	if hardKill {
		return runHardKill(sessionName, killResult.TmuxSessionName)
	}

	// Soft kill: confirm before executing the shared mutation.
	// JSON mode is treated as non-interactive: the confirmation prompt is
	// skipped (the ops layer still enforces --confirmed-stuck for active
	// sessions, so this stays safe).
	if interactiveSoftKill {
		if killResult.RecentlyActive {
			ui.PrintWarning(fmt.Sprintf("Session '%s' was recently active", sessionName))
		}
		confirmed, confirmErr := confirmKill(sessionName, killResult.TmuxSessionName)
		if confirmErr != nil || !confirmed {
			fmt.Println("Cancelled")
			return nil
		}

		opCtx.DryRun = false
		killResult, killErr = ops.KillSession(opCtx, &ops.KillSessionRequest{
			Identifier:     sessionName,
			Force:          true,
			ConfirmedStuck: confirmedStuck,
		})
		if killErr != nil {
			return killErr
		}
	}

	// Success message
	if isJSONOutput() {
		out := map[string]string{"status": "killed", "session": sessionName}
		if killResult.HarnessDead {
			out["harness_dead"] = "true"
			out["liveness_evidence"] = killResult.LivenessEvidence
			if killResult.ZombieWriter {
				out["zombie_writer"] = "true"
			}
		}
		return printJSON(out)
	}
	if killResult.HarnessDead {
		renderZombieNotice(killResult)
	}
	renderSuccessMessage(sessionName)
	return nil
}

// renderZombieNotice explains WHY a session that tmux still listed was
// killable without --confirmed-stuck: the process check proved no harness
// was running in the pane tree (ce-axsr). If an orphaned agm process was
// found, it was likely keeping a heartbeat file falsely fresh (ce-qkf7) and
// died with the pane.
func renderZombieNotice(r *ops.KillSessionResult) {
	ui.PrintWarning(fmt.Sprintf(
		"Session was a zombie: tmux session existed but no harness process was running (pane tree: %s)",
		r.LivenessEvidence))
	if r.ZombieWriter {
		ui.PrintWarning("An orphaned agm process was in the pane tree — it may have been writing a stale-but-fresh-looking heartbeat; it dies with the pane.")
	}
}

// renderKillError emits a kill failure. In JSON mode it prints a structured
// {"error","session"} object to stdout and returns an error (so the process
// exits non-zero); in text mode it runs the human-readable renderer.
func renderKillError(sessionName, message string, textRender func() error) error {
	if isJSONOutput() {
		if err := printJSON(map[string]string{"error": message, "session": sessionName}); err != nil {
			return err
		}
		return fmt.Errorf("%s", message)
	}
	return textRender()
}

// handleKillError dispatches an error from ops.KillSession into specific
// rendering or interactive flows. Returns the (possibly updated) killResult,
var confirmKillProtected = func(sessionName string) (bool, error) {
	var confirmed bool
	err := huh.NewConfirm().
		Title("Kill recently active session?").
		Description("This session has recent activity. Are you sure you want to kill it?").
		Affirmative("Yes, kill it").
		Negative("Cancel").
		Value(&confirmed).
		WithTheme(ui.GetTheme()).
		Run()
	return confirmed, err
}

// the (possibly resolved) error, and a `done` flag indicating whether the
// caller should return immediately. When done=false, the original killErr
// has been resolved and the caller should continue with killResult.
func handleKillError(opCtx *ops.OpContext, sessionName string, killResult *ops.KillSessionResult, killErr error) (*ops.KillSessionResult, bool, error) {
	var opErr *ops.OpError
	if !errors.As(killErr, &opErr) {
		if isJSONOutput() {
			return killResult, true, renderKillError(sessionName, killErr.Error(), nil)
		}
		return killResult, true, killErr
	}
	switch opErr.Code {
	case ops.ErrCodeSessionNotFound:
		return killResult, true, renderKillError(sessionName, "session not found",
			func() error { return renderSessionNotFoundError(sessionName) })
	case ops.ErrCodeSessionArchived:
		return killResult, true, renderKillError(sessionName, "session is archived",
			func() error { return renderSessionArchivedError(sessionName) })
	case ops.ErrCodeActiveSessionKill:
		// Agent mode (JSON) is non-interactive: rather than blocking on a TTY
		// prompt, emit a confirmation envelope (exit 4) carrying the exact
		// re-run command. Human mode keeps the existing text warning.
		if isJSONOutput() {
			return killResult, true, emitConfirmationEnvelope(ConfirmationEnvelope{
				Action:   "session_kill",
				Target:   sessionName,
				RerunCmd: fmt.Sprintf("agm session kill --confirmed-stuck %s", sessionName),
				Reason:   "session is active; add --confirmed-stuck to force",
			})
		}
		return killResult, true, renderActiveSessionError(sessionName)
	case ops.ErrCodeKillProtected:
		// Agent mode (JSON) is non-interactive: emit a confirmation envelope
		// (exit 4) instead of prompting. Here the bypass flag is --force, since
		// the session was recently (not currently) active.
		if isJSONOutput() {
			return killResult, true, emitConfirmationEnvelope(ConfirmationEnvelope{
				Action:   "session_kill",
				Target:   sessionName,
				RerunCmd: fmt.Sprintf("agm session kill --force %s", sessionName),
				Reason:   "session was recently active; add --force to kill",
			})
		}
		ago := "recently"
		if killResult != nil && killResult.LastActivity != nil {
			ago = fmt.Sprintf("%s ago", time.Since(*killResult.LastActivity).Truncate(time.Second))
		}
		ui.PrintWarning(fmt.Sprintf("Session '%s' was active %s", sessionName, ago))
		confirmed, confirmErr := confirmKillProtected(sessionName)
		if confirmErr != nil || !confirmed {
			fmt.Println("Cancelled")
			return killResult, true, nil //nolint:nilerr // user cancellation is not an error
		}
		newResult, err := ops.KillSession(opCtx, &ops.KillSessionRequest{
			Identifier: sessionName,
			Force:      true,
		})
		if err != nil {
			return newResult, true, err
		}
		return newResult, false, nil
	}
	// Unrecognized op error: still surface as structured JSON when requested.
	if isJSONOutput() {
		return killResult, true, renderKillError(sessionName, opErr.Detail, nil)
	}
	return killResult, true, killErr
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

// startKillSpan starts an OTel span for the session kill operation.
func startKillSpan(ctx context.Context, sessionName string, hard, force bool) (context.Context, trace.Span) {
	mode := "soft"
	if hard {
		mode = "hard"
	}
	return otel.Tracer("agm").Start(ctx, "agm.session.kill",
		trace.WithAttributes(
			attribute.String("session.name", sessionName),
			attribute.String("operation", "kill"),
			attribute.String("kill.mode", mode),
			attribute.Bool("kill.force", force),
		))
}
