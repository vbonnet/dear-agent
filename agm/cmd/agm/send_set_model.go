package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var setModelDryRun bool
var setModelHarness string

var sendSetModelCmd = &cobra.Command{
	Use:   "set-model <session-name> <model>",
	Short: "Change the AI model of a running harness session",
	Long: `Send the harness model-switch command to a running AGM session.

The command resolves the session harness from AGM metadata, then resolves model
aliases through the shared harness model registry. Use --harness when changing
a live tmux session that is not yet associated with AGM.

Examples:
  # Switch a Claude Code session to Opus
  agm send set-model my-session opus-1m

  # Switch a Codex session to its fast model
  agm send set-model codex-worker 5.4-mini

  # Preview an OpenRouter-compatible OpenCode route
  agm send set-model open-worker glm-5.2 --harness=opencode-cli --dry-run

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
	sendSetModelCmd.Flags().StringVar(&setModelHarness, "harness", "", "Explicit harness when the session is not associated with AGM")
	sendGroupCmd.AddCommand(sendSetModelCmd)
}

type setModelInstruction struct {
	Harness       string
	ResolvedModel string
	Command       string
}

// resolveSetModelInstruction validates a model in the context of the target
// harness and returns the tmux command that should be sent to the pane.
func resolveSetModelInstruction(harnessName, modelInput string) (setModelInstruction, error) {
	normalized := agent.NormalizeHarnessName(harnessName)
	if normalized == "" {
		normalized = "claude-code"
	}
	if err := agent.ValidateHarnessName(normalized); err != nil {
		return setModelInstruction{}, err
	}

	alias := strings.ToLower(modelInput)
	if normalized == "claude-code" {
		alias = normalizeClaudeSetModelAlias(alias)
	}
	if err := agent.ValidateModel(normalized, alias); err != nil {
		return setModelInstruction{}, err
	}

	resolved := agent.ResolveModelFullName(normalized, alias)
	if resolved == "" {
		return setModelInstruction{}, fmt.Errorf("model %q resolved to an empty identifier for harness %q", modelInput, normalized)
	}

	return setModelInstruction{
		Harness:       normalized,
		ResolvedModel: resolved,
		Command:       "/model " + resolved,
	}, nil
}

func normalizeClaudeSetModelAlias(alias string) string {
	switch alias {
	case "default":
		return "sonnet"
	case "sonnet-1m":
		return "sonnet"
	case "opus-1m":
		return "opus"
	default:
		return alias
	}
}

func resolveSetModelHarness(sessionName string) string {
	if setModelHarness != "" {
		return setModelHarness
	}
	m, err := findManifestBySession(sessionName)
	if err != nil || m == nil || m.Harness == "" {
		return "claude-code"
	}
	return m.Harness
}

// verifyModelSet captures pane output and checks for model confirmation.
// Claude Code prints "Set model to ..." when a model change succeeds. Other
// harnesses may not expose a stable confirmation line, so this remains a
// best-effort check for the common slash-command path.
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

	instruction, err := resolveSetModelInstruction(resolveSetModelHarness(sessionName), modelInput)
	if err != nil {
		return fmt.Errorf("invalid model change request: %w", err)
	}

	if setModelDryRun {
		fmt.Printf("Dry-run: would send %q to session '%s' (harness: %s, model: %s)\n",
			instruction.Command, sessionName, instruction.Harness, instruction.ResolvedModel)
		return nil
	}

	// Check tmux session exists
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  - List sessions: agm session list\n  - Create session: agm session new %s", sessionName, sessionName)
	}

	// Send /model command
	if err := tmux.SendSlashCommandSafe(sessionName, instruction.Command); err != nil {
		return fmt.Errorf("failed to send model command: %w", err)
	}

	// Verify model was set
	verified, confirmation := verifyModelSet(sessionName, 5*time.Second)
	if verified {
		ui.PrintSuccess(fmt.Sprintf("Model changed for session '%s': %s", sessionName, confirmation))
	} else {
		ui.PrintWarning(fmt.Sprintf("Sent %q to session '%s' but could not verify confirmation. Attach to verify: agm session attach %s",
			instruction.Command, sessionName, sessionName))
	}

	return nil
}
