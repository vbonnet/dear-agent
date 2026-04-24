package ecphory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

// MockEventBus implements EventBus for testing
type MockEventBus struct {
	mu     sync.Mutex
	events []*Event
}

func (m *MockEventBus) Publish(ctx context.Context, event *Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func TestEcphoryTelemetry_EventPublishing(t *testing.T) {
	// Create mock event bus
	mockBus := &MockEventBus{events: make([]*Event, 0)}

	// Create ecphory instance (would need proper setup with real engram path)
	// This is a conceptual test showing the API
	e := &Ecphory{
		eventBus:    mockBus,
		basePath:    "/tmp/test/.engram/engrams",
		tokenBudget: 10000,
	}

	// Simulate query results
	results := []*engram.Engram{
		{
			Path: "/tmp/test/.engram/engrams/go/errors.ai.md",
			Frontmatter: engram.Frontmatter{
				Title: "Error Handling",
				Tags:  []string{"go", "errors"},
			},
			Content: "Error handling patterns...",
		},
	}

	// Publish event
	e.publishEcphoryEvent(
		context.Background(),
		"error handling patterns",
		"test-session-telemetry",
		"test transcript",
		[]string{"go"},
		"claude-code",
		results,
		100,
		50*time.Millisecond,
	)

	// Wait for async publish
	time.Sleep(100 * time.Millisecond)

	// Verify event was published
	mockBus.mu.Lock()
	eventCount := len(mockBus.events)
	var event *Event
	if eventCount > 0 {
		event = mockBus.events[0]
	}
	mockBus.mu.Unlock()

	if eventCount != 1 {
		t.Fatalf("expected 1 event, got %d", eventCount)
	}

	// Verify event structure
	if event.Topic != "ecphory.query" {
		t.Errorf("expected topic 'ecphory.query', got '%s'", event.Topic)
	}

	if event.Publisher != "ecphory" {
		t.Errorf("expected publisher 'ecphory', got '%s'", event.Publisher)
	}

	// Verify event data
	if event.Data["query"] != "error handling patterns" {
		t.Errorf("unexpected query: %v", event.Data["query"])
	}

	if event.Data["result_count"] != 1 {
		t.Errorf("expected result_count=1, got %v", event.Data["result_count"])
	}

	if event.Data["tokens_used"] != 100 {
		t.Errorf("expected tokens_used=100, got %v", event.Data["tokens_used"])
	}

	// Verify relative paths (privacy)
	paths := event.Data["result_paths"].([]string)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	// Path should be relative, not absolute
	if paths[0] == "/tmp/test/.engram/engrams/go/errors.ai.md" {
		t.Error("path should be relative, not absolute (privacy violation)")
	}
}

func TestEcphoryTelemetry_NoEventBus(t *testing.T) {
	// Create ecphory without event bus
	e := &Ecphory{
		eventBus:    nil, // Telemetry disabled
		basePath:    "/tmp/test/.engram/engrams",
		tokenBudget: 10000,
	}

	// This should not panic
	e.publishEcphoryEvent(
		context.Background(),
		"test query",
		"test-session-disabled",
		"test transcript",
		[]string{"test"},
		"claude-code",
		nil,
		0,
		10*time.Millisecond,
	)

	// Success - no crash when eventBus is nil
}

func TestRelativePath_Privacy(t *testing.T) {
	e := &Ecphory{
		basePath: "/tmp/test/.engram/engrams",
	}

	tests := []struct {
		absPath  string
		expected string
	}{
		{
			absPath:  "/tmp/test/.engram/engrams/go/errors.ai.md",
			expected: "go/errors.ai.md",
		},
		{
			absPath:  "/tmp/test/.engram/engrams/python/fastapi.ai.md",
			expected: "python/fastapi.ai.md",
		},
	}

	for _, tt := range tests {
		result := e.relativePath(tt.absPath)
		if result != tt.expected {
			t.Errorf("relativePath(%s) = %s, want %s", tt.absPath, result, tt.expected)
		}

		// Verify no username in result
		if len(result) > 0 && (result[0] == '/' || result == tt.absPath) {
			t.Errorf("relativePath should not return absolute path: %s", result)
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	e := &Ecphory{}

	engrams := []*engram.Engram{
		{Content: "1234"},     // 4 chars = 1 token
		{Content: "12345678"}, // 8 chars = 2 tokens
	}

	tokens := e.estimateTokens(engrams)
	if tokens != 3 {
		t.Errorf("expected 3 tokens, got %d", tokens)
	}
}
