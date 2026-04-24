package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TransitionValidation holds validation results for phase transitions
type TransitionValidation struct {
	Valid    bool
	Errors   []string
	Warnings []string
}

// validateTransition checks if transition from current to next phase is allowed
func (o *PhaseOrchestratorV2) validateTransition(current, next string) error {
	// Check for phase skipping (forward jumps)
	if isPhaseSkipping(current, next) {
		return o.buildPhaseSkippingError(current, next)
	}

	// Check if transition is in valid transitions map
	if !isValidSequence(current, next) {
		return fmt.Errorf("invalid transition from %s to %s", current, next)
	}

	// Check for transition-specific warnings
	warnings := o.checkTransitionWarnings(current, next)
	if len(warnings.Errors) > 0 {
		// Errors block the transition
		return fmt.Errorf("cannot proceed: %v", warnings.Errors)
	}

	// Log warnings but don't block
	if len(warnings.Warnings) > 0 {
		for _, w := range warnings.Warnings {
			fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
		}
	}

	return nil
}

// isValidSequence checks if current → next is a valid transition
func isValidSequence(current, next string) bool {
	// Define valid forward and backward transitions
	validTransitions := map[string][]string{
		status.PhaseV2Charter:  {status.PhaseV2Problem},                                        // Forward only
		status.PhaseV2Problem:  {status.PhaseV2Charter, status.PhaseV2Research},                // Forward (RESEARCH) or rewind (CHARTER)
		status.PhaseV2Research: {status.PhaseV2Problem, status.PhaseV2Design},                  // Forward (DESIGN) or rewind (PROBLEM)
		status.PhaseV2Design:   {status.PhaseV2Research, status.PhaseV2Spec},                   // Forward (SPEC) or rewind (RESEARCH)
		status.PhaseV2Spec:     {status.PhaseV2Design, status.PhaseV2Plan},                     // Forward (PLAN) or rewind (DESIGN)
		status.PhaseV2Plan:     {status.PhaseV2Spec, status.PhaseV2Setup},                      // Forward (SETUP) or rewind (SPEC)
		status.PhaseV2Setup:    {status.PhaseV2Plan, status.PhaseV2Build},                      // Forward (BUILD) or rewind (PLAN)
		status.PhaseV2Build:    {status.PhaseV2Setup, status.PhaseV2Plan, status.PhaseV2Retro}, // Forward (RETRO) or rewind (SETUP, PLAN)
		status.PhaseV2Retro:    {},                                                             // Terminal phase
	}

	allowedNext, exists := validTransitions[current]
	if !exists {
		return false
	}

	for _, allowed := range allowedNext {
		if allowed == next {
			return true
		}
	}

	return false
}

// isPhaseSkipping checks if transition skips intermediate phases
func isPhaseSkipping(current, next string) bool {
	sequence := status.AllPhasesV2Schema()

	currentIdx := -1
	nextIdx := -1

	for i, phase := range sequence {
		if phase == current {
			currentIdx = i
		}
		if phase == next {
			nextIdx = i
		}
	}

	if currentIdx == -1 || nextIdx == -1 {
		return false
	}

	// Forward: cannot skip (nextIdx must be currentIdx + 1)
	// Backward: rewinding allowed (nextIdx < currentIdx)
	if nextIdx > currentIdx+1 {
		return true // Skipping phases
	}

	return false
}

// buildPhaseSkippingError creates detailed error message for phase skipping
func (o *PhaseOrchestratorV2) buildPhaseSkippingError(current, next string) error {
	sequence := status.AllPhasesV2Schema()

	currentIdx := -1
	nextIdx := -1

	for i, phase := range sequence {
		if phase == current {
			currentIdx = i
		}
		if phase == next {
			nextIdx = i
		}
	}

	// Build list of skipped phases
	skippedPhases := []string{}
	for i := currentIdx + 1; i < nextIdx; i++ {
		skippedPhases = append(skippedPhases, sequence[i])
	}

	// Special error message for SPEC->BUILD (most common anti-pattern)
	if current == status.PhaseV2Spec && next == status.PhaseV2Build {
		return fmt.Errorf("cannot skip phases. Must complete PLAN, SETUP before BUILD. "+
			"This is the #1 anti-pattern. Design and planning are required for BUILD phase. "+
			"Skipped phases: %v", skippedPhases)
	}

	return fmt.Errorf("cannot skip phases. Must complete %v before %s. "+
		"Skipped phases: %v", skippedPhases, next, skippedPhases)
}

// checkTransitionWarnings checks for transition-specific warnings
func (o *PhaseOrchestratorV2) checkTransitionWarnings(current, next string) TransitionValidation {
	result := TransitionValidation{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// SPEC -> PLAN: Check stakeholder approval
	if current == status.PhaseV2Spec && next == status.PhaseV2Plan {
		if !o.hasStakeholderApproval() {
			result.Warnings = append(result.Warnings,
				"Stakeholder approval not documented. "+
					"For solo projects, add self-approval rationale in SPEC-requirements.md. "+
					"For team projects, obtain formal sign-off before proceeding.")
		}
	}

	// PLAN -> SETUP: Check TESTS.feature
	if current == status.PhaseV2Plan && next == status.PhaseV2Setup {
		if !o.hasTestsFeature() {
			result.Valid = false
			result.Errors = append(result.Errors,
				"TESTS.feature not created. "+
					"Test-first discipline required. "+
					"Cannot proceed without concrete test scenarios.")
		}
	}

	// SETUP -> BUILD: Check roadmap tasks
	if current == status.PhaseV2Setup && next == status.PhaseV2Build {
		if !o.hasRoadmapTasks() {
			result.Valid = false
			result.Errors = append(result.Errors,
				"No tasks defined in roadmap. "+
					"SETUP planning must populate WAYFINDER-STATUS.md roadmap with tasks. "+
					"Use 'wayfinder-session add-task' to create tasks from SETUP-plan.md.")
		}
	}

	// BUILD -> RETRO: Check deployment and quality
	if current == status.PhaseV2Build && next == status.PhaseV2Retro {
		if !o.hasDeploymentCompleted() {
			result.Valid = false
			result.Errors = append(result.Errors,
				"Deployment not completed. "+
					"BUILD phase includes deployment. "+
					"Deploy to target environment or mark deployment_status as 'not-applicable' with reason.")
		}

		if !o.hasValidationPassed() {
			result.Valid = false
			result.Errors = append(result.Errors,
				"Validation not passed. "+
					"All tests in TESTS.feature must pass before completion. "+
					"Run tests and fix failures before proceeding.")
		}

		if o.hasUnresolvedP0Issues() {
			result.Valid = false
			result.Errors = append(result.Errors,
				"Unresolved P0 issues. "+
					"Multi-persona review identified critical issues. "+
					"Fix all P0 issues before completion.")
		}

		if o.hasUnresolvedP1Issues() {
			result.Warnings = append(result.Warnings,
				"P1 issues should be resolved before completion")
		}

		if o.hasLowAssertionDensity() {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Low assertion density detected (%.2f assertions per 10 LOC). "+
					"Tests may be gaming the system with tautologies (assert True, assert 1==1). "+
					"Review tests for meaningful assertions before deployment. "+
					"Target: %.2f assertions per 10 LOC.",
					o.getAssertionDensity(),
					o.getAssertionDensityTarget()))
		}
	}

	return result
}

// hasStakeholderApproval checks if SPEC has stakeholder approval
func (o *PhaseOrchestratorV2) hasStakeholderApproval() bool {
	for i := len(o.status.WaypointHistory) - 1; i >= 0; i-- {
		if o.status.WaypointHistory[i].Name == status.PhaseV2Spec {
			if o.status.WaypointHistory[i].StakeholderApproved != nil {
				return *o.status.WaypointHistory[i].StakeholderApproved
			}
		}
	}
	return false
}

// hasTestsFeature checks if TESTS.feature file exists
func (o *PhaseOrchestratorV2) hasTestsFeature() bool {
	// Check if TESTS.feature was created in PLAN
	for i := len(o.status.WaypointHistory) - 1; i >= 0; i-- {
		if o.status.WaypointHistory[i].Name == status.PhaseV2Plan {
			if o.status.WaypointHistory[i].TestsFeatureCreated != nil {
				return *o.status.WaypointHistory[i].TestsFeatureCreated
			}
		}
	}

	// Fallback: check filesystem (would need project directory context)
	// For now, return false if not marked in history
	return false
}

// hasRoadmapTasks checks if roadmap has tasks defined
func (o *PhaseOrchestratorV2) hasRoadmapTasks() bool {
	if o.status.Roadmap == nil {
		return false
	}

	// Check for BUILD phase in roadmap
	for _, phase := range o.status.Roadmap.Phases {
		if phase.ID == status.PhaseV2Build {
			return len(phase.Tasks) > 0
		}
	}

	return false
}

// hasDeploymentCompleted checks if BUILD deployment is complete
func (o *PhaseOrchestratorV2) hasDeploymentCompleted() bool {
	for i := len(o.status.WaypointHistory) - 1; i >= 0; i-- {
		if o.status.WaypointHistory[i].Name == status.PhaseV2Build {
			deployStatus := o.status.WaypointHistory[i].DeploymentStatus
			return deployStatus == status.DeploymentStatusDeployed || deployStatus == "not-applicable"
		}
	}
	return false
}

// hasValidationPassed checks if BUILD validation passed
func (o *PhaseOrchestratorV2) hasValidationPassed() bool {
	for i := len(o.status.WaypointHistory) - 1; i >= 0; i-- {
		if o.status.WaypointHistory[i].Name == status.PhaseV2Build {
			return o.status.WaypointHistory[i].ValidationStatus == status.ValidationStatusPassed
		}
	}
	return false
}

// hasUnresolvedP0Issues checks for unresolved P0 issues
func (o *PhaseOrchestratorV2) hasUnresolvedP0Issues() bool {
	if o.status.QualityMetrics == nil {
		return false
	}
	return o.status.QualityMetrics.P0Issues > 0
}

// hasUnresolvedP1Issues checks for unresolved P1 issues
func (o *PhaseOrchestratorV2) hasUnresolvedP1Issues() bool {
	if o.status.QualityMetrics == nil {
		return false
	}
	return o.status.QualityMetrics.P1Issues > 0
}

// hasLowAssertionDensity checks if assertion density is below target
func (o *PhaseOrchestratorV2) hasLowAssertionDensity() bool {
	if o.status.QualityMetrics == nil {
		return false
	}

	density := o.status.QualityMetrics.AssertionDensity
	target := o.status.QualityMetrics.AssertionDensityTarget

	if target == 0 {
		target = 0.5 // Default target
	}

	return density < target
}

// getAssertionDensity returns current assertion density
func (o *PhaseOrchestratorV2) getAssertionDensity() float64 {
	if o.status.QualityMetrics == nil {
		return 0
	}
	return o.status.QualityMetrics.AssertionDensity
}

// getAssertionDensityTarget returns target assertion density
func (o *PhaseOrchestratorV2) getAssertionDensityTarget() float64 {
	if o.status.QualityMetrics == nil {
		return 0.5 // Default target
	}
	target := o.status.QualityMetrics.AssertionDensityTarget
	if target == 0 {
		return 0.5 // Default target
	}
	return target
}

// validateExitCriteria checks exit criteria for given phase
func (o *PhaseOrchestratorV2) validateExitCriteria(phase string) error {
	switch phase {
	case status.PhaseV2Charter:
		return o.validateW0Exit()
	case status.PhaseV2Problem:
		return o.validateD1Exit()
	case status.PhaseV2Research:
		return o.validateD2Exit()
	case status.PhaseV2Design:
		return o.validateD3Exit()
	case status.PhaseV2Spec:
		return o.validateD4Exit()
	case status.PhaseV2Plan:
		return o.validateS6Exit()
	case status.PhaseV2Setup:
		return o.validateS7Exit()
	case status.PhaseV2Build:
		return o.validateS8Exit()
	case status.PhaseV2Retro:
		return o.validateS11Exit()
	default:
		return fmt.Errorf("unknown phase: %s", phase)
	}
}

// validateW0Exit validates W0 (Intake) exit criteria
func (o *PhaseOrchestratorV2) validateW0Exit() error {
	// Check required fields
	if o.status.ProjectName == "" {
		return fmt.Errorf("project_name not set")
	}
	if o.status.ProjectType == "" {
		return fmt.Errorf("project_type not assigned")
	}
	if o.status.RiskLevel == "" {
		return fmt.Errorf("risk_level not assigned")
	}

	// Check W0-intake.md exists (would need filesystem access)
	// For now, just validate status file fields
	return nil
}

// validateD1Exit validates D1 (Discovery) exit criteria
func (o *PhaseOrchestratorV2) validateD1Exit() error {
	// Check D1-discovery.md exists in deliverables
	if !o.hasDeliverable(status.PhaseV2Problem, "D1-discovery.md") {
		return fmt.Errorf("D1-discovery.md not in deliverables")
	}
	return nil
}

// validateD2Exit validates D2 (Investigation) exit criteria
func (o *PhaseOrchestratorV2) validateD2Exit() error {
	// Check D2-investigation.md exists
	if !o.hasDeliverable(status.PhaseV2Research, "D2-investigation.md") {
		return fmt.Errorf("D2-investigation.md not in deliverables")
	}
	return nil
}

// validateD3Exit validates D3 (Architecture) exit criteria
func (o *PhaseOrchestratorV2) validateD3Exit() error {
	// Check D3-architecture.md exists
	if !o.hasDeliverable(status.PhaseV2Design, "D3-architecture.md") {
		return fmt.Errorf("D3-architecture.md not in deliverables")
	}
	return nil
}

// validateD4Exit validates D4 (Requirements) exit criteria
func (o *PhaseOrchestratorV2) validateD4Exit() error {
	// Check D4-requirements.md exists
	if !o.hasDeliverable(status.PhaseV2Spec, "D4-requirements.md") {
		return fmt.Errorf("D4-requirements.md not in deliverables")
	}

	// Check TESTS.outline exists
	if !o.hasDeliverable(status.PhaseV2Spec, "TESTS.outline") {
		return fmt.Errorf("TESTS.outline not created")
	}

	return nil
}

// validateS6Exit validates S6 (Design) exit criteria
func (o *PhaseOrchestratorV2) validateS6Exit() error {
	// Check S6-design.md exists
	if !o.hasDeliverable(status.PhaseV2Plan, "S6-design.md") {
		return fmt.Errorf("S6-design.md not in deliverables")
	}

	// Check TESTS.feature created (critical for test-first discipline)
	if !o.hasTestsFeature() {
		return fmt.Errorf("TESTS.feature not created (required for S7 planning)")
	}

	return nil
}

// validateS7Exit validates S7 (Planning) exit criteria
func (o *PhaseOrchestratorV2) validateS7Exit() error {
	// Check S7-plan.md exists
	if !o.hasDeliverable(status.PhaseV2Setup, "S7-plan.md") {
		return fmt.Errorf("S7-plan.md not in deliverables")
	}

	// Check roadmap has tasks
	if !o.hasRoadmapTasks() {
		return fmt.Errorf("no tasks defined in roadmap")
	}

	return nil
}

// validateS8Exit validates S8 (BUILD) exit criteria
func (o *PhaseOrchestratorV2) validateS8Exit() error {
	// Check S8-build.md exists
	if !o.hasDeliverable(status.PhaseV2Build, "S8-build.md") {
		return fmt.Errorf("S8-build.md not in deliverables")
	}

	// Check all tasks completed
	if !o.allTasksCompleted() {
		return fmt.Errorf("not all tasks completed")
	}

	// Check validation passed
	if !o.hasValidationPassed() {
		return fmt.Errorf("validation not passed")
	}

	// Check deployment completed
	if !o.hasDeploymentCompleted() {
		return fmt.Errorf("deployment not completed")
	}

	// Check P0 issues resolved
	if o.hasUnresolvedP0Issues() {
		return fmt.Errorf("unresolved P0 issues")
	}

	return nil
}

// validateS11Exit validates S11 (Closure) exit criteria
func (o *PhaseOrchestratorV2) validateS11Exit() error {
	// Check S11-retrospective.md exists
	if !o.hasDeliverable(status.PhaseV2Retro, "S11-retrospective.md") {
		return fmt.Errorf("S11-retrospective.md not in deliverables")
	}

	// Check status is completed
	if o.status.Status != status.StatusV2Completed {
		return fmt.Errorf("project status not marked as completed")
	}

	// Check completion date set
	if o.status.CompletionDate == nil {
		return fmt.Errorf("completion_date not set")
	}

	return nil
}

// hasDeliverable checks if a deliverable exists for given phase
func (o *PhaseOrchestratorV2) hasDeliverable(phase, deliverable string) bool {
	for i := len(o.status.WaypointHistory) - 1; i >= 0; i-- {
		if o.status.WaypointHistory[i].Name == phase {
			for _, d := range o.status.WaypointHistory[i].Deliverables {
				if d == deliverable {
					return true
				}
			}
		}
	}
	return false
}

// allTasksCompleted checks if all S8 tasks are completed
func (o *PhaseOrchestratorV2) allTasksCompleted() bool {
	if o.status.Roadmap == nil {
		return false
	}

	// Find S8 phase in roadmap
	for _, phase := range o.status.Roadmap.Phases {
		if phase.ID == status.PhaseV2Build {
			// Check all tasks are completed
			for _, task := range phase.Tasks {
				if task.Status != status.TaskStatusCompleted {
					return false
				}
			}
			return true
		}
	}

	return false
}

// fileExists checks if a file exists (helper for filesystem checks)
func fileExists(projectDir, filename string) bool {
	path := filepath.Join(projectDir, filename)
	_, err := os.Stat(path)
	return err == nil
}
