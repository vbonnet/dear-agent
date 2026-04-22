package wikibrain

import (
	"fmt"
	"strings"
	"time"
)

// LogPrefix constants for parseable log entries.
const (
	LogPrefixIngest  = "INGEST"
	LogPrefixLint    = "LINT"
	LogPrefixIndex   = "INDEX"
	LogPrefixQuery   = "QUERY"
	LogPrefixUpdate  = "UPDATE"
	LogPrefixBacklink = "BACKLINK"
)

// LogEntry represents one operation to append to log.md.
type LogEntry struct {
	Time    time.Time
	Prefix  string
	Message string
}

// FormatLogEntry returns a single line suitable for appending to log.md.
// Format: `YYYY-MM-DDTHH:MM:SSZ [PREFIX] message`
func FormatLogEntry(e LogEntry) string {
	ts := e.Time.UTC().Format("2006-01-02T15:04:05Z")
	return fmt.Sprintf("%s [%s] %s", ts, e.Prefix, e.Message)
}

// FormatLintLogEntry produces a log line summarising a lint run.
func FormatLintLogEntry(report *LintReport) string {
	return FormatLogEntry(LogEntry{
		Time:   report.RunAt,
		Prefix: LogPrefixLint,
		Message: fmt.Sprintf("pages=%d errors=%d warnings=%d info=%d",
			report.Stats.TotalPages,
			report.Stats.ErrorCount,
			report.Stats.WarningCount,
			report.Stats.InfoCount,
		),
	})
}

// FormatIndexLogEntry produces a log line summarising an index run.
func FormatIndexLogEntry(pageCount int, runAt time.Time) string {
	return FormatLogEntry(LogEntry{
		Time:    runAt,
		Prefix:  LogPrefixIndex,
		Message: fmt.Sprintf("pages=%d index.md regenerated", pageCount),
	})
}

// FormatIngestLogEntry produces a log line for an ingest operation.
func FormatIngestLogEntry(pagePath string, suggestions int, runAt time.Time) string {
	return FormatLogEntry(LogEntry{
		Time:    runAt,
		Prefix:  LogPrefixIngest,
		Message: fmt.Sprintf("page=%s backlink_suggestions=%d", pagePath, suggestions),
	})
}

// FormatQuerySaveLogEntry produces a log line when a query answer is saved as a page.
func FormatQuerySaveLogEntry(query, outputPath string, runAt time.Time) string {
	// Truncate long queries so the log stays unix-tool-friendly (no newlines)
	q := strings.ReplaceAll(query, "\n", " ")
	if len(q) > 80 {
		q = q[:77] + "..."
	}
	return FormatLogEntry(LogEntry{
		Time:    runAt,
		Prefix:  LogPrefixQuery,
		Message: fmt.Sprintf("saved=%s query=%q", outputPath, q),
	})
}
