package deep_research

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/workflow"
)

// ResearchLogger handles crash-resilient logging for deep-research sessions.
type ResearchLogger struct {
	logPath   string
	sessionID string
	urls      []string
	results   map[string]*URLResult
	mutex     sync.Mutex
	startTime time.Time
}

// URLResult tracks the result of researching a single URL.
type URLResult struct {
	URL        string
	Status     string // "pending", "in_progress", "completed", "failed"
	ReportPath string
	Error      string
	StartTime  time.Time
	EndTime    time.Time
	Duration   time.Duration
}

// NewResearchLogger creates a new research logger.
func NewResearchLogger(sessionID string, urls []string, outputDir string) (*ResearchLogger, error) {
	// Create log file path
	if outputDir == "" {
		outputDir = "."
	}

	logPath := filepath.Join(outputDir, fmt.Sprintf("research-%s-log.md", sessionID))

	logger := &ResearchLogger{
		logPath:   logPath,
		sessionID: sessionID,
		urls:      urls,
		results:   make(map[string]*URLResult),
		startTime: time.Now(),
	}

	// Initialize results for all URLs
	for _, url := range urls {
		logger.results[url] = &URLResult{
			URL:    url,
			Status: "pending",
		}
	}

	// Try to resume from existing log
	if err := logger.tryResume(); err != nil {
		// If resume fails, start fresh
		if err := logger.initializeLog(); err != nil {
			return nil, fmt.Errorf("initialize log: %w", err)
		}
	}

	return logger, nil
}

// tryResume attempts to resume from an existing log file.
func (l *ResearchLogger) tryResume() error {
	// Check if log file exists
	if _, err := os.Stat(l.logPath); os.IsNotExist(err) {
		return fmt.Errorf("log file does not exist")
	}

	// Read existing log
	content, err := os.ReadFile(l.logPath)
	if err != nil {
		return fmt.Errorf("read log file: %w", err)
	}

	// Parse log to extract completed URLs
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		// Look for completed URL markers: "- [x] https://url.com"
		if strings.HasPrefix(line, "- [x] ") {
			// Extract URL
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				url := parts[2]
				if result, exists := l.results[url]; exists {
					result.Status = "completed"
					// Try to extract report path from the line
					if strings.Contains(line, "report:") {
						// Format: "- [x] url (completed at HH:MM:SS, report: path)"
						if idx := strings.Index(line, "report: "); idx != -1 {
							rest := line[idx+8:]
							if endIdx := strings.Index(rest, ")"); endIdx != -1 {
								result.ReportPath = strings.TrimSpace(rest[:endIdx])
							}
						}
					}
				}
			}
		}
	}

	fmt.Printf("✓ Resumed from existing log: %s\n", l.logPath)
	return nil
}

// initializeLog creates a new log file with initial structure.
func (l *ResearchLogger) initializeLog() error {
	var content strings.Builder

	content.WriteString("# Deep Research Session Log\n\n")
	content.WriteString(fmt.Sprintf("**Session ID**: %s\n", l.sessionID))
	content.WriteString(fmt.Sprintf("**Started**: %s\n", l.startTime.Format(time.RFC3339)))
	content.WriteString(fmt.Sprintf("**URLs**: %d total\n\n", len(l.urls)))

	content.WriteString("## Progress\n\n")
	for _, url := range l.urls {
		content.WriteString(fmt.Sprintf("- [ ] %s (pending)\n", url))
	}

	content.WriteString("\n## Results\n\n")
	for i, url := range l.urls {
		content.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, url))
		content.WriteString("Status: ⏸️ Pending\n\n")
	}

	content.WriteString("## Proposals\n\n")
	content.WriteString("_Will be generated after all research completes_\n")

	// Ensure parent directory exists
	dir := filepath.Dir(l.logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write to file
	if err := os.WriteFile(l.logPath, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("write log file: %w", err)
	}

	fmt.Printf("✓ Initialized log: %s\n", l.logPath)
	return nil
}

// MarkStarted marks a URL as started.
func (l *ResearchLogger) MarkStarted(url string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if result, exists := l.results[url]; exists {
		result.Status = "in_progress"
		result.StartTime = time.Now()
		l.updateLog()
	}
}

// MarkCompleted marks a URL as completed and updates the log.
func (l *ResearchLogger) MarkCompleted(url, reportPath string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if result, exists := l.results[url]; exists {
		result.Status = "completed"
		result.ReportPath = reportPath
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		l.updateLog()
	}
}

// MarkFailed marks a URL as failed and updates the log.
func (l *ResearchLogger) MarkFailed(url string, err error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if result, exists := l.results[url]; exists {
		result.Status = "failed"
		result.Error = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		l.updateLog()
	}
}

// GetCompletedURLs returns URLs that have already been researched.
func (l *ResearchLogger) GetCompletedURLs() []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	var completed []string
	for _, url := range l.urls {
		if result := l.results[url]; result.Status == "completed" {
			completed = append(completed, url)
		}
	}
	return completed
}

// GetPendingURLs returns URLs that still need to be researched.
func (l *ResearchLogger) GetPendingURLs() []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	var pending []string
	for _, url := range l.urls {
		if result := l.results[url]; result.Status == "pending" {
			pending = append(pending, url)
		}
	}
	return pending
}

// updateLog rewrites the log file with current state.
func (l *ResearchLogger) updateLog() {
	var content strings.Builder

	content.WriteString("# Deep Research Session Log\n\n")
	content.WriteString(fmt.Sprintf("**Session ID**: %s\n", l.sessionID))
	content.WriteString(fmt.Sprintf("**Started**: %s\n", l.startTime.Format(time.RFC3339)))
	content.WriteString(fmt.Sprintf("**URLs**: %d total\n\n", len(l.urls)))

	content.WriteString("## Progress\n\n")
	for _, url := range l.urls {
		result := l.results[url]
		checkbox := "[ ]"
		status := "pending"
		if result.Status == "completed" {
			checkbox = "[x]"
			status = fmt.Sprintf("completed at %s, report: %s", result.EndTime.Format("15:04:05"), result.ReportPath)
		} else if result.Status == "failed" {
			checkbox = "[x]"
			status = fmt.Sprintf("failed: %s", result.Error)
		} else if result.Status == "in_progress" {
			status = "in progress..."
		}
		content.WriteString(fmt.Sprintf("- %s %s (%s)\n", checkbox, url, status))
	}

	content.WriteString("\n## Results\n\n")
	for i, url := range l.urls {
		result := l.results[url]
		content.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, url))

		switch result.Status {
		case "completed":
			content.WriteString("Status: ✅ Complete\n")
			content.WriteString(fmt.Sprintf("Completed: %s\n", result.EndTime.Format("15:04:05")))
			content.WriteString(fmt.Sprintf("Report: %s\n", result.ReportPath))
			content.WriteString(fmt.Sprintf("Duration: %s\n\n", formatDuration(result.Duration)))
		case "failed":
			content.WriteString("Status: ❌ Failed\n")
			content.WriteString(fmt.Sprintf("Error: %s\n\n", result.Error))
		case "in_progress":
			content.WriteString("Status: 🔄 In Progress\n")
			content.WriteString(fmt.Sprintf("Started: %s\n\n", result.StartTime.Format("15:04:05")))
		default:
			content.WriteString("Status: ⏸️ Pending\n\n")
		}
	}

	content.WriteString("## Proposals\n\n")
	content.WriteString("_Will be generated after all research completes_\n")

	// Write to file (ignore errors during update, best effort)
	os.WriteFile(l.logPath, []byte(content.String()), 0644)
}

// AddProposals adds the generated proposals to the log.
func (l *ResearchLogger) AddProposals(result ApplicationResult) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Read current log
	content, err := os.ReadFile(l.logPath)
	if err != nil {
		return fmt.Errorf("read log file: %w", err)
	}

	// Replace proposals section
	logContent := string(content)
	proposalsStart := strings.Index(logContent, "## Proposals")
	if proposalsStart == -1 {
		return fmt.Errorf("proposals section not found in log")
	}

	// Keep everything before proposals section
	updatedContent := logContent[:proposalsStart]

	// Add new proposals section
	var proposalsContent strings.Builder
	proposalsContent.WriteString("## Proposals\n\n")
	proposalsContent.WriteString(fmt.Sprintf("**Generated**: %s\n", time.Now().Format(time.RFC3339)))
	proposalsContent.WriteString(fmt.Sprintf("**Summary**: %s\n\n", result.Summary))
	proposalsContent.WriteString("---\n\n")

	// Add proposals by repository
	for repo, proposals := range result.Proposals {
		proposalsContent.WriteString(fmt.Sprintf("### %s Proposals\n\n", repo))
		for i, proposal := range proposals {
			proposalsContent.WriteString(fmt.Sprintf("#### %d. %s\n\n", i+1, proposal.Title))
			proposalsContent.WriteString(fmt.Sprintf("**Category**: %s  \n", proposal.Category))
			proposalsContent.WriteString(fmt.Sprintf("**Priority**: %s\n\n", proposal.Priority))
			proposalsContent.WriteString(fmt.Sprintf("%s\n\n", proposal.Description))

			if len(proposal.TestableIdeas) > 0 {
				proposalsContent.WriteString("**Testable Ideas**:\n")
				for _, idea := range proposal.TestableIdeas {
					proposalsContent.WriteString(fmt.Sprintf("- %s\n", idea))
				}
				proposalsContent.WriteString("\n")
			}
		}
	}

	// Add cross-cutting ideas
	if len(result.CrossCuttingIdeas) > 0 {
		proposalsContent.WriteString("### Cross-Cutting Ideas\n\n")
		for _, idea := range result.CrossCuttingIdeas {
			proposalsContent.WriteString(fmt.Sprintf("- %s\n", idea))
		}
		proposalsContent.WriteString("\n")
	}

	updatedContent += proposalsContent.String()

	// Write updated log
	if err := os.WriteFile(l.logPath, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("write updated log: %w", err)
	}

	return nil
}

// GetLogPath returns the path to the log file.
func (l *ResearchLogger) GetLogPath() string {
	return l.logPath
}

// GetArtifacts returns workflow artifacts from completed research.
func (l *ResearchLogger) GetArtifacts() []workflow.Artifact {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	var artifacts []workflow.Artifact

	for _, url := range l.urls {
		result := l.results[url]
		if result.Status == "completed" && result.ReportPath != "" {
			artifact := workflow.Artifact{
				Type: "research-report",
				Path: result.ReportPath,
				Metadata: map[string]interface{}{
					"url": url,
				},
			}

			// Get file size
			if stat, err := os.Stat(result.ReportPath); err == nil {
				artifact.Size = stat.Size()
			}

			artifacts = append(artifacts, artifact)
		}
	}

	return artifacts
}

// formatDuration formats a duration in human-readable format.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm%.0fs", d.Minutes(), d.Seconds()-d.Minutes()*60)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) - hours*60
	return fmt.Sprintf("%dh%dm", hours, minutes)
}
