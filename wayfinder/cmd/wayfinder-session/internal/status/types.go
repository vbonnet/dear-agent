package status

import (
	"fmt"
	"regexp"
	"time"
)

// Phase is the validator-facing view of a named phase history entry.
type Phase struct {
	Name        string
	Status      string
	StartedAt   *time.Time
	CompletedAt *time.Time
	Outcome     string
}

// SchemaVersion and StatusFilename define the canonical status-file contract.
const (
	SchemaVersion  = "2.0"
	StatusFilename = "WAYFINDER-STATUS.md"

	// WayfinderV2 is retained as a display label. SchemaVersion is the runtime
	// compatibility boundary; there is no alternate workflow model.
	WayfinderV2 = "v2"

	OutcomeSuccess = "success"
	OutcomePartial = "partial"
	OutcomeSkipped = "skipped"

	LifecycleWorking           = "working"
	LifecycleInputRequired     = "input-required"
	LifecycleDependencyBlocked = "dependency-blocked"
	LifecycleValidating        = "validating"
	LifecycleCompleted         = "completed"
	LifecycleFailed            = "failed"
	LifecycleCanceled          = "canceled"
)

// Phase status aliases keep command and validator vocabulary concise while the
// serialized status uses the same values in WaypointHistory.
const (
	PhaseStatusPending    = WaypointStatusV2Pending
	PhaseStatusInProgress = WaypointStatusV2InProgress
	PhaseStatusCompleted  = WaypointStatusV2Completed
	PhaseStatusSkipped    = WaypointStatusV2Skipped
)

// AllPhases returns the only supported phase sequence.
func AllPhases() []string {
	return AllWaypointsV2Schema()
}

// AllPhasesV2 is kept as a descriptive read-only alias for callers that display
// the schema label. It returns the same single sequence.
func AllPhasesV2() []string {
	return AllPhases()
}

var phasePattern = regexp.MustCompile(`^(CHARTER|PROBLEM|RESEARCH|DESIGN|SPEC|PLAN|SETUP|BUILD|RETRO)$`)

// IsValidV2Phase reports whether phase belongs to the canonical named sequence.
func IsValidV2Phase(phase string) bool {
	return phasePattern.MatchString(phase)
}

var (
	// ErrMaxDepthExceeded is returned when nesting depth exceeds MaxNestingDepth.
	ErrMaxDepthExceeded = fmt.Errorf("maximum nesting depth exceeded (%d levels)", 10)
)
