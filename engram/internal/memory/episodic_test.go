package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewEpisodicMemory(t *testing.T) {
	tmpDir := t.TempDir()

	// Test: Create new episodic memory
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	if em == nil {
		t.Fatal("EpisodicMemory is nil")
	}

	// Verify DECISION_LOG.md was created
	logPath := filepath.Join(tmpDir, "DECISION_LOG.md")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("DECISION_LOG.md was not created")
	}

	// Verify template content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	if !strings.Contains(string(content), "Decision Log") {
		t.Error("DECISION_LOG.md missing expected template header")
	}

	if !strings.Contains(string(content), "Molt Behavior") {
		t.Error("DECISION_LOG.md missing Molt Behavior section")
	}
}

func TestAppendEntry(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	ctx := context.Background()

	entry := &MemoryEntry{
		Timestamp: time.Now(),
		Session:   "test-session-001",
		Event:     "decision",
		Summary:   "Chose library X over library Y",
		Details:   "Library X has better performance and active maintenance",
		Tokens:    50,
		Metadata: map[string]string{
			"library": "X",
		},
	}

	// Test: Append entry
	if err := em.AppendEntry(ctx, entry); err != nil {
		t.Fatalf("AppendEntry failed: %v", err)
	}

	// Verify entry was written
	content, err := os.ReadFile(em.logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "test-session-001") {
		t.Error("Entry missing session ID")
	}

	if !strings.Contains(contentStr, "Chose library X over library Y") {
		t.Error("Entry missing summary")
	}

	if !strings.Contains(contentStr, "Library X has better performance") {
		t.Error("Entry missing details")
	}
}

func TestShouldMolt(t *testing.T) {
	tmpDir := t.TempDir()
	maxTokens := 200000
	em, err := NewEpisodicMemory(tmpDir, maxTokens)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	tests := []struct {
		name          string
		sessionTokens int
		expectedMolt  bool
	}{
		{
			name:          "Below threshold (50%)",
			sessionTokens: 100000,
			expectedMolt:  false,
		},
		{
			name:          "At threshold (80%)",
			sessionTokens: 160000,
			expectedMolt:  true,
		},
		{
			name:          "Above threshold (90%)",
			sessionTokens: 180000,
			expectedMolt:  true,
		},
		{
			name:          "Just below threshold (79%)",
			sessionTokens: 158000,
			expectedMolt:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldMolt := em.ShouldMolt(tt.sessionTokens)
			if shouldMolt != tt.expectedMolt {
				t.Errorf("ShouldMolt(%d) = %v, want %v (threshold: 80%% of %d = %d)",
					tt.sessionTokens, shouldMolt, tt.expectedMolt, maxTokens, int(float64(maxTokens)*0.8))
			}
		})
	}
}

func TestGetTokenUsage(t *testing.T) {
	tmpDir := t.TempDir()
	maxTokens := 200000
	em, err := NewEpisodicMemory(tmpDir, maxTokens)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	// Initial state
	current, max, percentage := em.GetTokenUsage()
	if current != 0 {
		t.Errorf("Initial token count = %d, want 0", current)
	}
	if max != maxTokens {
		t.Errorf("Max tokens = %d, want %d", max, maxTokens)
	}
	if percentage != 0.0 {
		t.Errorf("Initial percentage = %.2f, want 0.00", percentage)
	}

	// After appending entry
	ctx := context.Background()
	entry := &MemoryEntry{
		Timestamp: time.Now(),
		Session:   "test-session",
		Event:     "decision",
		Summary:   "Test entry",
		Details:   "Details",
		Tokens:    1000,
	}

	if err := em.AppendEntry(ctx, entry); err != nil {
		t.Fatalf("AppendEntry failed: %v", err)
	}

	current, _, percentage = em.GetTokenUsage()
	if current != 1000 {
		t.Errorf("Token count after append = %d, want 1000", current)
	}

	expectedPercentage := 0.5 // 1000/200000 = 0.5%
	if percentage < expectedPercentage-0.01 || percentage > expectedPercentage+0.01 {
		t.Errorf("Percentage = %.2f, want %.2f", percentage, expectedPercentage)
	}
}

func TestMoltSession(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	ctx := context.Background()
	sessionID := "molt-test-001"
	summary := "Session ended at 85% token usage"
	details := "Tried library A (failed), switched to library B (succeeded), implemented feature X"

	// Test: Molt session
	if err := em.MoltSession(ctx, sessionID, summary, details); err != nil {
		t.Fatalf("MoltSession failed: %v", err)
	}

	// Verify entry was written
	content, err := os.ReadFile(em.logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "molt-test-001") {
		t.Error("Molt entry missing session ID")
	}

	if !strings.Contains(contentStr, "Session ended at 85% token usage") {
		t.Error("Molt entry missing summary")
	}

	if !strings.Contains(contentStr, "token_threshold") {
		t.Error("Molt entry missing trigger metadata")
	}

	if !strings.Contains(contentStr, "molt") {
		t.Error("Molt entry missing event type")
	}
}

func TestRecordDecision(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	ctx := context.Background()

	// Test: Record decision
	if err := em.RecordDecision(ctx, "session-001", "Use PostgreSQL", "Better ACID guarantees than MongoDB"); err != nil {
		t.Fatalf("RecordDecision failed: %v", err)
	}

	// Verify
	content, err := os.ReadFile(em.logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Use PostgreSQL") {
		t.Error("Decision entry missing summary")
	}

	if !strings.Contains(contentStr, "decision") {
		t.Error("Decision entry missing event type")
	}
}

func TestRecordError(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	ctx := context.Background()

	// Test: Record error
	if err := em.RecordError(ctx, "session-001", "API timeout on endpoint /users", "Increased timeout from 5s to 30s"); err != nil {
		t.Fatalf("RecordError failed: %v", err)
	}

	// Verify
	content, err := os.ReadFile(em.logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "API timeout on endpoint /users") {
		t.Error("Error entry missing summary")
	}

	if !strings.Contains(contentStr, "error") {
		t.Error("Error entry missing event type")
	}

	if !strings.Contains(contentStr, "Increased timeout from 5s to 30s") {
		t.Error("Error entry missing resolution")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text           string
		expectedTokens int
	}{
		{"Hello world", 2},              // 11 chars / 4 = 2.75 -> 2
		{"This is a test", 3},           // 14 chars / 4 = 3.5 -> 3
		{"", 0},                         // Empty string
		{"A", 0},                        // 1 char / 4 = 0.25 -> 0
		{"ABCD", 1},                     // 4 chars / 4 = 1
		{strings.Repeat("A", 400), 100}, // 400 chars / 4 = 100
	}

	for _, tt := range tests {
		tokens := estimateTokens(tt.text)
		if tokens != tt.expectedTokens {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.text, tokens, tt.expectedTokens)
		}
	}
}

func TestFormatMemoryEntry(t *testing.T) {
	entry := &MemoryEntry{
		Timestamp: time.Date(2026, 2, 3, 15, 30, 0, 0, time.UTC),
		Session:   "test-session",
		Event:     "decision",
		Summary:   "Test summary",
		Details:   "Test details",
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	formatted := formatMemoryEntry(entry)

	// Verify structure
	if !strings.Contains(formatted, "2026-02-03 15:30:00") {
		t.Error("Formatted entry missing timestamp")
	}

	if !strings.Contains(formatted, "decision") {
		t.Error("Formatted entry missing event type")
	}

	if !strings.Contains(formatted, "test-session") {
		t.Error("Formatted entry missing session")
	}

	if !strings.Contains(formatted, "Test summary") {
		t.Error("Formatted entry missing summary")
	}

	if !strings.Contains(formatted, "Test details") {
		t.Error("Formatted entry missing details")
	}

	if !strings.Contains(formatted, "key1: value1") {
		t.Error("Formatted entry missing metadata")
	}

	if !strings.Contains(formatted, "---") {
		t.Error("Formatted entry missing separator")
	}
}

func TestMultipleEntries(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	ctx := context.Background()

	// Append multiple entries
	entries := []struct {
		session string
		summary string
	}{
		{"session-001", "First decision"},
		{"session-001", "Second decision"},
		{"session-002", "Third decision"},
	}

	for _, e := range entries {
		entry := &MemoryEntry{
			Timestamp: time.Now(),
			Session:   e.session,
			Event:     "decision",
			Summary:   e.summary,
			Tokens:    10,
		}

		if err := em.AppendEntry(ctx, entry); err != nil {
			t.Fatalf("AppendEntry failed: %v", err)
		}
	}

	// Verify all entries present
	content, err := os.ReadFile(em.logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)
	for _, e := range entries {
		if !strings.Contains(contentStr, e.summary) {
			t.Errorf("Missing entry: %s", e.summary)
		}
	}
}
