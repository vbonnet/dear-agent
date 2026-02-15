package daemon

import (
	"fmt"
	"time"

	"github.com/vbonnet/ai-tools/astrocyte/internal/tmux"
)

// SessionHistory tracks cursor positions over time for freeze detection.
type SessionHistory struct {
	// cursorPositions stores (x,y,timestamp) tuples
	cursorPositions []CursorSnapshot
	// maxHistory is the number of snapshots to keep
	maxHistory int
}

// CursorSnapshot represents a cursor position at a point in time.
type CursorSnapshot struct {
	X         int
	Y         int
	Timestamp time.Time
}

// NewSessionHistory creates a new session history tracker.
func NewSessionHistory(maxHistory int) *SessionHistory {
	return &SessionHistory{
		cursorPositions: make([]CursorSnapshot, 0, maxHistory),
		maxHistory:      maxHistory,
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

// StuckSessionDetector detects stuck sessions using multiple indicators.
type StuckSessionDetector struct {
	// sessionHistories tracks cursor movement over time
	sessionHistories map[string]*SessionHistory

	// Thresholds (in minutes)
	MusteringTimeout         int
	ZeroTokenWaitingTimeout  int
	CursorFrozenTimeout      int
	PermissionPromptDuration int
}

// NewStuckSessionDetector creates a new stuck session detector with default thresholds.
func NewStuckSessionDetector() *StuckSessionDetector {
	return &StuckSessionDetector{
		sessionHistories:         make(map[string]*SessionHistory),
		MusteringTimeout:         20, // 20 minutes (conservative)
		ZeroTokenWaitingTimeout:  15, // 15 minutes
		CursorFrozenTimeout:      30, // 30 minutes (very conservative)
		PermissionPromptDuration: 10, // 10 minutes
	}
}

// TrackSession adds a cursor position snapshot for a session.
func (d *StuckSessionDetector) TrackSession(sessionName string, x, y int) {
	if _, exists := d.sessionHistories[sessionName]; !exists {
		d.sessionHistories[sessionName] = NewSessionHistory(10)
	}

	d.sessionHistories[sessionName].AddSnapshot(x, y, time.Now())
}

// IsSessionStuck determines if a session is stuck based on multiple indicators.
// Returns true if the session appears stuck, along with the reason.
func (d *StuckSessionDetector) IsSessionStuck(pane *tmux.PaneInfo) (bool, string) {
	indicators := pane.DetectStuckIndicators()

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

	// Check for permission prompt
	if indicators["permission_prompt"] {
		// Session waiting for user permission
		return true, "stuck_permission_prompt"
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
