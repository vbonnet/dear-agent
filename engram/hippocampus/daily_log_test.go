package hippocampus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetLogPath(t *testing.T) {
	logger := NewDailyLogger("/tmp/logs")
	date := time.Date(2026, 4, 2, 10, 30, 0, 0, time.UTC)

	path := logger.GetLogPath(date)
	expected := "/tmp/logs/2026/04/2026-04-02.md"

	if path != expected {
		t.Errorf("GetLogPath = %q, want %q", path, expected)
	}
}

func TestAppendEntry_CreatesDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()
	logger := NewDailyLogger(dir)

	now := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	err := logger.AppendEntry(LogEntry{
		Timestamp: now,
		Category:  "command",
		Content:   "engram doctor",
	})
	if err != nil {
		t.Fatalf("AppendEntry: %v", err)
	}

	logPath := logger.GetLogPath(now)
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log file not created: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "# Daily Log — 2026-03-15") {
		t.Error("missing header")
	}
	if !strings.Contains(text, "[command]") {
		t.Error("missing category")
	}
	if !strings.Contains(text, "engram doctor") {
		t.Error("missing content")
	}
}

func TestAppendEntry_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	logger := NewDailyLogger(dir)

	now := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	logger.AppendEntry(LogEntry{Timestamp: now, Category: "command", Content: "first"})
	logger.AppendEntry(LogEntry{Timestamp: now.Add(time.Hour), Category: "decision", Content: "second"})

	content, _ := logger.ReadDailyLog(now)

	// Should have header only once
	if strings.Count(content, "# Daily Log") != 1 {
		t.Error("header should appear exactly once")
	}

	if !strings.Contains(content, "first") || !strings.Contains(content, "second") {
		t.Error("both entries should be present")
	}
}

func TestLogSessionStartEnd(t *testing.T) {
	dir := t.TempDir()
	logger := NewDailyLogger(dir)

	logger.LogSessionStart("sess-123", "my-project")
	logger.LogSessionEnd("sess-123")

	content, err := logger.ReadDailyLog(time.Now())
	if err != nil {
		t.Fatalf("ReadDailyLog: %v", err)
	}

	if !strings.Contains(content, "session_start") {
		t.Error("missing session_start")
	}
	if !strings.Contains(content, "session_end") {
		t.Error("missing session_end")
	}
	if !strings.Contains(content, "sess-123") {
		t.Error("missing session ID")
	}
}

func TestFeedToAutodream(t *testing.T) {
	dir := t.TempDir()
	logger := NewDailyLogger(dir)

	// Create logs for today and yesterday
	today := time.Now()
	yesterday := today.AddDate(0, 0, -1)

	logger.AppendEntry(LogEntry{Timestamp: today, Category: "command", Content: "today"})
	logger.AppendEntry(LogEntry{Timestamp: yesterday, Category: "command", Content: "yesterday"})

	paths, err := logger.FeedToAutodream(7)
	if err != nil {
		t.Fatalf("FeedToAutodream: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("expected 2 log paths, got %d", len(paths))
	}
}

func TestFeedToAutodream_Empty(t *testing.T) {
	dir := t.TempDir()
	logger := NewDailyLogger(dir)

	paths, err := logger.FeedToAutodream(7)
	if err != nil {
		t.Fatalf("FeedToAutodream: %v", err)
	}

	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
	}
}

func TestListLogFiles(t *testing.T) {
	dir := t.TempDir()
	logger := NewDailyLogger(dir)

	// Create a couple of log files
	d1 := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)

	logger.AppendEntry(LogEntry{Timestamp: d1, Category: "command", Content: "a"})
	logger.AppendEntry(LogEntry{Timestamp: d2, Category: "command", Content: "b"})

	files, err := logger.ListLogFiles()
	if err != nil {
		t.Fatalf("ListLogFiles: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestReadDailyLog_NonExistent(t *testing.T) {
	dir := t.TempDir()
	logger := NewDailyLogger(dir)

	_, err := logger.ReadDailyLog(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Error("expected error for non-existent log")
	}
}

func TestNewDailyLogger_DefaultDir(t *testing.T) {
	logger := NewDailyLogger("")
	if logger.baseDir == "" {
		t.Error("expected non-empty default baseDir")
	}
	if !strings.Contains(logger.baseDir, ".engram") {
		t.Errorf("expected baseDir to contain .engram, got %s", logger.baseDir)
	}
}

func TestGetLogPath_DirectoryStructure(t *testing.T) {
	logger := NewDailyLogger("/base")

	// Verify YYYY/MM/YYYY-MM-DD.md structure
	date := time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)
	path := logger.GetLogPath(date)

	if !strings.Contains(path, filepath.Join("2026", "12")) {
		t.Errorf("path should contain 2026/12, got %s", path)
	}
	if !strings.HasSuffix(path, "2026-12-25.md") {
		t.Errorf("path should end with 2026-12-25.md, got %s", path)
	}
}
