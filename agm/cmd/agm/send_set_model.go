package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var setModelDryRun bool

var sendSetModelCmd = &cobra.Command{
	Use:   "set-model <session-name> <model>",
	Short: "Change the AI model of a running Claude Code session",
	Long: `Send /model command to a running Claude Code session to switch models.

Supported model aliases:
  default, sonnet  → sonnet (latest Sonnet)
  sonnet-1m        → sonnet with 1M context window
  opus             → opus (latest Opus)
  opus-1m          → opus with 1M context window
  haiku            → haiku

Examples:
  # Switch to opus with 1M context
  agm send set-model my-session opus-1m

  # Switch to haiku
  agm send set-model my-session haiku

  # Preview without sending
  agm send set-model my-session opus --dry-run

See Also:
  • agm send mode - Switch permission mode (plan/auto/default)
  • agm send msg  - Send messages to sessions`,
	Args: cobra.ExactArgs(2),
	RunE: runSendSetModel,
}

func init() {
	sendSetModelCmd.Flags().BoolVar(&setModelDryRun, "dry-run", false, "Print command without sending")
	sendGroupCmd.AddCommand(sendSetModelCmd)
}

// modelAliases maps user-friendly names to /model command arguments.
var modelAliases = map[string]string{
	"default":   "sonnet",
	"sonnet":    "sonnet",
	"sonnet-1m": "sonnet[1m]",
	"opus":      "opus",
	"opus-1m":   "opus[1m]",
	"haiku":     "haiku",
}

// resolveModelAlias converts a user-provided alias to a /model argument.
// Returns the resolved model name and true if valid, or empty string and false.
func resolveModelAlias(alias string) (string, bool) {
	resolved, ok := modelAliases[strings.ToLower(alias)]
	return resolved, ok
}

// verifyModelSet captures pane output and checks for model confirmation.
// Claude Code prints "Set model to ..." when a model change succeeds.
func verifyModelSet(sessionName string, timeout time.Duration) (bool, string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		output, err := tmux.CapturePaneOutput(sessionName, 10)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Set model to") {
				return true, trimmed
			}
		}
	}
	return false, ""
}

func runSendSetModel(_ *cobra.Command, args []string) error {
	sessionName := args[0]
	modelInput := args[1]

	// Resolve alias
	modelArg, valid := resolveModelAlias(modelInput)
	if !valid {
		validAliases := make([]string, 0, len(modelAliases))
		for k := range modelAliases {
			validAliases = append(validAliases, k)
		}
		return fmt.Errorf("unknown model %q\n\nSupported models: %s",
			modelInput, strings.Join(validAliases, ", "))
	}

	// Check tmux session exists
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  - List sessions: agm session list\n  - Create session: agm session new %s", sessionName, sessionName)
	}

	command := "/model " + modelArg

	if setModelDryRun {
		fmt.Printf("Dry-run: would send %q to session '%s'\n", command, sessionName)
		return nil
	}

	// Send /model command
	if err := tmux.SendSlashCommandSafe(sessionName, command); err != nil {
		return fmt.Errorf("failed to send model command: %w", err)
	}

	// Verify model was set
	verified, confirmation := verifyModelSet(sessionName, 5*time.Second)
	if verified {
		ui.PrintSuccess(fmt.Sprintf("Model changed for session '%s': %s", sessionName, confirmation))
	} else {
		ui.PrintWarning(fmt.Sprintf("Sent %q to session '%s' but could not verify confirmation. Attach to verify: agm session attach %s",
			command, sessionName, sessionName))
	}

	return nil
}
