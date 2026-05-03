package ops

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectBatchMetrics_MixedStates(t *testing.T) {
	// Use temp HOME to avoid loading stale merge data
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sessions := []SessionSummary{
		{Name: "w1", Status: "stopped"},
		{Name: "w2", Status: "stopped"},
		{Name: "w3", Status: "stopped"},
		{Name: "w4", Status: "active"},
		{Name: "w5", Status: "active"},
	}

	m := CollectBatchMetrics(sessions)

	if m.SessionsSpawned != 5 {
		t.Errorf("SessionsSpawned = %d, want 5", m.SessionsSpawned)
	}
	if m.SessionsCompleted != 3 {
		t.Errorf("SessionsCompleted = %d, want 3", m.SessionsCompleted)
	}
	if m.SessionsFailed != 0 {
		t.Errorf("SessionsFailed = %d, want 0", m.SessionsFailed)
	}
	if m.RecordedAt == "" {
		t.Error("RecordedAt should not be empty")
	}
}

func TestCollectBatchMetrics_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	m := CollectBatchMetrics(nil)

	if m.SessionsSpawned != 0 {
		t.Errorf("SessionsSpawned = %d, want 0", m.SessionsSpawned)
	}
	if m.SessionsCompleted != 0 {
		t.Errorf("SessionsCompleted = %d, want 0", m.SessionsCompleted)
	}
	if m.SessionsFailed != 0 {
		t.Errorf("SessionsFailed = %d, want 0", m.SessionsFailed)
	}
}

func TestCollectBatchMetrics_AllDone(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sessions := []SessionSummary{
		{Name: "w1", Status: "stopped"},
		{Name: "w2", Status: "stopped"},
		{Name: "w3", Status: "stopped"},
	}

	m := CollectBatchMetrics(sessions)

	if m.SessionsSpawned != 3 {
		t.Errorf("SessionsSpawned = %d, want 3", m.SessionsSpawned)
	}
	if m.SessionsCompleted != 3 {
		t.Errorf("SessionsCompleted = %d, want 3", m.SessionsCompleted)
	}
	if m.SessionsFailed != 0 {
		t.Errorf("SessionsFailed = %d, want 0", m.SessionsFailed)
	}
}

func TestRecordAndLoadMergeDuration(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// No existing record
	d, err := LoadMergeDuration()
	if err != nil {
		t.Fatalf("unexpected error loading empty: %v", err)
	}
	if d != 0 {
		t.Errorf("expected 0 for nonexistent, got %f", d)
	}

	// Record a duration
	dur := 3500 * time.Millisecond
	if err := RecordMergeDuration(dur); err != nil {
		t.Fatalf("failed to record: %v", err)
	}

	// Verify file
	path := filepath.Join(tmpDir, ".agm", "metrics", "merge-last.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("merge metrics file not created")
	}

	// Load and verify
	d, err = LoadMergeDuration()
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if d != 3.5 {
		t.Errorf("duration = %f, want 3.5", d)
	}
}

func TestCollectBatchMetrics_WithMergeDuration(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Record a merge duration first
	if err := RecordMergeDuration(5 * time.Second); err != nil {
		t.Fatalf("failed to record merge duration: %v", err)
	}

	sessions := []SessionSummary{
		{Name: "w1", Status: "stopped"},
	}

	m := CollectBatchMetrics(sessions)

	if m.MergeDurationSecs != 5.0 {
		t.Errorf("MergeDurationSecs = %f, want 5.0", m.MergeDurationSecs)
	}
}

func TestLoadMergeDuration_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	metricsDir := filepath.Join(tmpDir, ".agm", "metrics")
	os.MkdirAll(metricsDir, 0o755)
	os.WriteFile(filepath.Join(metricsDir, "merge-last.json"), []byte("{bad"), 0o644)

	_, err := LoadMergeDuration()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
