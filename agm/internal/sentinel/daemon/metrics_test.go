package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordIncident(t *testing.T) {
	mc := NewMetricsCollector(filepath.Join(t.TempDir(), "metrics.json"))

	mc.RecordIncident("cursor_stuck")
	mc.RecordIncident("cursor_stuck")
	mc.RecordIncident("high_cpu")

	mc.mu.Lock()
	defer mc.mu.Unlock()
	assert.Equal(t, int64(2), mc.incidentCounts["cursor_stuck"])
	assert.Equal(t, int64(1), mc.incidentCounts["high_cpu"])
	assert.Equal(t, int64(3), mc.totalIncidents)
}

func TestRecordEscalation(t *testing.T) {
	mc := NewMetricsCollector(filepath.Join(t.TempDir(), "metrics.json"))

	mc.RecordEscalation("restart")
	mc.RecordEscalation("restart")
	mc.RecordEscalation("notify")

	mc.mu.Lock()
	defer mc.mu.Unlock()
	assert.Equal(t, int64(2), mc.escalationCounts["restart"])
	assert.Equal(t, int64(1), mc.escalationCounts["notify"])
	assert.Equal(t, int64(3), mc.totalEscalations)
}

func TestRecordSuppression(t *testing.T) {
	mc := NewMetricsCollector(filepath.Join(t.TempDir(), "metrics.json"))

	mc.RecordSuppression()
	mc.RecordSuppression()
	mc.RecordSuppression()

	mc.mu.Lock()
	defer mc.mu.Unlock()
	assert.Equal(t, int64(3), mc.dedupSuppressions)
}

func TestSnapshot(t *testing.T) {
	mc := NewMetricsCollector(filepath.Join(t.TempDir(), "metrics.json"))

	mc.RecordIncident("cursor_stuck")
	mc.RecordIncident("high_cpu")
	mc.RecordEscalation("restart")
	mc.RecordSuppression()

	snap := mc.Snapshot()

	assert.Equal(t, int64(1), snap.IncidentCounts["cursor_stuck"])
	assert.Equal(t, int64(1), snap.IncidentCounts["high_cpu"])
	assert.Equal(t, int64(1), snap.EscalationCounts["restart"])
	assert.Equal(t, int64(1), snap.DedupSuppressions)
	assert.Equal(t, int64(2), snap.TotalIncidents)
	assert.Equal(t, int64(1), snap.TotalEscalations)
	assert.NotEmpty(t, snap.Timestamp)

	// Verify snapshot maps are independent copies (no shared references).
	snap.IncidentCounts["cursor_stuck"] = 999
	snap.EscalationCounts["restart"] = 999

	mc.mu.Lock()
	assert.Equal(t, int64(1), mc.incidentCounts["cursor_stuck"])
	assert.Equal(t, int64(1), mc.escalationCounts["restart"])
	mc.mu.Unlock()
}

func TestFlush(t *testing.T) {
	metricsPath := filepath.Join(t.TempDir(), "sub", "metrics.json")
	mc := NewMetricsCollector(metricsPath)

	mc.RecordIncident("cursor_stuck")
	mc.RecordEscalation("restart")
	mc.RecordSuppression()

	err := mc.Flush()
	require.NoError(t, err)

	data, err := os.ReadFile(metricsPath)
	require.NoError(t, err)

	var snap MetricsSnapshot
	err = json.Unmarshal(data, &snap)
	require.NoError(t, err)

	assert.Equal(t, int64(1), snap.IncidentCounts["cursor_stuck"])
	assert.Equal(t, int64(1), snap.EscalationCounts["restart"])
	assert.Equal(t, int64(1), snap.DedupSuppressions)
	assert.Equal(t, int64(1), snap.TotalIncidents)
	assert.Equal(t, int64(1), snap.TotalEscalations)
	assert.NotEmpty(t, snap.Timestamp)
}

func TestFlushIfDue_NotDue(t *testing.T) {
	metricsPath := filepath.Join(t.TempDir(), "metrics.json")
	mc := NewMetricsCollector(metricsPath)

	// Set lastFlush to now so interval hasn't elapsed.
	mc.mu.Lock()
	mc.lastFlush = time.Now()
	mc.mu.Unlock()

	mc.RecordIncident("test")

	flushed, err := mc.FlushIfDue()
	require.NoError(t, err)
	assert.False(t, flushed)

	// File should not exist since no flush occurred.
	_, err = os.Stat(metricsPath)
	assert.True(t, os.IsNotExist(err))
}

func TestFlushIfDue_Due(t *testing.T) {
	metricsPath := filepath.Join(t.TempDir(), "metrics.json")
	mc := NewMetricsCollector(metricsPath)

	// Set lastFlush far in the past so interval has elapsed.
	mc.mu.Lock()
	mc.lastFlush = time.Now().Add(-10 * time.Minute)
	mc.mu.Unlock()

	mc.RecordIncident("test")

	flushed, err := mc.FlushIfDue()
	require.NoError(t, err)
	assert.True(t, flushed)

	// Verify file was written.
	_, err = os.Stat(metricsPath)
	assert.NoError(t, err)
}

func TestConcurrentMetricsAccess(t *testing.T) {
	mc := NewMetricsCollector(filepath.Join(t.TempDir(), "metrics.json"))

	var wg sync.WaitGroup
	iterations := 100

	// Launch goroutines that record incidents concurrently.
	wg.Add(3)
	go func() {
		defer wg.Done()
		for range iterations {
			mc.RecordIncident("symptom_a")
		}
	}()
	go func() {
		defer wg.Done()
		for range iterations {
			mc.RecordEscalation("action_b")
		}
	}()
	go func() {
		defer wg.Done()
		for range iterations {
			mc.RecordSuppression()
		}
	}()

	wg.Wait()

	snap := mc.Snapshot()
	assert.Equal(t, int64(iterations), snap.IncidentCounts["symptom_a"])
	assert.Equal(t, int64(iterations), snap.EscalationCounts["action_b"])
	assert.Equal(t, int64(iterations), snap.DedupSuppressions)
	assert.Equal(t, int64(iterations), snap.TotalIncidents)
	assert.Equal(t, int64(iterations), snap.TotalEscalations)
}
