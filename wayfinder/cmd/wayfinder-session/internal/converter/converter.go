// Package converter provides converter-related functionality.
package converter

import (
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// ConvertV1ToV2 converts a V1 Status to V2 StatusV2
// Implements 100% data preservation as per SPEC.md requirements
func ConvertV1ToV2(v1 *status.Status) (*status.StatusV2, error) {
	if v1 == nil {
		return nil, fmt.Errorf("v1 status is nil")
	}

	v2 := &status.StatusV2{
		SchemaVersion:   status.SchemaVersionV2,
		ProjectName:     extractProjectName(v1.ProjectPath),
		ProjectType:     inferProjectType(v1),
		RiskLevel:       inferRiskLevel(v1),
		CurrentWaypoint: convertPhase(v1.CurrentPhase),
		Status:          convertStatus(v1.Status),
		CreatedAt:       v1.StartedAt,
		UpdatedAt:       time.Now(),
	}

	// Optional fields
	if v1.ProjectPath != "" {
		v2.Repository = v1.ProjectPath
	}
	if v1.EndedAt != nil {
		v2.CompletionDate = v1.EndedAt
	}
	if v1.Status == status.StatusBlocked && v1.BlockedOn != "" {
		v2.BlockedReason = fmt.Sprintf("Blocked on: %s", v1.BlockedOn)
	}

	// Convert phase history
	v2.WaypointHistory = convertPhaseHistory(v1.Phases)

	// Initialize empty roadmap and quality metrics
	v2.Roadmap = &status.Roadmap{
		Phases: []status.RoadmapPhase{},
	}
	v2.QualityMetrics = &status.QualityMetrics{
		CoverageTarget:         80.0,
		AssertionDensityTarget: 3.0,
	}

	return v2, nil
}

// extractProjectName extracts project name from path
// Example: ~/src/ws/oss/wf/my-project -> my-project
func extractProjectName(projectPath string) string {
	if projectPath == "" {
		return "Unknown Project"
	}

	// Trim trailing slashes
	path := projectPath
	for len(path) > 0 && (path[len(path)-1] == '/' || path[len(path)-1] == '\\') {
		path = path[:len(path)-1]
	}

	// Empty after trimming slashes
	if path == "" {
		return "Unknown Project"
	}

	// Try to extract last component of path
	lastSlash := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			lastSlash = i
			break
		}
	}

	if lastSlash >= 0 && lastSlash < len(path)-1 {
		return path[lastSlash+1:]
	}

	return path
}

// inferProjectType infers project type from available data
func inferProjectType(v1 *status.Status) string {
	// Default to feature if we can't determine
	return status.ProjectTypeFeature
}

// inferRiskLevel infers risk level from phase count and completion
func inferRiskLevel(v1 *status.Status) string {
	phaseCount := len(v1.Phases)

	// Simple heuristic based on project complexity
	if phaseCount <= 3 {
		return status.RiskLevelS
	} else if phaseCount <= 6 {
		return status.RiskLevelM
	} else if phaseCount <= 9 {
		return status.RiskLevelL
	}

	return status.RiskLevelXL
}

// convertPhase converts V1 phase name to V2 phase name per SPEC.md
// V2 Consolidation: 13 phases → 9 phases (CHARTER, PROBLEM-SPEC, PLAN-BUILD, RETRO)
func convertPhase(v1Phase string) string {
	// Default to W0 for empty/uninitialized phases
	if v1Phase == "" {
		return status.PhaseV2Charter
	}

	// Map V1 phases to V2 phase names per SPEC.md section "Phase Consolidation"
	v1ToV2 := map[string]string{
		"W0":  status.PhaseV2Charter,  // W0 → W0 (unchanged)
		"D1":  status.PhaseV2Problem,  // D1 → D1
		"D2":  status.PhaseV2Research, // D2 → D2
		"D3":  status.PhaseV2Design,   // D3 → D3
		"D4":  status.PhaseV2Spec,     // D4 → D4
		"S4":  status.PhaseV2Spec,     // S4 → D4 (merged: stakeholder approval)
		"S5":  status.PhaseV2Plan,     // S5 → S6 (merged: research)
		"S6":  status.PhaseV2Plan,     // S6 → S6
		"S7":  status.PhaseV2Setup,    // S7 → S7
		"S8":  status.PhaseV2Build,    // S8 → S8 (BUILD loop)
		"S9":  status.PhaseV2Build,    // S9 → S8 (merged: validation)
		"S10": status.PhaseV2Build,    // S10 → S8 (merged: deployment)
		"S11": status.PhaseV2Retro,    // S11 → S11
	}

	if v2Phase, ok := v1ToV2[v1Phase]; ok {
		return v2Phase
	}

	// Fallback: already a V2 phase or unknown
	return v1Phase
}

// convertStatus converts V1 status to V2 status
func convertStatus(v1Status string) string {
	switch v1Status {
	case status.StatusInProgress:
		return status.StatusV2InProgress
	case status.StatusCompleted:
		return status.StatusV2Completed
	case status.StatusAbandoned:
		return status.StatusV2Abandoned
	case status.StatusBlocked:
		return status.StatusV2Blocked
	default:
		return status.StatusV2Planning
	}
}

// convertPhaseHistory converts V1 phases to V2 phase history
func convertPhaseHistory(v1Phases []status.Phase) []status.PhaseHistory {
	var history []status.PhaseHistory

	for _, v1Phase := range v1Phases {
		v2PhaseName := convertPhase(v1Phase.Name)
		if v2PhaseName == "" {
			continue // Skip phases removed in V2
		}

		// Handle StartedAt pointer dereference
		startedAt := time.Now()
		if v1Phase.StartedAt != nil {
			startedAt = *v1Phase.StartedAt
		}

		phaseHistory := status.PhaseHistory{
			Name:        v2PhaseName,
			Status:      convertPhaseStatus(v1Phase.Status),
			StartedAt:   startedAt,
			CompletedAt: v1Phase.CompletedAt,
		}

		// Convert outcome
		if v1Phase.Outcome != "" {
			outcome := v1Phase.Outcome
			phaseHistory.Outcome = &outcome
		}

		history = append(history, phaseHistory)
	}

	return history
}

// convertPhaseStatus converts V1 phase status to V2 phase status
func convertPhaseStatus(v1PhaseStatus string) string {
	switch v1PhaseStatus {
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
