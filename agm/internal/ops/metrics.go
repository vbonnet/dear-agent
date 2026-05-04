package ops

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"golang.org/x/sys/unix"
)

// MetricsRequest defines input for collecting system metrics.
type MetricsRequest struct {
	// Window is the duration to look back for throughput metrics.
	// Defaults to 1 hour.
	Window time.Duration `json:"window,omitempty"`
}

// MetricsResult is the full metrics payload.
type MetricsResult struct {
	Operation  string           `json:"operation"`
	Timestamp  string           `json:"timestamp"`
	Sessions   SessionMetrics   `json:"sessions"`
	Throughput ThroughputMetrics `json:"throughput"`
	Cost       CostMetrics      `json:"cost"`
	Resources  ResourceMetrics  `json:"resources"`
	Alerts     []Alert          `json:"alerts"`
	Workflow   *WorkflowMetrics `json:"workflow,omitempty"`
	Batch      *BatchMetrics    `json:"batch,omitempty"`
}

// CostMetrics tracks aggregate spending across sessions.
type CostMetrics struct {
	TotalSpend    float64 `json:"total_spend"`
	CostPerWorker float64 `json:"cost_per_worker"`
	CostPerCommit float64 `json:"cost_per_commit"`
	WorkerCount   int     `json:"worker_count"`
	CommitCount   int     `json:"commit_count"`
}

// SessionMetrics provides session counts grouped by status.
type SessionMetrics struct {
	Total    int            `json:"total"`
	ByState  map[string]int `json:"by_state"`
	Active   int            `json:"active"`
	Stopped  int            `json:"stopped"`
	Archived int            `json:"archived"`
}

// ThroughputMetrics tracks work output rates.
type ThroughputMetrics struct {
	CommitsPerHour  int `json:"commits_per_hour"`
	WorkersLaunched int `json:"workers_launched"`
	WindowSeconds   int `json:"window_seconds"`
}

// ResourceMetrics tracks system resource usage.
type ResourceMetrics struct {
	Load   LoadMetrics   `json:"load"`
	Memory MemoryMetrics `json:"memory"`
	Disk   []DiskMetrics `json:"disk"`
}

// LoadMetrics tracks system load averages.
type LoadMetrics struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// MemoryMetrics tracks RAM usage.
type MemoryMetrics struct {
	TotalMB     int     `json:"total_mb"`
	UsedMB      int     `json:"used_mb"`
	AvailableMB int     `json:"available_mb"`
	UsedPercent float64 `json:"used_percent"`
}

// DiskMetrics tracks disk usage for a mount point.
type DiskMetrics struct {
	Mount       string  `json:"mount"`
	TotalGB     float64 `json:"total_gb"`
	UsedGB      float64 `json:"used_gb"`
	AvailGB     float64 `json:"avail_gb"`
	UsedPercent float64 `json:"used_percent"`
}

// Alert represents a threshold violation.
type Alert struct {
	Level   string `json:"level"`   // "warning", "critical"
	Type    string `json:"type"`    // "load", "memory", "disk", "throughput"
	Message string `json:"message"`
	Value   string `json:"value"`
}

// GetMetrics collects system-wide metrics for the AGM installation.
func GetMetrics(ctx *OpContext, req *MetricsRequest) (*MetricsResult, error) {
	if req == nil {
		req = &MetricsRequest{}
	}
	window := req.Window
	if window <= 0 {
		window = time.Hour
	}

	now := time.Now()

	// Collect session metrics
	sessionMetrics, sessionList, err := collectSessionMetrics(ctx)
	if err != nil {
		return nil, err
	}

	// Collect throughput metrics
	throughput := collectThroughputMetrics(sessionList, now, window)

	// Collect cost metrics
	costMetrics := collectCostMetrics(sessionList)

	// Collect resource metrics
	resources := collectResourceMetrics()

	// Generate alerts
	alerts := generateAlerts(resources, throughput, sessionList, now)

	// Collect workflow metrics (last execution)
	workflowMetrics, _ := LoadWorkflowMetrics()

	// Collect batch metrics from session data
	batchMetrics := CollectBatchMetrics(sessionList)

	return &MetricsResult{
		Operation:  "metrics",
		Timestamp:  now.Format(time.RFC3339),
		Sessions:   sessionMetrics,
		Throughput: throughput,
		Cost:       costMetrics,
		Resources:  resources,
		Alerts:     alerts,
		Workflow:   workflowMetrics,
		Batch:      batchMetrics,
	}, nil
}

// collectSessionMetrics gathers session counts by status.
func collectSessionMetrics(ctx *OpContext) (SessionMetrics, []SessionSummary, error) {
	listResult, err := ListSessions(ctx, &ListSessionsRequest{
		Status: "all",
		Limit:  1000,
	})
	if err != nil {
		return SessionMetrics{}, nil, fmt.Errorf("listing sessions: %w", err)
	}

	metrics := SessionMetrics{
		Total:   len(listResult.Sessions),
		ByState: make(map[string]int),
	}

	for _, s := range listResult.Sessions {
		// Count by Status (lifecycle) since State was removed
		metrics.ByState[s.Status]++

		switch s.Status {
		case "active":
			metrics.Active++
		case "stopped":
			metrics.Stopped++
		case "archived":
			metrics.Archived++
		}
	}

	return metrics, listResult.Sessions, nil
}

// collectThroughputMetrics calculates work output rates.
func collectThroughputMetrics(sessions []SessionSummary, now time.Time, window time.Duration) ThroughputMetrics {
	windowStart := now.Add(-window)

	workersLaunched := 0
	for _, s := range sessions {
		created, err := time.Parse("2006-01-02T15:04:05Z", s.CreatedAt)
		if err != nil {
			continue
		}
		if created.After(windowStart) {
			workersLaunched++
		}
	}

	return ThroughputMetrics{
		CommitsPerHour:  -1, // Sentinel: requires git scanning, populated by CLI layer
		WorkersLaunched: workersLaunched,
		WindowSeconds:   int(window.Seconds()),
	}
}

// collectCostMetrics aggregates cost data across sessions.
// CommitCount is set to -1 as a sentinel; the CLI layer populates the real value.
// WorkerCount only includes non-archived sessions tagged with "role:worker".
func collectCostMetrics(sessions []SessionSummary) CostMetrics {
	var totalSpend float64
	workerCount := 0
	for _, s := range sessions {
		totalSpend += s.EstimatedCost
		if s.Status != "archived" && hasWorkerTag(s.Tags) {
			workerCount++
		}
	}

	var costPerWorker float64
	if workerCount > 0 {
		costPerWorker = totalSpend / float64(workerCount)
	}

	return CostMetrics{
		TotalSpend:    totalSpend,
		CostPerWorker: costPerWorker,
		CostPerCommit: -1, // Sentinel: requires git scanning, populated by CLI layer
		WorkerCount:   workerCount,
		CommitCount:   -1,
	}
}

// collectResourceMetrics gathers system resource information.
func collectResourceMetrics() ResourceMetrics {
	return ResourceMetrics{
		Load:   readLoadAvg(),
		Memory: readMemoryInfo(),
		Disk:   readDiskUsage(),
	}
}

// readLoadAvg reads system load averages from /proc/loadavg.
func readLoadAvg() LoadMetrics {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return LoadMetrics{}
	}
	return parseLoadAvg(string(data))
}

// parseLoadAvg parses /proc/loadavg content.
func parseLoadAvg(content string) LoadMetrics {
	fields := strings.Fields(content)
	if len(fields) < 3 {
		return LoadMetrics{}
	}

	load1, _ := strconv.ParseFloat(fields[0], 64)
	load5, _ := strconv.ParseFloat(fields[1], 64)
	load15, _ := strconv.ParseFloat(fields[2], 64)

	return LoadMetrics{
		Load1:  load1,
		Load5:  load5,
		Load15: load15,
	}
}

// readMemoryInfo reads RAM usage from /proc/meminfo.
func readMemoryInfo() MemoryMetrics {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemoryMetrics{}
	}
	return parseMemInfo(string(data))
}

// parseMemInfo parses /proc/meminfo content.
func parseMemInfo(content string) MemoryMetrics {
	values := make(map[string]int64)
	for _, line := range strings.Split(content, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		val, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			continue
		}
		values[key] = val
	}

	totalKB := values["MemTotal"]
	availKB := values["MemAvailable"]
	usedKB := totalKB - availKB

	totalMB := int(totalKB / 1024)
	usedMB := int(usedKB / 1024)
	availMB := int(availKB / 1024)

	var usedPct float64
	if totalKB > 0 {
		usedPct = float64(usedKB) / float64(totalKB) * 100
	}

	return MemoryMetrics{
		TotalMB:     totalMB,
		UsedMB:      usedMB,
		AvailableMB: availMB,
		UsedPercent: roundTo1(usedPct),
	}
}

// readDiskUsage reads disk usage for / and /home using statfs.
func readDiskUsage() []DiskMetrics {
	mounts := []string{"/", "/home"}
	var results []DiskMetrics

	for _, mount := range mounts {
		dm := statfsDisk(mount)
		if dm.TotalGB > 0 {
			results = append(results, dm)
		}
	}

	// Deduplicate if /home is on the same filesystem as /
	if len(results) == 2 && results[0].TotalGB == results[1].TotalGB &&
		results[0].UsedGB == results[1].UsedGB {
		results = results[:1]
	}

	return results
}

// statfsDisk gets disk usage for a mount point using the statfs syscall.
func statfsDisk(mount string) DiskMetrics {
	var stat unix.Statfs_t
	if err := unix.Statfs(mount, &stat); err != nil {
		return DiskMetrics{Mount: mount}
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - (stat.Bfree * uint64(stat.Bsize))

	totalGB := float64(totalBytes) / (1024 * 1024 * 1024)
	usedGB := float64(usedBytes) / (1024 * 1024 * 1024)
	availGB := float64(freeBytes) / (1024 * 1024 * 1024)

	var usedPct float64
	if totalBytes > 0 {
		usedPct = float64(usedBytes) / float64(totalBytes) * 100
	}

	return DiskMetrics{
		Mount:       mount,
		TotalGB:     roundTo1(totalGB),
		UsedGB:      roundTo1(usedGB),
		AvailGB:     roundTo1(availGB),
		UsedPercent: roundTo1(usedPct),
	}
}

// generateAlerts checks thresholds and returns violations.
// NOTE: PERMISSION_PROMPT alert was removed — State field produced false
// positives that caused hours of deadlock. Will be reimplemented with
// capture-pane-based ground truth.
func generateAlerts(res ResourceMetrics, tp ThroughputMetrics, _ []SessionSummary, now time.Time) []Alert {
	slo := contracts.Load()
	oa := slo.OpsAlerts

	var alerts []Alert

	if res.Load.Load1 > oa.LoadThreshold {
		alerts = append(alerts, Alert{
			Level:   "critical",
			Type:    "load",
			Message: fmt.Sprintf("System load %.1f exceeds threshold %.0f", res.Load.Load1, oa.LoadThreshold),
			Value:   fmt.Sprintf("%.1f", res.Load.Load1),
		})
	}

	if res.Memory.UsedPercent > oa.MemoryThresholdPercent {
		alerts = append(alerts, Alert{
			Level:   "warning",
			Type:    "memory",
			Message: fmt.Sprintf("RAM usage %.1f%% exceeds threshold %.0f%%", res.Memory.UsedPercent, oa.MemoryThresholdPercent),
			Value:   fmt.Sprintf("%.1f%%", res.Memory.UsedPercent),
		})
	}

	for _, d := range res.Disk {
		if d.UsedPercent > oa.DiskThresholdPercent {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Type:    "disk",
				Message: fmt.Sprintf("Disk %s usage %.1f%% exceeds threshold %.0f%%", d.Mount, d.UsedPercent, oa.DiskThresholdPercent),
				Value:   fmt.Sprintf("%.1f%%", d.UsedPercent),
			})
		}
	}

	if tp.CommitsPerHour == 0 {
		alerts = append(alerts, Alert{
			Level:   "warning",
			Type:    "throughput",
			Message: "No commits in the last hour",
			Value:   "0",
		})
	}

	return alerts
}

// hasWorkerTag returns true if the tag list contains "role:worker".
func hasWorkerTag(tags []string) bool {
	for _, t := range tags {
		if t == "role:worker" {
			return true
		}
	}
	return false
}

// roundTo1 rounds a float to 1 decimal place.
func roundTo1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
