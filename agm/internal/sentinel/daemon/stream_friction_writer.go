package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StreamFrictionItem is a work item written to stream-friction/pending.jsonl
// when a friction pattern crosses the cross-session threshold.
type StreamFrictionItem struct {
	PatternID              string   `json:"pattern_id"`
	Description            string   `json:"description"`
	OccurrenceCount        int      `json:"occurrence_count"`
	SessionCount           int      `json:"session_count"`
	Sessions               []string `json:"sessions"`
	FirstSeen              string   `json:"first_seen"`
	LastSeen               string   `json:"last_seen"`
	Timestamp              string   `json:"timestamp"`
	Source                 string   `json:"source"`
	SuggestedInvestigation string   `json:"suggested_investigation"`
}

// StreamFrictionWriter appends friction work items to stream-friction/pending.jsonl.
type StreamFrictionWriter struct {
	dir string
	mu  sync.Mutex
	// emitted tracks patterns already written to avoid duplicates within a run
	emitted map[string]bool
}

// NewStreamFrictionWriter creates a writer targeting ~/.agm/intake/stream-friction/.
// If baseDir is empty, defaults to ~/.agm/intake/stream-friction.
func NewStreamFrictionWriter(baseDir string) (*StreamFrictionWriter, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".agm", "intake", "stream-friction")
	}
	if err := os.MkdirAll(baseDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create stream-friction dir: %w", err)
	}
	return &StreamFrictionWriter{
		dir:     baseDir,
		emitted: make(map[string]bool),
	}, nil
}

// WriteItem appends a friction work item to pending.jsonl. Deduplicates by
// patternID so each pattern is only written once per daemon lifetime.
func (w *StreamFrictionWriter) WriteItem(item *StreamFrictionItem) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.emitted[item.PatternID] {
		return nil // already emitted
	}

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal friction item: %w", err)
	}

	filePath := filepath.Join(w.dir, "pending.jsonl")
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open pending.jsonl: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write item: %w", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync pending.jsonl: %w", err)
	}

	w.emitted[item.PatternID] = true
	return nil
}

// suggestedInvestigations maps friction pattern IDs to investigation hints.
var suggestedInvestigations = map[string]string{
	"friction:same_as_always":   "Identify the specific recurring problem and check if a systemic fix exists",
	"friction:keeps_happening":  "Search incident logs for prior occurrences and identify root cause",
	"friction:recurring_issue":  "Check if a work item already exists; if not, create one with root cause analysis",
	"friction:every_session":    "High-frequency friction — prioritize as it affects every session",
	"friction:every_time":       "Deterministic failure — should be reproducible and fixable",
	"friction:same_error_again": "Search error logs for this error pattern and trace to source",
	"friction:workaround":       "Document the workaround and investigate a permanent fix",
	"friction:known_issue":      "Check if tracked; if not, create a tracking work item",
	"friction:keeps_failing":    "Investigate flaky or consistently failing component",
	"friction:hit_again":        "Cross-reference with previous incidents for this bug",
	"friction:always_fails":     "Investigate the specific failure point for a structural fix",
	"friction:not_again":        "Identify the source of frustration and check for systemic pattern",
}

// BuildStreamFrictionItem constructs a StreamFrictionItem from accumulator state.
// It uses a dedicated friction accumulator to get occurrence counts and session lists.
func BuildStreamFrictionItem(acc *PatternAccumulator, patternID, description string) *StreamFrictionItem {
	acc.mu.Lock()
	defer acc.mu.Unlock()

	cutoff := time.Now().Add(-acc.window)
	var sessions []string
	var firstSeen, lastSeen time.Time
	totalOccurrences := 0

	for name, sv := range acc.sessions {
		sessionHit := false
		for _, v := range sv.Violations {
			if v.PatternID == patternID && v.Timestamp.After(cutoff) {
				totalOccurrences++
				if !sessionHit {
					sessions = append(sessions, name)
					sessionHit = true
				}
				if firstSeen.IsZero() || v.Timestamp.Before(firstSeen) {
					firstSeen = v.Timestamp
				}
				if v.Timestamp.After(lastSeen) {
					lastSeen = v.Timestamp
				}
			}
		}
	}

	investigation := suggestedInvestigations[patternID]
	if investigation == "" {
		investigation = "Investigate recurring friction pattern across sessions"
	}

	return &StreamFrictionItem{
		PatternID:              patternID,
		Description:            description,
		OccurrenceCount:        totalOccurrences,
		SessionCount:           len(sessions),
		Sessions:               sessions,
		FirstSeen:              firstSeen.Format(time.RFC3339),
		LastSeen:               lastSeen.Format(time.RFC3339),
		Timestamp:              time.Now().Format(time.RFC3339),
		Source:                 "astrocyte-friction-detector",
		SuggestedInvestigation: investigation,
	}
}
