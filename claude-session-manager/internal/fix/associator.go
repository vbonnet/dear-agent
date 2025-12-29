package fix

import (
	"fmt"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/detection"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/history"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// Associator handles manual UUID association for sessions
type Associator struct {
	detector      *detection.Detector
	historyParser *history.Parser
}

// NewAssociator creates a new UUID associator
func NewAssociator(detector *detection.Detector, historyParser *history.Parser) *Associator {
	return &Associator{
		detector:      detector,
		historyParser: historyParser,
	}
}

// Suggestion represents a UUID suggestion with context
type Suggestion struct {
	UUID       string
	Source     string    // "history", "recent", "manual"
	Confidence string    // "high", "medium", "low"
	Context    string    // Human-readable context
	Timestamp  time.Time // When this UUID was last seen
}

// GetSuggestions returns UUID suggestions for a manifest
func (a *Associator) GetSuggestions(m *manifest.Manifest, limit int) ([]*Suggestion, error) {
	var suggestions []*Suggestion

	// 1. Try auto-detection first
	detectionResult, err := a.detector.DetectUUID(m)
	if err == nil && detectionResult.UUID != "" && detectionResult.Source != "none" {
		suggestions = append(suggestions, &Suggestion{
			UUID:       detectionResult.UUID,
			Source:     detectionResult.Source,
			Confidence: detectionResult.Confidence,
			Context:    fmt.Sprintf("Detected from %s", detectionResult.Source),
			Timestamp:  detectionResult.MatchedAt,
		})
	}

	// 2. Get recent UUIDs from history (for manual selection)
	recentEntries, err := a.historyParser.GetRecentEntries(10)
	if err == nil {
		for _, entry := range recentEntries {
			// Skip if already suggested
			if detectionResult != nil && entry.UUID == detectionResult.UUID {
				continue
			}

			context := fmt.Sprintf("Recent session in %s", entry.Directory)
			if entry.Name != "" {
				context = fmt.Sprintf("%s (named: %s)", context, entry.Name)
			}

			suggestions = append(suggestions, &Suggestion{
				UUID:       entry.UUID,
				Source:     "recent",
				Confidence: "low",
				Context:    context,
				Timestamp:  entry.Timestamp,
			})

			if len(suggestions) >= limit {
				break
			}
		}
	}

	return suggestions, nil
}

// Associate manually associates a UUID with a manifest
func (a *Associator) Associate(m *manifest.Manifest, manifestPath string, uuid string) error {
	// Validate UUID format (basic check)
	if uuid == "" {
		return fmt.Errorf("UUID cannot be empty")
	}

	// Update manifest
	m.Claude.UUID = uuid
	m.UpdatedAt = time.Now()

	// Write to disk
	if err := manifest.Write(manifestPath, m); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// Clear removes UUID association from a manifest
func (a *Associator) Clear(m *manifest.Manifest, manifestPath string) error {
	m.Claude.UUID = ""
	m.UpdatedAt = time.Now()

	if err := manifest.Write(manifestPath, m); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// ScanUnassociated finds all sessions without UUID associations
func (a *Associator) ScanUnassociated(sessionsDir string) ([]*manifest.Manifest, error) {
	manifests, err := manifest.List(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list manifests: %w", err)
	}

	var unassociated []*manifest.Manifest
	for _, m := range manifests {
		if m.Claude.UUID == "" {
			unassociated = append(unassociated, m)
		}
	}

	return unassociated, nil
}

// ScanBroken finds sessions with invalid UUID associations
func (a *Associator) ScanBroken(sessionsDir string) ([]*manifest.Manifest, error) {
	manifests, err := manifest.List(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list manifests: %w", err)
	}

	var broken []*manifest.Manifest
	for _, m := range manifests {
		if m.Claude.UUID != "" {
			// Check if UUID exists in history
			entries, err := a.historyParser.FindByUUID(m.Claude.UUID)
			if err == nil && len(entries) == 0 {
				// UUID doesn't exist in history - potentially broken
				broken = append(broken, m)
			}
		}
	}

	return broken, nil
}
