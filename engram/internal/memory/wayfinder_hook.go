package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// WayfinderHook integrates episodic memory with Wayfinder phase transitions.
// When a Wayfinder session ends, it automatically appends a summary to DECISION_LOG.md.
type WayfinderHook struct {
	memory   *EpisodicMemory
	eventBus *eventbus.LocalBus
	mu       sync.Mutex
}

// WayfinderHookOption is a functional option for configuring WayfinderHook.
type WayfinderHookOption func(*WayfinderHook)

// WithEventBus configures the WayfinderHook to publish events to the given EventBus.
func WithEventBus(bus *eventbus.LocalBus) WayfinderHookOption {
	return func(wh *WayfinderHook) {
		wh.eventBus = bus
	}
}

// NewWayfinderHook creates a new Wayfinder integration hook.
func NewWayfinderHook(memory *EpisodicMemory, opts ...WayfinderHookOption) *WayfinderHook {
	wh := &WayfinderHook{
		memory: memory,
	}
	for _, opt := range opts {
		opt(wh)
	}
	return wh
}

// publishEvent publishes an event to the EventBus if one is configured.
func (wh *WayfinderHook) publishEvent(topic string, data map[string]interface{}) {
	if wh.eventBus == nil {
		return
	}
	event := eventbus.NewEvent(topic, "wayfinder-hook", data)
	go wh.eventBus.Publish(context.Background(), event)
}

// OnPhaseComplete is called when a Wayfinder phase completes.
// It records the phase outcome in DECISION_LOG.md.
func (wh *WayfinderHook) OnPhaseComplete(ctx context.Context, event *PhaseCompleteEvent) error {
	wh.mu.Lock()
	defer wh.mu.Unlock()

	// Build summary from phase event
	summary := fmt.Sprintf("Wayfinder Phase %s completed: %s", event.PhaseName, event.Outcome)

	// Build details
	details := fmt.Sprintf(`Phase: %s
Outcome: %s
Duration: %s
Errors: %d
Rework: %d
Quality Score: %.2f

Key Decisions:
%s

Lessons Learned:
%s`,
		event.PhaseName,
		event.Outcome,
		event.Duration,
		event.ErrorCount,
		event.ReworkCount,
		event.QualityScore,
		event.KeyDecisions,
		event.LessonsLearned,
	)

	// Publish event to EventBus
	wh.publishEvent("wayfinder.phase.completed", map[string]interface{}{
		"session_id":    event.SessionID,
		"phase_name":    event.PhaseName,
		"outcome":       event.Outcome,
		"duration":      event.Duration,
		"error_count":   event.ErrorCount,
		"rework_count":  event.ReworkCount,
		"quality_score": event.QualityScore,
	})

	// Record as episodic memory
	return wh.memory.RecordDecision(ctx, event.SessionID, summary, details)
}

// OnSessionComplete is called when a Wayfinder session ends.
// This is where the "Molt" behavior occurs if token threshold is exceeded.
func (wh *WayfinderHook) OnSessionComplete(ctx context.Context, event *SessionCompleteEvent) error {
	wh.mu.Lock()
	defer wh.mu.Unlock()

	// Publish event to EventBus
	wh.publishEvent("wayfinder.session.completed", map[string]interface{}{
		"session_id":       event.SessionID,
		"total_phases":     event.TotalPhases,
		"successful":       event.SuccessfulPhases,
		"failed":           event.FailedPhases,
		"total_duration":   event.TotalDuration,
		"total_cost":       event.TotalCost,
		"total_tokens":     event.TotalTokens,
		"token_percentage": event.TokenPercentage,
	})

	// Check if we should molt (token > 80%)
	shouldMolt := wh.memory.ShouldMolt(event.TotalTokens)

	if shouldMolt {
		summary := fmt.Sprintf("Session %s completed (token threshold exceeded: %d tokens)", event.SessionID, event.TotalTokens)
		details := fmt.Sprintf(`Session Summary:
- Total Phases: %d
- Successful Phases: %d
- Failed Phases: %d
- Total Duration: %s
- Total Cost: $%.4f
- Token Usage: %d (%.1f%% of max)

Project Evolution:
%s

Key Learnings:
%s`,
			event.TotalPhases,
			event.SuccessfulPhases,
			event.FailedPhases,
			event.TotalDuration,
			event.TotalCost,
			event.TotalTokens,
			event.TokenPercentage,
			event.ProjectEvolution,
			event.KeyLearnings,
		)

		return wh.memory.MoltSession(ctx, event.SessionID, summary, details)
	}

	// Otherwise, just record the session end as a normal decision
	summary := fmt.Sprintf("Session %s completed normally", event.SessionID)
	details := fmt.Sprintf("Total phases: %d, Duration: %s", event.TotalPhases, event.TotalDuration)

	return wh.memory.RecordDecision(ctx, event.SessionID, summary, details)
}

// PhaseCompleteEvent represents a completed Wayfinder phase.
type PhaseCompleteEvent struct {
	SessionID      string
	PhaseName      string // "D1", "D2", "I1", etc.
	Outcome        string // "success", "failure", "skipped"
	Duration       string // "2h30m"
	ErrorCount     int
	ReworkCount    int
	QualityScore   float64 // 0.0-1.0
	KeyDecisions   string  // Markdown list of decisions made
	LessonsLearned string  // Markdown list of lessons
}

// SessionCompleteEvent represents a completed Wayfinder session.
type SessionCompleteEvent struct {
	SessionID        string
	TotalPhases      int
	SuccessfulPhases int
	FailedPhases     int
	TotalDuration    string
	TotalCost        float64
	TotalTokens      int
	TokenPercentage  float64
	ProjectEvolution string // Narrative of how project evolved
	KeyLearnings     string // Consolidated lessons from all phases
}
