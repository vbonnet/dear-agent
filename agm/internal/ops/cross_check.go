package ops

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// CrossCheckState represents the detected state of a session from tmux capture.
type CrossCheckState int

const (
	// StateHealthy indicates the session is operating normally.
	StateHealthy CrossCheckState = iota
	// StateDown indicates the tmux session no longer exists.
	StateDown
	// StateStuck indicates a permission prompt has been visible for too long.
	StateStuck
	// StateNotLooping indicates a supervisor session has no scan output recently.
	StateNotLooping
	// StateEnterBug indicates an unsent [From: message is stuck in the input buffer.
	StateEnterBug
)

// String returns the string representation of CrossCheckState.
func (s CrossCheckState) String() string {
	switch s {
	case StateHealthy:
		return "HEALTHY"
	case StateDown:
		return "DOWN"
	case StateStuck:
		return "STUCK"
	case StateNotLooping:
		return "NOT_LOOPING"
	case StateEnterBug:
		return "ENTER_BUG"
	default:
		return "UNKNOWN"
	}
}

// CrossCheckResult holds the result of a cross-check for a single session.
type CrossCheckResult struct {
	SessionName string          `json:"session_name"`
	State       CrossCheckState `json:"state"`
	StateStr    string          `json:"state_str"`
	Detail      string          `json:"detail,omitempty"`
	Action      string          `json:"action_taken,omitempty"`
}

// CrossCheckReport holds the full cross-check report.
type CrossCheckReport struct {
	Timestamp         time.Time          `json:"timestamp"`
	Results           []CrossCheckResult `json:"results"`
	UnmanagedSessions []string           `json:"unmanaged_sessions,omitempty"`
	Errors            []string           `json:"errors,omitempty"`
}

// Default RBAC allowlist for auto-approve. These tool patterns are safe to approve.
var defaultRBACAllowlist = []string{
	"Read",
	"Glob",
	"Grep",
	"Bash(git",
	"Bash(go test",
	"Bash(go build",
	"Bash(go vet",
	"Bash(ls",
	"Write",
	"Edit",
}

// Well-known tmux sessions that are not AGM-managed.
var wellKnownNonAGMSessions = map[string]bool{
	"main":    true,
	"default": true,
}

// PermissionPromptIndicators are strings that indicate a permission prompt is visible.
var PermissionPromptIndicators = []string{
	"Allow this action?",
	"Do you want to allow",
	"Allow tool",
	"Press Enter to allow",
	"Allow once",
	"Allow for this session",
	"(Y)es",
	"approve this",
}

// CrossCheckConfig holds configuration for cross-check operations.
type CrossCheckConfig struct {
	StuckTimeout   time.Duration // How long a permission prompt must be visible to be STUCK
	ScanGapTimeout time.Duration // How long without scan output before NOT_LOOPING
	RBACAllowlist  []string      // Tool patterns allowed for auto-approve
	DryRun         bool          // If true, don't take actions
	CallerSession  string        // Name of the session running the cross-check (excluded from targets)
}

// DefaultCrossCheckConfig returns the default configuration from SLO contracts.
func DefaultCrossCheckConfig() *CrossCheckConfig {
	slo := contracts.Load()
	return &CrossCheckConfig{
		StuckTimeout:   slo.ScanLoop.StuckTimeout.Duration,
		ScanGapTimeout: slo.ScanLoop.ScanGapTimeout.Duration,
		RBACAllowlist:  defaultRBACAllowlist,
		DryRun:         false,
	}
}

// DetectSessionState analyzes tmux capture-pane output to determine session state.
// This is the ONLY authority for state -- do NOT use agm session list STATE column.
// isScanLoopSession should be true only for sessions running `agm scan --loop`,
// NOT for orchestrator sessions that work interactively.
func DetectSessionState(captureOutput string, isScanLoopSession bool, stateUpdatedAt time.Time, cfg *CrossCheckConfig) CrossCheckState {
	if cfg == nil {
		cfg = DefaultCrossCheckConfig()
	}

	// Check for permission prompt
	if containsPermissionPrompt(captureOutput) {
		elapsed := time.Since(stateUpdatedAt)
		if elapsed >= cfg.StuckTimeout {
			return StateStuck
		}
		return StateHealthy
	}

	// Check for ENTER bug (unsent message in input buffer)
	if HasEnterBug(captureOutput) {
		return StateEnterBug
	}

	// Check for NOT_LOOPING (scan-loop sessions only, not orchestrators)
	if isScanLoopSession {
		if isNotLooping(captureOutput) {
			return StateNotLooping
		}
	}

	return StateHealthy
}

// containsPermissionPrompt checks if the capture output contains a permission prompt.
func containsPermissionPrompt(output string) bool {
	for _, indicator := range PermissionPromptIndicators {
		if strings.Contains(output, indicator) {
			return true
		}
	}
	return false
}

// HasEnterBug checks if the input line (last non-empty line) has an unsent [From: message.
// Returns true if the input line starts with "[From:" (allowing leading whitespace).
// This is the #1 deadlock source -- messages arrive while the session is mid-response,
// get pasted into the input buffer but never sent.
func HasEnterBug(captureOutput string) bool {
	inputLine := ExtractInputLine(captureOutput)
	return IsFromHeader(inputLine)
}

// ExtractInputLine returns the last non-empty line from capture output.
// This represents the input buffer content (what would be sent on Enter).
func ExtractInputLine(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			return lines[i]
		}
	}
	return ""
}

// IsFromHeader checks if a line starts with "[From:" (allowing leading whitespace).
func IsFromHeader(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "[From:")
}

// HasStuckCrossCheckNudge checks if a previous cross-check nudge message is stuck
// in the input buffer. This prevents sending duplicate nudges that compound the
// ENTER bug problem.
func HasStuckCrossCheckNudge(captureOutput string) bool {
	inputLine := ExtractInputLine(captureOutput)
	trimmed := strings.TrimLeft(inputLine, " \t")
	// Check for nudge text stuck directly in input
	if strings.Contains(trimmed, "[cross-check]") {
		return true
	}
	// Check for [From: cross-check ...] wrapper stuck in input
	if IsFromHeader(inputLine) && strings.Contains(inputLine, "cross-check") {
		return true
	}
	return false
}

// isNotLooping checks if a supervisor session has no recent scan output.
// It looks for scan activity markers in the captured output.
func isNotLooping(output string) bool {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "AGM Orchestrator Scan") ||
			strings.Contains(line, "next scan in") ||
			strings.Contains(line, "\"timestamp\"") {
			return false
		}
	}
	return true
}

// MatchesRBACAllowlist checks if a permission prompt command matches the RBAC allowlist.
func MatchesRBACAllowlist(captureOutput string, allowlist []string) bool {
	if len(allowlist) == 0 {
		allowlist = defaultRBACAllowlist
	}

	toolName := extractToolFromPrompt(captureOutput)
	if toolName == "" {
		return false
	}

	for _, allowed := range allowlist {
		if strings.HasPrefix(toolName, allowed) {
			return true
		}
	}
	return false
}

// extractToolFromPrompt attempts to extract the tool name from a permission prompt.
func extractToolFromPrompt(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, prefix := range []string{"Tool: ", "Command: ", "Action: "} {
			if strings.HasPrefix(trimmed, prefix) {
				return strings.TrimPrefix(trimmed, prefix)
			}
		}
	}
	return ""
}

// CheckUnmanagedSessions compares tmux sessions against AGM-managed sessions.
// Returns a list of tmux sessions that are not in AGM's database.
func CheckUnmanagedSessions(tmuxSessions []string, managedSessions []string) []string {
	managed := make(map[string]bool, len(managedSessions))
	for _, name := range managedSessions {
		managed[name] = true
	}

	var unmanaged []string
	for _, tmuxSession := range tmuxSessions {
		if wellKnownNonAGMSessions[tmuxSession] {
			continue
		}
		if !managed[tmuxSession] {
			unmanaged = append(unmanaged, tmuxSession)
		}
	}
	return unmanaged
}

// FilterCrossCheckTargets returns only the sessions that should be cross-checked.
// Cross-check targets must be supervisor sessions (orchestrator, overseer, meta-)
// and must not be the caller itself.
func FilterCrossCheckTargets(sessions []SessionSummary, callerSession string) []SessionSummary {
	var targets []SessionSummary
	callerLower := strings.ToLower(callerSession)
	for _, s := range sessions {
		if !IsSupervisorSession(s.Name) {
			continue
		}
		if callerLower != "" && strings.ToLower(s.Name) == callerLower {
			continue
		}
		targets = append(targets, s)
	}
	return targets
}

// RunCrossCheck performs the full cross-check scan on all managed sessions.
func RunCrossCheck(ctx *OpContext, cfg *CrossCheckConfig) (*CrossCheckReport, error) {
	if cfg == nil {
		cfg = DefaultCrossCheckConfig()
	}

	report := &CrossCheckReport{
		Timestamp: time.Now(),
	}

	sessionResult, err := ListSessions(ctx, &ListSessionsRequest{
		Status: "active",
		Limit:  contracts.Load().ScanLoop.SessionListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list managed sessions: %w", err)
	}

	var managedNames []string
	for _, s := range sessionResult.Sessions {
		managedNames = append(managedNames, s.Name)
	}

	// Filter to only cross-check supervisor sessions, excluding self.
	targets := FilterCrossCheckTargets(sessionResult.Sessions, cfg.CallerSession)
	for _, session := range targets {
		result := checkSingleSession(session, cfg)
		report.Results = append(report.Results, result)
	}

	tmuxSessions, err := listTmuxSessionNames()
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("failed to list tmux sessions: %v", err))
	} else {
		report.UnmanagedSessions = CheckUnmanagedSessions(tmuxSessions, managedNames)
	}

	return report, nil
}

// checkSingleSession performs cross-check on a single session.
func checkSingleSession(session SessionSummary, cfg *CrossCheckConfig) CrossCheckResult {
	result := CrossCheckResult{
		SessionName: session.Name,
	}

	captureOutput, err := capturePaneForCrossCheck(session.Name)
	if err != nil {
		if strings.Contains(err.Error(), "can't find") ||
			strings.Contains(err.Error(), "no such session") ||
			strings.Contains(err.Error(), "session not found") {
			result.State = StateDown
			result.StateStr = StateDown.String()
			result.Detail = "tmux session no longer exists"
			return result
		}
		result.State = StateHealthy
		result.StateStr = StateHealthy.String()
		result.Detail = fmt.Sprintf("capture failed: %v", err)
		return result
	}

	// Orchestrator-role sessions (orchestrator, overseer, meta-) work interactively
	// via CronCreate, NOT via `agm scan --loop`. Only true scan-loop sessions
	// (supervisor, scan) should be checked for NOT_LOOPING.
	isOrchestratorRole := IsSupervisorSession(session.Name)
	isScanLoopSession := !isOrchestratorRole &&
		(strings.Contains(session.Name, "supervisor") ||
			strings.Contains(session.Name, "scan"))

	stateUpdatedAt := time.Now().Add(-10 * time.Minute)
	if session.UpdatedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, session.UpdatedAt); err == nil {
			stateUpdatedAt = parsed
		}
	}

	state := DetectSessionState(captureOutput, isScanLoopSession, stateUpdatedAt, cfg)
	result.State = state
	result.StateStr = state.String()

	applyCrossCheckAction(state, &result, captureOutput, session.Name, cfg)

	return result
}

// applyCrossCheckAction populates result.Detail/Action and triggers any
// recovery side-effects (auto-approve permission, send Enter, nudge supervisor)
// based on the detected state.
func applyCrossCheckAction(state CrossCheckState, result *CrossCheckResult, captureOutput, sessionName string, cfg *CrossCheckConfig) {
	switch state {
	case StateStuck:
		result.Detail = "permission prompt visible for too long"
		recordErrorMemory(
			"permission prompt visible for too long",
			ErrMemCatPermissionPrompt,
			"",
			"Auto-approve safe patterns or escalate to orchestrator",
			SourceAGMCrossCheck,
			sessionName,
		)
		if !cfg.DryRun && MatchesRBACAllowlist(captureOutput, cfg.RBACAllowlist) {
			if err := autoApprovePermission(sessionName); err != nil {
				result.Detail += fmt.Sprintf("; auto-approve failed: %v", err)
			} else {
				result.Action = "auto-approved via select-option"
			}
		}
	case StateEnterBug:
		result.Detail = "unsent [From: message in input buffer"
		recordErrorMemory(
			"unsent [From: message in input buffer",
			ErrMemCatEnterBug,
			ExtractInputLine(captureOutput),
			"Send Enter key to deliver stuck message",
			SourceAGMCrossCheck,
			sessionName,
		)
		if !cfg.DryRun {
			if err := sendEnterKey(sessionName); err != nil {
				result.Detail += fmt.Sprintf("; Enter send failed: %v", err)
			} else {
				result.Action = "sent Enter to deliver stuck message"
			}
		}
	case StateNotLooping:
		result.Detail = "no scan output detected in recent capture"
		recordErrorMemory(
			"supervisor appears stalled -- no scan output detected",
			ErrMemCatStall,
			"",
			"Send restart nudge to supervisor session",
			SourceAGMCrossCheck,
			sessionName,
		)
		if !cfg.DryRun {
			if HasStuckCrossCheckNudge(captureOutput) {
				result.Detail += "; skipped nudge (previous nudge stuck in input)"
				result.Action = "nudge skipped (stuck)"
			} else if err := nudgeSupervisor(sessionName); err != nil {
				result.Detail += fmt.Sprintf("; nudge failed: %v", err)
			} else {
				result.Action = "sent restart nudge to supervisor"
			}
		}
	case StateDown:
		result.Detail = "tmux session is down"
		recordErrorMemory(
			fmt.Sprintf("tmux session %s no longer exists", sessionName),
			ErrMemCatSessionDown,
			"",
			"Check if session was killed; review process logs",
			SourceAGMCrossCheck,
			sessionName,
		)
	case StateHealthy:
		result.Detail = "operating normally"
	}
}

// capturePaneForCrossCheck captures the last 30 lines from a session.
func capturePaneForCrossCheck(sessionName string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-S", fmt.Sprintf("-%d", contracts.Load().ScanLoop.TmuxCaptureDepth))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("capture-pane failed for %s: %w (%s)", sessionName, err, string(output))
	}
	return string(output), nil
}

// autoApprovePermission approves a permission prompt by selecting option 1.
func autoApprovePermission(sessionName string) error {
	cmd := exec.Command("agm", "session", "select-option", sessionName, "1", "--force")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("select-option failed: %w (%s)", err, string(output))
	}
	return nil
}

// sendEnterKey sends Enter to a session to deliver a stuck message.
func sendEnterKey(sessionName string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, "Enter")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("send-keys Enter failed: %w (%s)", err, string(output))
	}
	return nil
}

// nudgeSupervisor sends a restart message to a supervisor session.
func nudgeSupervisor(sessionName string) error {
	msg := "[cross-check] Supervisor appears stalled -- no scan output detected. Please resume scanning."
	cmd := exec.Command("agm", "send", "msg", sessionName, "--prompt", msg, "--sender", "cross-check")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("send msg failed: %w (%s)", err, string(output))
	}
	return nil
}

// listTmuxSessionNames returns all tmux session names.
func listTmuxSessionNames() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "no server running") {
			return nil, nil
		}
		return nil, fmt.Errorf("list-sessions failed: %w", err)
	}

	text := strings.TrimSpace(string(output))
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}
