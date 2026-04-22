package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

func TestWayfinderHook_OnPhaseComplete(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	hook := NewWayfinderHook(em)
	ctx := context.Background()

	event := &PhaseCompleteEvent{
		SessionID:      "wayfinder-001",
		PhaseName:      "D2",
		Outcome:        "success",
		Duration:       "45m",
		ErrorCount:     0,
		ReworkCount:    1,
		QualityScore:   0.95,
		KeyDecisions:   "- Chose PostgreSQL over MongoDB\n- Used React for frontend",
		LessonsLearned: "- Database normalization improves query performance\n- Component reusability reduces code duplication",
	}

	// Test: Record phase completion
	if err := hook.OnPhaseComplete(ctx, event); err != nil {
		t.Fatalf("OnPhaseComplete failed: %v", err)
	}

	// Verify DECISION_LOG.md contains phase details
	logPath := filepath.Join(tmpDir, "DECISION_LOG.md")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)

	// Check for expected content
	expectedStrings := []string{
		"wayfinder-001",
		"Phase D2 completed: success",
		"Duration: 45m",
		"Errors: 0",
		"Rework: 1",
		"Quality Score: 0.95",
		"Chose PostgreSQL over MongoDB",
		"Database normalization improves query performance",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("DECISION_LOG.md missing expected string: %q", expected)
		}
	}
}

func TestWayfinderHook_OnSessionComplete_NoMolt(t *testing.T) {
	tmpDir := t.TempDir()
	maxTokens := 200000
	em, err := NewEpisodicMemory(tmpDir, maxTokens)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	hook := NewWayfinderHook(em)
	ctx := context.Background()

	event := &SessionCompleteEvent{
		SessionID:        "wayfinder-002",
		TotalPhases:      5,
		SuccessfulPhases: 5,
		FailedPhases:     0,
		TotalDuration:    "3h15m",
		TotalCost:        2.50,
		TotalTokens:      100000, // 50% - below 80% threshold
		TokenPercentage:  50.0,
		ProjectEvolution: "Started with prototype, evolved to production-ready system",
		KeyLearnings:     "Incremental development reduces risk",
	}

	// Test: Record session completion (no molt)
	if err := hook.OnSessionComplete(ctx, event); err != nil {
		t.Fatalf("OnSessionComplete failed: %v", err)
	}

	// Verify normal session end was recorded
	logPath := filepath.Join(tmpDir, "DECISION_LOG.md")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)

	// Should NOT contain molt trigger
	if strings.Contains(contentStr, "token threshold exceeded") {
		t.Error("Session should not have triggered molt (below 80%)")
	}

	// Should contain normal session end
	if !strings.Contains(contentStr, "Session wayfinder-002 completed normally") {
		t.Error("Missing normal session completion entry")
	}
}

func TestWayfinderHook_OnSessionComplete_WithMolt(t *testing.T) {
	tmpDir := t.TempDir()
	maxTokens := 200000
	em, err := NewEpisodicMemory(tmpDir, maxTokens)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	hook := NewWayfinderHook(em)
	ctx := context.Background()

	event := &SessionCompleteEvent{
		SessionID:        "wayfinder-003",
		TotalPhases:      8,
		SuccessfulPhases: 7,
		FailedPhases:     1,
		TotalDuration:    "6h30m",
		TotalCost:        8.75,
		TotalTokens:      165000, // 82.5% - above 80% threshold
		TokenPercentage:  82.5,
		ProjectEvolution: "Implemented core features, refactored architecture twice, achieved 90% test coverage",
		KeyLearnings:     "Early refactoring prevents technical debt accumulation. Test-first development reduces rework.",
	}

	// Test: Record session completion (should molt)
	if err := hook.OnSessionComplete(ctx, event); err != nil {
		t.Fatalf("OnSessionComplete failed: %v", err)
	}

	// Verify molt was triggered
	logPath := filepath.Join(tmpDir, "DECISION_LOG.md")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)

	// Should contain molt indicators
	expectedStrings := []string{
		"token threshold exceeded",
		"165000 tokens",
		"Total Phases: 8",
		"Successful Phases: 7",
		"Failed Phases: 1",
		"Total Duration: 6h30m",
		"Total Cost: $8.7500",
		"Token Usage: 165000 (82.5% of max)",
		"Implemented core features, refactored architecture twice",
		"Early refactoring prevents technical debt accumulation",
		"molt", // Event type should be "molt"
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("DECISION_LOG.md missing molt indicator: %q", expected)
		}
	}
}

func TestWayfinderHook_MultiplePhases(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	hook := NewWayfinderHook(em)
	ctx := context.Background()

	// Simulate multiple phase completions
	phases := []PhaseCompleteEvent{
		{
			SessionID:      "wayfinder-multi",
			PhaseName:      "D1",
			Outcome:        "success",
			Duration:       "30m",
			ErrorCount:     0,
			ReworkCount:    0,
			QualityScore:   1.0,
			KeyDecisions:   "Defined project scope",
			LessonsLearned: "Clear scope prevents scope creep",
		},
		{
			SessionID:      "wayfinder-multi",
			PhaseName:      "D2",
			Outcome:        "success",
			Duration:       "45m",
			ErrorCount:     1,
			ReworkCount:    0,
			QualityScore:   0.95,
			KeyDecisions:   "Selected technology stack",
			LessonsLearned: "Technology choices impact development velocity",
		},
		{
			SessionID:      "wayfinder-multi",
			PhaseName:      "I1",
			Outcome:        "success",
			Duration:       "2h",
			ErrorCount:     2,
			ReworkCount:    1,
			QualityScore:   0.85,
			KeyDecisions:   "Implemented core functionality",
			LessonsLearned: "Incremental commits make debugging easier",
		},
	}

	for _, phase := range phases {
		if err := hook.OnPhaseComplete(ctx, &phase); err != nil {
			t.Fatalf("OnPhaseComplete failed for phase %s: %v", phase.PhaseName, err)
		}
	}

	// Verify all phases recorded
	logPath := filepath.Join(tmpDir, "DECISION_LOG.md")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)

	// Check all phase names present
	phaseNames := []string{"D1", "D2", "I1"}
	for _, name := range phaseNames {
		if !strings.Contains(contentStr, "Phase "+name) {
			t.Errorf("Missing phase: %s", name)
		}
	}
}

func TestWayfinderHook_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	hook := NewWayfinderHook(em)
	ctx := context.Background()

	// Simulate concurrent phase completions
	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			event := &PhaseCompleteEvent{
				SessionID:      "concurrent-session",
				PhaseName:      "D" + string(rune('1'+id)),
				Outcome:        "success",
				Duration:       "10m",
				ErrorCount:     0,
				ReworkCount:    0,
				QualityScore:   1.0,
				KeyDecisions:   "Concurrent phase decision",
				LessonsLearned: "Concurrency is hard",
			}

			if err := hook.OnPhaseComplete(ctx, event); err != nil {
				t.Errorf("Concurrent OnPhaseComplete failed: %v", err)
			}

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify log integrity (no corrupted entries)
	logPath := filepath.Join(tmpDir, "DECISION_LOG.md")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	contentStr := string(content)

	// Count phase entries (should have 3)
	phaseCount := strings.Count(contentStr, "Wayfinder Phase")
	if phaseCount != 3 {
		t.Errorf("Expected 3 phase entries, got %d", phaseCount)
	}
}

func TestOnPhaseCompletePublishesEvent(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	bus := eventbus.NewBus(nil)
	defer bus.Close()

	var mu sync.Mutex
	var receivedEvents []*eventbus.Event

	// Subscribe to phase completed events
	bus.Subscribe("wayfinder.phase.completed", "test-subscriber", func(ctx context.Context, evt *eventbus.Event) (*eventbus.Response, error) {
		mu.Lock()
		receivedEvents = append(receivedEvents, evt)
		mu.Unlock()
		return nil, nil
	})

	hook := NewWayfinderHook(em, WithEventBus(bus))
	ctx := context.Background()

	event := &PhaseCompleteEvent{
		SessionID:      "test-session-eb",
		PhaseName:      "D2",
		Outcome:        "success",
		Duration:       "30m",
		ErrorCount:     0,
		ReworkCount:    0,
		QualityScore:   0.90,
		KeyDecisions:   "- Used EventBus for integration",
		LessonsLearned: "- EventBus enables loose coupling",
	}

	if err := hook.OnPhaseComplete(ctx, event); err != nil {
		t.Fatalf("OnPhaseComplete failed: %v", err)
	}

	// Wait for async publish to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(receivedEvents))
	}

	evt := receivedEvents[0]
	if evt.Type != "wayfinder.phase.completed" {
		t.Errorf("Expected topic 'wayfinder.phase.completed', got %q", evt.Type)
	}
	if evt.Source != "wayfinder-hook" {
		t.Errorf("Expected publisher 'wayfinder-hook', got %q", evt.Source)
	}
	if evt.Data["session_id"] != "test-session-eb" {
		t.Errorf("Expected session_id 'test-session-eb', got %v", evt.Data["session_id"])
	}
	if evt.Data["phase_name"] != "D2" {
		t.Errorf("Expected phase_name 'D2', got %v", evt.Data["phase_name"])
	}
	if evt.Data["outcome"] != "success" {
		t.Errorf("Expected outcome 'success', got %v", evt.Data["outcome"])
	}
}

func TestOnSessionCompletePublishesEvent(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	bus := eventbus.NewBus(nil)
	defer bus.Close()

	var mu sync.Mutex
	var receivedEvents []*eventbus.Event

	// Subscribe to session completed events
	bus.Subscribe("wayfinder.session.completed", "test-subscriber", func(ctx context.Context, evt *eventbus.Event) (*eventbus.Response, error) {
		mu.Lock()
		receivedEvents = append(receivedEvents, evt)
		mu.Unlock()
		return nil, nil
	})

	hook := NewWayfinderHook(em, WithEventBus(bus))
	ctx := context.Background()

	event := &SessionCompleteEvent{
		SessionID:        "test-session-eb-2",
		TotalPhases:      3,
		SuccessfulPhases: 3,
		FailedPhases:     0,
		TotalDuration:    "1h",
		TotalCost:        1.00,
		TotalTokens:      50000,
		TokenPercentage:  25.0,
		ProjectEvolution: "Built core system",
		KeyLearnings:     "EventBus works",
	}

	if err := hook.OnSessionComplete(ctx, event); err != nil {
		t.Fatalf("OnSessionComplete failed: %v", err)
	}

	// Wait for async publish to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(receivedEvents))
	}

	evt := receivedEvents[0]
	if evt.Type != "wayfinder.session.completed" {
		t.Errorf("Expected topic 'wayfinder.session.completed', got %q", evt.Type)
	}
	if evt.Data["session_id"] != "test-session-eb-2" {
		t.Errorf("Expected session_id 'test-session-eb-2', got %v", evt.Data["session_id"])
	}
	if evt.Data["total_phases"] != 3 {
		t.Errorf("Expected total_phases 3, got %v", evt.Data["total_phases"])
	}
}

func TestOnPhaseCompleteNoEventBus(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	// Create hook WITHOUT EventBus - should not panic
	hook := NewWayfinderHook(em)
	ctx := context.Background()

	event := &PhaseCompleteEvent{
		SessionID:      "test-no-bus",
		PhaseName:      "D1",
		Outcome:        "success",
		Duration:       "15m",
		ErrorCount:     0,
		ReworkCount:    0,
		QualityScore:   1.0,
		KeyDecisions:   "- No bus configured",
		LessonsLearned: "- Should not panic without EventBus",
	}

	// This should NOT panic even without an EventBus
	if err := hook.OnPhaseComplete(ctx, event); err != nil {
		t.Fatalf("OnPhaseComplete should not fail without EventBus: %v", err)
	}

	// Verify the decision was still recorded
	logPath := filepath.Join(tmpDir, "DECISION_LOG.md")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read DECISION_LOG.md: %v", err)
	}

	if !strings.Contains(string(content), "Phase D1 completed: success") {
		t.Error("Decision should still be recorded without EventBus")
	}
}

func TestOnSessionCompleteNoEventBus(t *testing.T) {
	tmpDir := t.TempDir()
	em, err := NewEpisodicMemory(tmpDir, 200000)
	if err != nil {
		t.Fatalf("NewEpisodicMemory failed: %v", err)
	}

	// Create hook WITHOUT EventBus - should not panic
	hook := NewWayfinderHook(em)
	ctx := context.Background()

	event := &SessionCompleteEvent{
		SessionID:        "test-no-bus-session",
		TotalPhases:      2,
		SuccessfulPhases: 2,
		FailedPhases:     0,
		TotalDuration:    "30m",
		TotalCost:        0.50,
		TotalTokens:      20000,
		TokenPercentage:  10.0,
		ProjectEvolution: "Simple project",
		KeyLearnings:     "No bus is fine",
	}

	// This should NOT panic even without an EventBus
	if err := hook.OnSessionComplete(ctx, event); err != nil {
		t.Fatalf("OnSessionComplete should not fail without EventBus: %v", err)
	}
}
