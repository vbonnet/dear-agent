package logs

import (
	"testing"
	"time"
)

func TestLogEntryFields(t *testing.T) {
	now := time.Now()
	entry := &LogEntry{
		ID:        "log-001",
		SessionID: "sess-abc",
		Timestamp: now,
		Level:     "ERROR",
		Source:    "workflow",
		Message:   "something went wrong",
		Data:      map[string]interface{}{"code": 500},
	}

	if entry.ID != "log-001" {
		t.Errorf("expected ID log-001, got %s", entry.ID)
	}
	if entry.SessionID != "sess-abc" {
		t.Errorf("expected SessionID sess-abc, got %s", entry.SessionID)
	}
	if entry.Level != "ERROR" {
		t.Errorf("expected Level ERROR, got %s", entry.Level)
	}
	if entry.Source != "workflow" {
		t.Errorf("expected Source workflow, got %s", entry.Source)
	}
	if entry.Message != "something went wrong" {
		t.Errorf("expected correct message, got %s", entry.Message)
	}
	if entry.Data["code"] != 500 {
		t.Errorf("expected data code=500, got %v", entry.Data["code"])
	}
	if !entry.Timestamp.Equal(now) {
		t.Errorf("expected Timestamp %v, got %v", now, entry.Timestamp)
	}
}

func TestLogEntryNilData(t *testing.T) {
	entry := &LogEntry{
		ID:      "log-002",
		Level:   "INFO",
		Message: "test",
	}

	if entry.Data != nil {
		t.Errorf("expected nil data, got %v", entry.Data)
	}
}

func TestListOptsDefaults(t *testing.T) {
	opts := &ListOpts{}

	if opts.MinLevel != "" {
		t.Errorf("expected empty MinLevel, got %s", opts.MinLevel)
	}
	if !opts.Since.IsZero() {
		t.Errorf("expected zero Since, got %v", opts.Since)
	}
	if opts.Limit != 0 {
		t.Errorf("expected zero Limit, got %d", opts.Limit)
	}
	if opts.Offset != 0 {
		t.Errorf("expected zero Offset, got %d", opts.Offset)
	}
}

func TestListOptsWithValues(t *testing.T) {
	since := time.Now().Add(-1 * time.Hour)
	opts := &ListOpts{
		MinLevel: "WARN",
		Since:    since,
		Limit:    50,
		Offset:   10,
	}

	if opts.MinLevel != "WARN" {
		t.Errorf("expected MinLevel WARN, got %s", opts.MinLevel)
	}
	if !opts.Since.Equal(since) {
		t.Errorf("expected Since %v, got %v", since, opts.Since)
	}
	if opts.Limit != 50 {
		t.Errorf("expected Limit 50, got %d", opts.Limit)
	}
	if opts.Offset != 10 {
		t.Errorf("expected Offset 10, got %d", opts.Offset)
	}
}

func TestLogEntryLevels(t *testing.T) {
	levels := []string{"INFO", "WARN", "ERROR", "CRITICAL"}
	for _, level := range levels {
		entry := &LogEntry{Level: level}
		if entry.Level != level {
			t.Errorf("expected Level %s, got %s", level, entry.Level)
		}
	}
}
