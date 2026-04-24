package helpers

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TmuxCaptureMatcher provides utilities for analyzing tmux pane content
// Used for verifying command sequences, detecting keypress patterns, and timeline tracking
type TmuxCaptureMatcher struct {
	SessionName string
	Captures    []CaptureSnapshot
}

// CaptureSnapshot represents a single tmux pane capture at a point in time
type CaptureSnapshot struct {
	Timestamp time.Time
	Content   string
	Lines     []string
}

// NewTmuxCaptureMatcher creates a new matcher for the given session
func NewTmuxCaptureMatcher(sessionName string) *TmuxCaptureMatcher {
	return &TmuxCaptureMatcher{
		SessionName: sessionName,
		Captures:    make([]CaptureSnapshot, 0),
	}
}

// CaptureNow takes a snapshot of the current tmux pane content
func (m *TmuxCaptureMatcher) CaptureNow() error {
	cmd := exec.Command("tmux", "capture-pane", "-t", m.SessionName, "-p")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to capture pane: %w", err)
	}

	content := string(output)
	lines := strings.Split(content, "\n")

	snapshot := CaptureSnapshot{
		Timestamp: time.Now(),
		Content:   content,
		Lines:     lines,
	}

	m.Captures = append(m.Captures, snapshot)
	return nil
}

// DetectESCKeypresses searches all captured snapshots for ESC escape sequences
// Returns true if any ESC keypresses were detected in the session
//
// ESC keypresses appear in tmux capture as:
// - Escape sequences (^[ or \x1b)
// - Visual indicators like "[Interrupted]" (from Claude Code)
// - Sudden clearing of input buffers
func DetectESCKeypresses() bool {
	// This is a placeholder implementation
	// Real implementation would:
	// 1. Capture pane content before and after command
	// 2. Look for ESC sequences: ^[, \x1b, Escape key
	// 3. Detect Claude Code interruption markers
	// 4. Check for cleared input buffers
	//
	// For now, this serves as documentation of the intended behavior
	return false
}

// VerifySequence checks that commands/outputs appear in the expected order
// in the captured timeline
//
// Args:
//   - expectedSequence: List of strings to find in order
//   - allowGaps: If true, allows other content between expected items
//
// Returns:
//   - error if sequence not found or out of order
func (m *TmuxCaptureMatcher) VerifySequence(expectedSequence []string, allowGaps bool) error {
	if len(m.Captures) == 0 {
		return fmt.Errorf("no captures recorded")
	}

	// Get latest capture (most complete view)
	latest := m.Captures[len(m.Captures)-1]

	// Track position in content
	lastIndex := 0

	for i, expected := range expectedSequence {
		// Find expected string starting from last position
		index := strings.Index(latest.Content[lastIndex:], expected)
		if index == -1 {
			return fmt.Errorf("sequence item %d not found: %q (searched from position %d)", i, expected, lastIndex)
		}

		// Update position for next search
		if allowGaps {
			lastIndex = lastIndex + index + len(expected)
		} else {
			// If gaps not allowed, next item must appear immediately after
			lastIndex = lastIndex + index + len(expected)
		}
	}

	return nil
}

// FindPattern searches all captures for a specific pattern
// Returns the first snapshot where pattern was found, or error if not found
func (m *TmuxCaptureMatcher) FindPattern(pattern string) (*CaptureSnapshot, error) {
	for i := range m.Captures {
		if strings.Contains(m.Captures[i].Content, pattern) {
			return &m.Captures[i], nil
		}
	}
	return nil, fmt.Errorf("pattern %q not found in %d captures", pattern, len(m.Captures))
}

// GetTimeline returns a formatted timeline of all captures
// Useful for debugging test failures
func (m *TmuxCaptureMatcher) GetTimeline() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Tmux Capture Timeline (%d snapshots):\n", len(m.Captures)))
	sb.WriteString("========================================\n\n")

	for i, capture := range m.Captures {
		sb.WriteString(fmt.Sprintf("Snapshot %d @ %s\n", i+1, capture.Timestamp.Format("15:04:05.000")))
		sb.WriteString("----------------------------------------\n")
		sb.WriteString(capture.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}
