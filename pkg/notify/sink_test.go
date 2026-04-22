package notify

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// mockDispatcher records dispatched notifications for testing.
type mockDispatcher struct {
	mu            sync.Mutex
	name          string
	notifications []*Notification
	err           error
}

func (m *mockDispatcher) Name() string { return m.name }

func (m *mockDispatcher) Dispatch(_ context.Context, n *Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, n)
	return m.err
}

func (m *mockDispatcher) Close() error { return nil }

func (m *mockDispatcher) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.notifications)
}

func (m *mockDispatcher) last() *Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.notifications) == 0 {
		return nil
	}
	return m.notifications[len(m.notifications)-1]
}

func TestNotificationSink_Name(t *testing.T) {
	s := NewNotificationSink(nil)
	if s.Name() != "notify" {
		t.Errorf("Name() = %q, want %q", s.Name(), "notify")
	}
}

func TestNotificationSink_HandleEvent_Dispatches(t *testing.T) {
	d := &mockDispatcher{name: "test"}
	s := NewNotificationSink(slog.Default(), d)

	event := &eventbus.Event{
		ID:        "evt-1",
		Type:      eventbus.TypeNotificationPhaseComplete,
		Channel:   eventbus.ChannelNotification,
		Source:    "test-source",
		Timestamp: time.Now(),
		Level:     slog.LevelInfo,
		Data: map[string]interface{}{
			"title": "Phase Complete",
			"body":  "Phase 3 finished successfully",
		},
	}

	if err := s.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	if d.count() != 1 {
		t.Fatalf("expected 1 notification, got %d", d.count())
	}

	n := d.last()
	if n.ID != "evt-1" {
		t.Errorf("notification ID = %q, want %q", n.ID, "evt-1")
	}
	if n.Title != "Phase Complete" {
		t.Errorf("notification Title = %q, want %q", n.Title, "Phase Complete")
	}
	if n.Body != "Phase 3 finished successfully" {
		t.Errorf("notification Body = %q, want %q", n.Body, "Phase 3 finished successfully")
	}
	if n.Source != "test-source" {
		t.Errorf("notification Source = %q, want %q", n.Source, "test-source")
	}
}

func TestNotificationSink_SkipsNonNotificationChannel(t *testing.T) {
	d := &mockDispatcher{name: "test"}
	s := NewNotificationSink(nil, d)

	event := &eventbus.Event{
		ID:      "evt-2",
		Type:    "telemetry.agent.launch",
		Channel: eventbus.ChannelTelemetry,
	}

	if err := s.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	if d.count() != 0 {
		t.Errorf("expected 0 notifications for telemetry channel, got %d", d.count())
	}
}

func TestNotificationSink_HandleEvent_DispatcherError(t *testing.T) {
	failing := &mockDispatcher{name: "fail", err: errors.New("send failed")}
	passing := &mockDispatcher{name: "pass"}
	s := NewNotificationSink(slog.Default(), failing, passing)

	event := &eventbus.Event{
		ID:      "evt-3",
		Type:    eventbus.TypeNotificationSessionEnd,
		Channel: eventbus.ChannelNotification,
		Data:    map[string]interface{}{},
	}

	err := s.HandleEvent(context.Background(), event)
	if err == nil {
		t.Fatal("expected error from failing dispatcher")
	}

	// Both dispatchers should still be called.
	if failing.count() != 1 {
		t.Errorf("failing dispatcher: expected 1 call, got %d", failing.count())
	}
	if passing.count() != 1 {
		t.Errorf("passing dispatcher: expected 1 call, got %d", passing.count())
	}
}

func TestNotificationSink_FallbackTitle(t *testing.T) {
	d := &mockDispatcher{name: "test"}
	s := NewNotificationSink(nil, d)

	event := &eventbus.Event{
		ID:      "evt-4",
		Type:    "notification.custom.event",
		Channel: eventbus.ChannelNotification,
		Data:    map[string]interface{}{},
	}

	if err := s.HandleEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	n := d.last()
	if n.Title != "notification.custom.event" {
		t.Errorf("Title = %q, want event type as fallback", n.Title)
	}
}

func TestNotificationSink_Close(t *testing.T) {
	d1 := &mockDispatcher{name: "a"}
	d2 := &mockDispatcher{name: "b"}
	s := NewNotificationSink(nil, d1, d2)

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestEventToNotification(t *testing.T) {
	event := &eventbus.Event{
		ID:        "id-1",
		Type:      "notification.test",
		Channel:   eventbus.ChannelNotification,
		Source:    "src",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Level:     slog.LevelWarn,
		Data: map[string]interface{}{
			"title": "Test Title",
			"body":  "Test Body",
			"extra": 42,
		},
	}

	n := eventToNotification(event)

	if n.ID != "id-1" {
		t.Errorf("ID = %q, want %q", n.ID, "id-1")
	}
	if n.Title != "Test Title" {
		t.Errorf("Title = %q, want %q", n.Title, "Test Title")
	}
	if n.Body != "Test Body" {
		t.Errorf("Body = %q, want %q", n.Body, "Test Body")
	}
	if n.Level != slog.LevelWarn {
		t.Errorf("Level = %v, want %v", n.Level, slog.LevelWarn)
	}
	if n.Meta["extra"] != 42 {
		t.Errorf("Meta[extra] = %v, want 42", n.Meta["extra"])
	}
}

func TestBuildDispatchers(t *testing.T) {
	disabled := false
	cfg := &Config{
		Dispatchers: []DispatcherConfig{
			{Type: "log"},
			{Type: "webhook", URL: "http://localhost:9999/hook"},
			{Type: "tmux", Target: "main"},
			{Type: "desktop"},
			{Type: "log", Enabled: &disabled},
		},
	}

	dispatchers, err := BuildDispatchers(cfg, slog.Default())
	if err != nil {
		t.Fatalf("BuildDispatchers() error = %v", err)
	}

	if len(dispatchers) != 4 {
		t.Fatalf("expected 4 dispatchers (1 disabled), got %d", len(dispatchers))
	}

	names := make([]string, len(dispatchers))
	for i, d := range dispatchers {
		names[i] = d.Name()
	}

	expected := []string{"log", "webhook", "tmux", "desktop"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("dispatcher[%d].Name() = %q, want %q", i, names[i], want)
		}
	}
}

func TestBuildDispatchers_WebhookMissingURL(t *testing.T) {
	cfg := &Config{
		Dispatchers: []DispatcherConfig{
			{Type: "webhook"},
		},
	}

	_, err := BuildDispatchers(cfg, nil)
	if err == nil {
		t.Fatal("expected error for webhook without URL")
	}
}

func TestBuildDispatchers_UnknownType(t *testing.T) {
	cfg := &Config{
		Dispatchers: []DispatcherConfig{
			{Type: "carrier-pigeon"},
		},
	}

	_, err := BuildDispatchers(cfg, nil)
	if err == nil {
		t.Fatal("expected error for unknown dispatcher type")
	}
}

// Verify NotificationSink implements eventbus.Sink at compile time.
var _ eventbus.Sink = (*NotificationSink)(nil)
