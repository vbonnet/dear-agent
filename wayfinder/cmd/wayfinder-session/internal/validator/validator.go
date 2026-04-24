package validator

import (
	"strings"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// Validator provides pre-transition validation logic
type Validator struct {
	status status.StatusInterface
}

// NewValidator creates a new Validator with the current status
func NewValidator(s status.StatusInterface) *Validator {
	return &Validator{
		status: s,
	}
}

// CanStartPhase validates if a phase can be started
// Returns ValidationError if:
// - Phase is already in_progress or completed
// - Previous phase is not completed
// - Phase doesn't exist in AllPhases()
// - DESIGN phase attempted without valid RESEARCH content
func (v *Validator) CanStartPhase(phaseName, projectDir string) error {
	allPhases := status.AllPhases(v.status.GetVersion())

	// Validate phase exists
	phaseIdx := -1
	for i, phase := range allPhases {
		if phase == phaseName {
			phaseIdx = i
			break
		}
	}
	if phaseIdx == -1 {
		return NewValidationError(
			"start "+phaseName,
			"phase does not exist in Wayfinder workflow",
			"Use a valid phase: "+joinPhases(allPhases),
		)
	}

	// Check if phase is already started or completed
	existingPhase := v.status.FindPhase(phaseName)
	if existingPhase != nil {
		if existingPhase.Status == status.PhaseStatusInProgress {
			return NewValidationError(
				"start "+phaseName,
				"phase is already in progress",
				"Complete the phase first with 'complete-phase "+phaseName+"'",
			)
		}
		if existingPhase.Status == status.PhaseStatusCompleted {
			return NewValidationError(
				"start "+phaseName,
				"phase is already completed",
				"Use 'rewind-to "+phaseName+"' if you want to restart this phase",
			)
		}
	}

	// For first phase (CHARTER), no previous phase to check
	if phaseIdx == 0 {
		return nil
	}

	// Validate previous phase is completed
	prevPhaseName := allPhases[phaseIdx-1]

	// Skip roadmap phase (SETUP) if skip_roadmap is enabled
	if v.status.GetVersion() == status.WayfinderV2 && v.status.GetSkipRoadmap() && prevPhaseName == status.PhaseV2Setup {
		// If trying to start BUILD and skip_roadmap is true, check PLAN instead of SETUP
		if phaseIdx >= 2 {
			prevPhaseName = allPhases[phaseIdx-2]
		}
	}

	prevPhase := v.status.FindPhase(prevPhaseName)
	if prevPhase == nil || prevPhase.Status != status.PhaseStatusCompleted {
		return NewValidationError(
			"start "+phaseName,
			"previous phase "+prevPhaseName+" is not completed",
			"Complete "+prevPhaseName+" first with 'complete-phase "+prevPhaseName+"'",
		)
	}

	// Special case: D3 requires validated D2 content
	if phaseName == "DESIGN" {
		if err := validateD2Content(projectDir, v.status); err != nil {
			return err
		}
	}

	return nil
}

// CanCompletePhase validates if a phase can be completed
// Returns ValidationError if:
// - Phase is not currently in_progress
// - Phase was never started
// - Deliverable file is missing or invalid (size, frontmatter, hash)
// - BUILD implementation is missing (for BUILD phase)
func (v *Validator) CanCompletePhase(phaseName, projectDir, hashMismatchReason string) error {
	// Basic validations
	if err := v.validatePhaseExists(phaseName); err != nil {
		return err
	}
	if err := v.validatePhaseStarted(phaseName); err != nil {
		return err
	}
	if err := v.validatePhaseInProgress(phaseName); err != nil {
		return err
	}

	// Deliverable validations
	if err := v.runDeliverableValidations(projectDir, phaseName); err != nil {
		return err
	}

	// Content validations
	if err := v.runContentValidations(projectDir, phaseName, hashMismatchReason); err != nil {
		return err
	}

	// Gate validations
	if err := v.runGateValidations(projectDir, phaseName); err != nil {
		return err
	}

	return nil
}

// validatePhaseExists checks if phase exists in Wayfinder workflow.
func (v *Validator) validatePhaseExists(phaseName string) error {
	allPhases := status.AllPhases(v.status.GetVersion())
	for _, phase := range allPhases {
		if phase == phaseName {
			return nil
		}
	}
	return NewValidationError(
		"complete "+phaseName,
		"phase does not exist in Wayfinder workflow",
		"Use a valid phase: "+joinPhases(allPhases),
	)
}

// validatePhaseStarted checks if phase was started.
func (v *Validator) validatePhaseStarted(phaseName string) error {
	existingPhase := v.status.FindPhase(phaseName)
	if existingPhase == nil {
		return NewValidationError(
			"complete "+phaseName,
			"phase was never started",
			"Start the phase first with 'start-phase "+phaseName+"'",
		)
	}
	return nil
}

// validatePhaseInProgress checks if phase is in_progress status.
func (v *Validator) validatePhaseInProgress(phaseName string) error {
	existingPhase := v.status.FindPhase(phaseName)
	if existingPhase.Status != status.PhaseStatusInProgress {
		if existingPhase.Status == status.PhaseStatusCompleted {
			return NewValidationError(
				"complete "+phaseName,
				"phase is already completed",
				"Use 'rewind-to "+phaseName+"' if you want to restart this phase",
			)
		}
		return NewValidationError(
			"complete "+phaseName,
			"phase is not in progress (current status: "+existingPhase.Status+")",
			"Start the phase first with 'start-phase "+phaseName+"'",
		)
	}
	return nil
}

// runDeliverableValidations runs deliverable file validations.
func (v *Validator) runDeliverableValidations(projectDir, phaseName string) error {
	if err := validateDeliverableExists(projectDir, phaseName); err != nil {
		return err
	}
	if err := validateDeliverableSize(projectDir, phaseName); err != nil {
		return err
	}
	if phaseName == "BUILD" {
		if err := validateBuildImplementation(projectDir); err != nil {
			return err
		}
	}
	return nil
}

// runContentValidations runs content and methodology validations.
func (v *Validator) runContentValidations(projectDir, phaseName, hashMismatchReason string) error {
	if err := validateMethodologyFreshness(projectDir, phaseName, hashMismatchReason); err != nil {
		return err
	}
	if err := validatePhaseQuestions(projectDir, phaseName); err != nil {
		return err
	}
	if err := v.validatePhaseBoundaries(phaseName, projectDir); err != nil {
		return err
	}
	return nil
}

// runGateValidations runs completion gate validations.
func (v *Validator) runGateValidations(projectDir, phaseName string) error {
	if err := validateGitCommitStatus(projectDir, phaseName); err != nil {
		return err
	}
	if err := validateCompilation(projectDir, phaseName); err != nil {
		return err
	}
	if err := CheckChildrenComplete(projectDir); err != nil {
		return NewValidationError(
			"complete "+phaseName,
			"child projects must be completed first",
			err.Error()+"\n\nComplete all child projects before completing parent phase.",
		)
	}
	if err := ValidateGate(projectDir, phaseName, v.status); err != nil {
		return err
	}
	// Documentation quality gate for SPEC and PLAN phases
	if err := validateDocQuality(phaseName, projectDir); err != nil {
		return err
	}
	// Gate 9: Code verification gate for all phases with code deliverables
	if err := validateCodeDeliverables(phaseName, projectDir); err != nil {
		return err
	}
	return nil
}

// CanRewindTo validates if can rewind to target phase
// Returns ValidationError if:
// - Target phase is after current phase (not a rewind)
// - Target phase doesn't exist
// - Target phase is not completed (can't rewind to pending)
func (v *Validator) CanRewindTo(targetPhase string) error {
	allPhases := status.AllPhases(v.status.GetVersion())

	// Validate target phase exists
	targetIdx := -1
	for i, phase := range allPhases {
		if phase == targetPhase {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return NewValidationError(
			"rewind to "+targetPhase,
			"phase does not exist in Wayfinder workflow",
			"Use a valid phase: "+joinPhases(allPhases),
		)
	}

	// Get current phase index
	currentIdx := -1
	currentPhase := v.status.GetCurrentPhase()
	if currentPhase != "" {
		for i, phase := range allPhases {
			if phase == currentPhase {
				currentIdx = i
				break
			}
		}
	}

	// Validate target is before current (is actually a rewind)
	if currentIdx != -1 && targetIdx >= currentIdx {
		return NewValidationError(
			"rewind to "+targetPhase,
			"target phase is not before current phase (not a rewind)",
			"Current phase is "+currentPhase+". Use 'start-phase' to move forward",
		)
	}

	// Validate target phase is completed
	targetPhaseData := v.status.FindPhase(targetPhase)
	if targetPhaseData == nil {
		return NewValidationError(
			"rewind to "+targetPhase,
			"target phase was never started",
			"Cannot rewind to a phase that was never completed",
		)
	}
	if targetPhaseData.Status != status.PhaseStatusCompleted {
		return NewValidationError(
			"rewind to "+targetPhase,
			"target phase is not completed (current status: "+targetPhaseData.Status+")",
			"Can only rewind to completed phases",
		)
	}

	return nil
}

// joinPhases joins phase names with commas for error messages
func joinPhases(phases []string) string {
	return strings.Join(phases, ", ")
}
