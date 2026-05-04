// Package daemon provides background daemon monitoring.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
	"github.com/vbonnet/dear-agent/agm/internal/sentinel/tmux"
	"github.com/vbonnet/dear-agent/pkg/enforcement"
)

// FlagEscalationTimeout is how long to wait for a flag-based interrupt to take
// effect before escalating to tmux key injection (Ctrl-C).
const FlagEscalationTimeout = 60 * time.Second

// RecoveryStrategy defines the approach for recovering a stuck session.
type RecoveryStrategy int

const (
	// RecoveryEscape sends Escape key to clear prompts/dialogs
	RecoveryEscape RecoveryStrategy = iota
	// RecoveryEnter sends Enter key to dismiss permission prompts
	RecoveryEnter
	// RecoveryCtrlC sends Ctrl-C to interrupt current operation
	RecoveryCtrlC
	// RecoveryRestart kills and restarts the tmux session
	RecoveryRestart
	// RecoveryManual logs incident but doesn't attempt automated recovery
	RecoveryManual
	// RecoveryFlagInterrupt writes a flag file that pre-tool hooks check.
	// Non-destructive: doesn't inject keystrokes. Preferred over Ctrl-C.
	RecoveryFlagInterrupt
)

// String returns human-readable strategy name.
func (s RecoveryStrategy) String() string {
	switch s {
	case RecoveryEscape:
		return "escape"
	case RecoveryEnter:
		return "enter"
	case RecoveryCtrlC:
		return "ctrl_c"
	case RecoveryRestart:
		return "restart"
	case RecoveryManual:
		return "manual"
	case RecoveryFlagInterrupt:
		return "flag_interrupt"
	default:
		return "unknown"
	}
}

// ParseStrategy converts string to RecoveryStrategy.
func ParseStrategy(s string) (RecoveryStrategy, error) {
	switch strings.ToLower(s) {
	case "escape":
		return RecoveryEscape, nil
	case "enter":
		return RecoveryEnter, nil
	case "ctrl_c":
		return RecoveryCtrlC, nil
	case "restart":
		return RecoveryRestart, nil
	case "manual":
		return RecoveryManual, nil
	case "flag_interrupt":
		return RecoveryFlagInterrupt, nil
	default:
		return RecoveryManual, fmt.Errorf("unknown recovery strategy: %s", s)
	}
}

// StrategyForSymptom returns the appropriate recovery strategy for a given symptom.
// This replaces the single-strategy approach with symptom-specific recovery:
//   - Zero-token / mustering: ESC (clear spinner, least disruptive)
//   - Permission prompt: Enter (dismiss the prompt)
//   - Permission prompt escalate: Manual (recovery failed, needs human/orchestrator)
//   - Cursor frozen: Ctrl+C (interrupt frozen process)
func StrategyForSymptom(symptom string) RecoveryStrategy {
	switch symptom {
	case "stuck_zero_token_waiting", "stuck_mustering":
		return RecoveryEscape
	case "stuck_permission_prompt":
		return RecoveryEnter
	case "stuck_permission_prompt_escalate":
		return RecoveryManual
	case "cursor_frozen":
		return RecoveryCtrlC
	default:
		return RecoveryEscape
	}
}

// RecoveryResult contains the outcome of a recovery attempt.
type RecoveryResult struct {
	Success      bool             // Whether recovery succeeded
	Strategy     RecoveryStrategy // Strategy that was used
	DurationMs   int64            // How long recovery took (milliseconds)
	Error        error            // Error if recovery failed
	BeforeCursor CursorPosition   // Cursor position before recovery
	AfterCursor  CursorPosition   // Cursor position after recovery (if verified)
}

// CursorPosition represents X,Y coordinates in tmux pane.
type CursorPosition struct {
	X int
	Y int
}

// RecoveryHistory tracks recovery attempts for a session.
type RecoveryHistory struct {
	SessionName      string
	Attempts         []RecoveryAttempt
	LastAttempt      time.Time
	TotalAttempts    int
	MaxAttempts      int           // Circuit breaker threshold
	CooldownDuration time.Duration // Time-based cooldown for circuit breaker reset
}

// RecoveryAttempt records a single recovery attempt.
type RecoveryAttempt struct {
	Timestamp time.Time
	Strategy  RecoveryStrategy
	Success   bool
	Reason    string
}

// NewRecoveryHistory creates a new recovery history tracker.
func NewRecoveryHistory(sessionName string, maxAttempts int, cooldownDuration time.Duration) *RecoveryHistory {
	return &RecoveryHistory{
		SessionName:      sessionName,
		Attempts:         make([]RecoveryAttempt, 0),
		MaxAttempts:      maxAttempts,
		CooldownDuration: cooldownDuration,
	}
}

// CanAttemptRecovery checks if another recovery attempt is allowed.
// Returns false if max attempts reached (circuit breaker).
// Resets after cooldown period if configured.
func (h *RecoveryHistory) CanAttemptRecovery() bool {
	if h.TotalAttempts < h.MaxAttempts {
		return true
	}
	// Time-based cooldown: reset after cooldown period
	if h.CooldownDuration > 0 && !h.LastAttempt.IsZero() && time.Since(h.LastAttempt) > h.CooldownDuration {
		h.TotalAttempts = 0
		h.Attempts = h.Attempts[:0]
		return true
	}
	return false
}

// RecordAttempt logs a recovery attempt.
func (h *RecoveryHistory) RecordAttempt(strategy RecoveryStrategy, success bool, reason string) {
	h.Attempts = append(h.Attempts, RecoveryAttempt{
		Timestamp: time.Now(),
		Strategy:  strategy,
		Success:   success,
		Reason:    reason,
	})
	h.LastAttempt = time.Now()
	h.TotalAttempts++
}

// ApplyRecovery executes the specified recovery strategy on a stuck session.
// Returns RecoveryResult with outcome details.
// If a human is attached or typing, downgrades to RecoveryManual to avoid interference.
//
// For Ctrl-C recovery, this function first attempts a flag-based interrupt
// (non-destructive) and only escalates to tmux key injection after
// FlagEscalationTimeout (60s) has passed without recovery.
func ApplyRecovery(sessionName string, strategy RecoveryStrategy, client *tmux.Client) (*RecoveryResult, error) {
	startTime := time.Now()

	// Safety check: don't send ESC/Ctrl-C/restart if a human is present
	if strategy != RecoveryManual {
		if humanPresent := isHumanPresent(sessionName); humanPresent {
			// Downgrade to manual — log but don't act
			return &RecoveryResult{
				Strategy:   RecoveryManual,
				Success:    false,
				DurationMs: time.Since(startTime).Milliseconds(),
				Error:      fmt.Errorf("safety guard: human detected, downgraded to manual recovery"),
			}, nil
		}
	}

	// For Ctrl-C recovery, prefer flag-based interrupt first.
	// The flag is checked by the pretool hook before each tool call,
	// which is non-destructive (no keystroke injection, no buffer corruption).
	// Only escalate to Ctrl-C after FlagEscalationTimeout.
	if strategy == RecoveryCtrlC {
		flagResult, err := applyFlagInterrupt(sessionName, "sentinel recovery: cursor frozen")
		if err == nil && flagResult.Success {
			return flagResult, nil
		}
		// Flag interrupt didn't work or failed — fall through to Ctrl-C
	}

	// Get cursor position before recovery
	beforePaneInfo, err := client.GetPaneInfo(sessionName)
	beforeCursor := CursorPosition{X: 0, Y: 0}
	if err == nil && beforePaneInfo != nil {
		beforeCursor = CursorPosition{X: beforePaneInfo.CursorX, Y: beforePaneInfo.CursorY}
	}

	result := &RecoveryResult{
		Strategy:     strategy,
		BeforeCursor: beforeCursor,
	}

	// Verify session state via capture-pane before sending recovery keys.
	// Bug fix: recovery keys must not be sent blindly — verify session is
	// actually reachable and in a state that warrants the recovery strategy.
	if strategy != RecoveryRestart && strategy != RecoveryManual && strategy != RecoveryFlagInterrupt {
		_, captureErr := client.GetPaneContent(sessionName)
		if captureErr != nil {
			result.Success = false
			result.Error = fmt.Errorf("capture-pane failed before recovery: %w (session may be down)", captureErr)
			result.DurationMs = time.Since(startTime).Milliseconds()
			return result, captureErr
		}
	}

	var done bool
	done, err = dispatchRecoveryStrategy(strategy, sessionName, client, result, startTime)
	if done {
		return result, nil
	}

	// Record duration
	result.DurationMs = time.Since(startTime).Milliseconds()

	if err != nil {
		result.Success = false
		result.Error = err
		return result, err
	}

	if strategy != RecoveryManual {
		verifyCursorMovedAfterRecovery(client, sessionName, result)
	}
	return result, nil
}

// verifyCursorMovedAfterRecovery sleeps briefly so tmux can process the
// recovery keystroke, then re-reads the cursor position. result.Success is
// set to true when the cursor moved (or when read fails).
func verifyCursorMovedAfterRecovery(client *tmux.Client, sessionName string, result *RecoveryResult) {
	time.Sleep(500 * time.Millisecond)
	afterPaneInfo, err := client.GetPaneInfo(sessionName)
	if err == nil && afterPaneInfo != nil {
		result.AfterCursor = CursorPosition{X: afterPaneInfo.CursorX, Y: afterPaneInfo.CursorY}
		result.Success = result.BeforeCursor.X != result.AfterCursor.X ||
			result.BeforeCursor.Y != result.AfterCursor.Y
		return
	}
	result.Success = true
}

// dispatchRecoveryStrategy executes the per-strategy keystroke or process
// action and returns (done, err). When done is true the caller should return
// result immediately (used for the flag-interrupt early-return cases).
func dispatchRecoveryStrategy(strategy RecoveryStrategy, sessionName string, client *tmux.Client, result *RecoveryResult, startTime time.Time) (bool, error) {
	switch strategy {
	case RecoveryEscape:
		return false, client.SendKeys(sessionName, "Escape")
	case RecoveryEnter:
		return false, client.SendKeys(sessionName, "Enter")
	case RecoveryCtrlC:
		if shouldDeferToFlag(sessionName) {
			result.Strategy = RecoveryFlagInterrupt
			result.Success = false
			result.Error = fmt.Errorf("flag-based interrupt pending, deferring Ctrl-C for up to %s", FlagEscalationTimeout)
			result.DurationMs = time.Since(startTime).Milliseconds()
			return true, nil
		}
		return false, client.SendKeys(sessionName, "C-c")
	case RecoveryFlagInterrupt:
		flagResult, flagErr := applyFlagInterrupt(sessionName, "sentinel recovery")
		if flagErr != nil {
			return false, flagErr
		}
		result.Success = flagResult.Success
		result.DurationMs = flagResult.DurationMs
		return true, nil
	case RecoveryRestart:
		return false, restartSession(sessionName)
	case RecoveryManual:
		result.Success = false
		return false, nil
	default:
		return false, fmt.Errorf("unknown recovery strategy: %d", strategy)
	}
}

// restartSession kills and restarts a tmux session.
// Most aggressive recovery - last resort for completely frozen sessions.
func restartSession(sessionName string) error {
	// Kill session
	killCmd := exec.CommandContext(context.Background(), "tmux", "kill-session", "-t", sessionName)
	if err := killCmd.Run(); err != nil {
		return fmt.Errorf("failed to kill session: %w", err)
	}

	// Note: Restarting session requires AGM integration (not implemented here).
	// This would need to call AGM's session creation logic.
	// For now, we just kill the session and log it.
	return nil
}

// SendRejectionMessage sends a violation rejection message to the session.
// This uses tmux send-keys to inject the message into the session pane.
func SendRejectionMessage(sessionName string, message string, pattern *enforcement.Pattern) error {
	// Create formatted rejection message
	fullMessage := formatRejectionForTmux(message, pattern)

	// Verify session state via capture-pane before sending rejection message.
	// Bug fix: must confirm session is reachable before injecting keys.
	checkCmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p")
	if checkErr := checkCmd.Run(); checkErr != nil {
		return fmt.Errorf("capture-pane failed before sending rejection: %w (session may be down)", checkErr)
	}

	// Send message via tmux send-keys
	// Note: In production, this would use AGM's messaging system instead.
	// For now, we use tmux directly as a fallback.
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, "-l", fullMessage)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send rejection message: %w", err)
	}

	// Send Enter to submit message
	enterCmd := exec.Command("tmux", "send-keys", "-t", sessionName, "Enter")
	return enterCmd.Run()
}

// formatRejectionForTmux formats rejection message for tmux injection.
// Adds filing instructions and resume-work directives.
func formatRejectionForTmux(message string, pattern *enforcement.Pattern) string {
	var formatted strings.Builder

	// Main rejection message
	formatted.WriteString(message)
	formatted.WriteString("\n\n")

	// Filing instructions (matches Python Astrocyte format)
	formatted.WriteString("📋 NEXT STEPS:\n")
	formatted.WriteString("1. File this violation using the Task tool:\n")
	formatted.WriteString("   - Create task: 'File violation: " + pattern.ID + "'\n")
	formatted.WriteString("   - Include command, context, and pattern details\n")
	formatted.WriteString("2. After filing, RESUME YOUR WORK immediately\n")
	formatted.WriteString("3. Do not stop or wait - continue with your task\n\n")

	// Resume work directive (prevents agent stopping)
	formatted.WriteString("⚠️  IMPORTANT: This is an automated notification.\n")
	formatted.WriteString("File the violation and continue working. Do NOT stop your task.\n")

	return formatted.String()
}

// isHumanPresent calls `agm safety check --json` to detect human presence.
// Returns true if a human is attached or typing. Fails open (returns false) if
// the agm binary is unavailable or the check fails.
func isHumanPresent(sessionName string) bool {
	output, err := exec.Command("agm", "safety", "check", sessionName, "--json",
		"--skip-init", "--skip-mid-response").CombinedOutput()
	if err != nil {
		// Try to parse even on exit code 1 (violations found)
		var result safetyCheckResult
		if jsonErr := json.Unmarshal(output, &result); jsonErr == nil {
			if !result.Safe {
				for _, v := range result.Violations {
					if v.Guard == "human_attached" || v.Guard == "human_typing" {
						return true
					}
				}
			}
		}
		return false // fail-open
	}

	var result safetyCheckResult
	if err := json.Unmarshal(output, &result); err != nil {
		return false // fail-open
	}

	if !result.Safe {
		for _, v := range result.Violations {
			if v.Guard == "human_attached" || v.Guard == "human_typing" {
				return true
			}
		}
	}

	return false
}

// VerifyRecovery checks if a session has recovered from stuck state.
// Returns true if session shows signs of activity (cursor movement, new output).
func VerifyRecovery(client *tmux.Client, sessionName string, beforePaneInfo *tmux.PaneInfo) (bool, error) {
	// Get current pane state
	afterPaneInfo, err := client.GetPaneInfo(sessionName)
	if err != nil {
		return false, fmt.Errorf("failed to get pane info: %w", err)
	}

	// Check for cursor movement (indicates session responded)
	if beforePaneInfo.CursorX != afterPaneInfo.CursorX ||
		beforePaneInfo.CursorY != afterPaneInfo.CursorY {
		return true, nil
	}

	// Check for content changes (new output)
	if beforePaneInfo.Content != afterPaneInfo.Content {
		return true, nil
	}

	// Check for cleared stuck indicators
	beforeIndicators := beforePaneInfo.DetectStuckIndicators()
	afterIndicators := afterPaneInfo.DetectStuckIndicators()

	// If permission prompt or mustering is cleared, consider recovered
	if beforeIndicators["permission_prompt"] && !afterIndicators["permission_prompt"] {
		return true, nil
	}
	if beforeIndicators["mustering"] && !afterIndicators["mustering"] {
		return true, nil
	}

	// No signs of recovery detected
	return false, nil
}

// applyFlagInterrupt writes an interrupt flag file for the session.
// The pre-tool hook will pick this up and block subsequent tool calls.
func applyFlagInterrupt(sessionName, reason string) (*RecoveryResult, error) {
	startTime := time.Now()
	dir := interrupt.DefaultDir()

	flag := &interrupt.Flag{
		Type:     interrupt.TypeStop,
		Reason:   reason,
		IssuedBy: "sentinel",
		IssuedAt: time.Now().UTC(),
	}

	if err := interrupt.Write(dir, sessionName, flag); err != nil {
		return &RecoveryResult{
			Strategy:   RecoveryFlagInterrupt,
			Success:    false,
			DurationMs: time.Since(startTime).Milliseconds(),
			Error:      err,
		}, err
	}

	return &RecoveryResult{
		Strategy:   RecoveryFlagInterrupt,
		Success:    true,
		DurationMs: time.Since(startTime).Milliseconds(),
	}, nil
}

// shouldDeferToFlag checks if a flag-based interrupt is still pending and
// young enough that we should wait rather than escalating to Ctrl-C.
func shouldDeferToFlag(sessionName string) bool {
	dir := interrupt.DefaultDir()
	flag, err := interrupt.Read(dir, sessionName)
	if err != nil || flag == nil {
		return false
	}
	return time.Since(flag.IssuedAt) < FlagEscalationTimeout
}
