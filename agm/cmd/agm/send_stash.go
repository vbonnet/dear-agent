package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

var sendStashCmd = &cobra.Command{
	Use:   "stash <session-name>",
	Short: "Preserve and clear the current harness input message",
	Long: `Preserve and clear the current input message before automated delivery.

This preserves any human-typed text in the input line while clearing it,
allowing AGM message delivery to proceed. The stashed message is automatically
restored on the next user interaction when the harness supports native stash.
Claude Code has a verified native mapping. Codex CLI, AGY, and OpenCode use the
declared best-effort preservation fallback until their native mappings are
verified.

Use this before sending a message to a session that has human text in the
input line — it saves the text instead of discarding it.

Note: Unstash happens automatically when the user next interacts with the
session. No manual unstash is needed.

Examples:
  agm send stash my-session`,
	Args: cobra.ExactArgs(1),
	RunE: runSendStash,
}

func init() {
	sendGroupCmd.AddCommand(sendStashCmd)
}

func runSendStash(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Step 1: Capture pane to verify there's content to stash
	paneContent, err := tmux.CapturePaneOutput(sessionName, 50)
	if err != nil {
		return fmt.Errorf("failed to capture pane for session '%s': %w", sessionName, err)
	}

	hasQueued := false
	inputType, _ := tmux.ClassifyQueuedInput(paneContent)
	if inputType != tmux.QueuedInputNone {
		hasQueued = true
	}
	hasTyped := tmux.InputLineHasContent(paneContent)

	if !hasQueued && !hasTyped {
		_, _ = fmt.Fprintf(os.Stdout, "Input already empty in session '%s' — nothing to stash\n", sessionName)
		return nil
	}

	harness := "claude-code"
	if m, findErr := findManifestBySession(sessionName); findErr == nil && m != nil && m.Harness != "" {
		harness = m.Harness
	}
	key, verified := tmux.StashKeyForHarness(harness)
	if !verified {
		fmt.Fprintf(os.Stderr, "Warning: harness '%s' uses the best-effort input preservation fallback (%s)\n", harness, key)
	}

	// Step 2: Send the harness-specific preservation key.
	if err := tmux.SendKeys(sessionName, key); err != nil {
		return fmt.Errorf("failed to send preservation key %s to session '%s': %w", key, sessionName, err)
	}

	// Step 3: Verify input was cleared (stash should clear the input line)
	time.Sleep(500 * time.Millisecond)

	afterContent, err := tmux.CapturePaneOutput(sessionName, 50)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not verify stash worked: %v\n", err)
		_, _ = fmt.Fprintf(os.Stdout, "Stash sent to session '%s' (verification skipped)\n", sessionName)
		return nil
	}

	afterType, _ := tmux.ClassifyQueuedInput(afterContent)
	afterTyped := tmux.InputLineHasContent(afterContent)

	if afterType != tmux.QueuedInputNone || afterTyped {
		fmt.Fprintf(os.Stderr, "Warning: input may not have been preserved in session '%s' — %s is not verified for harness '%s'\n", sessionName, key, harness)
		return nil
	}

	_, _ = fmt.Fprintf(os.Stdout, "Message stashed in session '%s'\n", sessionName)
	_, _ = fmt.Fprintf(os.Stdout, "Note: stashed message will be restored automatically on next send\n")

	if os.Getenv("AGM_DEBUG") == "1" {
		_, _ = fmt.Fprintf(os.Stdout, "DEBUG: send-stash session=%s\n", sessionName)
	}

	return nil
}
