package channel

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/protocol"
)

func TestNewChannel(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantTopic string
	}{
		{
			name:      "extracts topic from filename with date suffix",
			path:      "/tmp/channels/design-review-2024-03-15.md",
			wantTopic: "design-review",
		},
		{
			name:      "extracts topic from filename without date suffix",
			path:      "/tmp/channels/refactor-plan.md",
			wantTopic: "refactor-plan",
		},
		{
			name:      "handles nested path",
			path:      "/a/b/c/my-topic-2025-01-01.md",
			wantTopic: "my-topic",
		},
		{
			name:      "handles filename with no .md extension and no date",
			path:      "/tmp/plain-name",
			wantTopic: "plain-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewChannel(tt.path)
			if ch.Path != tt.path {
				t.Errorf("Path = %q, want %q", ch.Path, tt.path)
			}
			if ch.Topic != tt.wantTopic {
				t.Errorf("Topic = %q, want %q", ch.Topic, tt.wantTopic)
			}
		})
	}
}

func TestChannelCreateAndExists(t *testing.T) {
	dir := t.TempDir()
	chPath := filepath.Join(dir, "subdir", "test-channel-2024-06-01.md")
	ch := NewChannel(chPath)

	// Before creation, Exists should return false.
	if ch.Exists() {
		t.Fatal("Exists() returned true before Create()")
	}

	// Create the channel file.
	if err := ch.Create(); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// After creation, Exists should return true.
	if !ch.Exists() {
		t.Fatal("Exists() returned false after Create()")
	}

	// Verify the file was actually written to disk.
	info, err := os.Stat(chPath)
	if err != nil {
		t.Fatalf("os.Stat failed after Create: %v", err)
	}
	if info.Size() == 0 {
		t.Error("created file is empty, expected header content")
	}

	// Duplicate Create should return an error.
	if err := ch.Create(); err == nil {
		t.Fatal("expected error on duplicate Create(), got nil")
	}
}

func TestAppendMessageAndRead(t *testing.T) {
	dir := t.TempDir()
	chPath := filepath.Join(dir, "msg-test-2024-07-01.md")
	ch := NewChannel(chPath)

	if err := ch.Create(); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	msg := &protocol.Message{
		AgentID:       "test-agent",
		Timestamp:     time.Date(2024, 7, 1, 10, 30, 0, 0, time.UTC),
		Status:        protocol.StatusPending,
		MessageNumber: 1,
		Context:       "Test context",
		Proposal:      "Test proposal",
		Questions:     []string{},
		Blockers:      []string{},
		NextSteps:     []string{},
	}

	if err := ch.AppendMessage(msg); err != nil {
		t.Fatalf("AppendMessage() error: %v", err)
	}

	// Read back the content and verify it contains the message fields.
	content, err := ch.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	for _, want := range []string{
		"test-agent",
		"pending",
		"Test context",
		"Test proposal",
	} {
		if !containsString(content, want) {
			t.Errorf("Read() content missing %q", want)
		}
	}

	// GetAllMessages should parse back the message we wrote.
	messages, err := ch.GetAllMessages()
	if err != nil {
		t.Fatalf("GetAllMessages() error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("GetAllMessages() returned %d messages, want 1", len(messages))
	}

	got := messages[0]
	if got.AgentID != "test-agent" {
		t.Errorf("parsed AgentID = %q, want %q", got.AgentID, "test-agent")
	}
	if got.Status != protocol.StatusPending {
		t.Errorf("parsed Status = %q, want %q", got.Status, protocol.StatusPending)
	}
	if got.MessageNumber != 1 {
		t.Errorf("parsed MessageNumber = %d, want 1", got.MessageNumber)
	}
	if got.Context != "Test context" {
		t.Errorf("parsed Context = %q, want %q", got.Context, "Test context")
	}
	if got.Proposal != "Test proposal" {
		t.Errorf("parsed Proposal = %q, want %q", got.Proposal, "Test proposal")
	}

	// Test with list sections populated (exercises extractListSection indirectly).
	msg2 := &protocol.Message{
		AgentID:       "agent-b",
		Timestamp:     time.Date(2024, 7, 1, 11, 0, 0, 0, time.UTC),
		Status:        protocol.StatusAwaitingResponse,
		MessageNumber: 2,
		Context:       "Second message context",
		Proposal:      "Second proposal",
		Questions:     []string{"What is the deadline?", "Who reviews this?"},
		Blockers:      []string{"Waiting on CI"},
		NextSteps:     []string{"Run tests", "Deploy to staging"},
	}

	if err := ch.AppendMessage(msg2); err != nil {
		t.Fatalf("AppendMessage(msg2) error: %v", err)
	}

	messages, err = ch.GetAllMessages()
	if err != nil {
		t.Fatalf("GetAllMessages() after second message error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("GetAllMessages() returned %d messages, want 2", len(messages))
	}

	got2 := messages[1]
	if got2.AgentID != "agent-b" {
		t.Errorf("second message AgentID = %q, want %q", got2.AgentID, "agent-b")
	}
	if len(got2.Questions) != 2 {
		t.Errorf("second message Questions count = %d, want 2", len(got2.Questions))
	}
	if len(got2.Blockers) != 1 {
		t.Errorf("second message Blockers count = %d, want 1", len(got2.Blockers))
	}
	if len(got2.NextSteps) != 2 {
		t.Errorf("second message NextSteps count = %d, want 2", len(got2.NextSteps))
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && // quick guard
		len(needle) > 0 &&
		stringContains(haystack, needle)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
