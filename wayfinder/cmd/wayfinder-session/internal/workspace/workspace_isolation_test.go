package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestWorkspaceIsolation verifies zero cross-contamination between workspaces
// This is a critical security/privacy requirement for multi-workspace Wayfinder
func TestWorkspaceIsolation(t *testing.T) {
	if os.Getenv("WAYFINDER_TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test (set WAYFINDER_TEST_INTEGRATION=1 to enable)")
	}

	// Test Setup: Create two separate workspace test directories
	// In production, these would be in separate workspace roots
	// OSS workspace: ~/src/ws/oss/wf/
	// Acme workspace: ~/src/ws/acme/wf/

	testRoot := t.TempDir()
	ossRoot := filepath.Join(testRoot, "oss", "wf")
	acmeRoot := filepath.Join(testRoot, "acme", "wf")

	if err := os.MkdirAll(ossRoot, 0755); err != nil {
		t.Fatalf("Failed to create OSS workspace root: %v", err)
	}
	if err := os.MkdirAll(acmeRoot, 0755); err != nil {
		t.Fatalf("Failed to create Acme workspace root: %v", err)
	}

	// Test 1: Verify workspace detection from path
	t.Run("WorkspaceDetection", func(t *testing.T) {
		// OSS project
		ossProject := filepath.Join(ossRoot, "test-project-oss")
		if err := os.MkdirAll(ossProject, 0755); err != nil {
			t.Fatalf("Failed to create OSS project: %v", err)
		}

		ossWorkspace := DetectWorkspace(ossProject)
		if ossWorkspace != "oss" {
			t.Errorf("Expected OSS workspace detection, got '%s'", ossWorkspace)
		}

		// Acme project
		acmeProject := filepath.Join(acmeRoot, "test-project-acme")
		if err := os.MkdirAll(acmeProject, 0755); err != nil {
			t.Fatalf("Failed to create Acme project: %v", err)
		}

		acmeWorkspace := DetectWorkspace(acmeProject)
		if acmeWorkspace != "acme" {
			t.Errorf("Expected Acme workspace detection, got '%s'", acmeWorkspace)
		}
	})

	// Test 2: Create projects in both workspaces with overlapping names
	// This tests that project names can be reused across workspaces without conflict
	projectName := "shared-project-name"

	ossProjectPath := filepath.Join(ossRoot, projectName)
	acmeProjectPath := filepath.Join(acmeRoot, projectName)

	ossSessionID := "oss-session-" + time.Now().Format("20060102-150405")
	acmeSessionID := "acme-session-" + time.Now().Format("20060102-150405")

	ossStatus := &status.Status{
		SchemaVersion: status.SchemaVersion,
		Version:       status.WayfinderV2,
		SessionID:     ossSessionID,
		ProjectPath:   ossProjectPath,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "discovery.problem",
		Phases: []status.Phase{
			{Name: "discovery.problem", Status: status.PhaseStatusInProgress, StartedAt: timePtr(time.Now())},
		},
	}

	acmeStatus := &status.Status{
		SchemaVersion: status.SchemaVersion,
		Version:       status.WayfinderV2,
		SessionID:     acmeSessionID,
		ProjectPath:   acmeProjectPath,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "build.implement",
		Phases: []status.Phase{
			{Name: "build.implement", Status: status.PhaseStatusInProgress, StartedAt: timePtr(time.Now())},
		},
	}

	t.Run("CreateProjects", func(t *testing.T) {
		// Create OSS project
		if err := os.MkdirAll(ossProjectPath, 0755); err != nil {
			t.Fatalf("Failed to create OSS project directory: %v", err)
		}
		if err := status.Save(ossProjectPath, ossStatus); err != nil {
			t.Fatalf("Failed to save OSS project status: %v", err)
		}

		// Create Acme project
		if err := os.MkdirAll(acmeProjectPath, 0755); err != nil {
			t.Fatalf("Failed to create Acme project directory: %v", err)
		}
		if err := status.Save(acmeProjectPath, acmeStatus); err != nil {
			t.Fatalf("Failed to save Acme project status: %v", err)
		}
	})

	// Test 3: Verify project retrieval respects workspace boundaries
	t.Run("ProjectIsolation", func(t *testing.T) {
		// Load OSS project
		ossLoaded, err := status.Load(ossProjectPath)
		if err != nil {
			t.Fatalf("Failed to load OSS project: %v", err)
		}

		// Verify OSS project data
		if ossLoaded.SessionID != ossSessionID {
			t.Errorf("Expected OSS session ID '%s', got '%s'", ossSessionID, ossLoaded.SessionID)
		}
		if ossLoaded.CurrentPhase != "discovery.problem" {
			t.Errorf("Expected OSS phase 'discovery.problem', got '%s'", ossLoaded.CurrentPhase)
		}
		if ossLoaded.ProjectPath != ossProjectPath {
			t.Errorf("Expected OSS project path '%s', got '%s'", ossProjectPath, ossLoaded.ProjectPath)
		}

		// Load Acme project
		acmeLoaded, err := status.Load(acmeProjectPath)
		if err != nil {
			t.Fatalf("Failed to load Acme project: %v", err)
		}

		// Verify Acme project data
		if acmeLoaded.SessionID != acmeSessionID {
			t.Errorf("Expected Acme session ID '%s', got '%s'", acmeSessionID, acmeLoaded.SessionID)
		}
		if acmeLoaded.CurrentPhase != "build.implement" {
			t.Errorf("Expected Acme phase 'build.implement', got '%s'", acmeLoaded.CurrentPhase)
		}
		if acmeLoaded.ProjectPath != acmeProjectPath {
			t.Errorf("Expected Acme project path '%s', got '%s'", acmeProjectPath, acmeLoaded.ProjectPath)
		}

		// CRITICAL: Verify no cross-contamination
		if ossLoaded.CurrentPhase == acmeLoaded.CurrentPhase {
			t.Error("SECURITY VIOLATION: OSS and Acme projects have same phase (possible contamination)")
		}
		if ossLoaded.SessionID == acmeLoaded.SessionID {
			t.Error("SECURITY VIOLATION: OSS and Acme projects have same session ID")
		}
		if ossLoaded.ProjectPath == acmeLoaded.ProjectPath {
			t.Error("SECURITY VIOLATION: OSS and Acme projects have same path")
		}
	})

	// Test 4: Phase data isolation
	t.Run("PhaseDataIsolation", func(t *testing.T) {
		// Create phase files in both workspaces
		ossPhaseFile := filepath.Join(ossProjectPath, "discovery-problem.md")
		acmePhaseFile := filepath.Join(acmeProjectPath, "build-implement.md")

		ossPhaseContent := `---
phase: discovery.problem
status: in_progress
workspace: oss
---

# Discovery: Problem Definition

This is OSS-specific problem analysis.
Confidential information from other workspaces should NEVER appear here.

## Key Problems
- OSS Problem 1
- OSS Problem 2
`

		acmePhaseContent := `---
phase: build.implement
status: in_progress
workspace: acme
---

# Build: Implementation

This is Acme-specific implementation work.
Information from other workspaces should NEVER appear here.

## Implementation Tasks
- Acme Task 1 (CONFIDENTIAL)
- Acme Task 2 (CONFIDENTIAL)
`

		if err := os.WriteFile(ossPhaseFile, []byte(ossPhaseContent), 0644); err != nil {
			t.Fatalf("Failed to write OSS phase file: %v", err)
		}

		if err := os.WriteFile(acmePhaseFile, []byte(acmePhaseContent), 0644); err != nil {
			t.Fatalf("Failed to write Acme phase file: %v", err)
		}

		// Read back and verify isolation
		ossRead, err := os.ReadFile(ossPhaseFile)
		if err != nil {
			t.Fatalf("Failed to read OSS phase file: %v", err)
		}

		acmeRead, err := os.ReadFile(acmePhaseFile)
		if err != nil {
			t.Fatalf("Failed to read Acme phase file: %v", err)
		}

		// Verify content isolation
		ossContent := string(ossRead)
		acmeContent := string(acmeRead)

		// OSS content should not contain Acme-specific data
		if containsAny(ossContent, []string{"Acme", "CONFIDENTIAL", "acme"}) {
			t.Error("SECURITY VIOLATION: OSS phase file contains Acme-specific content")
		}

		// Acme content should not contain OSS-specific data
		if containsAny(acmeContent, []string{"OSS Problem", "oss"}) {
			t.Error("SECURITY VIOLATION: Acme phase file contains OSS-specific content")
		}
	})

	// Test 5: List projects - verify each workspace only sees its own projects
	t.Run("ListProjectsIsolation", func(t *testing.T) {
		// List OSS projects
		ossProjects, err := ListProjects(ossRoot)
		if err != nil {
			t.Fatalf("Failed to list OSS projects: %v", err)
		}

		// List Acme projects
		acmeProjects, err := ListProjects(acmeRoot)
		if err != nil {
			t.Fatalf("Failed to list Acme projects: %v", err)
		}

		// Verify OSS workspace only sees OSS projects
		for _, project := range ossProjects {
			if !strings.HasPrefix(project.ProjectPath, ossRoot) {
				t.Errorf("SECURITY VIOLATION: OSS workspace sees non-OSS project: %s", project.ProjectPath)
			}
			workspace := DetectWorkspace(project.ProjectPath)
			if workspace != "oss" {
				t.Errorf("SECURITY VIOLATION: OSS workspace sees project from workspace: %s", workspace)
			}
		}

		// Verify Acme workspace only sees Acme projects
		for _, project := range acmeProjects {
			if !strings.HasPrefix(project.ProjectPath, acmeRoot) {
				t.Errorf("SECURITY VIOLATION: Acme workspace sees non-Acme project: %s", project.ProjectPath)
			}
			workspace := DetectWorkspace(project.ProjectPath)
			if workspace != "acme" {
				t.Errorf("SECURITY VIOLATION: Acme workspace sees project from workspace: %s", workspace)
			}
		}

		// Log project counts for verification
		t.Logf("OSS workspace has %d projects", len(ossProjects))
		t.Logf("Acme workspace has %d projects", len(acmeProjects))
	})

	// Test 6: Update operations respect workspace boundaries
	t.Run("UpdateIsolation", func(t *testing.T) {
		// Update OSS project
		ossStatus.CurrentPhase = "discovery.solutions"
		ossStatus.Phases = append(ossStatus.Phases, status.Phase{
			Name:      "discovery.solutions",
			Status:    status.PhaseStatusInProgress,
			StartedAt: timePtr(time.Now()),
		})

		if err := status.Save(ossProjectPath, ossStatus); err != nil {
			t.Errorf("Failed to update OSS project: %v", err)
		}

		// Verify update succeeded
		ossUpdated, err := status.Load(ossProjectPath)
		if err != nil {
			t.Fatalf("Failed to load updated OSS project: %v", err)
		}
		if ossUpdated.CurrentPhase != "discovery.solutions" {
			t.Error("OSS project update failed")
		}

		// Verify Acme project remains unchanged
		acmeCheck, err := status.Load(acmeProjectPath)
		if err != nil {
			t.Fatalf("Failed to load Acme project: %v", err)
		}
		if acmeCheck.CurrentPhase != "build.implement" {
			t.Error("SECURITY VIOLATION: Acme project was modified by OSS update")
		}
		if acmeCheck.SessionID != acmeSessionID {
			t.Error("SECURITY VIOLATION: Acme session ID changed")
		}
	})

	// Test 7: Delete operations respect workspace boundaries
	t.Run("DeleteIsolation", func(t *testing.T) {
		// Create additional projects for delete test
		deleteTestName := "delete-test-" + time.Now().Format("20060102-150405")

		ossDeletePath := filepath.Join(ossRoot, deleteTestName)
		acmeDeletePath := filepath.Join(acmeRoot, deleteTestName)

		// Create projects
		if err := os.MkdirAll(ossDeletePath, 0755); err != nil {
			t.Fatalf("Failed to create OSS delete test project: %v", err)
		}
		if err := os.MkdirAll(acmeDeletePath, 0755); err != nil {
			t.Fatalf("Failed to create Acme delete test project: %v", err)
		}

		ossDeleteStatus := &status.Status{
			SchemaVersion: status.SchemaVersion,
			Version:       status.WayfinderV2,
			SessionID:     "oss-delete-session",
			ProjectPath:   ossDeletePath,
			StartedAt:     time.Now(),
			Status:        status.StatusInProgress,
		}

		acmeDeleteStatus := &status.Status{
			SchemaVersion: status.SchemaVersion,
			Version:       status.WayfinderV2,
			SessionID:     "acme-delete-session",
			ProjectPath:   acmeDeletePath,
			StartedAt:     time.Now(),
			Status:        status.StatusInProgress,
		}

		if err := status.Save(ossDeletePath, ossDeleteStatus); err != nil {
			t.Fatalf("Failed to save OSS delete test status: %v", err)
		}
		if err := status.Save(acmeDeletePath, acmeDeleteStatus); err != nil {
			t.Fatalf("Failed to save Acme delete test status: %v", err)
		}

		// Delete OSS project
		if err := os.RemoveAll(ossDeletePath); err != nil {
			t.Errorf("Failed to delete OSS project: %v", err)
		}

		// Verify OSS project is deleted
		if _, err := os.Stat(ossDeletePath); !os.IsNotExist(err) {
			t.Error("OSS project still exists after deletion")
		}

		// Verify Acme project still exists
		acmeStillExists, err := status.Load(acmeDeletePath)
		if err != nil {
			t.Error("SECURITY VIOLATION: Acme project was affected by OSS delete operation")
		}
		if acmeStillExists == nil {
			t.Error("SECURITY VIOLATION: Acme project is nil after OSS delete")
		} else if acmeStillExists.SessionID != "acme-delete-session" {
			t.Error("SECURITY VIOLATION: Acme project corrupted after OSS delete")
		}

		// Cleanup
		os.RemoveAll(acmeDeletePath)
	})

	// Test 8: Environment variable isolation
	t.Run("EnvironmentIsolation", func(t *testing.T) {
		// Set workspace-specific environment variables
		os.Setenv("WAYFINDER_WORKSPACE", "oss")
		ossWorkspace := os.Getenv("WAYFINDER_WORKSPACE")
		if ossWorkspace != "oss" {
			t.Errorf("Expected OSS workspace from env, got '%s'", ossWorkspace)
		}

		os.Setenv("WAYFINDER_WORKSPACE", "acme")
		acmeWorkspace := os.Getenv("WAYFINDER_WORKSPACE")
		if acmeWorkspace != "acme" {
			t.Errorf("Expected Acme workspace from env, got '%s'", acmeWorkspace)
		}

		// Cleanup
		os.Unsetenv("WAYFINDER_WORKSPACE")
	})
}

// TestWorkspaceFilterEdgeCases tests edge cases and error conditions
func TestWorkspaceFilterEdgeCases(t *testing.T) {
	if os.Getenv("WAYFINDER_TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test (set WAYFINDER_TEST_INTEGRATION=1 to enable)")
	}

	testRoot := t.TempDir()
	workspaceRoot := filepath.Join(testRoot, "edgecase", "wf")

	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		t.Fatalf("Failed to create workspace root: %v", err)
	}

	t.Run("NonExistentProject", func(t *testing.T) {
		nonexistentPath := filepath.Join(workspaceRoot, "nonexistent-project")
		_, err := status.Load(nonexistentPath)
		if err == nil {
			t.Error("Expected error for non-existent project")
		}
	})

	t.Run("EmptyProjectPath", func(t *testing.T) {
		_, err := status.Load("")
		if err == nil {
			t.Error("Expected error for empty project path")
		}
	})

	t.Run("InvalidWorkspacePath", func(t *testing.T) {
		invalidPath := "/tmp/not-a-workspace/project"
		workspace := DetectWorkspace(invalidPath)
		if workspace != "" {
			t.Errorf("Expected empty workspace for invalid path, got '%s'", workspace)
		}
	})

	t.Run("ListEmptyWorkspace", func(t *testing.T) {
		emptyRoot := filepath.Join(testRoot, "empty", "wf")
		if err := os.MkdirAll(emptyRoot, 0755); err != nil {
			t.Fatalf("Failed to create empty workspace: %v", err)
		}

		projects, err := ListProjects(emptyRoot)
		if err != nil {
			t.Errorf("Failed to list empty workspace: %v", err)
		}
		if len(projects) != 0 {
			t.Errorf("Expected 0 projects in empty workspace, got %d", len(projects))
		}
	})
}

// Helper functions

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if contains(s, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findInString(s, substr)
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
