package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// EventAgentLaunch is the event type emitted when an agent launches.
const EventAgentLaunch = "agent_launch"

// Telemetry provides the public API for agent telemetry logging.
type Telemetry struct {
	bus     *eventbus.LocalBus
	storage *Storage
}

// NewTelemetry creates a new telemetry instance.
//
// If storage is nil, telemetry will only publish events to EventBus (no persistence).
// If bus is nil, telemetry will only persist to storage (no event publishing).
//
// Example:
//
//	storage, _ := NewStorage()
//	telemetry := NewTelemetry(eventBus, storage)
//	defer telemetry.Close()
func NewTelemetry(bus *eventbus.LocalBus, storage *Storage) *Telemetry {
	return &Telemetry{
		bus:     bus,
		storage: storage,
	}
}

// Close closes the telemetry instance and releases resources.
func (t *Telemetry) Close() error {
	if t.storage != nil {
		return t.storage.Close()
	}
	return nil
}

// LogAgentLaunch logs an agent launch with extracted prompt features.
//
// Returns the launch ID for later updating with outcome data.
//
// Example:
//
//	id, err := telemetry.LogAgentLaunch(ctx, prompt, "claude-sonnet-4.5")
//	if err != nil {
//	    log.Printf("Failed to log agent launch: %v", err)
//	}
func (t *Telemetry) LogAgentLaunch(ctx context.Context, prompt, model string) (int64, error) {
	return t.LogAgentLaunchFull(ctx, prompt, model, "", "", "")
}

// LogAgentLaunchFull logs an agent launch with all metadata.
//
// Parameters:
//   - prompt: The prompt text used to launch the agent
//   - model: Model name (e.g., "claude-sonnet-4.5")
//   - taskDescription: Optional task description
//   - sessionID: Optional session ID for grouping related launches
//   - parentAgentID: Optional parent agent ID for nested agents
//
// Example:
//
//	id, err := telemetry.LogAgentLaunchFull(
//	    ctx, prompt, "claude-sonnet-4.5", "Analyze codebase", sessionID, "",
//	)
func (t *Telemetry) LogAgentLaunchFull(ctx context.Context, prompt, model, taskDesc, sessionID, parentID string) (int64, error) {
	// Extract features from prompt
	features := ExtractFeatures(prompt)

	// Persist to storage
	var id int64
	var err error
	if t.storage != nil {
		id, err = t.storage.LogLaunchFull(ctx, prompt, model, taskDesc, sessionID, parentID, features)
		if err != nil {
			return 0, fmt.Errorf("failed to persist launch: %w", err)
		}
	}

	// Publish event to EventBus
	if t.bus != nil {
		event := createLaunchEvent(id, prompt, model, taskDesc, sessionID, parentID, features)
		if err := t.bus.Publish(ctx, event); err != nil {
			// Log error but don't fail (telemetry is non-critical)
			// TODO: Use proper logger
			_ = err
		}
	}

	return id, nil
}

// LogAgentCompletion updates an agent launch with completion data.
//
// Parameters:
//   - launchID: The ID returned from LogAgentLaunch
//   - outcome: "success", "failure", or "partial"
//   - tokensUsed: Number of tokens consumed
//
// Example:
//
//	err := telemetry.LogAgentCompletion(ctx, id, "success", 1500)
func (t *Telemetry) LogAgentCompletion(ctx context.Context, launchID int64, outcome string, tokensUsed int) error {
	return t.LogAgentCompletionFull(ctx, launchID, outcome, tokensUsed, 0, "", 0)
}

// LogAgentCompletionFull updates an agent launch with all completion data.
func (t *Telemetry) LogAgentCompletionFull(ctx context.Context, launchID int64, outcome string, tokensUsed, retryCount int, errorMsg string, durationMs int) error {
	if t.storage != nil {
		return t.storage.UpdateOutcomeFull(ctx, launchID, outcome, tokensUsed, retryCount, errorMsg, durationMs)
	}
	return nil
}

// Query retrieves agent launches matching the specified filters.
func (t *Telemetry) Query(ctx context.Context, filters QueryFilters) ([]AgentLaunch, error) {
	if t.storage == nil {
		return nil, fmt.Errorf("storage not configured")
	}
	return t.storage.Query(ctx, filters)
}

// Stats returns aggregate statistics for agent launches.
func (t *Telemetry) Stats(ctx context.Context, model string) (*AgentStats, error) {
	if t.storage == nil {
		return nil, fmt.Errorf("storage not configured")
	}
	return t.storage.Stats(ctx, model)
}

// createLaunchEvent creates an EventBus event for agent launch.
func createLaunchEvent(id int64, prompt, model, taskDesc, sessionID, parentID string, features Features) *eventbus.Event {
	data := map[string]interface{}{
		"launch_id":               id,
		"prompt_text":             prompt,
		"model":                   model,
		"task_description":        taskDesc,
		"session_id":              sessionID,
		"parent_agent_id":         parentID,
		"word_count":              features.WordCount,
		"token_count":             features.TokenCount,
		"specificity_score":       features.SpecificityScore,
		"has_examples":            features.HasExamples,
		"has_constraints":         features.HasConstraints,
		"context_embedding_score": features.ContextEmbeddingScore,
		"timestamp":               time.Now().Format(time.RFC3339),
	}

	return eventbus.NewEvent(EventAgentLaunch, "telemetry", data)
}
