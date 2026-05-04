package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// PhaseOrchestratorV2 manages phase transitions for Wayfinder V2 (9-phase consolidated schema)
type PhaseOrchestratorV2 struct {
	status *status.StatusV2
}

// NewPhaseOrchestratorV2 creates a new V2 orchestrator instance
func NewPhaseOrchestratorV2(st *status.StatusV2) *PhaseOrchestratorV2 {
	return &PhaseOrchestratorV2{
		status: st,
	}
}

// AdvancePhase attempts to advance from current phase to next phase.
//
// Deprecated: Use AdvancePhaseCtx for OTel span propagation.
func (o *PhaseOrchestratorV2) AdvancePhase() (string, error) {
	return o.AdvancePhaseCtx(context.Background())
}

// AdvancePhaseCtx attempts to advance from current phase to next phase.
// Creates a "wayfinder_phase" span for the transition.
func (o *PhaseOrchestratorV2) AdvancePhaseCtx(ctx context.Context) (string, error) {
	currentPhase := o.status.CurrentWaypoint

	// Determine next phase in sequence
	nextPhase, err := o.getNextPhaseInSequence(currentPhase)
	if err != nil {
		return "", err
	}

	tracer := otel.Tracer("engram/wayfinder")
	_, span := tracer.Start(ctx, "wayfinder_phase",
		trace.WithAttributes(
			attribute.String("phase.from", currentPhase),
			attribute.String("phase.to", nextPhase),
		))
	defer span.End()

	// Validate transition is allowed (includes phase skipping check)
	if err := o.validateTransition(currentPhase, nextPhase); err != nil {
		span.RecordError(err)
		return "", err
	}

	// Validate exit criteria for current phase
	if err := o.validateExitCriteria(currentPhase); err != nil {
		span.RecordError(err)
		return "", fmt.Errorf("exit criteria not met for %s: %w", currentPhase, err)
	}

	// Execute transition
	if err := o.executeTransition(currentPhase, nextPhase); err != nil {
		span.RecordError(err)
		return "", fmt.Errorf("failed to execute transition: %w", err)
	}

	return nextPhase, nil
}

// RewindPhase rewinds to a previous phase with reason.
//
// Deprecated: Use RewindPhaseCtx for OTel span propagation.
func (o *PhaseOrchestratorV2) RewindPhase(targetPhase string, reason string) error {
	return o.RewindPhaseCtx(context.Background(), targetPhase, reason)
}

// RewindPhaseCtx rewinds to a previous phase with reason.
// Creates a "wayfinder_phase" span for the rewind.
func (o *PhaseOrchestratorV2) RewindPhaseCtx(ctx context.Context, targetPhase string, reason string) error {
	currentPhase := o.status.CurrentWaypoint

	tracer := otel.Tracer("engram/wayfinder")
	_, span := tracer.Start(ctx, "wayfinder_phase",
		trace.WithAttributes(
			attribute.String("phase.from", currentPhase),
			attribute.String("phase.to", targetPhase),
			attribute.String("phase.rewind_reason", reason),
		))
	defer span.End()

	// Validate target phase is valid
	if !IsValidPhaseV2(targetPhase) {
		span.RecordError(fmt.Errorf("invalid target phase: %s", targetPhase))
		return fmt.Errorf("invalid target phase: %s", targetPhase)
	}

	// Validate target is before current phase
	if !IsRewindValid(currentPhase, targetPhase) {
		err := fmt.Errorf("cannot rewind from %s to %s: target must be an earlier phase", currentPhase, targetPhase)
		span.RecordError(err)
		return err
	}

	// Execute rewind
	o.executeRewind(targetPhase, reason)

	return nil
}

// ValidateCurrentPhase checks if current phase can proceed to next
// Returns nil if ready, error with details if not ready
func (o *PhaseOrchestratorV2) ValidateCurrentPhase() error {
	return o.validateExitCriteria(o.status.CurrentWaypoint)
}

// GetCurrentPhase returns the current phase
func (o *PhaseOrchestratorV2) GetCurrentPhase() string {
	return o.status.CurrentWaypoint
}

// GetNextPhase returns what the next phase would be (without advancing)
func (o *PhaseOrchestratorV2) GetNextPhase() (string, error) {
	return o.getNextPhaseInSequence(o.status.CurrentWaypoint)
}

// GetPhaseHistory returns the complete phase history
func (o *PhaseOrchestratorV2) GetPhaseHistory() []status.WaypointHistory {
	return o.status.WaypointHistory
}

// getNextPhaseInSequence determines the next phase after current
func (o *PhaseOrchestratorV2) getNextPhaseInSequence(current string) (string, error) {
	sequence := status.AllPhasesV2Schema()

	// Find current phase index
	currentIdx := -1
	for i, phase := range sequence {
		if phase == current {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return "", fmt.Errorf("current phase %s not found in sequence", current)
	}

	// Check if at final phase
	if currentIdx == len(sequence)-1 {
		return "", fmt.Errorf("already at final phase %s", current)
	}

	// Return next phase
	return sequence[currentIdx+1], nil
}

// executeTransition performs the phase transition
func (o *PhaseOrchestratorV2) executeTransition(_, to string) error {
	now := time.Now()

	// Mark current phase as completed
	if err := o.completeCurrentPhase(); err != nil {
		return err
	}

	// Update current phase
	o.status.CurrentWaypoint = to
	o.status.UpdatedAt = now

	// Add new phase to history
	newPhaseEntry := status.WaypointHistory{
		Name:      to,
		Status:    status.PhaseStatusV2InProgress,
		StartedAt: now,
	}
	o.status.WaypointHistory = append(o.status.WaypointHistory, newPhaseEntry)

	return nil
}

// executeRewind performs the phase rewind
func (o *PhaseOrchestratorV2) executeRewind(targetPhase, reason string) {
	now := time.Now()

	// Add rewind note to current phase
	if len(o.status.WaypointHistory) > 0 {
		lastIdx := len(o.status.WaypointHistory) - 1
		rewindNote := fmt.Sprintf("Rewound to %s on %s. Reason: %s",
			targetPhase, now.Format("2006-01-02"), reason)
		if o.status.WaypointHistory[lastIdx].Notes != "" {
			o.status.WaypointHistory[lastIdx].Notes += "\n" + rewindNote
		} else {
			o.status.WaypointHistory[lastIdx].Notes = rewindNote
		}
	}

	// Update current phase
	o.status.CurrentWaypoint = targetPhase
	o.status.UpdatedAt = now

	// Add new phase history entry for rewound phase
	newEntry := status.WaypointHistory{
		Name:      targetPhase,
		Status:    status.PhaseStatusV2InProgress,
		StartedAt: now,
		Notes:     fmt.Sprintf("Reworking after rewind. Original reason: %s", reason),
	}
	o.status.WaypointHistory = append(o.status.WaypointHistory, newEntry)
}

// completeCurrentPhase marks the current phase as completed
func (o *PhaseOrchestratorV2) completeCurrentPhase() error {
	if len(o.status.WaypointHistory) == 0 {
		// No history yet, nothing to complete
		return nil
	}

	// Find the last phase entry matching current phase
	now := time.Now()
	for i := len(o.status.WaypointHistory) - 1; i >= 0; i-- {
		if o.status.WaypointHistory[i].Name == o.status.CurrentWaypoint {
			o.status.WaypointHistory[i].Status = status.PhaseStatusV2Completed
			o.status.WaypointHistory[i].CompletedAt = &now
			outcome := "success"
			o.status.WaypointHistory[i].Outcome = &outcome
			break
		}
	}

	return nil
}

// IsValidPhaseV2 checks if a phase name is valid in V2 schema
func IsValidPhaseV2(phase string) bool {
	validPhases := status.AllPhasesV2Schema()
	for _, p := range validPhases {
		if p == phase {
			return true
		}
	}
	return false
}

// IsRewindValid checks if rewinding from current to target is valid
func IsRewindValid(current, target string) bool {
	sequence := status.AllPhasesV2Schema()

	currentIdx := -1
	targetIdx := -1

	for i, phase := range sequence {
		if phase == current {
			currentIdx = i
		}
		if phase == target {
			targetIdx = i
		}
	}

	// Target must be before current in sequence
	return targetIdx >= 0 && currentIdx >= 0 && targetIdx < currentIdx
}
