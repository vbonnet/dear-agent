// Package workspace provides workspace-related functionality.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestDataConfig configures test data generation
type TestDataConfig struct {
	RootDir           string
	OSSProjects       int
	AcmeProjects      int
	IncludePhaseFiles bool
}

// GenerateTestData creates test projects in OSS and Acme workspaces
func GenerateTestData(config TestDataConfig) error {
	ossRoot := filepath.Join(config.RootDir, "oss", "wf")
	acmeRoot := filepath.Join(config.RootDir, "acme", "wf")

	// Create workspace roots
	if err := os.MkdirAll(ossRoot, 0o700); err != nil {
		return fmt.Errorf("failed to create OSS workspace root: %w", err)
	}
	if err := os.MkdirAll(acmeRoot, 0o700); err != nil {
		return fmt.Errorf("failed to create Acme workspace root: %w", err)
	}

	// Generate OSS projects
	for i := 0; i < config.OSSProjects; i++ {
		projectName := fmt.Sprintf("oss-project-%d", i+1)
		projectPath := filepath.Join(ossRoot, projectName)

		phase := getPhaseByIndex(i)
		if err := createTestProject(projectPath, "oss", projectName, phase, config.IncludePhaseFiles); err != nil {
			return fmt.Errorf("failed to create OSS project %s: %w", projectName, err)
		}
	}

	// Generate Acme projects
	for i := 0; i < config.AcmeProjects; i++ {
		projectName := fmt.Sprintf("acme-project-%d", i+1)
		projectPath := filepath.Join(acmeRoot, projectName)

		phase := getPhaseByIndex(i)
		if err := createTestProject(projectPath, "acme", projectName, phase, config.IncludePhaseFiles); err != nil {
			return fmt.Errorf("failed to create Acme project %s: %w", projectName, err)
		}
	}

	return nil
}

// createTestProject creates a single test project with status file
func createTestProject(projectPath, workspace, projectName, phase string, includePhaseFiles bool) error {
	// Create project directory
	if err := os.MkdirAll(projectPath, 0o700); err != nil {
		return err
	}

	// Create status
	sessionID := fmt.Sprintf("%s-%s-%s", workspace, projectName, time.Now().Format("20060102-150405"))

	st := &status.Status{
		SchemaVersion: status.SchemaVersion,
		Version:       status.WayfinderV2,
		SessionID:     sessionID,
		ProjectPath:   projectPath,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  phase,
		Phases:        []status.Phase{},
	}

	// Add phases up to current phase
	allPhases := status.AllPhasesV2()
	for _, p := range allPhases {
		phaseStatus := status.PhaseStatusCompleted
		var completedAt = timePtr(time.Now().Add(-1 * time.Hour))

		if p == phase {
			phaseStatus = status.PhaseStatusInProgress
			completedAt = nil
		} else if isPhaseAfter(p, phase, allPhases) {
			phaseStatus = status.PhaseStatusPending
			completedAt = nil
		}

		st.Phases = append(st.Phases, status.Phase{
			Name:        p,
			Status:      phaseStatus,
			StartedAt:   timePtr(time.Now().Add(-2 * time.Hour)),
			CompletedAt: completedAt,
		})

		// Create phase file if requested and phase is active
		if includePhaseFiles && (phaseStatus == status.PhaseStatusInProgress || phaseStatus == status.PhaseStatusCompleted) {
			if err := createPhaseFile(projectPath, workspace, projectName, p); err != nil {
				return err
			}
		}
	}

	// Save status file
	return status.Save(projectPath, st)
}

// createPhaseFile creates a phase markdown file with workspace-specific content
func createPhaseFile(projectPath, workspace, projectName, phase string) error {
	filename := status.PhaseToFileName(phase)
	filepath := filepath.Join(projectPath, filename)

	content := fmt.Sprintf(`---
phase: %s
status: in_progress
workspace: %s
project: %s
---

# %s

## Workspace: %s
## Project: %s

This phase file is specific to the **%s** workspace.
It should NEVER contain data from other workspaces.

## Phase Content

- Task 1 for %s in %s
- Task 2 for %s in %s
- Task 3 for %s in %s

## Confidentiality

This file is part of workspace: **%s**
Cross-workspace data leakage would be a critical security violation.
`, phase, workspace, projectName, phase, workspace, projectName, workspace,
		projectName, workspace, projectName, workspace, projectName, workspace, workspace)

	return os.WriteFile(filepath, []byte(content), 0o600)
}

// getPhaseByIndex returns a phase for test data generation
// Distributes projects across different phases
func getPhaseByIndex(index int) string {
	phases := []string{
		"discovery.problem",
		"build.implement",
		"deploy",
		"discovery.solutions",
		"design.tech-lead",
		"retrospective",
	}
	return phases[index%len(phases)]
}

// isPhaseAfter returns true if phase1 comes after phase2 in the sequence
func isPhaseAfter(phase1, phase2 string, allPhases []string) bool {
	index1 := -1
	index2 := -1

	for i, p := range allPhases {
		if p == phase1 {
			index1 = i
		}
		if p == phase2 {
			index2 = i
		}
	}

	if index1 == -1 || index2 == -1 {
		return false
	}

	return index1 > index2
}

// ValidateTestData verifies test data was created correctly
func ValidateTestData(config TestDataConfig) (ValidationResult, error) {
	result := ValidationResult{
		OSSProjects:  []ProjectInfo{},
		AcmeProjects: []ProjectInfo{},
		Violations:   []string{},
	}

	ossRoot := filepath.Join(config.RootDir, "oss", "wf")
	acmeRoot := filepath.Join(config.RootDir, "acme", "wf")

	// List OSS projects
	ossProjects, err := ListProjects(ossRoot)
	if err != nil {
		return result, fmt.Errorf("failed to list OSS projects: %w", err)
	}
	result.OSSProjects = ossProjects

	// List Acme projects
	acmeProjects, err := ListProjects(acmeRoot)
	if err != nil {
		return result, fmt.Errorf("failed to list Acme projects: %w", err)
	}
	result.AcmeProjects = acmeProjects

	// Validate counts
	if len(ossProjects) != config.OSSProjects {
		result.Violations = append(result.Violations,
			fmt.Sprintf("Expected %d OSS projects, found %d", config.OSSProjects, len(ossProjects)))
	}

	if len(acmeProjects) != config.AcmeProjects {
		result.Violations = append(result.Violations,
			fmt.Sprintf("Expected %d Acme projects, found %d", config.AcmeProjects, len(acmeProjects)))
	}

	// Validate workspace isolation
	for _, project := range ossProjects {
		if project.Workspace != "oss" {
			result.Violations = append(result.Violations,
				fmt.Sprintf("OSS project has wrong workspace: %s (path: %s)", project.Workspace, project.ProjectPath))
		}
		if !strings.HasPrefix(project.ProjectPath, ossRoot) {
			result.Violations = append(result.Violations,
				fmt.Sprintf("OSS project outside OSS root: %s", project.ProjectPath))
		}
	}

	for _, project := range acmeProjects {
		if project.Workspace != "acme" {
			result.Violations = append(result.Violations,
				fmt.Sprintf("Acme project has wrong workspace: %s (path: %s)", project.Workspace, project.ProjectPath))
		}
		if !strings.HasPrefix(project.ProjectPath, acmeRoot) {
			result.Violations = append(result.Violations,
				fmt.Sprintf("Acme project outside Acme root: %s", project.ProjectPath))
		}
	}

	// Check for duplicate session IDs across workspaces
	sessionIDs := make(map[string]string) // sessionID -> workspace
	for _, project := range ossProjects {
		if existingWorkspace, exists := sessionIDs[project.SessionID]; exists {
			result.Violations = append(result.Violations,
				fmt.Sprintf("Duplicate session ID %s in workspaces %s and oss", project.SessionID, existingWorkspace))
		}
		sessionIDs[project.SessionID] = "oss"
	}

	for _, project := range acmeProjects {
		if existingWorkspace, exists := sessionIDs[project.SessionID]; exists {
			result.Violations = append(result.Violations,
				fmt.Sprintf("Duplicate session ID %s in workspaces %s and acme", project.SessionID, existingWorkspace))
		}
		sessionIDs[project.SessionID] = "acme"
	}

	result.IsValid = len(result.Violations) == 0

	return result, nil
}

// ValidationResult contains test data validation results
type ValidationResult struct {
	IsValid      bool
	OSSProjects  []ProjectInfo
	AcmeProjects []ProjectInfo
	Violations   []string
}

// timePtr returns a pointer to a time.Time value
func timePtr(t time.Time) *time.Time {
	return &t
}
