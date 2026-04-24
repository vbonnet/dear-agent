package consolidation

import (
	"log/slog"
	"testing"
	"time"
)

func TestPurgeContext_StripOldToolResults(t *testing.T) {
	events := make([]SessionEvent, 80)
	for i := range events {
		events[i] = SessionEvent{
			Timestamp: time.Now(),
			Type:      toolResultEventType,
			Data:      "some tool output",
		}
	}
	// Insert structural events in old region
	events[5].Type = "phase_started"
	events[10].Type = "memory_stored"

	wc := &WorkingContext{
		SessionID:     "test-session",
		RecentHistory: events,
	}

	stats := PurgeContext(wc, slog.Default())

	if stats.EventsTotal != 80 {
		t.Errorf("expected 80 total events, got %d", stats.EventsTotal)
	}

	// 30 old events minus 2 structural = 28 stripped
	if stats.ToolResultsStripped != 28 {
		t.Errorf("expected 28 stripped, got %d", stats.ToolResultsStripped)
	}

	// 80 - 28 = 52 preserved
	if stats.EventsPreserved != 52 {
		t.Errorf("expected 52 preserved, got %d", stats.EventsPreserved)
	}
}

func TestPurgeContext_PreservesAllWhenUnderThreshold(t *testing.T) {
	events := make([]SessionEvent, 30)
	for i := range events {
		events[i] = SessionEvent{
			Timestamp: time.Now(),
			Type:      toolResultEventType,
			Data:      "output",
		}
	}

	wc := &WorkingContext{
		SessionID:     "test-session",
		RecentHistory: events,
	}

	stats := PurgeContext(wc, slog.Default())

	if stats.ToolResultsStripped != 0 {
		t.Errorf("expected 0 stripped, got %d", stats.ToolResultsStripped)
	}
	if stats.EventsPreserved != 30 {
		t.Errorf("expected 30 preserved, got %d", stats.EventsPreserved)
	}
}

func TestPurgeContext_RedactsPII(t *testing.T) {
	wc := &WorkingContext{
		SessionID: "test-session",
		RecentHistory: []SessionEvent{
			{
				Timestamp: time.Now(),
				Type:      "user_input",
				Data:      "my key is sk-abcdefghijklmnopqrstuvwxyz and email is test@example.com",
			},
			{
				Timestamp: time.Now(),
				Type:      "tool_result",
				Data:      "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0",
			},
		},
		RelevantMemory: []Memory{
			{
				ID:      "mem-1",
				Content: "connection: postgres://admin:secret@db.example.com:5432/mydb",
			},
		},
		PinnedItems: []Memory{
			{
				ID:      "pin-1",
				Content: "SSN is 123-45-6789",
			},
		},
	}

	stats := PurgeContext(wc, slog.Default())

	// Check redactions occurred
	if stats.PIIRedactions["api_key"] != 1 {
		t.Errorf("expected 1 api_key redaction, got %d", stats.PIIRedactions["api_key"])
	}
	if stats.PIIRedactions["email"] < 1 {
		t.Errorf("expected at least 1 email redaction, got %d", stats.PIIRedactions["email"])
	}
	if stats.PIIRedactions["bearer_token"] != 1 {
		t.Errorf("expected 1 bearer_token redaction, got %d", stats.PIIRedactions["bearer_token"])
	}
	if stats.PIIRedactions["connection_string"] != 1 {
		t.Errorf("expected 1 connection_string redaction, got %d", stats.PIIRedactions["connection_string"])
	}
	if stats.PIIRedactions["ssn"] != 1 {
		t.Errorf("expected 1 ssn redaction, got %d", stats.PIIRedactions["ssn"])
	}

	// Verify content was actually redacted
	if s, ok := wc.RecentHistory[0].Data.(string); ok {
		if s == "my key is sk-abcdefghijklmnopqrstuvwxyz and email is test@example.com" {
			t.Error("expected event data to be redacted")
		}
	}
	if s, ok := wc.RelevantMemory[0].Content.(string); ok {
		if s == "connection: postgres://admin:secret@db.example.com:5432/mydb" {
			t.Error("expected memory content to be redacted")
		}
	}
	if s, ok := wc.PinnedItems[0].Content.(string); ok {
		if s == "SSN is 123-45-6789" {
			t.Error("expected pinned item content to be redacted")
		}
	}
}

func TestPurgeStats_String(t *testing.T) {
	stats := PurgeStats{
		ToolResultsStripped: 5,
		PIIRedactions:       map[string]int{"api_key": 2, "email": 1},
		EventsPreserved:     45,
		EventsTotal:         50,
	}

	s := stats.String()
	expected := "purge: stripped=5 pii_redactions=3 preserved=45/50"
	if s != expected {
		t.Errorf("expected %q, got %q", expected, s)
	}
}

func TestDefaultPurgePatterns(t *testing.T) {
	patterns := DefaultPurgePatterns()

	tests := []struct {
		name    string
		input   string
		pattern string
		match   bool
	}{
		{"openai key", "sk-abcdefghijklmnopqrstuv", "api_key", true},
		{"google key", "AIzaSyAbCdEfGhIjKlMnOpQrStUvWxYz0123456", "api_key", true},
		{"github pat", "ghp_abcdefghijklmnopqrstuvwxyz0123456789", "api_key", true},
		{"github oauth", "gho_abcdefghijklmnopqrstuvwxyz0123456789", "api_key", true},
		{"email", "user@example.com", "email", true},
		{"bearer", "Bearer eyJhbGciOiJIUzI1NiJ9", "bearer_token", true},
		{"postgres", "postgres://user:pass@host:5432/db", "connection_string", true},
		{"ssn", "123-45-6789", "ssn", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var found bool
			for _, p := range patterns {
				if p.Name == tt.pattern && p.Pattern.MatchString(tt.input) {
					found = true
					break
				}
			}
			if found != tt.match {
				t.Errorf("pattern %s match=%v, want %v for input %q", tt.pattern, found, tt.match, tt.input)
			}
		})
	}
}
