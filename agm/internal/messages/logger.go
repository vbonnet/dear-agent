package messages

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MessageLogEntry represents a logged message
type MessageLogEntry struct {
	MessageID      string            `json:"message_id"`
	Sender         string            `json:"sender"`
	Recipient      string            `json:"recipient"`
	Timestamp      string            `json:"timestamp"` // RFC3339
	Message        string            `json:"message"`
	ReplyTo        string            `json:"reply_to,omitempty"`
	DeliveryStatus string            `json:"delivery_status"` // "sent", "delivered", "failed"
	DeliveryAckAt  string            `json:"delivery_ack_at,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// MessageLogger handles message logging to daily JSONL files
type MessageLogger struct {
	logsDir string
	mu      sync.Mutex
}

// NewMessageLogger creates a new message logger
func NewMessageLogger(logsDir string) (*MessageLogger, error) {
	// Ensure logs directory exists
	if err := os.MkdirAll(logsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	return &MessageLogger{
		logsDir: logsDir,
	}, nil
}

// LogMessage appends a message to the daily log file
func (l *MessageLogger) LogMessage(entry *MessageLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Determine log file for today
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(l.logsDir, fmt.Sprintf("%s.jsonl", today))

	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	// Append newline
	data = append(data, '\n')

	// Open file in append mode (create if doesn't exist)
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	// Write atomically
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	return nil
}

// CreateLogEntry creates a log entry for a sent message
func CreateLogEntry(messageID, sender, recipient, message, replyTo string) *MessageLogEntry {
	return &MessageLogEntry{
		MessageID:      messageID,
		Sender:         sender,
		Recipient:      recipient,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Message:        message,
		ReplyTo:        replyTo,
		DeliveryStatus: "sent",
		Metadata:       make(map[string]string),
	}
}

// CleanupOldLogs removes log files older than the retention period
func (l *MessageLogger) CleanupOldLogs(retentionDays int) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Read log directory
	entries, err := os.ReadDir(l.logsDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read logs directory: %w", err)
	}

	// Calculate cutoff date
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	deletedCount := 0
	for _, entry := range entries {
		// Skip non-JSONL files
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

		// Parse date from filename (YYYY-MM-DD.jsonl)
		filename := entry.Name()
		dateStr := filename[:len(filename)-6] // Remove ".jsonl"

		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			// Skip files with invalid date format
			continue
		}

		// Delete if older than cutoff
		if fileDate.Before(cutoff) {
			filePath := filepath.Join(l.logsDir, filename)
			if err := os.Remove(filePath); err != nil {
				return deletedCount, fmt.Errorf("failed to delete old log file %s: %w", filename, err)
			}
			deletedCount++
		}
	}

	return deletedCount, nil
}

// GetLogStats returns statistics about logged messages
type LogStats struct {
	TotalFiles    int
	TotalMessages int
	OldestDate    string
	NewestDate    string
}

// GetStats returns statistics about the message logs
func (l *MessageLogger) GetStats() (*LogStats, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entries, err := os.ReadDir(l.logsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read logs directory: %w", err)
	}

	stats := &LogStats{}
	var dates []string

	for _, entry := range entries {
		// Skip non-JSONL files
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}

		stats.TotalFiles++

		// Parse date from filename
		filename := entry.Name()
		dateStr := filename[:len(filename)-6]
		dates = append(dates, dateStr)

		// Count messages in file
		filePath := filepath.Join(l.logsDir, filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		// Count lines (one message per line)
		lines := 0
		for _, b := range data {
			if b == '\n' {
				lines++
			}
		}
		stats.TotalMessages += lines
	}

	// Find oldest and newest dates
	if len(dates) > 0 {
		// Sort dates (YYYY-MM-DD format sorts lexicographically)
		for i := 0; i < len(dates); i++ {
			for j := i + 1; j < len(dates); j++ {
				if dates[j] < dates[i] {
					dates[i], dates[j] = dates[j], dates[i]
				}
			}
		}
		stats.OldestDate = dates[0]
		stats.NewestDate = dates[len(dates)-1]
	}

	return stats, nil
}
