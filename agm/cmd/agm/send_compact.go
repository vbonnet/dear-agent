package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/compaction"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	compactFocus string
	compactVerify bool
	compactDryRun bool
	compactForce bool
)

var sendCompactCmd = &cobra.Command{
	Use:   "compact <session-name>",
	Short: "Trigger /compact in a target session with safety checks",
	Long: `Send the /compact slash command to a running session with pre-flight checks,
anti-loop safety, auto-generated prompts, and an audit trail.

Features:
  - Pre-flight checks: verifies session is at prompt and not already compacting
  - Anti-loop safety: 2-hour cooldown, max 3 compactions per session lifetime
  - Auto-generated prompt: includes session context for preservation
  - Audit trail: saves each prompt to ~/.agm/compaction-prompts/

Examples:
  # Trigger compaction with auto-generated prompt
  agm send compact my-session

  # Compact with custom preservation instructions
  agm send compact my-session --focus "preserve context about auth refactor"

  # Preview the prompt without sending
  agm send compact my-session --dry-run

  # Send and verify completion
  agm send compact my-session --verify

  # Override safety limits
  agm send compact my-session --force

See Also:
  • agm session compact  - Higher-level compact with Dolt resolution and monitoring
  • agm send msg         - Send messages to sessions`,
	Args: cobra.ExactArgs(1),
	RunE: runSendCompact,
}

func init() {
	sendCompactCmd.Flags().StringVar(&compactFocus, "focus", "", "Custom preservation instructions appended to compaction prompt")
	sendCompactCmd.Flags().BoolVar(&compactVerify, "verify", false, "Poll session state every 10s until compaction completes")
	sendCompactCmd.Flags().BoolVar(&compactDryRun, "dry-run", false, "Output the compaction prompt without sending")
	sendCompactCmd.Flags().BoolVar(&compactForce, "force", false, "Override anti-loop safety (cooldown and max compactions)")
	sendGroupCmd.AddCommand(sendCompactCmd)
}

// buildCompactCommand constructs the /compact command string with optional args.
// Preserved for backward compatibility with session_compact.go.
func buildCompactCommand(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return "/compact"
	}
	return "/compact " + args
}

// agmBaseDir returns ~/.agm, or AGM_HOME if set.
func agmBaseDir() string {
	if d := os.Getenv("AGM_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agm")
}

func runSendCompact(_ *cobra.Command, args []string) error {
	sessionName := args[0]

	// Check tmux session exists
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  - List sessions: agm session list\n  - Create session: agm session new %s", sessionName, sessionName)
	}

	// Detect current state
	currentState, err := session.DetectState(sessionName)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not detect session state: %v (proceeding with checks skipped)", err))
		currentState = manifest.StateDone // assume ready if detection fails
	}

	// Load anti-loop state
	baseDir := agmBaseDir()
	compState, err := compaction.LoadState(baseDir, sessionName)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not load compaction state: %v (proceeding without anti-loop checks)", err))
		compState = &compaction.CompactionState{SessionName: sessionName}
	}

	// Pre-flight checks
	preflight := compaction.RunPreflight(currentState, compState, compactForce)
	for _, w := range preflight.Warnings {
		ui.PrintWarning(w)
	}
	if !preflight.OK {
		return fmt.Errorf("pre-flight checks failed:\n  %s", strings.Join(preflight.Errors, "\n  "))
	}

	// Build prompt from state file (primary) or Dolt manifest (fallback)
	var command string
	sessionState, stateFilePath, stateErr := compaction.LoadSessionState(baseDir, sessionName)
	if stateErr != nil {
		// No state file found
		if compactFocus == "" {
			// No state file and no --focus: error out
			return fmt.Errorf("no state file found for session '%s'.\n\nUse --focus to provide preservation instructions:\n  agm send compact %s --focus \"preserve context about ...\"", sessionName, sessionName)
		}
		// Fall back to Dolt-enriched prompt with --focus text
		promptInput := &compaction.PromptInput{
			SessionName: sessionName,
			FocusText:   compactFocus,
		}
		enrichPromptFromDolt(promptInput, sessionName)
		command = compaction.GeneratePrompt(promptInput)
	} else {
		// Auto-generate PRESERVE prompt from state file
		command = compaction.GeneratePreservePrompt(sessionState, stateFilePath, compactFocus, sessionName)
	}

	// Determine next prompt number and save audit trail
	promptNum, err := compaction.NextPromptNumber(baseDir, sessionName)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not determine prompt number: %v", err))
		promptNum = 1
	}
	promptFile, err := compaction.SavePrompt(baseDir, sessionName, promptNum, command)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not save prompt audit trail: %v", err))
		promptFile = "(unsaved)"
	}

	// Dry-run: output prompt and exit
	if compactDryRun {
		fmt.Printf("=== Dry Run: Compaction Prompt ===\n\n%s\n\n=== Saved to: %s ===\n", command, promptFile)
		return nil
	}

	// Send via tmux
	if err := tmux.SendSlashCommandSafe(sessionName, command); err != nil {
		return fmt.Errorf("failed to send compact command: %w", err)
	}

	// Update compaction state
	compaction.RecordCompaction(compState, promptFile, compactForce)
	if err := compaction.SaveState(baseDir, compState); err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not save compaction state: %v", err))
	}

	ui.PrintSuccess(fmt.Sprintf("Sent compaction to session '%s' (prompt saved: %s)", sessionName, promptFile))

	// Verify completion if requested
	if compactVerify {
		fmt.Println()
		return verifyCompaction(sessionName, 5*time.Minute)
	}

	return nil
}

// enrichPromptFromDolt tries to load manifest data to enrich the prompt.
func enrichPromptFromDolt(input *compaction.PromptInput, sessionName string) {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return // graceful degradation
	}
	defer cleanup()

	result, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: sessionName,
	})
	if opErr != nil {
		return
	}

	s := result.Session
	input.Project = s.Project
	input.Purpose = s.Purpose
	input.Harness = s.Harness
	if s.Tags != nil {
		input.Tags = s.Tags
	}
}

// verifyCompaction polls session state every 10s until DONE or timeout.
func verifyCompaction(sessionName string, timeout time.Duration) error {
	const pollInterval = 10 * time.Second

	start := time.Now()
	deadline := start.Add(timeout)
	lastState := ""

	fmt.Printf("Verifying compaction completion (polling every %s, timeout %s)...\n", pollInterval, timeout)

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		state, err := session.DetectState(sessionName)
		if err != nil {
			continue
		}

		elapsed := time.Since(start).Round(time.Second)

		if state != lastState {
			fmt.Printf("  [%s] State: %s\n", elapsed, state)
			lastState = state
		}

		if state == manifest.StateDone {
			ui.PrintSuccess(fmt.Sprintf("Compaction verified complete in %s", elapsed))
			return nil
		}
	}

	elapsed := time.Since(start).Round(time.Second)
	ui.PrintWarning(fmt.Sprintf("Verification timed out after %s. Compaction may still be running.", elapsed))
	return nil
}
