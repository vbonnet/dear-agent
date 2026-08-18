package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var modeDryRun bool

var sendModeCmd = &cobra.Command{
	Use:   "mode <mode> <session-name>",
	Short: "Switch AI harness mode (plan, auto, default)",
	Long: `Switch the permission mode of a running AI harness session.

Supported modes:
  plan     Planning mode - AI explains what it would do without executing
  auto     Auto-approve mode - AI executes without permission prompts
  default  Default mode - AI asks for permission before executing

Harness-specific behavior:
  claude-code   Uses /plan slash command or Shift+Tab cycling
  gemini-cli    Uses /plan or Ctrl+Y toggle for auto mode
  opencode-cli  Uses Tab to cycle between plan and default
  codex-cli     Reports an explicit restart configuration fallback
  agy           Reports an explicit restart flag fallback
  pi-cli        Uses the managed /agm-mode command and native tool interception

Examples:
  # Switch to plan mode
  agm send mode plan my-session

  # Switch to auto mode
  agm send mode auto my-session

  # Switch back to default mode
  agm send mode default my-session

  # Preview what would be sent
  agm send mode plan my-session --dry-run

See Also:
  • agm send approve - Approve permission prompts
  • agm send reject  - Reject permission prompts
  • agm send msg     - Send messages to sessions`,
	Args: cobra.ExactArgs(2),
	RunE: runSendMode,
}

func init() {
	sendModeCmd.Flags().BoolVar(&modeDryRun, "dry-run", false, "Print key sequences without sending")
	sendGroupCmd.AddCommand(sendModeCmd)
}

// validModes is the set of accepted mode values.
var validModes = map[string]bool{
	"plan":    true,
	"auto":    true,
	"default": true,
}

var sendPiModeCommand = tmux.SendSlashCommandSafeContext

// calculateShiftTabPresses computes how many Shift+Tab presses are needed
// to cycle from currentMode to targetMode in Claude Code.
// Cycle order: default(0) -> auto(1) -> plan(2) -> default(0)
func calculateShiftTabPresses(currentMode, targetMode string) int {
	modeIndex := map[string]int{"default": 0, "auto": 1, "plan": 2}
	currentIdx, ok := modeIndex[currentMode]
	if !ok {
		currentIdx = 0 // unknown defaults to 0
	}
	targetIdx := modeIndex[targetMode]
	return (targetIdx - currentIdx + 3) % 3
}

// modeSessionInfo holds resolved session metadata for mode switching.
type modeSessionInfo struct {
	harness     string
	currentMode string
	adapter     *dolt.Adapter
}

// loadModeSessionInfo loads the harness type and current permission mode from the session manifest.
// Returns defaults (claude-code, default) if manifest can't be loaded.
func loadModeSessionInfo(sessionName string) *modeSessionInfo {
	info := &modeSessionInfo{harness: "claude-code", currentMode: "default"}

	adapter, err := getStorage()
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not connect to storage: %v (proceeding with defaults)", err))
		return info
	}
	info.adapter = adapter

	m, _, err := session.ResolveIdentifier(sessionName, cfg.SessionsDir, adapter)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not resolve session manifest: %v (proceeding with defaults)", err))
		return info
	}
	if m.Harness != "" {
		info.harness = m.Harness
	}
	if m.PermissionMode != "" {
		info.currentMode = m.PermissionMode
	}
	return info
}

// updateModeManifest writes the new permission mode to the session manifest.
// source indicates how the mode was set (e.g. "manual", "creation").
func updateModeManifest(adapter *dolt.Adapter, sessionName, targetMode, source string) {
	if adapter == nil {
		return
	}
	m, _, err := session.ResolveIdentifier(sessionName, cfg.SessionsDir, adapter)
	if err != nil {
		return
	}
	now := time.Now()
	m.PermissionMode = targetMode
	m.PermissionModeSource = source
	m.PermissionModeUpdatedAt = &now
	m.UpdatedAt = now
	if updateErr := adapter.UpdateSession(m); updateErr != nil {
		ui.PrintWarning(fmt.Sprintf("Mode switched but failed to update manifest: %v", updateErr))
	}
}

// dispatchModeSwitch sends the appropriate key sequence for the given harness.
func dispatchModeSwitch(harness, sessionName, targetMode, currentMode string) error {
	return dispatchModeSwitchContext(context.Background(), harness, sessionName, targetMode, currentMode)
}

func dispatchModeSwitchContext(ctx context.Context, harness, sessionName, targetMode, currentMode string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch harness {
	case "claude-code":
		return sendModeClaudeCodeContext(ctx, sessionName, targetMode, currentMode)
	case "gemini-cli":
		return sendModeGeminiCLIContext(ctx, sessionName, targetMode, currentMode)
	case "opencode-cli":
		return sendModeOpenCodeContext(ctx, sessionName, targetMode, currentMode)
	case "codex-cli":
		return sendModeRestartFallback("codex-cli", targetMode)
	case "agy":
		return sendModeRestartFallback("agy", targetMode)
	case "pi-cli":
		return sendModePiContext(ctx, sessionName, targetMode)
	default:
		return fmt.Errorf("unsupported harness %q for mode switching", harness)
	}
}

func runSendMode(cmd *cobra.Command, args []string) error {
	targetMode := args[0]
	sessionName := args[1]

	if !validModes[targetMode] {
		return fmt.Errorf("invalid mode %q: must be one of plan, auto, default", targetMode)
	}

	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux.\n\nSuggestions:\n  - List sessions: agm session list\n  - Create session: agm session new %s", sessionName, sessionName)
	}

	info := loadModeSessionInfo(sessionName)
	if info.adapter != nil {
		defer func() { _ = info.adapter.Close() }()
	}

	if info.currentMode == targetMode {
		ui.PrintSuccess(fmt.Sprintf("Session '%s' is already in %s mode", sessionName, targetMode))
		return nil
	}

	if modeDryRun {
		fmt.Printf("Dry-run: would switch session '%s' from %s to %s mode (harness: %s)\n",
			sessionName, info.currentMode, targetMode, info.harness)
		printDryRunDetails(info.harness, targetMode, info.currentMode, sessionName)
		return nil
	}

	// Verify session state via capture-pane before sending mode-switching keys.
	// Bug fix: mode keys (S-Tab, C-y, Tab) are only valid when session is at
	// an idle prompt. Sending them blindly into a busy or permission-dialog
	// state can corrupt the session.
	canReceive := session.CheckSessionDelivery(sessionName)
	if canReceive != state.CanReceiveYes {
		return fmt.Errorf("session '%s' is not at idle prompt (state: %s) — cannot switch mode; wait for session to become idle", sessionName, canReceive)
	}

	if err := dispatchModeSwitchContext(cmd.Context(), info.harness, sessionName, targetMode, info.currentMode); err != nil {
		return err
	}

	updateModeManifest(info.adapter, sessionName, targetMode, "manual")
	ui.PrintSuccess(fmt.Sprintf("Switched session '%s' from %s to %s mode", sessionName, info.currentMode, targetMode))
	return nil
}

func printDryRunDetails(harness, targetMode, currentMode, sessionName string) {
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
	socketPath := tmux.GetSocketPath()

	switch harness {
	case "claude-code":
		if targetMode == "plan" {
			fmt.Printf("  Would send: /plan slash command\n")
		} else {
			presses := calculateShiftTabPresses(currentMode, targetMode)
			fmt.Printf("  Would send: %d x Shift+Tab via tmux -S %s send-keys -t %s S-Tab\n",
				presses, socketPath, normalizedName)
		}
	case "gemini-cli":
		switch {
		case targetMode == "plan":
			fmt.Printf("  Would send: /plan slash command\n")
		case targetMode == "auto":
			fmt.Printf("  Would send: Ctrl+Y via tmux -S %s send-keys -t %s C-y\n",
				socketPath, normalizedName)
		case currentMode == "auto":
			fmt.Printf("  Would send: Ctrl+Y via tmux -S %s send-keys -t %s C-y\n",
				socketPath, normalizedName)
		}
	case "opencode-cli":
		fmt.Printf("  Would send: Tab via tmux -S %s send-keys -t %s Tab\n",
			socketPath, normalizedName)
	case "codex-cli":
		fmt.Printf("  Fallback: %s\n", modeRestartInstruction("codex-cli", targetMode))
	case "agy":
		fmt.Printf("  Fallback: %s\n", modeRestartInstruction("agy", targetMode))
	case "pi-cli":
		fmt.Printf("  Would send: /agm-mode %s\n", targetMode)
	}
}

func sendModePiContext(ctx context.Context, sessionName, targetMode string) error {
	if err := sendPiModeCommand(ctx, sessionName, "/agm-mode "+targetMode); err != nil {
		return fmt.Errorf("failed to switch Pi mode: %w", err)
	}
	return nil
}

func sendModeClaudeCode(sessionName, targetMode, currentMode string) error {
	return sendModeClaudeCodeContext(context.Background(), sessionName, targetMode, currentMode)
}

func sendModeClaudeCodeContext(ctx context.Context, sessionName, targetMode, currentMode string) error {
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)

	if targetMode == "plan" {
		// Use /plan slash command - direct and reliable
		if err := tmux.SendSlashCommandSafeContext(ctx, sessionName, "/plan"); err != nil {
			return fmt.Errorf("failed to send /plan command: %w", err)
		}
		return nil
	}

	// For auto/default: use Shift+Tab cycling
	// NOTE: auto mode only appears in the cycle if --enable-auto-mode was passed at startup.
	// AGM always passes this flag when starting Claude Code sessions.
	presses := calculateShiftTabPresses(currentMode, targetMode)
	if presses == 0 {
		return nil // already in target mode
	}

	for i := 0; i < presses; i++ {
		if err := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "S-Tab").Run(); err != nil {
			return fmt.Errorf("failed to send Shift+Tab: %w", err)
		}
		timer := time.NewTimer(300 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	return nil
}

func sendModeGeminiCLI(sessionName, targetMode, currentMode string) error {
	return sendModeGeminiCLIContext(context.Background(), sessionName, targetMode, currentMode)
}

func sendModeGeminiCLIContext(ctx context.Context, sessionName, targetMode, currentMode string) error {
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)

	if targetMode == "plan" {
		if err := tmux.SendSlashCommandSafeContext(ctx, sessionName, "/plan"); err != nil {
			return fmt.Errorf("failed to send /plan command: %w", err)
		}
		return nil
	}

	// auto toggle via Ctrl+Y
	if targetMode == "auto" && currentMode != "auto" {
		if err := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "C-y").Run(); err != nil {
			return fmt.Errorf("failed to send Ctrl+Y: %w", err)
		}
		return nil
	}

	// default: if currently auto, toggle off with Ctrl+Y
	if targetMode == "default" && currentMode == "auto" {
		if err := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "C-y").Run(); err != nil {
			return fmt.Errorf("failed to send Ctrl+Y: %w", err)
		}
		return nil
	}

	return nil
}

func sendModeOpenCode(sessionName, targetMode, currentMode string) error {
	return sendModeOpenCodeContext(context.Background(), sessionName, targetMode, currentMode)
}

func sendModeOpenCodeContext(ctx context.Context, sessionName, targetMode, currentMode string) error {
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)

	// OpenCode cycles: default/build <-> plan with Tab
	needsTab := false
	if targetMode == "plan" && currentMode != "plan" {
		needsTab = true
	} else if targetMode != "plan" && currentMode == "plan" {
		needsTab = true
	}

	if needsTab {
		if err := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "Tab").Run(); err != nil {
			return fmt.Errorf("failed to send Tab: %w", err)
		}
	}

	return nil
}

func sendModeRestartFallback(harness, targetMode string) error {
	return fmt.Errorf("%s does not support verified in-session mode switching; %s", harness, modeRestartInstruction(harness, targetMode))
}

func modeRestartInstruction(harness, targetMode string) string {
	switch harness {
	case "codex-cli":
		switch targetMode {
		case "plan":
			return "restart Codex with read-only sandboxing and untrusted approvals"
		case "auto":
			return "restart with codex -a never -s workspace-write"
		default:
			return "restart with codex -a on-request -s workspace-write"
		}
	case "agy":
		switch targetMode {
		case "plan":
			return "restart with agy --mode plan"
		case "auto":
			return "restart with agy --dangerously-skip-permissions"
		default:
			return "restart with agy using its default permission mode"
		}
	default:
		return "restart the harness with the requested startup permission mode"
	}
}
