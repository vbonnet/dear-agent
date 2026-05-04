package migrate

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// ConvertOptions provides options for V1 to V2 conversion
type ConvertOptions struct {
	// DryRun performs validation without modifying files
	DryRun bool

	// ProjectName overrides the auto-detected project name
	ProjectName string

	// ProjectType specifies the project type (feature, bugfix, etc.)
	ProjectType string

	// RiskLevel specifies the risk level (XS, S, M, L, XL)
	RiskLevel string

	// PreserveSessionID keeps the V1 session ID as a tag
	PreserveSessionID bool
}

// V1ToV2PhaseMap maps V1 phase names to V2 phase names
// Note: Some V1 phases merge into V2 phases
var V1ToV2PhaseMap = map[string]string{
	"W0":  status.PhaseV2Charter,  // W0 → W0
	"D1":  status.PhaseV2Problem,  // D1 → D1
	"D2":  status.PhaseV2Research, // D2 → D2
	"D3":  status.PhaseV2Design,   // D3 → D3
	"D4":  status.PhaseV2Spec,     // D4 → D4
	"S4":  status.PhaseV2Spec,     // S4 → D4 (merged - stakeholder alignment)
	"S5":  status.PhaseV2Plan,     // S5 → S6 (merged - research)
	"S6":  status.PhaseV2Plan,     // S6 → S6
	"S7":  status.PhaseV2Setup,    // S7 → S7
	"S8":  status.PhaseV2Build,    // S8 → S8 (BUILD phase)
	"S9":  status.PhaseV2Build,    // S9 → S8 (merged - validation)
	"S10": status.PhaseV2Build,    // S10 → S8 (merged - deployment)
	"S11": status.PhaseV2Retro,    // S11 → S11
}

// ConvertV1ToV2 converts a V1 Status to V2 StatusV2
func ConvertV1ToV2(v1 *status.Status, opts *ConvertOptions) (*status.StatusV2, error) {
	if v1 == nil {
		return nil, fmt.Errorf("cannot convert nil status")
	}

	// Validate V1 schema
	if err := validateV1Schema(v1); err != nil {
		return nil, err
	}

	// Apply default options
	if opts == nil {
		opts = &ConvertOptions{}
	}

	// Create V2 status
	v2 := &status.StatusV2{
		SchemaVersion:   status.SchemaVersionV2,
		ProjectName:     deriveProjectName(v1, opts),
		ProjectType:     deriveProjectType(v1, opts),
		RiskLevel:       deriveRiskLevel(v1, opts),
		CurrentWaypoint: mapV1PhaseToV2(v1.CurrentPhase),
		Status:          mapV1StatusToV2(v1.Status),
		CreatedAt:       v1.StartedAt,
		UpdatedAt:       time.Now(),
	}

	// Convert optional fields
	if v1.EndedAt != nil {
		v2.CompletionDate = v1.EndedAt
	}

	// Add session ID as tag if requested
	if opts.PreserveSessionID && v1.SessionID != "" {
		v2.Tags = append(v2.Tags, fmt.Sprintf("v1-session:%s", v1.SessionID))
	}

	// Convert phase history
	phaseHistory, err := convertPhaseHistory(v1.Phases)
	if err != nil {
		return nil, fmt.Errorf("failed to convert phase history: %w", err)
	}
	v2.WaypointHistory = phaseHistory

	// Initialize roadmap structure
	v2.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{},
	}

	return v2, nil
}

// validateV1Schema validates the V1 status structure
func validateV1Schema(v1 *status.Status) error {
	if v1.SchemaVersion == "" {
		return fmt.Errorf("invalid schema version: empty")
	}

	if v1.SchemaVersion != "1.0" {
		return fmt.Errorf("expected version 1.0, got %s", v1.SchemaVersion)
	}

	if v1.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}

	if v1.ProjectPath == "" {
		return fmt.Errorf("project_path is required")
	}

	if v1.StartedAt.IsZero() {
		return fmt.Errorf("started_at is required")
	}

	return nil
}

// deriveProjectName extracts project name from path or uses override
func deriveProjectName(v1 *status.Status, opts *ConvertOptions) string {
	if opts.ProjectName != "" {
		return opts.ProjectName
	}

	// Extract from project path
	if v1.ProjectPath != "" {
		return filepath.Base(v1.ProjectPath)
	}

	return "Unnamed Project"
}

// deriveProjectType determines project type from V1 data or uses override
func deriveProjectType(_ *status.Status, opts *ConvertOptions) string {
	if opts.ProjectType != "" {
		return opts.ProjectType
	}

	// Default to feature if not specified
	return status.ProjectTypeFeature
}

// deriveRiskLevel determines risk level from V1 data or uses override
func deriveRiskLevel(_ *status.Status, opts *ConvertOptions) string {
	if opts.RiskLevel != "" {
		return opts.RiskLevel
	}

	// Try to infer from phase count and complexity
	// For now, default to M (Medium)
	return status.RiskLevelM
}

// mapV1PhaseToV2 maps a V1 phase name to V2
func mapV1PhaseToV2(v1Phase string) string {
	if v2Phase, ok := V1ToV2PhaseMap[v1Phase]; ok {
		return v2Phase
	}
	// If unknown, return as-is (shouldn't happen with valid V1)
	return v1Phase
}

// mapV1StatusToV2 maps V1 status values to V2
func mapV1StatusToV2(v1Status string) string {
	switch v1Status {
	case status.StatusInProgress:
		return status.StatusV2InProgress
	case status.StatusCompleted:
		return status.StatusV2Completed
	case status.StatusAbandoned:
		return status.StatusV2Abandoned
	case status.StatusBlocked:
		return status.StatusV2Blocked
	case status.StatusObsolete:
		return status.StatusV2Abandoned // Map obsolete to abandoned
	default:
		return status.StatusV2Planning
	}
}

// convertPhaseHistory converts V1 phases to V2 phase history with merging
func convertPhaseHistory(v1Phases []status.Phase) ([]status.PhaseHistory, error) {
	// Group phases by V2 phase name for merging
	phaseGroups := make(map[string][]status.Phase)

	for _, v1Phase := range v1Phases {
		v2PhaseName := mapV1PhaseToV2(v1Phase.Name)
		phaseGroups[v2PhaseName] = append(phaseGroups[v2PhaseName], v1Phase)
	}

	// Convert grouped phases to V2 phase history
	var history []status.PhaseHistory

	// Process in V2 phase order
	for _, v2PhaseName := range status.AllPhasesV2Schema() {
		phases, exists := phaseGroups[v2PhaseName]
		if !exists {
			continue
		}

		// Merge phases into a single V2 phase
		merged, err := mergePhases(v2PhaseName, phases)
		if err != nil {
			return nil, fmt.Errorf("failed to merge phases for %s: %w", v2PhaseName, err)
		}

		history = append(history, merged)
	}

	return history, nil
}

// mergePhases merges multiple V1 phases into a single V2 phase
func mergePhases(v2PhaseName string, v1Phases []status.Phase) (status.PhaseHistory, error) {
	if len(v1Phases) == 0 {
		return status.PhaseHistory{}, fmt.Errorf("no phases to merge")
	}

	// Start with merged phase
	// Use status from the last (most recent) phase in the merge
	lastPhase := v1Phases[len(v1Phases)-1]
	merged := status.PhaseHistory{
		Name:   v2PhaseName,
		Status: mapV1PhaseStatusToV2(lastPhase.Status),
	}

	// Find earliest start time and latest completion time
	var earliestStart *time.Time
	var latestCompletion *time.Time
	var deliverables []string
	var notes []string

	for _, phase := range v1Phases {
		// Track start time
		if phase.StartedAt != nil {
			if earliestStart == nil || phase.StartedAt.Before(*earliestStart) {
				earliestStart = phase.StartedAt
			}
		}

		// Track completion time
		if phase.CompletedAt != nil {
			if latestCompletion == nil || phase.CompletedAt.After(*latestCompletion) {
				latestCompletion = phase.CompletedAt
			}
		}

		// Collect notes
		notes = append(notes, fmt.Sprintf("V1 %s: %s", phase.Name, phase.Status))

		// Handle phase-specific merging
		mergePhaseSpecificData(&merged, phase)
	}

	// Set merged timestamps
	if earliestStart != nil {
		merged.StartedAt = *earliestStart
	}
	if latestCompletion != nil {
		merged.CompletedAt = latestCompletion
	}

	// Merge deliverables and notes
	merged.Deliverables = deliverables
	if len(notes) > 0 {
		merged.Notes = strings.Join(notes, "; ")
	}

	// Set outcome based on final status
	if merged.Status == status.PhaseStatusV2Completed {
		outcome := "success"
		merged.Outcome = &outcome
	}

	return merged, nil
}

// mergePhaseSpecificData handles phase-specific field mapping
func mergePhaseSpecificData(v2Phase *status.PhaseHistory, v1Phase status.Phase) {
	switch v1Phase.Name {
	case "S4":
		// S4 → D4: Add stakeholder approval marker
		approved := v1Phase.Status == status.PhaseStatusCompleted
		v2Phase.StakeholderApproved = &approved
		v2Phase.StakeholderNotes = "Migrated from V1 S4 (Stakeholder Alignment)"

	case "S5":
		// S5 → S6: Add research notes marker
		v2Phase.ResearchNotes = "Migrated from V1 S5 (Research phase)"

	case "S8":
		// S8 → S8: Mark as BUILD phase
		// No special fields needed, but could add notes
		if v2Phase.Notes == "" {
			v2Phase.Notes = "BUILD phase (implementation)"
		}

	case "S9":
		// S9 → S8: Add validation status
		switch v1Phase.Status {
		case status.PhaseStatusCompleted:
			v2Phase.ValidationStatus = status.ValidationStatusPassed
		case status.PhaseStatusInProgress:
			v2Phase.ValidationStatus = status.ValidationStatusInProgress
		default:
			v2Phase.ValidationStatus = status.ValidationStatusPending
		}

	case "S10":
		// S10 → S8: Add deployment status
		switch v1Phase.Status {
		case status.PhaseStatusCompleted:
			v2Phase.DeploymentStatus = status.DeploymentStatusDeployed
		case status.PhaseStatusInProgress:
			v2Phase.DeploymentStatus = status.DeploymentStatusInProgress
		default:
			v2Phase.DeploymentStatus = status.DeploymentStatusPending
		}
	}
}

// mapV1PhaseStatusToV2 maps V1 phase status to V2
func mapV1PhaseStatusToV2(v1Status string) string {
	switch v1Status {
	case status.PhaseStatusPending:
		return status.PhaseStatusV2Pending
	case status.PhaseStatusInProgress:
		return status.PhaseStatusV2InProgress
	case status.PhaseStatusCompleted:
		return status.PhaseStatusV2Completed
	case status.PhaseStatusSkipped:
		return status.PhaseStatusV2Skipped
	default:
		return status.PhaseStatusV2Pending
	}
}
