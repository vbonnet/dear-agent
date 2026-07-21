package simple

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

func TestSimpleFileProvider_WorkingContextRoundTrip(t *testing.T) {
	provider := &SimpleFileProvider{storagePath: t.TempDir()}
	phase := "DESIGN"
	ctx := context.Background()
	if err := provider.UpdateWorkingContext(ctx, "session-123", consolidation.ContextUpdate{
		SetPhase: &phase,
		AddTasks: []consolidation.Task{{ID: "task-1", Status: "in-progress"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := provider.UpdateWorkingContext(ctx, "session-123", consolidation.ContextUpdate{
		CompleteTasks: []string{"task-1"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := provider.GetWorkingContext(ctx, "session-123")
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "session-123" || got.CurrentPhase != phase {
		t.Fatalf("working context = %#v", got)
	}
	if len(got.ActiveTasks) != 1 || got.ActiveTasks[0].Status != "completed" {
		t.Fatalf("active tasks = %#v", got.ActiveTasks)
	}
}

func TestSimpleFileProvider_SessionHistoryRoundTripAndPersist(t *testing.T) {
	provider := &SimpleFileProvider{storagePath: t.TempDir()}
	bus := &mockEventBus{}
	ctx := consolidation.WithEventBus(context.Background(), bus)
	eventTime := time.Now().Add(-time.Minute)

	if err := provider.AppendSessionEvent(ctx, "session-123", consolidation.SessionEvent{
		Timestamp: eventTime,
		Type:      "phase_started",
		Data:      map[string]any{"phase": "DESIGN"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := provider.AppendSessionEvent(ctx, "session-123", consolidation.SessionEvent{Type: "phase_completed"}); err != nil {
		t.Fatal(err)
	}
	if err := provider.PersistSession(ctx, "session-123"); err != nil {
		t.Fatal(err)
	}

	got, err := provider.GetSessionHistory(ctx, "session-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Events) != 2 || got.EndTime == nil || !got.StartTime.Equal(eventTime) {
		t.Fatalf("session history = %#v", got)
	}
	if len(bus.events) != 1 || bus.events[0].Topic != consolidation.TopicSessionPersisted {
		t.Fatalf("events = %#v", bus.events)
	}
}

func TestSimpleFileProvider_SessionOperationsRejectTraversalAndMissing(t *testing.T) {
	provider := &SimpleFileProvider{storagePath: t.TempDir()}
	ctx := context.Background()
	if _, err := provider.GetWorkingContext(ctx, "missing"); !errors.Is(err, consolidation.ErrNotFound) {
		t.Fatalf("GetWorkingContext missing error = %v", err)
	}
	if err := provider.AppendSessionEvent(ctx, "../escape", consolidation.SessionEvent{}); !errors.Is(err, consolidation.ErrInvalidNamespace) {
		t.Fatalf("AppendSessionEvent traversal error = %v", err)
	}
	if err := provider.PersistSession(ctx, "missing"); !errors.Is(err, consolidation.ErrNotFound) {
		t.Fatalf("PersistSession missing error = %v", err)
	}
}
