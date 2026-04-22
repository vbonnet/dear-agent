package ops

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// StallEvent represents a detected stall condition.
type StallEvent struct {
	SessionName       string        // Which session is stalled
	DetectedAt        time.Time     // When detected
	StallType         string        // "permission_prompt" | "no_commit" | "error_loop"
	Duration          time.Duration // How long stalled
	Evidence          string        // Details (error message, last commit time, etc)
	Severity          string        // "warning" | "critical"
	RecommendedAction string        // Recovery suggestion
}

// StallDetector detects stalled sessions.
type StallDetector struct {
	ctx *OpContext
	// Configuration
	PermissionTimeout   time.Duration // Timeout for permission prompts (default 5m)
	NoCommitTimeout     time.Duration // Timeout for no commits (default 15m)
	ErrorRepeatThreshold int           // How many repeats = loop (default 3)
	bus                  eventbus.Broadcaster // Optional: publishes StallDetected events
}

// NewStallDetector creates a new stall detector with thresholds from SLO contracts.
func NewStallDetector(ctx *OpContext) *StallDetector {
	slo := contracts.Load()
	return &StallDetector{
		ctx:                  ctx,
		PermissionTimeout:    slo.StallDetection.PermissionTimeout.Duration,
		NoCommitTimeout:      slo.StallDetection.NoCommitTimeout.Duration,
		ErrorRepeatThreshold: slo.StallDetection.ErrorRepeatThreshold,
	}
}

// SetBus sets the event bus broadcaster for publishing stall events.
func (sd *StallDetector) SetBus(bus eventbus.Broadcaster) {
	sd.bus = bus
}

// DetectStalls scans all active sessions for stall conditions.
func (sd *StallDetector) DetectStalls(ctx context.Context) ([]StallEvent, error) {
	var events []StallEvent

	// List all active sessions
	filter := &dolt.SessionFilter{
		ExcludeArchived: true,
		Limit:           contracts.Load().StallDetection.SessionScanLimit,
	}
	sessions, err := sd.ctx.Storage.ListSessions(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	now := time.Now()

	for _, session := range sessions {
		// Check for permission prompt stall
		if event := sd.detectPermissionPromptStall(session, now); event != nil {
			events = append(events, *event)
		}

		// Check for no-commit stall (only for worker sessions)
		if isWorkerSession(session) {
			if event := sd.detectNoCommitStall(session, now); event != nil {
				events = append(events, *event)
			}
		}

		// Check for error loop stall
		if event := sd.detectErrorLoopStall(session); event != nil {
			events = append(events, *event)
		}
	}


	// Publish detected stalls to EventBus
	sd.publishStallEvents(events)
	return events, nil
}

// detectPermissionPromptStall checks if a session is stuck in a permission prompt.
func (sd *StallDetector) detectPermissionPromptStall(session *manifest.Manifest, now time.Time) *StallEvent {
	if session.State != manifest.StatePermissionPrompt {
		return nil
	}

	// Check how long in permission prompt
	stateUpdated := session.StateUpdatedAt
	if stateUpdated.IsZero() {
		return nil
	}

	duration := now.Sub(stateUpdated)
	if duration < sd.PermissionTimeout {
		return nil
	}

	evt := &StallEvent{
		SessionName:       session.Name,
		DetectedAt:        now,
		StallType:         "permission_prompt",
		Duration:          duration,
		Evidence:          fmt.Sprintf("Permission dialog open for %v", formatDuration(duration)),
		Severity:          "critical",
		RecommendedAction: "Send alert to orchestrator or auto-approve safe patterns",
	}

	recordErrorMemory(
		evt.Evidence,
		ErrMemCatPermissionPrompt,
		"",
		evt.RecommendedAction,
		SourceAGMStall,
		session.Name,
	)

	return evt
}

// detectNoCommitStall checks if a worker session has made no commits.
func (sd *StallDetector) detectNoCommitStall(session *manifest.Manifest, now time.Time) *StallEvent {
	if session.State != manifest.StateWorking {
		return nil
	}

	stateUpdated := session.StateUpdatedAt
	if stateUpdated.IsZero() {
		return nil
	}

	duration := now.Sub(stateUpdated)
	if duration < sd.NoCommitTimeout {
		return nil
	}

	// Check if any commits were made since stateUpdated
	since := stateUpdated.Format(time.RFC3339)
	recentCommits := countRecentCommits(since)

	// recentCommits == 0: no commits found; recentCommits == -1: git failed (skip check)
	if recentCommits == 0 {
		evt := &StallEvent{
			SessionName:       session.Name,
			DetectedAt:        now,
			StallType:         "no_commit",
			Duration:          duration,
			Evidence:          fmt.Sprintf("No commits in %v while in WORKING state", formatDuration(duration)),
			Severity:          "warning",
			RecommendedAction: "Send nudge message to worker or check for blocking errors",
		}

		recordErrorMemory(
			evt.Evidence,
			ErrMemCatStall,
			"",
			evt.RecommendedAction,
			SourceAGMStall,
			session.Name,
		)

		return evt
	}

	return nil
}

// detectErrorLoopStall checks if a session is stuck in an error loop.
func (sd *StallDetector) detectErrorLoopStall(session *manifest.Manifest) *StallEvent {
	// Skip offline sessions
	if session.State == manifest.StateOffline {
		return nil
	}

	// Skip ready/done states
	if session.State == manifest.StateReady || session.State == manifest.StateDone {
		return nil
	}

	// Capture tmux pane output
	output, err := captureSessionOutput(session.Name, contracts.Load().StallDetection.TmuxCaptureDepth)
	if err != nil {
		// If we can't capture output, skip this check
		return nil
	}

	// Parse for repeated error patterns
	errorPatterns := extractErrorPatterns(output)
	for pattern, count := range errorPatterns {
		if count >= sd.ErrorRepeatThreshold {
			evt := &StallEvent{
				SessionName:       session.Name,
				DetectedAt:        time.Now(),
				StallType:         "error_loop",
				Duration:          0, // Not applicable for error loops
				Evidence:          fmt.Sprintf("Error pattern '%s' appears %d times in output", pattern, count),
				Severity:          "warning",
				RecommendedAction: "Send diagnostic to orchestrator or manually review error",
			}

			recordErrorMemory(
				pattern,
				ErrMemCatErrorLoop,
				evt.Evidence,
				evt.RecommendedAction,
				SourceAGMStall,
				session.Name,
			)

			return evt
		}
	}

	return nil
}

// isWorkerSession checks if a session is a worker (not orchestrator).
func isWorkerSession(session *manifest.Manifest) bool {
	// Workers typically have "worker" in their name or tags
	for _, tag := range session.Context.Tags {
		if strings.Contains(strings.ToLower(tag), "worker") {
			return true
		}
	}
	// Also check session name
	return strings.Contains(strings.ToLower(session.Name), "worker")
}

// countRecentCommits counts commits since a given timestamp.
func countRecentCommits(since string) int {
	cmd := exec.Command("git", "log", "--all", "--oneline", "--since="+since)
	output, err := cmd.Output()
	if err != nil {
		return -1 // Error counting
	}
	lines := strings.TrimSpace(string(output))
	if lines == "" {
		return 0
	}
	return len(strings.Split(lines, "\n"))
}

// captureSessionOutput gets the last N lines from a session's tmux pane.
func captureSessionOutput(sessionName string, lines int) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", fmt.Sprintf("-S-%d", lines))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane output: %w", err)
	}
	return string(output), nil
}

// extractErrorPatterns finds repeated error messages in output.
func extractErrorPatterns(output string) map[string]int {
	patterns := make(map[string]int)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Look for common error patterns
		if isErrorLine(line) {
			// Normalize the error for pattern matching
			normalized := normalizeErrorMessage(line)
			patterns[normalized]++
		}
	}

	return patterns
}

// isErrorLine checks if a line looks like an error.
func isErrorLine(line string) bool {
	lower := strings.ToLower(line)
	errorKeywords := []string{
		"error:", "failed", "fatal", "panic", "cannot", "permission denied",
		"timeout", "operation timeout", "no such file", "connection refused",
		"bad", "invalid", "denied", "blocked",
	}
	for _, kw := range errorKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// normalizeErrorMessage extracts the essential error message.
func normalizeErrorMessage(line string) string {
	// Remove timestamps, file paths, and line numbers
	line = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`).ReplaceAllString(line, "")
	line = regexp.MustCompile(`:\d+:`).ReplaceAllString(line, ":<line>:")
	line = regexp.MustCompile(`/[^\s]+`).ReplaceAllString(line, "/<path>")
	line = strings.TrimSpace(line)

	// Keep only the first 100 chars to avoid bloat
	if len(line) > contracts.Load().StallDetection.ErrorMessageMaxLength {
		line = line[:contracts.Load().StallDetection.ErrorMessageMaxLength]
	}

	return line
}


// publishStallEvents publishes StallDetected events to the EventBus for each detected stall.
func (sd *StallDetector) publishStallEvents(events []StallEvent) {
	if sd.bus == nil {
		return
	}
	for _, evt := range events {
		busEvent, err := eventbus.NewEvent(eventbus.EventStallDetected, evt.SessionName, eventbus.StallDetectedPayload{
			StallType: evt.StallType,
			Session:   evt.SessionName,
			Duration:  evt.Duration,
			Details:   evt.Evidence,
			Severity:  evt.Severity,
		})
		if err != nil {
			continue
		}
		sd.bus.Broadcast(busEvent)
	}
}
