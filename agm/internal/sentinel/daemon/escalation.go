package daemon

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DecisionRecord captures an auto-approval decision for audit logging.
type DecisionRecord struct {
	Timestamp      string `json:"timestamp"`
	SessionName    string `json:"session_name"`
	Action         string `json:"action"`          // approve, notify, rate_limited
	Classification string `json:"classification"`  // safe, dangerous, unknown
	Command        string `json:"command,omitempty"`
	Symptom        string `json:"symptom"`
	Reason         string `json:"reason"`
	RateLimited    bool   `json:"rate_limited"`
}

// logDecision writes an auto-approval decision to the decisions log.
func logDecision(record *DecisionRecord) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	logDir := filepath.Join(homeDir, ".agm/logs/astrocyte")
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return
	}
	logPath := filepath.Join(logDir, "decisions.jsonl")
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
	f.WriteString("\n")
}

// CommandExecutor abstracts command execution for testability.
type CommandExecutor interface {
	Execute(name string, args ...string) ([]byte, error)
}

// ExecCommandExecutor runs real OS commands.
type ExecCommandExecutor struct{}

// Execute runs the command and returns combined output.
func (e *ExecCommandExecutor) Execute(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// EscalationAction defines what to do when circuit breaker fires.
type EscalationAction string

// EscalationAction values controlling circuit-breaker response.
const (
	ActionApprove    EscalationAction = "approve"
	ActionReject     EscalationAction = "reject"
	ActionKillResume EscalationAction = "kill_resume"
	ActionNotify     EscalationAction = "notify"
)

// SafetyClassification is the result of command safety analysis.
type SafetyClassification string

// SafetyClassification values for analyzed commands.
const (
	ClassificationSafe      SafetyClassification = "safe"
	ClassificationDangerous SafetyClassification = "dangerous"
	ClassificationUnknown   SafetyClassification = "unknown"
)

// EscalationResult captures the outcome of an escalation attempt.
type EscalationResult struct {
	Action    EscalationAction
	Success   bool
	Message   string
	Timestamp time.Time
}

// SafetyClassifier determines if a command is safe to auto-approve.
type SafetyClassifier struct {
	dangerousPatterns []*regexp.Regexp
	safePatterns      []*regexp.Regexp
}

// NewSafetyClassifier creates a classifier with default deny/allow lists.
func NewSafetyClassifier() *SafetyClassifier {
	return &SafetyClassifier{
		dangerousPatterns: compileDangerousPatterns(),
		safePatterns:      compileSafePatterns(),
	}
}

func compileDangerousPatterns() []*regexp.Regexp {
	patterns := []string{
		`git\s+push\s+.*--force`,
		`git\s+push\s+-f\b`,
		`git\s+clean\s+-[fd]`,
		`git\s+reset\s+--hard`,
		`git\s+checkout\s+\.\s*$`,
		`git\s+restore\s+\.\s*$`,
		`rm\s+-rf\s`,
		`rm\s+-r\s`,
		`chmod\s+777`,
		`chown\s`,
		`\.env\b`,
		`credentials`,
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}
	return compiled
}

func compileSafePatterns() []*regexp.Regexp {
	patterns := []string{
		`^cat\s`,
		`^ls\s`,
		`^head\s`,
		`^tail\s`,
		`^grep\s`,
		`^rg\s`,
		`^find\s`,
		`^wc\s`,
		`^git\s+status`,
		`^git\s+log`,
		`^git\s+diff`,
		`^git\s+show`,
		`^git\s+branch\s`,
		`^go\s+test`,
		`^go\s+build`,
		`^go\s+vet`,
		`^npm\s+test`,
		`^make\s+test`,
		`^mkdir\s`,
		`^touch\s`,
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}
	return compiled
}

// Classify determines the safety level of a command.
func (sc *SafetyClassifier) Classify(command string) SafetyClassification {
	// Check dangerous patterns first (deny list takes priority)
	for _, p := range sc.dangerousPatterns {
		if p.MatchString(command) {
			return ClassificationDangerous
		}
	}

	// Check safe patterns (allow list)
	for _, p := range sc.safePatterns {
		if p.MatchString(command) {
			return ClassificationSafe
		}
	}

	// Default: unknown = escalate to user
	return ClassificationUnknown
}

// EscalationPipeline handles post-circuit-breaker actions.
type EscalationPipeline struct {
	classifier     *SafetyClassifier
	executor       CommandExecutor
	logger         *slog.Logger
	agmBinary      string
	maxPerHour     int
	mu             sync.Mutex
	hourlyActions  map[string][]time.Time // session -> timestamps of auto-actions
	exemptSessions []string               // session name prefixes that must never be escalated
}

// NewEscalationPipeline creates a new pipeline with the given executor.
func NewEscalationPipeline(executor CommandExecutor, logger *slog.Logger, agmBinary string, maxPerHour int) *EscalationPipeline {
	if agmBinary == "" {
		agmBinary = "agm"
	}
	if maxPerHour <= 0 {
		maxPerHour = 5
	}
	return &EscalationPipeline{
		classifier:    NewSafetyClassifier(),
		executor:      executor,
		logger:        logger,
		agmBinary:     agmBinary,
		maxPerHour:    maxPerHour,
		hourlyActions: make(map[string][]time.Time),
	}
}

// SetExemptSessions sets session name prefixes that must never be escalated.
func (ep *EscalationPipeline) SetExemptSessions(prefixes []string) {
	ep.exemptSessions = prefixes
}

// isSessionExempt checks if a session name matches any exempt prefix.
func (ep *EscalationPipeline) isSessionExempt(sessionName string) bool {
	for _, prefix := range ep.exemptSessions {
		if strings.HasPrefix(sessionName, prefix) {
			return true
		}
	}
	return false
}

// Escalate determines and executes the appropriate escalation action.
func (ep *EscalationPipeline) Escalate(sessionName string, symptom string, command string) (*EscalationResult, error) {
	// Protected sessions must never be escalated (kill/resume/approve/reject).
	if ep.isSessionExempt(sessionName) {
		ep.logger.Info("Skipping escalation for exempt session",
			"session", sessionName, "symptom", symptom)
		return &EscalationResult{
			Action:    ActionNotify,
			Success:   false,
			Message:   fmt.Sprintf("session %s is exempt from escalation", sessionName),
			Timestamp: time.Now(),
		}, nil
	}

	ep.logger.Info("Escalating stuck session",
		"session", sessionName,
		"symptom", symptom,
		"command", command)

	switch symptom {
	case "stuck_permission_prompt":
		return ep.handlePermissionPrompt(sessionName, command)
	case "stuck_mustering", "stuck_zero_token_waiting", "cursor_frozen":
		return ep.handleStuckSession(sessionName, symptom)
	default:
		// stuck_waiting and unknown symptoms: notify only
		return ep.notifyUser(sessionName, fmt.Sprintf("Stuck session detected: %s", symptom))
	}
}

func (ep *EscalationPipeline) handlePermissionPrompt(sessionName, command string) (*EscalationResult, error) {
	// Safety check: don't auto-approve/reject if a human is present
	if safe, reason := ep.isSessionSafeForAction(sessionName); !safe {
		ep.logger.Info("Safety guard blocked auto-action on permission prompt, downgrading to notify",
			"session", sessionName, "reason", reason)
		return ep.notifyUser(sessionName, fmt.Sprintf("Human detected (%s). Permission prompt needs manual review: %s", reason, command))
	}

	classification := ep.classifier.Classify(command)

	switch classification {
	case ClassificationSafe:
		if !ep.canAutoAct(sessionName) {
			logDecision(&DecisionRecord{
				Timestamp: time.Now().Format(time.RFC3339), SessionName: sessionName,
				Action: "rate_limited", Classification: "safe", Command: command,
				Symptom: "stuck_permission_prompt", Reason: "rate limit reached", RateLimited: true,
			})
			return ep.notifyUser(sessionName, fmt.Sprintf("Rate limit reached. Safe command needs approval: %s", command))
		}
		logDecision(&DecisionRecord{
			Timestamp: time.Now().Format(time.RFC3339), SessionName: sessionName,
			Action: "approve", Classification: "safe", Command: command,
			Symptom: "stuck_permission_prompt", Reason: "matched safe pattern",
		})
		return ep.approve(sessionName, command)
	case ClassificationDangerous:
		// MVP: don't auto-reject — notify instead, let human/orchestrator decide
		ep.logger.Warn("Dangerous command detected, notifying (no auto-reject in MVP)",
			"session", sessionName, "command", command)
		logDecision(&DecisionRecord{
			Timestamp: time.Now().Format(time.RFC3339), SessionName: sessionName,
			Action: "notify", Classification: "dangerous", Command: command,
			Symptom: "stuck_permission_prompt", Reason: "dangerous pattern — manual review required",
		})
		return ep.notifyUser(sessionName, fmt.Sprintf("Dangerous command needs manual review: %s", command))
	case ClassificationUnknown:
		logDecision(&DecisionRecord{
			Timestamp: time.Now().Format(time.RFC3339), SessionName: sessionName,
			Action: "notify", Classification: "unknown", Command: command,
			Symptom: "stuck_permission_prompt", Reason: "no pattern match — manual review required",
		})
		return ep.notifyUser(sessionName, fmt.Sprintf("Unknown command needs review: %s", command))
	default:
		return ep.notifyUser(sessionName, fmt.Sprintf("Unknown command needs review: %s", command))
	}
}

func (ep *EscalationPipeline) handleStuckSession(sessionName, symptom string) (*EscalationResult, error) {
	// Safety check: don't kill/resume if a human is present
	if safe, reason := ep.isSessionSafeForAction(sessionName); !safe {
		ep.logger.Info("Safety guard blocked kill/resume on stuck session, downgrading to notify",
			"session", sessionName, "reason", reason)
		return ep.notifyUser(sessionName, fmt.Sprintf("Human detected (%s). Session stuck: %s — manual recovery needed", reason, symptom))
	}

	if !ep.canAutoAct(sessionName) {
		return ep.notifyUser(sessionName, fmt.Sprintf("Rate limit reached. Session stuck: %s", symptom))
	}

	ep.recordAction(sessionName)

	// Kill session
	ep.logger.Info("Killing stuck session", "session", sessionName)
	output, err := ep.executor.Execute(ep.agmBinary, "session", "kill", sessionName, "--force")
	if err != nil {
		ep.logger.Error("Failed to kill session", "session", sessionName, "error", err, "output", string(output))
		return &EscalationResult{
			Action:    ActionKillResume,
			Success:   false,
			Message:   fmt.Sprintf("kill failed: %s", string(output)),
			Timestamp: time.Now(),
		}, nil
	}

	// Resume session
	ep.logger.Info("Resuming session", "session", sessionName)
	output, err = ep.executor.Execute(ep.agmBinary, "session", "resume", sessionName, "--detached")
	if err != nil {
		ep.logger.Error("Failed to resume session", "session", sessionName, "error", err, "output", string(output))
		return &EscalationResult{
			Action:    ActionKillResume,
			Success:   false,
			Message:   fmt.Sprintf("resume failed: %s", string(output)),
			Timestamp: time.Now(),
		}, nil
	}

	return &EscalationResult{
		Action:    ActionKillResume,
		Success:   true,
		Message:   fmt.Sprintf("Session killed and resumed: %s", sessionName),
		Timestamp: time.Now(),
	}, nil
}

func (ep *EscalationPipeline) approve(sessionName, command string) (*EscalationResult, error) {
	ep.recordAction(sessionName)

	ep.logger.Info("Auto-approving safe command", "session", sessionName, "command", command)
	output, err := ep.executor.Execute(ep.agmBinary, "send", "approve", sessionName)
	if err != nil {
		ep.logger.Error("Failed to approve", "session", sessionName, "error", err, "output", string(output))
		return &EscalationResult{
			Action:    ActionApprove,
			Success:   false,
			Message:   fmt.Sprintf("approve failed: %s", string(output)),
			Timestamp: time.Now(),
		}, nil
	}

	return &EscalationResult{
		Action:    ActionApprove,
		Success:   true,
		Message:   fmt.Sprintf("Auto-approved safe command: %s", command),
		Timestamp: time.Now(),
	}, nil
}


func (ep *EscalationPipeline) notifyUser(sessionName, message string) (*EscalationResult, error) {
	ep.logger.Info("Notifying user", "session", sessionName, "message", message)

	output, err := ep.executor.Execute(ep.agmBinary, "send", "msg", sessionName,
		"--sender", "astrocyte",
		"--prompt", message)
	if err != nil {
		// Notification failure is non-fatal; log but don't fail
		ep.logger.Error("Failed to notify user", "session", sessionName, "error", err, "output", string(output))
	}

	return &EscalationResult{
		Action:    ActionNotify,
		Success:   true,
		Message:   message,
		Timestamp: time.Now(),
	}, nil
}

// canAutoAct checks if the rate limit allows another auto-action for this session.
func (ep *EscalationPipeline) canAutoAct(sessionName string) bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	oneHourAgo := time.Now().Add(-1 * time.Hour)
	actions := ep.hourlyActions[sessionName]

	// Filter to only recent actions
	recent := make([]time.Time, 0, len(actions))
	for _, t := range actions {
		if t.After(oneHourAgo) {
			recent = append(recent, t)
		}
	}
	ep.hourlyActions[sessionName] = recent

	return len(recent) < ep.maxPerHour
}

// recordAction records an auto-action timestamp for rate limiting.
func (ep *EscalationPipeline) recordAction(sessionName string) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	ep.hourlyActions[sessionName] = append(ep.hourlyActions[sessionName], time.Now())
}

// safetyCheckResult mirrors the JSON output of `agm safety check --json`.
type safetyCheckResult struct {
	Safe       bool              `json:"safe"`
	Violations []safetyViolation `json:"violations,omitempty"`
}

type safetyViolation struct {
	Guard      string `json:"guard"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
	Evidence   string `json:"evidence,omitempty"`
}

// isSessionSafeForAction calls `agm safety check --json` and returns whether
// the session is safe for automated interaction. If the safety check fails
// (e.g. agm binary not available), it returns true (fail-open) to preserve
// backward compatibility.
func (ep *EscalationPipeline) isSessionSafeForAction(sessionName string) (bool, string) {
	output, err := ep.executor.Execute(ep.agmBinary, "safety", "check", sessionName, "--json",
		"--skip-init",         // astrocyte only monitors initialized sessions
		"--skip-mid-response", // astrocyte handles mid-response detection separately
	)
	if err != nil {
		// Parse output even on error (exit code 1 = violations found, but JSON is valid)
		var result safetyCheckResult
		if jsonErr := json.Unmarshal(output, &result); jsonErr == nil {
			if !result.Safe && len(result.Violations) > 0 {
				reason := result.Violations[0].Guard + ": " + result.Violations[0].Message
				return false, reason
			}
		}
		// If we can't parse, fail-open (allow action)
		ep.logger.Debug("Safety check failed, allowing action (fail-open)", "session", sessionName, "error", err)
		return true, ""
	}

	var result safetyCheckResult
	if err := json.Unmarshal(output, &result); err != nil {
		ep.logger.Debug("Failed to parse safety check output", "error", err)
		return true, "" // fail-open
	}

	if !result.Safe && len(result.Violations) > 0 {
		reason := result.Violations[0].Guard + ": " + result.Violations[0].Message
		return false, reason
	}

	return true, ""
}
