package opencode

import (
	"sync"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// mockEventBus is a mock implementation of EventBusPublisher for testing
type mockEventBus struct {
	mu         sync.Mutex
	published  []*eventbus.Event
	shouldDrop bool // If true, simulate dropped events (queue full)
	dropCount  int
}

func (m *mockEventBus) Broadcast(event *eventbus.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldDrop {
		m.dropCount++
		return
	}

	m.published = append(m.published, event)
}

func (m *mockEventBus) GetEvents() []*eventbus.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.published
}

func (m *mockEventBus) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = nil
	m.dropCount = 0
}

func (m *mockEventBus) SetShouldDrop(drop bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldDrop = drop
}

func (m *mockEventBus) GetDropCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropCount
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(substr) == 0 || len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

// containsHelper is a helper function for contains
func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
