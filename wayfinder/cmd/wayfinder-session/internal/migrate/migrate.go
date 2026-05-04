package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// Migrator handles V1 to V2 migration for Wayfinder projects
type Migrator struct {
	projectDir   string
	fileMigrator *FileMigrator
	dryRun       bool
}

// NewMigrator creates a new migrator for a project directory
func NewMigrator(projectDir string) *Migrator {
	return &Migrator{
		projectDir:   projectDir,
		fileMigrator: NewFileMigrator(projectDir),
		dryRun:       false,
	}
}

// SetDryRun enables or disables dry-run mode
func (m *Migrator) SetDryRun(dryRun bool) {
	m.dryRun = dryRun
}

// Migrate performs complete V1 to V2 migration
func (m *Migrator) Migrate() (*status.StatusV2, error) {
	// 1. Detect current schema version
	statusPath := filepath.Join(m.projectDir, status.StatusFilename)
	currentVersion, err := status.DetectSchemaVersion(statusPath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect schema version: %w", err)
	}

	// If already V2, no migration needed
	if currentVersion == status.SchemaVersionV2 {
		return nil, fmt.Errorf("project is already V2, no migration needed")
	}

	// 2. Parse V1 status
	v1Status, err := status.ReadFrom(m.projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to parse V1 status: %w", err)
	}

	// 3. Convert V1 to V2 schema
	v2Status, err := m.convertV1ToV2(v1Status)
	if err != nil {
		return nil, fmt.Errorf("failed to convert V1 to V2: %w", err)
	}

	// 4. Migrate phase files
	if err := m.fileMigrator.MigrateFiles(v2Status); err != nil {
		return nil, fmt.Errorf("failed to migrate files: %w", err)
	}

	// 5. Write V2 status file (unless dry-run)
	if !m.dryRun {
		backupPath := filepath.Join(m.projectDir, ".wayfinder-v1-backup", status.StatusFilename)
		backupDir := filepath.Dir(backupPath)
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create backup directory: %w", err)
		}

		// Backup V1 status
		if err := os.Rename(statusPath, backupPath); err != nil {
			return nil, fmt.Errorf("failed to backup V1 status: %w", err)
		}

		// Write V2 status
		if err := status.WriteV2(v2Status, statusPath); err != nil {
			// Restore backup on failure
			_ = os.Rename(backupPath, statusPath)
			return nil, fmt.Errorf("failed to write V2 status: %w", err)
		}

		// Cleanup old files
		if err := m.fileMigrator.Cleanup(); err != nil {
			return nil, fmt.Errorf("cleanup failed (non-fatal): %w", err)
		}
	}

	return v2Status, nil
}

// convertV1ToV2 converts V1 Status to V2 StatusV2
func (m *Migrator) convertV1ToV2(v1 *status.Status) (*status.StatusV2, error) {
	now := time.Now()

	// Create V2 status with basic fields
	v2 := &status.StatusV2{
		SchemaVersion:   status.SchemaVersionV2,
		ProjectName:     m.extractProjectName(v1.ProjectPath),
		ProjectType:     m.inferProjectType(v1),
		RiskLevel:       m.calculateRiskLevel(v1),
		CurrentWaypoint: m.mapV1PhaseToV2(v1.CurrentPhase),
		Status:          m.mapV1StatusToV2(v1.Status),
		CreatedAt:       v1.StartedAt,
		UpdatedAt:       now,
		Description:     fmt.Sprintf("Migrated from V1 on %s", now.Format("2006-01-02")),
		Repository:      v1.ProjectPath,
	}

	// Set completion date if status is completed
	if v1.EndedAt != nil {
		v2.CompletionDate = v1.EndedAt
	}

	// Convert phase history
	v2.WaypointHistory = m.convertPhaseHistory(v1.Phases)

	// Initialize roadmap
	v2.Roadmap = &status.Roadmap{
		Phases: m.createInitialRoadmap(v2.CurrentWaypoint),
	}

	// Initialize quality metrics
	v2.QualityMetrics = &status.QualityMetrics{
		CoverageTarget:         80.0,
		AssertionDensityTarget: 3.0,
	}

	return v2, nil
}

// convertPhaseHistory converts V1 phases to V2 phase history
func (m *Migrator) convertPhaseHistory(v1Phases []status.Phase) []status.PhaseHistory {
	var history []status.PhaseHistory

	for _, v1Phase := range v1Phases {
		// Map V1 phase name to V2
		v2PhaseName := m.mapV1PhaseToV2(v1Phase.Name)
		if v2PhaseName == "" {
			continue // Skip invalid phases
		}

		v2Phase := status.PhaseHistory{
			Name:        v2PhaseName,
			Status:      m.mapV1PhaseStatusToV2(v1Phase.Status),
			StartedAt:   safeTimeValue(v1Phase.StartedAt),
			CompletedAt: v1Phase.CompletedAt,
			Notes:       fmt.Sprintf("Migrated from V1 phase: %s", v1Phase.Name),
		}

		// Set outcome
		if v1Phase.Outcome != "" {
			outcome := v1Phase.Outcome
			v2Phase.Outcome = &outcome
		}

		// Add phase-specific metadata based on phase type
		switch v2PhaseName {
		case status.PhaseV2Spec:
			// D4 includes S4 stakeholder data
			approved := v1Phase.Status == status.PhaseStatusCompleted
			v2Phase.StakeholderApproved = &approved
			v2Phase.StakeholderNotes = "Migrated from S4 stakeholder alignment"
		case status.PhaseV2Plan:
			// S6 includes S5 research
			v2Phase.ResearchNotes = "Migrated from S5 research phase"
		case status.PhaseV2Build:
			// S8 includes S8/S9/S10 build loop
			v2Phase.BuildIterations = 1
			v2Phase.ValidationStatus = m.inferValidationStatus(v1Phase)
			v2Phase.DeploymentStatus = m.inferDeploymentStatus(v1Phase)
		}

		history = append(history, v2Phase)
	}

	return history
}

// mapV1PhaseToV2 maps V1 phase names to V2 consolidated phases
func (m *Migrator) mapV1PhaseToV2(v1Phase string) string {
	mapping := map[string]string{
		"W0":  status.PhaseV2Charter,
		"D1":  status.PhaseV2Problem,
		"D2":  status.PhaseV2Research,
		"D3":  status.PhaseV2Design,
		"D4":  status.PhaseV2Spec,
		"S4":  status.PhaseV2Spec,  // S4 merged into D4
		"S5":  status.PhaseV2Plan,  // S5 merged into S6
		"S6":  status.PhaseV2Setup, // V1 S6 becomes V2 S7
		"S7":  status.PhaseV2Setup,
		"S8":  status.PhaseV2Build,
		"S9":  status.PhaseV2Build, // S9 merged into S8
		"S10": status.PhaseV2Build, // S10 merged into S8
		"S11": status.PhaseV2Retro,
	}

	return mapping[v1Phase]
}

// mapV1StatusToV2 maps V1 status values to V2
func (m *Migrator) mapV1StatusToV2(v1Status string) string {
	mapping := map[string]string{
		status.StatusInProgress: status.StatusV2InProgress,
		status.StatusCompleted:  status.StatusV2Completed,
		status.StatusAbandoned:  status.StatusV2Abandoned,
		status.StatusBlocked:    status.StatusV2Blocked,
		status.StatusObsolete:   status.StatusV2Abandoned,
	}

	if v2Status, ok := mapping[v1Status]; ok {
		return v2Status
	}
	return status.StatusV2Planning
}

// mapV1PhaseStatusToV2 maps V1 phase status to V2
func (m *Migrator) mapV1PhaseStatusToV2(v1Status string) string {
	mapping := map[string]string{
		status.PhaseStatusPending:    status.PhaseStatusV2Pending,
		status.PhaseStatusInProgress: status.PhaseStatusV2InProgress,
		status.PhaseStatusCompleted:  status.PhaseStatusV2Completed,
		status.PhaseStatusSkipped:    status.PhaseStatusV2Skipped,
	}

	if v2Status, ok := mapping[v1Status]; ok {
		return v2Status
	}
	return status.PhaseStatusV2Pending
}

// inferValidationStatus infers validation status for S8 from V1 phase
func (m *Migrator) inferValidationStatus(v1Phase status.Phase) string {
	if v1Phase.Status == status.PhaseStatusCompleted {
		return status.ValidationStatusPassed
	}
	if v1Phase.Status == status.PhaseStatusInProgress {
		return status.ValidationStatusInProgress
	}
	return status.ValidationStatusPending
}

// inferDeploymentStatus infers deployment status for S8 from V1 phase
func (m *Migrator) inferDeploymentStatus(v1Phase status.Phase) string {
	if v1Phase.Status == status.PhaseStatusCompleted {
		return status.DeploymentStatusDeployed
	}
	if v1Phase.Status == status.PhaseStatusInProgress {
		return status.DeploymentStatusInProgress
	}
	return status.DeploymentStatusPending
}

// extractProjectName extracts project name from path
func (m *Migrator) extractProjectName(projectPath string) string {
	if projectPath == "" {
		return "Unnamed Project"
	}
	return filepath.Base(projectPath)
}

// inferProjectType infers project type from V1 status
func (m *Migrator) inferProjectType(v1 *status.Status) string {
	// Default to feature if unknown
	return status.ProjectTypeFeature
}

// calculateRiskLevel calculates risk level based on V1 project data
func (m *Migrator) calculateRiskLevel(v1 *status.Status) string {
	// Count completed phases
	completedPhases := 0
	for _, phase := range v1.Phases {
		if phase.Status == status.PhaseStatusCompleted {
			completedPhases++
		}
	}

	// Simple heuristic based on phase count
	switch {
	case completedPhases <= 2:
		return status.RiskLevelXS
	case completedPhases <= 4:
		return status.RiskLevelS
	case completedPhases <= 6:
		return status.RiskLevelM
	case completedPhases <= 8:
		return status.RiskLevelL
	}
	return status.RiskLevelXL
}

// createInitialRoadmap creates initial roadmap phases for V2
func (m *Migrator) createInitialRoadmap(currentPhase string) []status.RoadmapPhase {
	allPhases := status.AllPhasesV2Schema()
	var roadmap []status.RoadmapPhase

	for _, phaseName := range allPhases {
		phase := status.RoadmapPhase{
			ID:     phaseName,
			Name:   m.getPhaseDisplayName(phaseName),
			Status: status.PhaseStatusV2Pending,
			Tasks:  []status.Task{},
		}

		// Mark phases before current as completed
		if m.isBeforePhase(phaseName, currentPhase) {
			phase.Status = status.PhaseStatusV2Completed
		} else if phaseName == currentPhase {
			phase.Status = status.PhaseStatusV2InProgress
		}

		roadmap = append(roadmap, phase)
	}

	return roadmap
}

// getPhaseDisplayName returns human-readable phase name
func (m *Migrator) getPhaseDisplayName(phase string) string {
	names := map[string]string{
		status.PhaseV2Charter:  "Project Intake & Bootstrapping",
		status.PhaseV2Problem:  "Problem Definition & Research",
		status.PhaseV2Research: "Solution Exploration",
		status.PhaseV2Design:   "Detailed Design",
		status.PhaseV2Spec:     "Requirements Sign-off",
		status.PhaseV2Plan:     "Implementation Planning",
		status.PhaseV2Setup:    "Development Environment Setup",
		status.PhaseV2Build:    "BUILD Loop",
		status.PhaseV2Retro:    "Documentation & Knowledge Transfer",
	}
	return names[phase]
}

// isBeforePhase checks if phase1 comes before phase2 in sequence
func (m *Migrator) isBeforePhase(phase1, phase2 string) bool {
	phases := status.AllPhasesV2Schema()
	idx1, idx2 := -1, -1

	for i, p := range phases {
		if p == phase1 {
			idx1 = i
		}
		if p == phase2 {
			idx2 = i
		}
	}

	return idx1 != -1 && idx2 != -1 && idx1 < idx2
}

// safeTimeValue returns time value or zero time if nil
func safeTimeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
