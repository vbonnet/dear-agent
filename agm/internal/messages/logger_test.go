package messages

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessageLogger(t *testing.T) {
	t.Run("creates logs directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		logsDir := filepath.Join(tmpDir, "logs")

		logger, err := NewMessageLogger(logsDir)
		require.NoError(t, err)
		require.NotNil(t, logger)

		info, err := os.Stat(logsDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("creates nested directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		logsDir := filepath.Join(tmpDir, "a", "b", "c", "logs")

		logger, err := NewMessageLogger(logsDir)
		require.NoError(t, err)
		require.NotNil(t, logger)

		info, err := os.Stat(logsDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("succeeds if directory already exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, logger)
	})
}

func TestLogMessage(t *testing.T) {
	t.Run("writes entry to daily file", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		entry := &MessageLogEntry{
			MessageID:      "msg-001",
			Sender:         "session-a",
			Recipient:      "session-b",
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
			Message:        "Hello",
			DeliveryStatus: "sent",
		}

		err = logger.LogMessage(entry)
		require.NoError(t, err)

		// Verify file exists with today's date
		today := time.Now().Format("2006-01-02")
		logFile := filepath.Join(tmpDir, today+".jsonl")
		data, err := os.ReadFile(logFile)
		require.NoError(t, err)

		// Verify content is valid JSON
		var parsed MessageLogEntry
		err = json.Unmarshal([]byte(strings.TrimSpace(string(data))), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "msg-001", parsed.MessageID)
		assert.Equal(t, "session-a", parsed.Sender)
		assert.Equal(t, "session-b", parsed.Recipient)
		assert.Equal(t, "Hello", parsed.Message)
		assert.Equal(t, "sent", parsed.DeliveryStatus)
	})

	t.Run("appends multiple entries", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		for i := 0; i < 3; i++ {
			entry := &MessageLogEntry{
				MessageID:      "msg-" + string(rune('a'+i)),
				Sender:         "sender",
				Recipient:      "recipient",
				Timestamp:      time.Now().UTC().Format(time.RFC3339),
				Message:        "Message",
				DeliveryStatus: "sent",
			}
			err = logger.LogMessage(entry)
			require.NoError(t, err)
		}

		today := time.Now().Format("2006-01-02")
		logFile := filepath.Join(tmpDir, today+".jsonl")
		data, err := os.ReadFile(logFile)
		require.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		assert.Len(t, lines, 3)
	})

	t.Run("preserves optional fields", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		entry := &MessageLogEntry{
			MessageID:      "msg-reply",
			Sender:         "sender",
			Recipient:      "recipient",
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
			Message:        "Reply",
			ReplyTo:        "msg-original",
			DeliveryStatus: "delivered",
			DeliveryAckAt:  time.Now().UTC().Format(time.RFC3339),
			Metadata:       map[string]string{"key": "value"},
		}

		err = logger.LogMessage(entry)
		require.NoError(t, err)

		today := time.Now().Format("2006-01-02")
		logFile := filepath.Join(tmpDir, today+".jsonl")
		data, err := os.ReadFile(logFile)
		require.NoError(t, err)

		var parsed MessageLogEntry
		err = json.Unmarshal([]byte(strings.TrimSpace(string(data))), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "msg-original", parsed.ReplyTo)
		assert.Equal(t, "delivered", parsed.DeliveryStatus)
		assert.Equal(t, "value", parsed.Metadata["key"])
	})
}

func TestCreateLogEntry(t *testing.T) {
	t.Run("creates entry with all fields", func(t *testing.T) {
		entry := CreateLogEntry("msg-001", "sender", "recipient", "Hello", "reply-to-id")

		assert.Equal(t, "msg-001", entry.MessageID)
		assert.Equal(t, "sender", entry.Sender)
		assert.Equal(t, "recipient", entry.Recipient)
		assert.Equal(t, "Hello", entry.Message)
		assert.Equal(t, "reply-to-id", entry.ReplyTo)
		assert.Equal(t, "sent", entry.DeliveryStatus)
		assert.NotNil(t, entry.Metadata)
		assert.NotEmpty(t, entry.Timestamp)
	})

	t.Run("creates entry without reply_to", func(t *testing.T) {
		entry := CreateLogEntry("msg-002", "sender", "recipient", "Hello", "")

		assert.Equal(t, "", entry.ReplyTo)
		assert.Equal(t, "sent", entry.DeliveryStatus)
	})

	t.Run("timestamp is valid RFC3339", func(t *testing.T) {
		before := time.Now().UTC().Add(-time.Second)
		entry := CreateLogEntry("msg-003", "s", "r", "m", "")
		after := time.Now().UTC().Add(time.Second)

		parsed, err := time.Parse(time.RFC3339, entry.Timestamp)
		require.NoError(t, err)
		assert.True(t, parsed.After(before) || parsed.Equal(before))
		assert.True(t, parsed.Before(after) || parsed.Equal(after))
	})

	t.Run("metadata map is initialized empty", func(t *testing.T) {
		entry := CreateLogEntry("msg-004", "s", "r", "m", "")
		assert.NotNil(t, entry.Metadata)
		assert.Len(t, entry.Metadata, 0)

		// Should be writable
		entry.Metadata["test"] = "value"
		assert.Equal(t, "value", entry.Metadata["test"])
	})
}

func TestCleanupOldLogs(t *testing.T) {
	t.Run("deletes old log files", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		// Create old log file (30 days ago)
		oldDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
		err = os.WriteFile(filepath.Join(tmpDir, oldDate+".jsonl"), []byte("{}\n"), 0600)
		require.NoError(t, err)

		// Create recent log file (1 day ago)
		recentDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		err = os.WriteFile(filepath.Join(tmpDir, recentDate+".jsonl"), []byte("{}\n"), 0600)
		require.NoError(t, err)

		deleted, err := logger.CleanupOldLogs(7)
		require.NoError(t, err)
		assert.Equal(t, 1, deleted)

		// Old file should be gone
		_, err = os.Stat(filepath.Join(tmpDir, oldDate+".jsonl"))
		assert.True(t, os.IsNotExist(err))

		// Recent file should remain
		_, err = os.Stat(filepath.Join(tmpDir, recentDate+".jsonl"))
		assert.NoError(t, err)
	})

	t.Run("skips non-jsonl files", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		// Create a non-JSONL file with a date-like name
		err = os.WriteFile(filepath.Join(tmpDir, "2020-01-01.txt"), []byte("not a log"), 0600)
		require.NoError(t, err)

		deleted, err := logger.CleanupOldLogs(1)
		require.NoError(t, err)
		assert.Equal(t, 0, deleted)

		// File should still exist
		_, err = os.Stat(filepath.Join(tmpDir, "2020-01-01.txt"))
		assert.NoError(t, err)
	})

	t.Run("skips files with invalid date names", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "not-a-date.jsonl"), []byte("{}\n"), 0600)
		require.NoError(t, err)

		deleted, err := logger.CleanupOldLogs(1)
		require.NoError(t, err)
		assert.Equal(t, 0, deleted)
	})

	t.Run("returns zero when no old logs", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		deleted, err := logger.CleanupOldLogs(7)
		require.NoError(t, err)
		assert.Equal(t, 0, deleted)
	})

	t.Run("deletes multiple old files", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		for i := 10; i <= 12; i++ {
			oldDate := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
			err = os.WriteFile(filepath.Join(tmpDir, oldDate+".jsonl"), []byte("{}\n"), 0600)
			require.NoError(t, err)
		}

		deleted, err := logger.CleanupOldLogs(7)
		require.NoError(t, err)
		assert.Equal(t, 3, deleted)
	})
}

func TestLoggerGetStats(t *testing.T) {
	t.Run("returns stats for log files", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		// Create log files with known content
		err = os.WriteFile(filepath.Join(tmpDir, "2026-04-08.jsonl"), []byte("{}\n{}\n{}\n"), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "2026-04-09.jsonl"), []byte("{}\n{}\n"), 0600)
		require.NoError(t, err)

		stats, err := logger.GetStats()
		require.NoError(t, err)
		assert.Equal(t, 2, stats.TotalFiles)
		assert.Equal(t, 5, stats.TotalMessages)
		assert.Equal(t, "2026-04-08", stats.OldestDate)
		assert.Equal(t, "2026-04-09", stats.NewestDate)
	})

	t.Run("returns empty stats for empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		stats, err := logger.GetStats()
		require.NoError(t, err)
		assert.Equal(t, 0, stats.TotalFiles)
		assert.Equal(t, 0, stats.TotalMessages)
		assert.Empty(t, stats.OldestDate)
		assert.Empty(t, stats.NewestDate)
	})

	t.Run("skips non-jsonl files", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "2026-04-08.jsonl"), []byte("{}\n"), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("not a log\n"), 0600)
		require.NoError(t, err)

		stats, err := logger.GetStats()
		require.NoError(t, err)
		assert.Equal(t, 1, stats.TotalFiles)
	})

	t.Run("sorts dates correctly", func(t *testing.T) {
		tmpDir := t.TempDir()
		logger, err := NewMessageLogger(tmpDir)
		require.NoError(t, err)

		// Create files in non-chronological order
		dates := []string{"2026-04-10", "2026-04-01", "2026-04-05"}
		for _, d := range dates {
			err = os.WriteFile(filepath.Join(tmpDir, d+".jsonl"), []byte("{}\n"), 0600)
			require.NoError(t, err)
		}

		stats, err := logger.GetStats()
		require.NoError(t, err)
		assert.Equal(t, "2026-04-01", stats.OldestDate)
		assert.Equal(t, "2026-04-10", stats.NewestDate)
	})
}
