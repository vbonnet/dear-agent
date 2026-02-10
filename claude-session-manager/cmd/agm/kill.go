package main

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var forceKill bool

var killCmd = &cobra.Command{
	Use:   "kill [session-name]",
	Short: "Kill tmux session for a CSM session",
	Long: `Kill tmux session for a CSM session without archiving.

This command terminates the tmux session immediately while preserving
session metadata. The session can be resumed later with 'csm resume'.

Use this when:
  • Tmux session is stuck or unresponsive
  • Terminal crashed but session still running
  • Need to force-stop without archiving

Note: This does NOT archive the session. Use 'csm exit' for graceful
shutdown with archiving.

Examples:
  # Kill with confirmation prompt
  csm kill my-session

  # Kill without confirmation (for scripts)
  csm kill my-session --force

  # Resume session after killing
  csm resume my-session`,
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

	// Step 3: Confirm action (unless --force)
	if !forceKill {
		confirmed, err := confirmKill(sessionName, m.Tmux.SessionName)
		if err != nil || !confirmed {
			fmt.Println("Cancelled")
			return nil
		}
	}

	// Step 4: Kill tmux session (idempotent)
	err = killTmuxSession(m.Tmux.SessionName)
	if err != nil {
		return renderKillError(sessionName, err)
	}

	// Step 5: Success message
	renderSuccessMessage(sessionName)
	return nil
}

func confirmKill(sessionName, tmuxName string) (bool, error) {
	var confirmed bool

	description := fmt.Sprintf(`Session: %s
Tmux session: %s

This will terminate the tmux process immediately.
Session data will be preserved and can be resumed later.

Resume with: csm resume %s`, sessionName, tmuxName, sessionName)

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
		`• List all sessions: csm list
• Create new session: csm new <name>`,
	)
	return fmt.Errorf("session not found")
}

func renderSessionArchivedError(sessionName string) error {
	ui.PrintError(
		fmt.Errorf("session is archived"),
		fmt.Sprintf("Cannot kill archived session '%s'", sessionName),
		fmt.Sprintf(`Archived sessions don't have active tmux processes.

To work with this session:
  1. Resume it: csm resume %s
  2. Then kill if needed: csm kill %s`, sessionName, sessionName),
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
	fmt.Printf("    csm resume %s\n", sessionName)
}
