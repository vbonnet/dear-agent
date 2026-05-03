package hippocampus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DailyLogger writes KAIROS-lite daily log files organized as logs/YYYY/MM/YYYY-MM-DD.md.
type DailyLogger struct {
	baseDir string // root directory for logs (e.g., ~/.engram/logs)
}

// LogEntry represents a single entry in the daily log.
type LogEntry struct {
	Timestamp time.Time
	Category  string // "command", "decision", "artifact", "session_start", "session_end"
	Content   string
}

// NewDailyLogger creates a logger rooted at the given directory.
func NewDailyLogger(baseDir string) *DailyLogger {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".engram", "logs")
	}
	return &DailyLogger{baseDir: baseDir}
}

// GetLogPath returns the file path for a given date: baseDir/YYYY/MM/YYYY-MM-DD.md
func (dl *DailyLogger) GetLogPath(date time.Time) string {
	year := date.Format("2006")
	month := date.Format("01")
	day := date.Format("2006-01-02")
	return filepath.Join(dl.baseDir, year, month, day+".md")
}

// AppendEntry adds an entry to today's log file, creating the directory
// structure and file header if needed.
func (dl *DailyLogger) AppendEntry(entry LogEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	logPath := dl.GetLogPath(entry.Timestamp)
	dir := filepath.Dir(logPath)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	// Check if file exists; create with header if not
	isNew := false
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		isNew = true
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	if isNew {
		header := fmt.Sprintf("# Daily Log — %s\n\n", entry.Timestamp.Format("2006-01-02"))
		if _, err := f.WriteString(header); err != nil {
			return fmt.Errorf("write header: %w", err)
		}
	}

	// Format entry
	timeStr := entry.Timestamp.Format("15:04")
	line := fmt.Sprintf("- **%s** [%s] %s\n", timeStr, entry.Category, entry.Content)
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}

	return nil
}

// LogSessionStart records a session start event.
func (dl *DailyLogger) LogSessionStart(sessionID, project string) error {
	content := fmt.Sprintf("Session started: %s", sessionID)
	if project != "" {
		content += fmt.Sprintf(" (project: %s)", project)
	}
	return dl.AppendEntry(LogEntry{
		Timestamp: time.Now(),
		Category:  "session_start",
		Content:   content,
	})
}

// LogSessionEnd records a session end event.
func (dl *DailyLogger) LogSessionEnd(sessionID string) error {
	return dl.AppendEntry(LogEntry{
		Timestamp: time.Now(),
		Category:  "session_end",
		Content:   fmt.Sprintf("Session ended: %s", sessionID),
	})
}

// LogCommand records a command execution.
func (dl *DailyLogger) LogCommand(command string) error {
	return dl.AppendEntry(LogEntry{
		Timestamp: time.Now(),
		Category:  "command",
		Content:   command,
	})
}

// LogDecisionEntry records a decision made during a session.
func (dl *DailyLogger) LogDecisionEntry(decision string) error {
	return dl.AppendEntry(LogEntry{
		Timestamp: time.Now(),
		Category:  "decision",
		Content:   decision,
	})
}

// LogArtifact records a key artifact created or modified.
func (dl *DailyLogger) LogArtifact(artifact string) error {
	return dl.AppendEntry(LogEntry{
		Timestamp: time.Now(),
		Category:  "artifact",
		Content:   artifact,
	})
}

// ReadDailyLog reads the contents of a daily log file for a given date.
func (dl *DailyLogger) ReadDailyLog(date time.Time) (string, error) {
	logPath := dl.GetLogPath(date)
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ListLogFiles returns paths to all daily log files, sorted chronologically.
func (dl *DailyLogger) ListLogFiles() ([]string, error) {
	var logs []string

	err := filepath.Walk(dl.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			logs = append(logs, path)
		}
		return nil
	})

	return logs, err
}

// FeedToAutodream returns log file paths from the last N days that can be
// used as additional signal sources for the autodream consolidation pipeline.
func (dl *DailyLogger) FeedToAutodream(days int) ([]string, error) {
	if days <= 0 {
		days = 7 // default to last 7 days
	}

	var paths []string
	now := time.Now()

	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i)
		logPath := dl.GetLogPath(date)
		if _, err := os.Stat(logPath); err == nil {
			paths = append(paths, logPath)
		}
	}

	return paths, nil
}
