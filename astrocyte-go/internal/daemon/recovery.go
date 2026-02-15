package daemon

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/ai-tools/astrocyte/internal/tmux"
	"github.com/vbonnet/ai-tools/astrocyte/pkg/enforcement"
)

// RecoveryStrategy defines the approach for recovering a stuck session.
type RecoveryStrategy int

const (
	// RecoveryEscape sends Escape key to clear prompts/dialogs
	RecoveryEscape RecoveryStrategy = iota
	// RecoveryCtrlC sends Ctrl-C to interrupt current operation
	RecoveryCtrlC
	// RecoveryRestart kills and restarts the tmux session
	RecoveryRestart
	// RecoveryManual logs incident but doesn't attempt automated recovery
	RecoveryManual
)

// String returns human-readable strategy name.
func (s RecoveryStrategy) String() string {
	switch s {
	case RecoveryEscape:
		return "escape"
	case RecoveryCtrlC:
		return "ctrl_c"
	case RecoveryRestart:
		return "restart"
	case RecoveryManual:
		return "manual"
	default:
		return "unknown"
	}
}

// ParseStrategy converts string to RecoveryStrategy.
func ParseStrategy(s string) (RecoveryStrategy, error) {
	switch strings.ToLower(s) {
	case "escape":
		return RecoveryEscape, nil
	case "ctrl_c":
		return RecoveryCtrlC, nil
	case "restart":
		return RecoveryRestart, nil
	case "manual":
		return RecoveryManual, nil
	default:
		return RecoveryManual, fmt.Errorf("unknown recovery strategy: %s", s)
	}
}

// RecoveryResult contains the outcome of a recovery attempt.
type RecoveryResult struct {
	Success       bool              // Whether recovery succeeded
	Strategy      RecoveryStrategy  // Strategy that was used
	DurationMs    int64             // How long recovery took (milliseconds)
	Error         error             // Error if recovery failed
	BeforeCursor  CursorPosition    // Cursor position before recovery
	AfterCursor   CursorPosition    // Cursor position after recovery (if verified)
}

// CursorPosition represents X,Y coordinates in tmux pane.
type CursorPosition struct {
	X int
	Y int
}

// RecoveryHistory tracks recovery attempts for a session.
type RecoveryHistory struct {
	SessionName    string
	Attempts       []RecoveryAttempt
	LastAttempt    time.Time
	TotalAttempts  int
	MaxAttempts    int // Circuit breaker threshold
}

// RecoveryAttempt records a single recovery attempt.
type RecoveryAttempt struct {
	Timestamp time.Time
	Strategy  RecoveryStrategy
	Success   bool
	Reason    string
}

// NewRecoveryHistory creates a new recovery history tracker.
func NewRecoveryHistory(sessionName string, maxAttempts int) *RecoveryHistory {
	return &RecoveryHistory{
		SessionName: sessionName,
		Attempts:    make([]RecoveryAttempt, 0),
		MaxAttempts: maxAttempts,
	}
}

// CanAttemptRecovery checks if another recovery attempt is allowed.
// Returns false if max attempts reached (circuit breaker).
func (h *RecoveryHistory) CanAttemptRecovery() bool {
	return h.TotalAttempts < h.MaxAttempts
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
func ApplyRecovery(sessionName string, strategy RecoveryStrategy, client *tmux.Client) (*RecoveryResult, error) {
	startTime := time.Now()

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

	// Execute recovery based on strategy
	switch strategy {
	case RecoveryEscape:
		err = sendEscapeKey(sessionName)
	case RecoveryCtrlC:
		err = sendCtrlC(sessionName)
	case RecoveryRestart:
		err = restartSession(sessionName)
	case RecoveryManual:
		// Manual strategy doesn't perform automated recovery
		err = nil
		result.Success = false
	default:
		err = fmt.Errorf("unknown recovery strategy: %d", strategy)
	}

	// Record duration
	result.DurationMs = time.Since(startTime).Milliseconds()

	if err != nil {
		result.Success = false
		result.Error = err
		return result, err
	}

	// For automated strategies, verify recovery by checking cursor movement
	if strategy != RecoveryManual {
		// Wait brief moment for tmux to process keys
		time.Sleep(500 * time.Millisecond)

		// Get cursor position after recovery
		afterPaneInfo, err := client.GetPaneInfo(sessionName)
		if err == nil && afterPaneInfo != nil {
			result.AfterCursor = CursorPosition{X: afterPaneInfo.CursorX, Y: afterPaneInfo.CursorY}
			// Success if cursor moved (indicates session responded)
			result.Success = (result.BeforeCursor.X != result.AfterCursor.X ||
				result.BeforeCursor.Y != result.AfterCursor.Y)
		} else {
			// Couldn't verify, assume success if no error occurred
			result.Success = true
		}
	}

	return result, nil
}

// sendEscapeKey sends Escape key to session.
// This is the safest recovery method - clears dialogs/prompts without interrupting work.
func sendEscapeKey(sessionName string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, "Escape")
	return cmd.Run()
}

// sendCtrlC sends Ctrl-C to session.
// More aggressive than Escape - interrupts current operation.
func sendCtrlC(sessionName string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, "C-c")
	return cmd.Run()
}

// restartSession kills and restarts a tmux session.
// Most aggressive recovery - last resort for completely frozen sessions.
func restartSession(sessionName string) error {
	// Kill session
	killCmd := exec.Command("tmux", "kill-session", "-t", sessionName)
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
