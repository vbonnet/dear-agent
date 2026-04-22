package gclog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "subdir", "gc.jsonl")

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if logger.Path() != logPath {
		t.Errorf("Path() = %q, want %q", logger.Path(), logPath)
	}
	if _, err := os.Stat(filepath.Dir(logPath)); err != nil {
		t.Errorf("directory not created: %v", err)
	}
}

func TestLogger_Log_AppendsJSONL(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "gc.jsonl")

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ts := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	entry1 := Entry{
		Timestamp:      ts,
		Operation:      "archive_sandbox_cleanup",
		SessionID:      "abc-123",
		SandboxRemoved: "/home/user/.agm/sandboxes/abc-123",
		BytesReclaimed: 1024 * 1024,
	}
	entry2 := Entry{
		Timestamp: ts.Add(time.Second),
		Operation: "admin_gc_worktree",
		WorktreesPaths: []string{
			"/home/user/src/ws/oss/worktrees/ai-tools/old-branch",
		},
		BytesReclaimed: 500 * 1024 * 1024,
	}

	if err := logger.Log(entry1); err != nil {
		t.Fatalf("Log entry1: %v", err)
	}
	if err := logger.Log(entry2); err != nil {
		t.Fatalf("Log entry2: %v", err)
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var entries []Entry
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal line: %v", err)
		}
		entries = append(entries, e)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Operation != "archive_sandbox_cleanup" {
		t.Errorf("entry[0].Operation = %q, want archive_sandbox_cleanup", entries[0].Operation)
	}
	if entries[0].BytesReclaimed != 1024*1024 {
		t.Errorf("entry[0].BytesReclaimed = %d, want %d", entries[0].BytesReclaimed, 1024*1024)
	}
	if entries[1].Operation != "admin_gc_worktree" {
		t.Errorf("entry[1].Operation = %q, want admin_gc_worktree", entries[1].Operation)
	}
}

func TestLogger_Log_SetsTimestampWhenZero(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "gc.jsonl")

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	before := time.Now()
	if err := logger.Log(Entry{Operation: "test"}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	var e Entry
	json.Unmarshal(data, &e)

	if e.Timestamp.Before(before) {
		t.Errorf("timestamp %v should be >= %v", e.Timestamp, before)
	}
}

func TestLogger_Log_DryRunEntry(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "gc.jsonl")

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := logger.Log(Entry{
		Operation: "admin_gc_sandbox",
		SessionID: "test-id",
		DryRun:    true,
	}); err != nil {
		t.Fatalf("Log: %v", err)
	}

	data, _ := os.ReadFile(logPath)
	var e Entry
	json.Unmarshal(data, &e)

	if !e.DryRun {
		t.Error("expected DryRun=true")
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), make([]byte, 1000), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), make([]byte, 2000), 0644)

	size := DirSize(dir)
	if size < 3000 {
		t.Errorf("DirSize = %d, want >= 3000", size)
	}
}

func TestDirSize_NonExistent(t *testing.T) {
	size := DirSize("/nonexistent/path/abc123")
	if size != 0 {
		t.Errorf("DirSize of nonexistent = %d, want 0", size)
	}
}
