package history

import (
	"os"
	"testing"
)

func TestReadConversations_Empty(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	sessions, err := p.ReadConversations(100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestReadConversations_NoSessionIDs(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{
		`{"display":"test 1","timestamp":1000,"project":"/tmp/p1"}`,
		`{"display":"test 2","timestamp":2000,"project":"/tmp/p2"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	sessions, err := p.ReadConversations(100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return empty since no sessionId fields
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions without sessionIds, got %d", len(sessions))
	}
}

func TestReadConversations_SingleSession(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{
		`{"display":"entry 1","timestamp":1000,"project":"/tmp/p1","sessionId":"session-1"}`,
		`{"display":"entry 2","timestamp":2000,"project":"/tmp/p1","sessionId":"session-1"}`,
		`{"display":"entry 3","timestamp":3000,"project":"/tmp/p1","sessionId":"session-1"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	sessions, err := p.ReadConversations(100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	session := sessions[0]
	if session.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", session.SessionID)
	}
	if len(session.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(session.Entries))
	}
	if session.Project != "/tmp/p1" {
		t.Errorf("expected project /tmp/p1, got %s", session.Project)
	}
}

func TestReadConversations_MultipleSessions(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{
		`{"display":"s1 e1","timestamp":5000,"project":"/tmp/p1","sessionId":"session-1"}`,
		`{"display":"s2 e1","timestamp":4000,"project":"/tmp/p2","sessionId":"session-2"}`,
		`{"display":"s1 e2","timestamp":3000,"project":"/tmp/p1","sessionId":"session-1"}`,
		`{"display":"s3 e1","timestamp":2000,"project":"/tmp/p3","sessionId":"session-3"}`,
		`{"display":"s2 e2","timestamp":1000,"project":"/tmp/p2","sessionId":"session-2"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	sessions, err := p.ReadConversations(100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	// Check sessions are grouped correctly
	sessionMap := make(map[string]*SessionHistory)
	for _, s := range sessions {
		sessionMap[s.SessionID] = s
	}

	if s1, ok := sessionMap["session-1"]; ok {
		if len(s1.Entries) != 2 {
			t.Errorf("session-1: expected 2 entries, got %d", len(s1.Entries))
		}
	} else {
		t.Error("session-1 not found")
	}

	if s2, ok := sessionMap["session-2"]; ok {
		if len(s2.Entries) != 2 {
			t.Errorf("session-2: expected 2 entries, got %d", len(s2.Entries))
		}
	} else {
		t.Error("session-2 not found")
	}

	if s3, ok := sessionMap["session-3"]; ok {
		if len(s3.Entries) != 1 {
			t.Errorf("session-3: expected 1 entry, got %d", len(s3.Entries))
		}
	} else {
		t.Error("session-3 not found")
	}
}

func TestReadConversations_SortedByTimestamp(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{
		`{"display":"old","timestamp":1000,"sessionId":"session-old"}`,
		`{"display":"newest","timestamp":5000,"sessionId":"session-new"}`,
		`{"display":"middle","timestamp":3000,"sessionId":"session-mid"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	sessions, err := p.ReadConversations(100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	// Should be sorted by most recent first
	if sessions[0].SessionID != "session-new" {
		t.Errorf("expected newest session first, got %s", sessions[0].SessionID)
	}
	if sessions[1].SessionID != "session-mid" {
		t.Errorf("expected middle session second, got %s", sessions[1].SessionID)
	}
	if sessions[2].SessionID != "session-old" {
		t.Errorf("expected oldest session last, got %s", sessions[2].SessionID)
	}
}

func TestReadConversations_LimitEntries(t *testing.T) {
	// Create more entries than limit
	lines := []string{}
	for i := 0; i < 20; i++ {
		line := `{"display":"entry ` + string(rune('0'+i)) + `","timestamp":` + string(rune('0'+i)) + `000,"sessionId":"session-1"}`
		lines = append(lines, line)
	}

	tmpFile := createTestHistoryFile(t, lines)
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	sessions, err := p.ReadConversations(10) // Limit to 10 entries

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should process only 10 most recent entries
	totalEntries := 0
	for _, s := range sessions {
		totalEntries += len(s.Entries)
	}

	if totalEntries > 10 {
		t.Errorf("expected at most 10 entries with limit, got %d", totalEntries)
	}
}

func TestReadConversations_MostCommonProject(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{
		`{"display":"e1","timestamp":1000,"project":"/tmp/p1","sessionId":"session-1"}`,
		`{"display":"e2","timestamp":2000,"project":"/tmp/p1","sessionId":"session-1"}`,
		`{"display":"e3","timestamp":3000,"project":"/tmp/p2","sessionId":"session-1"}`,
		`{"display":"e4","timestamp":4000,"project":"/tmp/p1","sessionId":"session-1"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	sessions, err := p.ReadConversations(100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Most common project is /tmp/p1 (3 occurrences)
	if sessions[0].Project != "/tmp/p1" {
		t.Errorf("expected most common project /tmp/p1, got %s", sessions[0].Project)
	}
}

func TestReadConversations_MalformedJSON(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{
		`{"display":"valid1","timestamp":1000,"sessionId":"session-1"}`,
		`this is not valid json`,
		`{"display":"valid2","timestamp":2000,"sessionId":"session-1"}`,
		`{malformed}`,
		`{"display":"valid3","timestamp":3000,"sessionId":"session-2"}`,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	sessions, err := p.ReadConversations(100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should skip malformed lines and continue
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions (skipping malformed), got %d", len(sessions))
	}

	// Check we got the valid entries
	sessionMap := make(map[string]int)
	for _, s := range sessions {
		sessionMap[s.SessionID] = len(s.Entries)
	}

	if sessionMap["session-1"] != 2 {
		t.Errorf("session-1: expected 2 entries, got %d", sessionMap["session-1"])
	}
	if sessionMap["session-2"] != 1 {
		t.Errorf("session-2: expected 1 entry, got %d", sessionMap["session-2"])
	}
}

func TestReadConversations_EmptyLines(t *testing.T) {
	tmpFile := createTestHistoryFile(t, []string{
		`{"display":"e1","timestamp":1000,"sessionId":"session-1"}`,
		``,
		`{"display":"e2","timestamp":2000,"sessionId":"session-1"}`,
		``,
		``,
	})
	defer os.Remove(tmpFile)

	p := NewParser(tmpFile)
	sessions, err := p.ReadConversations(100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if len(sessions[0].Entries) != 2 {
		t.Errorf("expected 2 entries (skipping empty lines), got %d", len(sessions[0].Entries))
	}
}

func TestReadConversations_NonexistentFile(t *testing.T) {
	p := NewParser("/tmp/nonexistent-history-file-xyz.jsonl")
	sessions, err := p.ReadConversations(100)

	if err != nil {
		t.Errorf("expected no error for missing file, got: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty list for missing file, got %d sessions", len(sessions))
	}
}

func TestGetConversationSummary(t *testing.T) {
	session := &SessionHistory{
		SessionID: "test-session",
		Entries: []*ConversationEntry{
			{Display: "First entry", Timestamp: 1000},
			{Display: "Second entry", Timestamp: 2000},
			{Display: "Third entry", Timestamp: 3000},
		},
		Project: "/tmp/test",
	}

	summary := GetConversationSummary(session, 1)
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	// Should contain first entry
	if len(summary) < 10 {
		t.Errorf("expected substantial summary, got: %s", summary)
	}
}

func TestGetConversationSummary_Empty(t *testing.T) {
	session := &SessionHistory{
		SessionID: "test-session",
		Entries:   []*ConversationEntry{},
	}

	summary := GetConversationSummary(session, 1)
	if summary != "" {
		t.Errorf("expected empty summary for no entries, got: %s", summary)
	}
}

func TestGetConversationSummary_Nil(t *testing.T) {
	summary := GetConversationSummary(nil, 1)
	if summary != "" {
		t.Errorf("expected empty summary for nil session, got: %s", summary)
	}
}

func TestGetConversationSummary_Truncation(t *testing.T) {
	longDisplay := ""
	for i := 0; i < 200; i++ {
		longDisplay += "x"
	}

	session := &SessionHistory{
		SessionID: "test",
		Entries: []*ConversationEntry{
			{Display: longDisplay, Timestamp: 1000},
		},
	}

	summary := GetConversationSummary(session, 1)
	// Should be truncated (display is truncated to 100 chars in the function)
	if len(summary) > 150 {
		t.Errorf("expected truncated summary, got length %d", len(summary))
	}
}
