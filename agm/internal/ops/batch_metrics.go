package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BatchMetrics holds observability data for batch operations.
type BatchMetrics struct {
	SessionsSpawned   int     `json:"sessions_spawned"`
	SessionsCompleted int     `json:"sessions_completed"`
	SessionsFailed    int     `json:"sessions_failed"`
	MergeDurationSecs float64 `json:"merge_duration_seconds"`
	RecordedAt        string  `json:"recorded_at"`
}

// CollectBatchMetrics derives batch metrics from session summaries.
// Completed/failed counts use lifecycle Status since State was removed
// (it produced unreliable results). "stopped" sessions are counted as
// completed; accurate failure detection requires capture-pane ground truth.
func CollectBatchMetrics(sessions []SessionSummary) *BatchMetrics {
	m := &BatchMetrics{
		RecordedAt: time.Now().Format(time.RFC3339),
	}

	for _, s := range sessions {
		m.SessionsSpawned++
		if s.Status == "stopped" {
			m.SessionsCompleted++
		}
	}

	// Merge duration loaded separately from last merge record
	if dm, _ := LoadMergeDuration(); dm > 0 {
		m.MergeDurationSecs = dm
	}

	return m
}

// mergeDurationPath returns the path for the merge duration file.
func mergeDurationPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agm", "metrics", "merge-last.json")
}

// mergeDurationRecord is the on-disk format for merge duration tracking.
type mergeDurationRecord struct {
	DurationSecs float64 `json:"duration_seconds"`
	RecordedAt   string  `json:"recorded_at"`
}

// RecordMergeDuration saves the duration of a batch merge operation.
func RecordMergeDuration(d time.Duration) error {
	path := mergeDurationPath()
	if path == "" {
		return fmt.Errorf("cannot determine metrics path")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create metrics dir: %w", err)
	}

	rec := mergeDurationRecord{
		DurationSecs: d.Seconds(),
		RecordedAt:   time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal merge duration: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

// LoadMergeDuration loads the most recent merge duration from disk.
// Returns 0 if no record exists.
func LoadMergeDuration() (float64, error) {
	path := mergeDurationPath()
	if path == "" {
		return 0, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read merge duration: %w", err)
	}

	var rec mergeDurationRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return 0, fmt.Errorf("parse merge duration: %w", err)
	}
	return rec.DurationSecs, nil
}
