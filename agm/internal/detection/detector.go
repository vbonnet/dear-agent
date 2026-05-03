// Package detection provides detection functionality.
package detection

import (
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// Result represents the outcome of UUID detection
type Result struct {
	UUID       string
	Source     string    // "history", "manual", or "none"
	Confidence string    // "high", "medium", "low"
	MatchedAt  time.Time // When the match was found
}

// Detector performs hybrid UUID auto-detection
type Detector struct {
	historyParser   *history.Parser
	detectionWindow time.Duration
	adapter         dolt.Storage
}

// NewDetector creates a new UUID detector
func NewDetector(historyPath string, windowDuration time.Duration, adapter dolt.Storage) *Detector {
	if windowDuration == 0 {
		windowDuration = 5 * time.Minute // Default 5-minute window
	}

	return &Detector{
		historyParser:   history.NewParser(historyPath),
		detectionWindow: windowDuration,
		adapter:         adapter,
	}
}

// DetectUUID attempts to auto-detect UUID for a manifest
// Returns Result with UUID and confidence level
func (d *Detector) DetectUUID(m *manifest.Manifest) (*Result, error) {
	// If UUID already set, return it (manual association takes precedence)
	if m.Claude.UUID != "" {
		return &Result{
			UUID:       m.Claude.UUID,
			Source:     "manual",
			Confidence: "high",
		}, nil
	}

	// Try history-based detection
	result, err := d.detectFromHistory(m)
	if err != nil {
		// No match found, return empty result
		return &Result{
			Source:     "none",
			Confidence: "low",
		}, nil
	}

	return result, nil
}

// detectFromHistory attempts to find UUID in Claude history
func (d *Detector) detectFromHistory(m *manifest.Manifest) (*Result, error) {
	// Find history entry for this project directory
	entry, err := d.historyParser.FindByDirectory(m.Context.Project)
	if err != nil {
		return nil, err
	}

	// Check if entry is within detection window
	timeSinceMatch := time.Since(entry.Timestamp)
	if timeSinceMatch > d.detectionWindow {
		// Match is too old, low confidence
		return &Result{
			UUID:       entry.UUID,
			Source:     "history",
			Confidence: "low",
			MatchedAt:  entry.Timestamp,
		}, nil
	}

	// Recent match within window - high confidence
	confidence := "high"
	if timeSinceMatch > d.detectionWindow/2 {
		confidence = "medium"
	}

	return &Result{
		UUID:       entry.UUID,
		Source:     "history",
		Confidence: confidence,
		MatchedAt:  entry.Timestamp,
	}, nil
}

// DetectCurrentUUID checks history for the most recent UUID for a manifest's project,
// regardless of whether a UUID is already set. This detects UUID changes caused by
// Plan→Execute transitions where Claude Code silently creates a new session.
// Returns the history-detected UUID (which may differ from m.Claude.UUID).
func (d *Detector) DetectCurrentUUID(m *manifest.Manifest) (*Result, error) {
	return d.detectFromHistory(m)
}

// DetectAndAssociate attempts to detect and update manifest with UUID
// Returns true if UUID was detected and associated
func (d *Detector) DetectAndAssociate(m *manifest.Manifest, manifestPath string, autoApply bool) (bool, error) {
	result, err := d.DetectUUID(m)
	if err != nil {
		return false, err
	}

	// If no UUID detected or already set, nothing to do
	if result.UUID == "" || result.Source == "manual" {
		return false, nil
	}

	// Only auto-apply if confidence is high and autoApply is enabled
	if !autoApply {
		return false, nil
	}

	if result.Confidence != "high" {
		// Low/medium confidence - don't auto-apply
		return false, nil
	}

	// Update manifest with detected UUID
	m.Claude.UUID = result.UUID
	m.UpdatedAt = time.Now()

	// Write to Dolt
	if d.adapter == nil {
		return false, fmt.Errorf("dolt adapter required")
	}
	if err := d.adapter.UpdateSession(m); err != nil {
		return false, fmt.Errorf("failed to update session in Dolt: %w", err)
	}

	return true, nil
}

// ScanAndDetect scans a sessions directory and detects UUIDs for all sessions
// Returns map of session name -> detection result
func (d *Detector) ScanAndDetect(sessionsDir string) (map[string]*Result, error) {
	// Query Dolt for all sessions
	if d.adapter == nil {
		return nil, fmt.Errorf("dolt adapter not available")
	}

	manifests, err := d.adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	results := make(map[string]*Result)

	for _, m := range manifests {
		result, err := d.DetectUUID(m)
		if err != nil {
			// Log error but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to detect UUID for %s: %v\n", m.Name, err)
			continue
		}

		results[m.Name] = result
	}

	return results, nil
}
