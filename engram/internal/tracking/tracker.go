// Package tracking provides metadata tracking for engram retrievals.
//
// The tracking system records when engrams are accessed and persists
// this information to engram frontmatter for analytics and optimization.
//
// Key components:
//   - Tracker: In-memory access logging with deferred persistence
//   - MetadataUpdater: Atomic file updates for frontmatter metadata
//
// Example usage:
//
//	updater := tracking.NewMetadataUpdater()
//	tracker := tracking.NewTracker(updater)
//
//	// During retrieval
//	tracker.RecordAccess("/path/to/engram.ai.md", time.Now())
//
//	// On exit
//	tracker.Flush()
package tracking

import (
	"log"
	"sync"
	"time"
)

// AccessRecord tracks access information for a single engram
type AccessRecord struct {
	Count      int       // Number of accesses in current session
	LastAccess time.Time // Most recent access timestamp
}

// Tracker manages in-memory access tracking and deferred persistence
type Tracker struct {
	mu        sync.RWMutex
	accessLog map[string]*AccessRecord // path -> access info
	updater   *MetadataUpdater
}

// NewTracker creates a new metadata tracker
func NewTracker(updater *MetadataUpdater) *Tracker {
	return &Tracker{
		accessLog: make(map[string]*AccessRecord),
		updater:   updater,
	}
}

// RecordAccess records an engram retrieval in memory.
// This is called during Tier 3 (load within budget) for each loaded engram.
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (t *Tracker) RecordAccess(path string, timestamp time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.accessLog[path] == nil {
		t.accessLog[path] = &AccessRecord{
			Count:      0,
			LastAccess: timestamp,
		}
	}

	t.accessLog[path].Count++
	t.accessLog[path].LastAccess = timestamp
}

// Flush persists all pending access records to disk.
// This is called on process exit or periodically.
//
// Errors are logged but don't prevent flushing other engrams.
// Failed updates are retained for retry on next flush.
//
// Thread-safe: Can be called concurrently (though not recommended).
func (t *Tracker) Flush() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	successfulPaths := make([]string, 0, len(t.accessLog))

	for path, record := range t.accessLog {
		if err := t.updater.UpdateMetadata(path, record); err != nil {
			// Log error but continue flushing other engrams
			log.Printf("tracking: failed to update %s: %v", path, err)
			// Don't clear this entry - retry on next flush
			continue
		}
		successfulPaths = append(successfulPaths, path)
	}

	// Clear successfully flushed entries
	for _, path := range successfulPaths {
		delete(t.accessLog, path)
	}

	return nil
}

// Len returns the number of pending updates.
// Useful for testing and monitoring.
func (t *Tracker) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.accessLog)
}
