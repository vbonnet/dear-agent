package status

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIntegrationV2Workflow tests the complete V2 workflow
func TestIntegrationV2Workflow(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Step 1: Create a new V2 status
	t.Log("Step 1: Creating new V2 status")
	status := NewStatusV2("Integration Test Project", ProjectTypeFeature, RiskLevelL)
	status.Description = "Test the complete V2 workflow"
	status.Repository = "https://github.com/test/integration"
	status.Branch = "feature/integration-test"
	status.Tags = []string{"test", "integration", "v2"}

	// Step 2: Add phase history
	t.Log("Step 2: Adding phase history")
	now := time.Now().Truncate(time.Second)
	completed := now.Add(-1 * time.Hour)
	status.WaypointHistory = []PhaseHistory{
		{
			Name:         PhaseV2Charter,
			Status:       PhaseStatusV2Completed,
			StartedAt:    now.Add(-2 * time.Hour),
			CompletedAt:  &completed,
			Deliverables: []string{"W0-intake.md"},
		},
	}

	// Step 3: Add roadmap with tasks
	t.Log("Step 3: Adding roadmap with tasks")
	status.Roadmap = &Roadmap{
		Phases: []RoadmapPhase{
			{
				ID:     PhaseV2Setup,
				Name:   "Planning & Task Breakdown",
				Status: PhaseStatusV2Completed,
				Tasks: []Task{
					{
						ID:         "task-7.1",
						Title:      "Create implementation plan",
						EffortDays: 0.5,
						Status:     TaskStatusCompleted,
						Priority:   PriorityP0,
					},
				},
			},
			{
				ID:     PhaseV2Build,
				Name:   "BUILD Loop",
				Status: PhaseStatusV2InProgress,
				Tasks: []Task{
					{
						ID:           "task-8.1",
						Title:        "Implement core functionality",
						EffortDays:   2.0,
						Status:       TaskStatusCompleted,
						Priority:     PriorityP0,
						DependsOn:    []string{},
						Blocks:       []string{"task-8.2"},
						Deliverables: []string{"src/core.go"},
					},
					{
						ID:         "task-8.2",
						Title:      "Add validation",
						EffortDays: 1.5,
						Status:     TaskStatusInProgress,
						Priority:   PriorityP0,
						DependsOn:  []string{"task-8.1"},
						Blocks:     []string{"task-8.3"},
					},
					{
						ID:         "task-8.3",
						Title:      "Integration tests",
						EffortDays: 1.0,
						Status:     TaskStatusPending,
						Priority:   PriorityP0,
						DependsOn:  []string{"task-8.2"},
					},
				},
			},
		},
	}

	// Step 4: Add quality metrics
	t.Log("Step 4: Adding quality metrics")
	status.QualityMetrics = &QualityMetrics{
		CoveragePercent:        85.5,
		CoverageTarget:         80.0,
		AssertionDensity:       3.5,
		AssertionDensityTarget: 3.0,
		MultiPersonaScore:      88.0,
		SecurityScore:          92.0,
		PerformanceScore:       85.0,
		ReliabilityScore:       90.0,
		MaintainabilityScore:   82.0,
		P0Issues:               0,
		P1Issues:               3,
		P2Issues:               7,
		EstimatedEffortHours:   40.0,
		ActualEffortHours:      38.5,
		EffortVariance:         -3.75,
	}

	// Step 5: Validate the status
	t.Log("Step 5: Validating status")
	if err := ValidateV2(status); err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Step 6: Write to file
	t.Log("Step 6: Writing to file")
	statusPath := filepath.Join(tmpDir, StatusFilename)
	if err := WriteV2(status, statusPath); err != nil {
		t.Fatalf("Failed to write status: %v", err)
	}

	// Step 7: Read back from file
	t.Log("Step 7: Reading back from file")
	readStatus, err := ParseV2(statusPath)
	if err != nil {
		t.Fatalf("Failed to read status: %v", err)
	}

	// Step 8: Validate read status
	t.Log("Step 8: Validating read status")
	if err := ValidateV2(readStatus); err != nil {
		t.Fatalf("Validation of read status failed: %v", err)
	}

	// Step 9: Verify data integrity
	t.Log("Step 9: Verifying data integrity")
	if readStatus.ProjectName != status.ProjectName {
		t.Errorf("ProjectName mismatch: want %s, got %s", status.ProjectName, readStatus.ProjectName)
	}
	if len(readStatus.Tags) != len(status.Tags) {
		t.Errorf("Tags length mismatch: want %d, got %d", len(status.Tags), len(readStatus.Tags))
	}
	if len(readStatus.WaypointHistory) != len(status.WaypointHistory) {
		t.Errorf("PhaseHistory length mismatch: want %d, got %d", len(status.WaypointHistory), len(readStatus.WaypointHistory))
	}
	if len(readStatus.Roadmap.Phases) != len(status.Roadmap.Phases) {
		t.Errorf("Roadmap phases length mismatch: want %d, got %d", len(status.Roadmap.Phases), len(readStatus.Roadmap.Phases))
	}

	// Verify task count
	originalTaskCount := 0
	readTaskCount := 0
	for _, phase := range status.Roadmap.Phases {
		originalTaskCount += len(phase.Tasks)
	}
	for _, phase := range readStatus.Roadmap.Phases {
		readTaskCount += len(phase.Tasks)
	}
	if readTaskCount != originalTaskCount {
		t.Errorf("Task count mismatch: want %d, got %d", originalTaskCount, readTaskCount)
	}

	// Step 10: Modify and save again
	t.Log("Step 10: Modifying and saving again")
	readStatus.UpdatedAt = time.Now().Truncate(time.Second)
	readStatus.CurrentWaypoint = PhaseV2Build

	// Add a new task
	newTask := Task{
		ID:         "task-8.4",
		Title:      "Documentation",
		EffortDays: 0.5,
		Status:     TaskStatusPending,
		Priority:   PriorityP1,
		DependsOn:  []string{"task-8.3"},
	}
	for i := range readStatus.Roadmap.Phases {
		if readStatus.Roadmap.Phases[i].ID == PhaseV2Build {
			readStatus.Roadmap.Phases[i].Tasks = append(
				readStatus.Roadmap.Phases[i].Tasks,
				newTask,
			)
			break
		}
	}

	// Validate modified status
	if err := ValidateV2(readStatus); err != nil {
		t.Fatalf("Validation of modified status failed: %v", err)
	}

	// Write modified status
	if err := WriteV2(readStatus, statusPath); err != nil {
		t.Fatalf("Failed to write modified status: %v", err)
	}

	// Read final version
	finalStatus, err := ParseV2(statusPath)
	if err != nil {
		t.Fatalf("Failed to read final status: %v", err)
	}

	// Verify new task was added
	finalTaskCount := 0
	for _, phase := range finalStatus.Roadmap.Phases {
		finalTaskCount += len(phase.Tasks)
	}
	if finalTaskCount != originalTaskCount+1 {
		t.Errorf("Final task count incorrect: want %d, got %d", originalTaskCount+1, finalTaskCount)
	}

	t.Log("Integration test completed successfully")
}

// TestValidExampleFiles tests parsing of valid example files
func TestValidExampleFiles(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{
			name: "valid-v2.yaml",
			file: "testdata/valid-v2.yaml",
		},
		{
			name: "minimal-v2.yaml",
			file: "testdata/minimal-v2.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := ParseV2(tt.file)
			if err != nil {
				t.Fatalf("Failed to parse %s: %v", tt.file, err)
			}

			if err := ValidateV2(status); err != nil {
				t.Errorf("Validation failed for %s: %v", tt.file, err)
			}
		})
	}
}

// TestInvalidExampleFiles tests validation of invalid files
func TestInvalidExampleFiles(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		errMsg string
	}{
		{
			name:   "cyclic dependencies",
			file:   "testdata/invalid-cycle.yaml",
			errMsg: "cyclic dependency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := ParseV2(tt.file)
			if err != nil {
				t.Fatalf("Failed to parse %s: %v", tt.file, err)
			}

			err = ValidateV2(status)
			if err == nil {
				t.Errorf("Expected validation to fail for %s", tt.file)
				return
			}

			if tt.errMsg != "" && !containsSubstring(err.Error(), tt.errMsg) {
				t.Errorf("Expected error containing %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

// contains is a helper that checks if error message contains substring
func containsSubstring(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) &&
		(haystack == needle || strings.Contains(haystack, needle))
}

func TestVerifyFieldsRoundTrip(t *testing.T) {
	t.Log("Test: Verify fields survive YAML round-trip")

	tmpDir := t.TempDir()

	// Create status with task that has verify fields
	st := NewStatusV2("verify-test", ProjectTypeFeature, RiskLevelS)
	now := time.Now()
	st.Roadmap = &Roadmap{
		Phases: []RoadmapPhase{
			{
				ID:     "BUILD",
				Name:   "Build",
				Status: WaypointStatusV2InProgress,
				Tasks: []Task{
					{
						ID:             "task-1",
						Title:          "Auth implementation",
						Status:         TaskStatusInProgress,
						VerifyCommand:  "go test ./auth/...",
						VerifyExpected: "exit code 0, all tests pass",
						VerifiedAt:     &now,
						VerifyResult:   "passed",
					},
				},
			},
		},
	}

	// Write to file
	statusFile := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if err := WriteV2(st, statusFile); err != nil {
		t.Fatalf("Failed to write status: %v", err)
	}

	// Read back
	readSt, err := ParseV2(statusFile)
	if err != nil {
		t.Fatalf("Failed to parse status: %v", err)
	}

	// Verify fields survived round-trip
	if readSt.Roadmap == nil || len(readSt.Roadmap.Phases) == 0 {
		t.Fatal("Roadmap missing after round-trip")
	}

	tasks := readSt.Roadmap.Phases[0].Tasks
	if len(tasks) == 0 {
		t.Fatal("Tasks missing after round-trip")
	}

	task := tasks[0]
	if task.VerifyCommand != "go test ./auth/..." {
		t.Errorf("VerifyCommand: expected %q, got %q", "go test ./auth/...", task.VerifyCommand)
	}
	if task.VerifyExpected != "exit code 0, all tests pass" {
		t.Errorf("VerifyExpected: expected %q, got %q", "exit code 0, all tests pass", task.VerifyExpected)
	}
	if task.VerifiedAt == nil {
		t.Error("VerifiedAt: expected non-nil, got nil")
	}
	if task.VerifyResult != "passed" {
		t.Errorf("VerifyResult: expected %q, got %q", "passed", task.VerifyResult)
	}

	t.Log("Verify fields survived YAML round-trip successfully")
}
