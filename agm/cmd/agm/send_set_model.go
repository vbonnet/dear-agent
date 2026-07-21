package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/pisession"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var setModelDryRun bool
var setModelHarness string

var (
	setModelHasSession                  = tmux.HasSession
	setModelCapturePaneOutputContext    = tmux.CapturePaneOutputContext
	setModelSendSlashCommandSafeContext = tmux.SendSlashCommandSafeContext
)

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

	alias := agent.NormalizeModelInput(normalized, modelInput)
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

	command := "/model " + resolved
	if normalized == "pi-cli" {
		command = "/agm-model " + resolved
	}
	return setModelInstruction{
		Harness:       normalized,
		ResolvedModel: resolved,
		Command:       command,
	}, nil
}

func normalizeClaudeSetModelAlias(alias string) string {
	switch strings.ToLower(alias) {
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

// newModelConfirmation returns a confirmation that was not present before the
// command. AGY additionally requires the confirmation to name the exact model
// requested, because persisting a different or stale model would poison cold
// resume provenance.
func newModelConfirmation(instruction setModelInstruction, baseline, current string) (string, bool) {
	prefix := "Set model to "
	if instruction.Harness == "pi-cli" {
		prefix = "AGM model: "
	}
	baselineCounts := make(map[string]int)
	for line := range strings.SplitSeq(baseline, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			baselineCounts[trimmed]++
		}
	}
	currentCounts := make(map[string]int)
	for line := range strings.SplitSeq(current, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		currentCounts[trimmed]++
		if currentCounts[trimmed] <= baselineCounts[trimmed] {
			continue
		}
		if (instruction.Harness == "agy" || instruction.Harness == "pi-cli") && strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)) != instruction.ResolvedModel {
			continue
		}
		return trimmed, true
	}
	return "", false
}

// verifyModelSet captures pane output and checks for a new model confirmation.
// Claude Code prints "Set model to ..." when a model change succeeds. Other
// harnesses may not expose a stable confirmation line, so this remains a
// best-effort check for the common slash-command path.
func verifyModelSet(ctx context.Context, sessionName string, instruction setModelInstruction, baseline string, baselineOK bool, timeout time.Duration) (bool, string, error) {
	if !baselineOK {
		return false, "", nil
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, "", ctx.Err()
		case <-timer.C:
		}
		output, err := setModelCapturePaneOutputContext(ctx, sessionName, 10)
		if err != nil {
			continue
		}
		if confirmation, ok := newModelConfirmation(instruction, baseline, output); ok {
			return true, confirmation, nil
		}
	}
	return false, "", nil
}

// persistAgyModelSwitch records only model provenance AGM can defend. A
// confirmed switch stores the exact resolved public label. An unverified AGY
// switch clears the creation-time override so cold resume retains the saved
// native selection. Pi does the same only after a native transcript exists;
// before Pi's first assistant response there is no saved selection to retain,
// so AGM preserves the configured launch model.
func persistAgyModelSwitch(storage dolt.Storage, m *manifest.Manifest, instruction setModelInstruction, verified bool) error {
	if storage == nil || m == nil {
		return fmt.Errorf("AGY model switch persistence requires session storage and a manifest")
	}
	if instruction.Harness != "agy" && instruction.Harness != "pi-cli" {
		return nil
	}
	if instruction.Harness == "pi-cli" && !verified && m.Pi != nil {
		_, transcriptErr := pisession.FindTranscript(m.Pi.SessionDir, m.Pi.SessionID)
		if errors.Is(transcriptErr, pisession.ErrTranscriptNotFound) {
			return nil
		}
		if transcriptErr != nil {
			return fmt.Errorf("resolve Pi model provenance: %w", transcriptErr)
		}
	}
	if verified {
		m.Model = instruction.ResolvedModel
	} else {
		m.Model = ""
	}
	if err := storage.UpdateSession(m); err != nil {
		return fmt.Errorf("update %s model provenance: %w", instruction.Harness, err)
	}
	return nil
}

func persistAgyModelSwitchForSession(sessionName string, instruction setModelInstruction, verified bool) error {
	if instruction.Harness != "agy" && instruction.Harness != "pi-cli" {
		return nil
	}
	adapter, err := getStorage()
	if err != nil {
		// An explicit --harness supports raw tmux sessions with no AGM record.
		if setModelHarness != "" {
			return nil
		}
		return fmt.Errorf("%s model command was sent but session storage could not be opened: %w", instruction.Harness, err)
	}
	defer func() { _ = adapter.Close() }()
	m, _, err := session.ResolveIdentifier(sessionName, cfg.SessionsDir, adapter)
	if err != nil {
		if setModelHarness != "" {
			return nil
		}
		return fmt.Errorf("%s model command was sent but its session manifest could not be resolved: %w", instruction.Harness, err)
	}
	return persistAgyModelSwitch(adapter, m, instruction, verified)
}

func runSendSetModel(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	modelInput := args[1]
	ctx := context.Background()
	if cmd != nil {
		ctx = cmd.Context()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

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
	exists, err := setModelHasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  - List sessions: agm session list\n  - Create session: agm session new %s", sessionName, sessionName)
	}
	baseline, baselineErr := setModelCapturePaneOutputContext(ctx, sessionName, 10)

	// Send /model command
	if err := setModelSendSlashCommandSafeContext(ctx, sessionName, instruction.Command); err != nil {
		return fmt.Errorf("failed to send model command: %w", err)
	}

	// Verify model was set
	verified, confirmation, err := verifyModelSet(ctx, sessionName, instruction, baseline, baselineErr == nil, 5*time.Second)
	if err != nil {
		// The command was already delivered, so cancellation leaves the runtime
		// selection uncertain. Clear AGY provenance before returning rather than
		// retaining a creation-time override that may now be stale.
		if persistErr := persistAgyModelSwitchForSession(sessionName, instruction, false); persistErr != nil {
			return fmt.Errorf("model verification stopped: %w; additionally failed to clear uncertain provenance: %w", err, persistErr)
		}
		return err
	}
	if err := persistAgyModelSwitchForSession(sessionName, instruction, verified); err != nil {
		return err
	}
	if verified {
		ui.PrintSuccess(fmt.Sprintf("Model changed for session '%s': %s", sessionName, confirmation))
	} else {
		ui.PrintWarning(fmt.Sprintf("Sent %q to session '%s' but could not verify confirmation. Attach to verify: agm session attach %s",
			instruction.Command, sessionName, sessionName))
	}

	return nil
}
