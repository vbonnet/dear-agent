package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StreamErrorItem is a work item written to pending.jsonl when a pattern
// crosses the cross-session threshold.
type StreamErrorItem struct {
	PatternID    string   `json:"pattern_id"`
	SessionCount int      `json:"session_count"`
	Sessions     []string `json:"sessions"`
	FirstSeen    string   `json:"first_seen"`
	LastSeen     string   `json:"last_seen"`
	Timestamp    string   `json:"timestamp"`
	Source       string   `json:"source"`
}

// StreamErrorWriter appends work items to the stream-errors pending.jsonl file.
type StreamErrorWriter struct {
	dir string
	mu  sync.Mutex
	// emitted tracks patterns already written to avoid duplicates within a run
	emitted map[string]bool
}

// NewStreamErrorWriter creates a writer targeting ~/.agm/intake/stream-errors/.
// If baseDir is empty, defaults to ~/.agm/intake/stream-errors.
func NewStreamErrorWriter(baseDir string) (*StreamErrorWriter, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home dir: %w", err)
		}
		baseDir = filepath.Join(home, ".agm", "intake", "stream-errors")
	}
	if err := os.MkdirAll(baseDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create stream-errors dir: %w", err)
	}
	return &StreamErrorWriter{
		dir:     baseDir,
		emitted: make(map[string]bool),
	}, nil
}

// WriteItem appends a work item to pending.jsonl. It deduplicates by patternID
// so that each pattern is only written once per daemon lifetime.
func (w *StreamErrorWriter) WriteItem(item *StreamErrorItem) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.emitted[item.PatternID] {
		return nil // already emitted
	}

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal stream error item: %w", err)
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

// BuildItem constructs a StreamErrorItem from accumulator state.
func BuildStreamErrorItem(acc *PatternAccumulator, patternID string) *StreamErrorItem {
	acc.mu.Lock()
	defer acc.mu.Unlock()

	cutoff := time.Now().Add(-acc.window)
	var sessions []string
	var firstSeen, lastSeen time.Time

	for name, sv := range acc.sessions {
		for _, v := range sv.Violations {
			if v.PatternID == patternID && v.Timestamp.After(cutoff) {
				sessions = append(sessions, name)
				if firstSeen.IsZero() || v.Timestamp.Before(firstSeen) {
					firstSeen = v.Timestamp
				}
				if v.Timestamp.After(lastSeen) {
					lastSeen = v.Timestamp
				}
				break
			}
		}
	}

	return &StreamErrorItem{
		PatternID:    patternID,
		SessionCount: len(sessions),
		Sessions:     sessions,
		FirstSeen:    firstSeen.Format(time.RFC3339),
		LastSeen:     lastSeen.Format(time.RFC3339),
		Timestamp:    time.Now().Format(time.RFC3339),
		Source:       "astrocyte-daemon",
	}
}
