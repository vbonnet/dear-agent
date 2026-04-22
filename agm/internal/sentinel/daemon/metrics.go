package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MetricsSnapshot represents a point-in-time snapshot of daemon metrics.
type MetricsSnapshot struct {
	Timestamp         string           `json:"timestamp"`
	IncidentCounts    map[string]int64 `json:"incident_counts"`
	EscalationCounts  map[string]int64 `json:"escalation_counts"`
	DedupSuppressions int64            `json:"dedup_suppressions"`
	TotalIncidents    int64            `json:"total_incidents"`
	TotalEscalations  int64            `json:"total_escalations"`
}

// MetricsCollector tracks incident and escalation counts for dashboard integration.
type MetricsCollector struct {
	mu                sync.Mutex
	incidentCounts    map[string]int64
	escalationCounts  map[string]int64
	dedupSuppressions int64
	totalIncidents    int64
	totalEscalations  int64
	metricsFile       string
	lastFlush         time.Time
	flushInterval     time.Duration
}

// NewMetricsCollector creates a new metrics collector writing to the given file path.
func NewMetricsCollector(metricsFile string) *MetricsCollector {
	return &MetricsCollector{
		incidentCounts:   make(map[string]int64),
		escalationCounts: make(map[string]int64),
		metricsFile:      metricsFile,
		flushInterval:    5 * time.Minute,
	}
}

// RecordIncident increments the count for the given symptom type.
func (mc *MetricsCollector) RecordIncident(symptom string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.incidentCounts[symptom]++
	mc.totalIncidents++
}

// RecordEscalation increments the count for the given escalation action.
func (mc *MetricsCollector) RecordEscalation(action string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.escalationCounts[action]++
	mc.totalEscalations++
}

// RecordSuppression increments the dedup suppression counter.
func (mc *MetricsCollector) RecordSuppression() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.dedupSuppressions++
}

// Snapshot returns a copy of current metrics.
func (mc *MetricsCollector) Snapshot() MetricsSnapshot {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	incidents := make(map[string]int64, len(mc.incidentCounts))
	for k, v := range mc.incidentCounts {
		incidents[k] = v
	}
	escalations := make(map[string]int64, len(mc.escalationCounts))
	for k, v := range mc.escalationCounts {
		escalations[k] = v
	}

	return MetricsSnapshot{
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		IncidentCounts:    incidents,
		EscalationCounts:  escalations,
		DedupSuppressions: mc.dedupSuppressions,
		TotalIncidents:    mc.totalIncidents,
		TotalEscalations:  mc.totalEscalations,
	}
}

// FlushIfDue writes metrics to disk if the flush interval has elapsed.
// Returns true if flush occurred.
func (mc *MetricsCollector) FlushIfDue() (bool, error) {
	mc.mu.Lock()
	if time.Since(mc.lastFlush) < mc.flushInterval {
		mc.mu.Unlock()
		return false, nil
	}
	mc.mu.Unlock()

	return true, mc.Flush()
}

// Flush writes the current metrics snapshot to the configured file.
func (mc *MetricsCollector) Flush() error {
	snapshot := mc.Snapshot()

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(mc.metricsFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	mc.mu.Lock()
	mc.lastFlush = time.Now()
	mc.mu.Unlock()

	return os.WriteFile(mc.metricsFile, data, 0o644)
}
