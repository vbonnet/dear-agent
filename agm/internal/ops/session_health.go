package ops

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// SessionHealthRequest defines the input for a session health check.
type SessionHealthRequest struct {
	// Identifier is a session ID, name, or UUID prefix (empty for --all).
	Identifier string `json:"identifier,omitempty"`

	// All checks all active sessions.
	All bool `json:"all,omitempty"`
}

// SessionHealthResult is the output of CheckSessionHealth.
type SessionHealthResult struct {
	Operation string                `json:"operation"`
	Sessions  []SessionHealthDetail `json:"sessions"`
	Total     int                   `json:"total"`
}

// SessionHealthDetail contains per-session health information.
type SessionHealthDetail struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Status string `json:"status"` // active, stopped, archived

	// State is the AGM state (WORKING, USER_PROMPT, DONE, etc.)
	State string `json:"state"`

	// Responsiveness
	TimeSinceLastUpdate string `json:"time_since_last_update"`
	LastUpdateAt        string `json:"last_update_at"`

	// Resource usage (populated only for active sessions with tmux pane)
	PanePID   int     `json:"pane_pid,omitempty"`
	CPUPct    float64 `json:"cpu_pct,omitempty"`
	MemoryMB  float64 `json:"memory_mb,omitempty"`
	MemoryPct float64 `json:"memory_pct,omitempty"`

	// Error rate from recent pane output
	ErrorLines int `json:"error_lines"`

	// Git commits on the working directory's branch since session started
	CommitCount int `json:"commit_count"`

	// Duration
	Duration  string `json:"duration"`
	StartedAt string `json:"started_at"`

	// Trust score
	TrustScore *int `json:"trust_score,omitempty"`

	// Overall health assessment
	Health   string   `json:"health"` // healthy, warning, critical
	Warnings []string `json:"warnings,omitempty"`
}

// CheckSessionHealth performs a deep health check on one or more sessions.
func CheckSessionHealth(ctx *OpContext, req *SessionHealthRequest) (*SessionHealthResult, error) {
	if req == nil {
		return nil, ErrInvalidInput("request", "Health check request is required.")
	}

	if !req.All && req.Identifier == "" {
		return nil, ErrInvalidInput("identifier", "Session identifier is required, or use --all for all sessions.")
	}

	var manifests []*manifest.Manifest

	if req.All {
		ms, err := ctx.Storage.ListSessions(nil)
		if err != nil {
			return nil, ErrStorageError("list_sessions", err)
		}
		// Filter to non-archived only
		for _, m := range ms {
			if m.Lifecycle != manifest.LifecycleArchived {
				manifests = append(manifests, m)
			}
		}
	} else {
		m, err := ctx.Storage.GetSession(req.Identifier)
		if err != nil {
			m, err = findByName(ctx, req.Identifier)
			if err != nil {
				return nil, err
			}
		}
		if m == nil {
			return nil, ErrSessionNotFound(req.Identifier)
		}
		manifests = []*manifest.Manifest{m}
	}

	// Compute statuses in batch
	statuses := make(map[string]string)
	if ctx.Tmux != nil {
		statuses = computeStatuses(manifests, ctx.Tmux)
	}

	details := make([]SessionHealthDetail, 0, len(manifests))
	for _, m := range manifests {
		detail := buildHealthDetail(m, statuses)
		details = append(details, detail)
	}

	return &SessionHealthResult{
		Operation: "session_health",
		Sessions:  details,
		Total:     len(details),
	}, nil
}

// buildHealthDetail constructs a full health detail for a single session.
func buildHealthDetail(m *manifest.Manifest, statuses map[string]string) SessionHealthDetail {
	status := "active"
	if m.Lifecycle == manifest.LifecycleArchived {
		status = "archived"
	} else if s, ok := statuses[m.Name]; ok {
		status = s
	}

	now := time.Now()
	duration := now.Sub(m.CreatedAt)
	timeSinceUpdate := now.Sub(m.UpdatedAt)

	d := SessionHealthDetail{
		Name:                m.Name,
		ID:                  m.SessionID,
		Status:              status,
		State:               m.State,
		TimeSinceLastUpdate: formatDuration(timeSinceUpdate),
		LastUpdateAt:        m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		Duration:            formatDuration(duration),
		StartedAt:           m.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}

	// Resource usage — only for active sessions
	if status == "active" {
		populateResourceUsage(&d, tmuxName)
		populateErrorRate(&d, tmuxName)
	}

	// Git commits
	if m.WorkingDirectory != "" {
		d.CommitCount = countCommitsSince(m.WorkingDirectory, m.CreatedAt)
	}

	// Trust score
	trustResult, err := TrustScore(nil, &TrustScoreRequest{SessionName: m.Name})
	if err == nil && trustResult != nil {
		d.TrustScore = &trustResult.Score
	}

	// Compute overall health
	d.Health, d.Warnings = assessHealth(d, status)

	return d
}

// populateResourceUsage fills CPU and memory fields from the tmux pane PID.
func populateResourceUsage(d *SessionHealthDetail, tmuxName string) {
	pid, err := tmux.GetPanePID(tmuxName)
	if err != nil || pid == 0 {
		return
	}
	d.PanePID = pid

	// Get process tree resource usage via ps
	out, err := exec.Command("ps", "-o", "pcpu=,rss=", "--pid", strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		return
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) >= 2 {
		d.CPUPct, _ = strconv.ParseFloat(fields[0], 64)
		rssKB, _ := strconv.ParseFloat(fields[1], 64)
		d.MemoryMB = rssKB / 1024.0

		// Calculate memory percentage from total system memory
		d.MemoryPct = getMemoryPercent(rssKB)
	}
}

// getMemoryPercent calculates the percentage of total system memory used.
func getMemoryPercent(rssKB float64) float64 {
	out, err := exec.Command("grep", "MemTotal", "/proc/meminfo").CombinedOutput()
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(out))
	if len(fields) >= 2 {
		totalKB, _ := strconv.ParseFloat(fields[1], 64)
		if totalKB > 0 {
			return (rssKB / totalKB) * 100.0
		}
	}
	return 0
}

// populateErrorRate counts error lines in recent pane output.
func populateErrorRate(d *SessionHealthDetail, tmuxName string) {
	output, err := tmux.CapturePaneOutput(tmuxName, 200)
	if err != nil {
		return
	}

	lines := strings.Split(output, "\n")
	errorCount := 0
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "panic") ||
			strings.Contains(lower, "fatal") {
			errorCount++
		}
	}
	d.ErrorLines = errorCount
}

// countCommitsSince counts git commits in the working directory since a given time.
func countCommitsSince(workDir string, since time.Time) int {
	sinceStr := since.Format("2006-01-02T15:04:05")
	out, err := exec.Command("git", "-C", workDir, "log", "--oneline",
		"--since="+sinceStr, "--no-merges").CombinedOutput()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}

// assessHealth determines overall health status and warnings.
func assessHealth(d SessionHealthDetail, status string) (string, []string) {
	slo := contracts.Load()
	sh := slo.SessionHealth

	if status == "stopped" {
		return "stopped", nil
	}
	if status == "archived" {
		return "archived", nil
	}

	health := "healthy"
	var warnings []string
	health, warnings = assessResponsiveness(d, sh, health, warnings)
	health, warnings = assessState(d, health, warnings)
	health, warnings = assessErrorRate(d, sh, health, warnings)
	health, warnings = assessCPU(d, sh, health, warnings)
	health, warnings = assessMemory(d, sh, health, warnings)
	health, warnings = assessTrustScore(d, sh, health, warnings)
	return health, warnings
}

// promoteToWarning returns "critical" if old is critical, otherwise "warning".
func promoteToWarning(old string) string {
	if old == "critical" {
		return "critical"
	}
	return "warning"
}

func assessResponsiveness(d SessionHealthDetail, sh contracts.SessionHealth, health string, warnings []string) (string, []string) {
	sinceUpdate := parseDurationFromFormatted(d.TimeSinceLastUpdate)
	if sinceUpdate > sh.ResponsivenessCriticalTimeout.Duration {
		warnings = append(warnings, fmt.Sprintf("No manifest update in %s", d.TimeSinceLastUpdate))
		return "critical", warnings
	}
	if sinceUpdate > sh.ResponsivenessWarningTimeout.Duration {
		warnings = append(warnings, fmt.Sprintf("No manifest update in %s", d.TimeSinceLastUpdate))
		return promoteToWarning(health), warnings
	}
	return health, warnings
}

func assessState(d SessionHealthDetail, health string, warnings []string) (string, []string) {
	if d.State != "PERMISSION_PROMPT" {
		return health, warnings
	}
	warnings = append(warnings, "Session waiting on permission prompt")
	return promoteToWarning(health), warnings
}

func assessErrorRate(d SessionHealthDetail, sh contracts.SessionHealth, health string, warnings []string) (string, []string) {
	if d.ErrorLines > sh.ErrorLinesCritical {
		warnings = append(warnings, fmt.Sprintf("High error rate: %d error lines in recent output", d.ErrorLines))
		return "critical", warnings
	}
	if d.ErrorLines > sh.ErrorLinesWarning {
		warnings = append(warnings, fmt.Sprintf("Elevated error rate: %d error lines in recent output", d.ErrorLines))
		return promoteToWarning(health), warnings
	}
	return health, warnings
}

func assessCPU(d SessionHealthDetail, sh contracts.SessionHealth, health string, warnings []string) (string, []string) {
	if d.CPUPct <= sh.CPUWarningPercent {
		return health, warnings
	}
	warnings = append(warnings, fmt.Sprintf("High CPU usage: %.1f%%", d.CPUPct))
	return promoteToWarning(health), warnings
}

func assessMemory(d SessionHealthDetail, sh contracts.SessionHealth, health string, warnings []string) (string, []string) {
	if d.MemoryMB <= float64(sh.MemoryWarningMB) {
		return health, warnings
	}
	warnings = append(warnings, fmt.Sprintf("High memory usage: %.0f MB", d.MemoryMB))
	return promoteToWarning(health), warnings
}

func assessTrustScore(d SessionHealthDetail, sh contracts.SessionHealth, health string, warnings []string) (string, []string) {
	if d.TrustScore == nil || *d.TrustScore >= sh.TrustScoreWarning {
		return health, warnings
	}
	warnings = append(warnings, fmt.Sprintf("Low trust score: %d", *d.TrustScore))
	return promoteToWarning(health), warnings
}

// parseDurationFromFormatted parses a formatted duration string back to time.Duration.
// Handles formats like "5m", "2h15m", "3d", "45s".
func parseDurationFromFormatted(s string) time.Duration {
	// Try standard Go duration parsing first for simple cases
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}

	var total time.Duration
	current := ""
	for _, ch := range s {
		switch ch {
		case 'd':
			n, _ := strconv.Atoi(current)
			total += time.Duration(n) * 24 * time.Hour
			current = ""
		case 'h':
			n, _ := strconv.Atoi(current)
			total += time.Duration(n) * time.Hour
			current = ""
		case 'm':
			n, _ := strconv.Atoi(current)
			total += time.Duration(n) * time.Minute
			current = ""
		case 's':
			n, _ := strconv.Atoi(current)
			total += time.Duration(n) * time.Second
			current = ""
		default:
			current += string(ch)
		}
	}
	return total
}
