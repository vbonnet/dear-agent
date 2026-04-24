// Package daemon provides the astrocyte session monitoring daemon.
package daemon

import (
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/sentinel/tmux"
)

// SessionHistory tracks cursor positions and content changes over time.
type SessionHistory struct {
	// cursorPositions stores (x,y,timestamp) tuples
	cursorPositions []CursorSnapshot
	// contentSnapshots stores (length,timestamp) tuples for token consumption tracking
	contentSnapshots []ContentSnapshot
	// maxHistory is the number of snapshots to keep
	maxHistory int
}

// CursorSnapshot represents a cursor position at a point in time.
type CursorSnapshot struct {
	X         int
	Y         int
	Timestamp time.Time
}

// ContentSnapshot represents pane content length at a point in time.
// Used for token-consumption-based stuck detection: if content length
// stops changing, the session is not producing tokens.
type ContentSnapshot struct {
	Length    int
	Timestamp time.Time
}

// NewSessionHistory creates a new session history tracker.
func NewSessionHistory(maxHistory int) *SessionHistory {
	return &SessionHistory{
		cursorPositions:  make([]CursorSnapshot, 0, maxHistory),
		contentSnapshots: make([]ContentSnapshot, 0, maxHistory),
		maxHistory:       maxHistory,
	}
}

// AddSnapshot adds a cursor position snapshot.
func (h *SessionHistory) AddSnapshot(x, y int, t time.Time) {
	snapshot := CursorSnapshot{
		X:         x,
		Y:         y,
		Timestamp: t,
	}

	h.cursorPositions = append(h.cursorPositions, snapshot)

	// Trim to max history
	if len(h.cursorPositions) > h.maxHistory {
		h.cursorPositions = h.cursorPositions[1:]
	}
}

// IsCursorFrozen checks if cursor hasn't moved for a given duration.
// Returns true if all snapshots within duration have the same position.
func (h *SessionHistory) IsCursorFrozen(duration time.Duration) bool {
	if len(h.cursorPositions) < 2 {
		return false
	}

	now := time.Now()
	cutoff := now.Add(-duration)

	// Get snapshots within the duration window
	var recentSnapshots []CursorSnapshot
	for _, snapshot := range h.cursorPositions {
		if snapshot.Timestamp.After(cutoff) {
			recentSnapshots = append(recentSnapshots, snapshot)
		}
	}

	// Need at least 2 snapshots to detect freeze
	if len(recentSnapshots) < 2 {
		return false
	}

	// Check if all positions are the same
	firstX := recentSnapshots[0].X
	firstY := recentSnapshots[0].Y

	for _, snapshot := range recentSnapshots[1:] {
		if snapshot.X != firstX || snapshot.Y != firstY {
			return false
		}
	}

	return true
}

// AddContentSnapshot records pane content length at a point in time.
func (h *SessionHistory) AddContentSnapshot(contentLen int, t time.Time) {
	snapshot := ContentSnapshot{
		Length:    contentLen,
		Timestamp: t,
	}

	h.contentSnapshots = append(h.contentSnapshots, snapshot)

	// Trim to max history
	if len(h.contentSnapshots) > h.maxHistory {
		h.contentSnapshots = h.contentSnapshots[1:]
	}
}

// IsContentStale checks if pane content hasn't changed for a given duration.
// Returns true if all content snapshots within the duration window have the
// same length, indicating no token production (output is static).
// Requires at least 2 snapshots spanning the duration to avoid false positives.
func (h *SessionHistory) IsContentStale(duration time.Duration) bool {
	if len(h.contentSnapshots) < 2 {
		return false
	}

	now := time.Now()
	cutoff := now.Add(-duration)

	// Get snapshots within the duration window
	var recentSnapshots []ContentSnapshot
	for _, snapshot := range h.contentSnapshots {
		if snapshot.Timestamp.After(cutoff) {
			recentSnapshots = append(recentSnapshots, snapshot)
		}
	}

	// Need at least 2 snapshots to confirm staleness
	if len(recentSnapshots) < 2 {
		return false
	}

	// Verify the window spans at least the required duration.
	// This prevents premature detection when snapshots are clustered.
	span := recentSnapshots[len(recentSnapshots)-1].Timestamp.Sub(recentSnapshots[0].Timestamp)
	if span < duration/2 {
		return false
	}

	// Check if all content lengths are the same
	firstLen := recentSnapshots[0].Length
	for _, snapshot := range recentSnapshots[1:] {
		if snapshot.Length != firstLen {
			return false
		}
	}

	return true
}

// StuckSessionDetector detects stuck sessions using multiple indicators.
type StuckSessionDetector struct {
	// sessionHistories tracks cursor movement over time
	sessionHistories map[string]*SessionHistory

	// permissionFirstSeen tracks when a permission prompt was first observed per session.
	// This enables time-based detection: stuck after PermissionPromptDuration,
	// escalation after PermissionEscalationDuration.
	permissionFirstSeen map[string]time.Time

	// Thresholds (in minutes)
	MusteringTimeout             int
	ZeroTokenWaitingTimeout      int
	CursorFrozenTimeout          int
	PermissionPromptDuration     int
	PermissionEscalationDuration int // Minutes before escalating unresolved permission prompt (default: 3)
}

// NewStuckSessionDetector creates a new stuck session detector with default thresholds.
func NewStuckSessionDetector() *StuckSessionDetector {
	return &StuckSessionDetector{
		sessionHistories:             make(map[string]*SessionHistory),
		permissionFirstSeen:          make(map[string]time.Time),
		MusteringTimeout:             20, // 20 minutes (conservative)
		ZeroTokenWaitingTimeout:      15, // 15 minutes
		CursorFrozenTimeout:          30, // 30 minutes (very conservative)
		PermissionPromptDuration:     2,  // 2 minutes — fast detection via token consumption tracking
		PermissionEscalationDuration: 3,  // 3 minutes — escalate if recovery hasn't resolved it
	}
}

// TrackSession adds a cursor position snapshot for a session.
func (d *StuckSessionDetector) TrackSession(sessionName string, x, y int) {
	if _, exists := d.sessionHistories[sessionName]; !exists {
		d.sessionHistories[sessionName] = NewSessionHistory(10)
	}

	d.sessionHistories[sessionName].AddSnapshot(x, y, time.Now())
}

// TrackContent records pane content length for token-consumption tracking.
// If content length stops changing across checks, the session is not producing
// tokens — distinguishing "stuck on prompt" from "actively thinking".
func (d *StuckSessionDetector) TrackContent(sessionName string, contentLen int) {
	if _, exists := d.sessionHistories[sessionName]; !exists {
		d.sessionHistories[sessionName] = NewSessionHistory(10)
	}

	d.sessionHistories[sessionName].AddContentSnapshot(contentLen, time.Now())
}

// IsSessionStuck determines if a session is stuck based on multiple indicators.
// Returns true if the session appears stuck, along with the reason.
func (d *StuckSessionDetector) IsSessionStuck(pane *tmux.PaneInfo) (bool, string) {
	indicators := pane.DetectStuckIndicators()

	// AskUserQuestion is a legitimate human-waiting state — never stuck.
	// This covers numbered option lists, selection menus, plan approval prompts,
	// and other interactive TUI dialogs where zero token emission is expected.
	if indicators["ask_user_question"] {
		return false, ""
	}

	// Check for mustering timeout
	if indicators["mustering"] && !indicators["idle_prompt"] {
		// Session stuck in mustering state
		return true, "stuck_mustering"
	}

	// Check for zero token waiting (most common freeze)
	if indicators["zero_token_waiting"] {
		// Session has spinner but no activity
		return true, "stuck_zero_token_waiting"
	}

	// Check for permission prompt — only stuck if content is stale (no token production).
	// This prevents false positives when permission-prompt-like text appears in output
	// while the session is still actively producing tokens.
	if indicators["permission_prompt"] {
		// Track when we first saw the permission prompt for this session
		if _, seen := d.permissionFirstSeen[pane.SessionName]; !seen {
			d.permissionFirstSeen[pane.SessionName] = time.Now()
		}

		history, exists := d.sessionHistories[pane.SessionName]
		if exists {
			staleDuration := time.Duration(d.PermissionPromptDuration) * time.Minute
			if history.IsContentStale(staleDuration) {
				// Check if we've exceeded the escalation threshold
				firstSeen := d.permissionFirstSeen[pane.SessionName]
				escalationDuration := time.Duration(d.PermissionEscalationDuration) * time.Minute
				if time.Since(firstSeen) >= escalationDuration {
					return true, "stuck_permission_prompt_escalate"
				}
				return true, "stuck_permission_prompt"
			}
		}
		// Content still changing — session is producing tokens, not stuck
	} else {
		// Permission prompt no longer visible — clear tracking
		delete(d.permissionFirstSeen, pane.SessionName)
	}

	// Check for cursor frozen (requires history)
	history, exists := d.sessionHistories[pane.SessionName]
	if exists {
		frozenDuration := time.Duration(d.CursorFrozenTimeout) * time.Minute
		if history.IsCursorFrozen(frozenDuration) {
			// Don't mark as stuck if session shows completion language
			if indicators["completed"] {
				return false, ""
			}
			// Don't mark as stuck if idle prompt is visible
			if indicators["idle_prompt"] {
				return false, ""
			}
			return true, "cursor_frozen"
		}
	}

	// Check for general waiting without completion
	if indicators["waiting"] && !indicators["idle_prompt"] && !indicators["completed"] {
		return true, "stuck_waiting"
	}

	return false, ""
}

// GetStuckReason returns a detailed reason why a session is stuck.
// Returns empty string if session is not stuck.
func (d *StuckSessionDetector) GetStuckReason(pane *tmux.PaneInfo) string {
	stuck, reason := d.IsSessionStuck(pane)
	if !stuck {
		return ""
	}
	return reason
}

// SessionStuckInfo contains detailed information about a stuck session.
type SessionStuckInfo struct {
	SessionName string
	Reason      string
	Indicators  map[string]bool
	LastCommand string
	CursorX     int
	CursorY     int
	DetectedAt  time.Time
}

// DetectStuckSession performs a comprehensive stuck session analysis.
// Returns nil if session is not stuck.
func (d *StuckSessionDetector) DetectStuckSession(pane *tmux.PaneInfo) *SessionStuckInfo {
	stuck, reason := d.IsSessionStuck(pane)
	if !stuck {
		return nil
	}

	return &SessionStuckInfo{
		SessionName: pane.SessionName,
		Reason:      reason,
		Indicators:  pane.DetectStuckIndicators(),
		LastCommand: pane.LastCommand,
		CursorX:     pane.CursorX,
		CursorY:     pane.CursorY,
		DetectedAt:  time.Now(),
	}
}

// String returns a human-readable description of the stuck session.
func (s *SessionStuckInfo) String() string {
	return fmt.Sprintf("Session %s stuck: %s (cursor: %d,%d, last command: %s)",
		s.SessionName, s.Reason, s.CursorX, s.CursorY, s.LastCommand)
}
