package validator

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// Test helpers

func createStatusFile(t *testing.T, dir string, projectStatus string, allPhasesComplete bool) {
	t.Helper()
	st := status.NewStatusV2("test", status.ProjectTypeFeature, status.RiskLevelM)
	st.Status = projectStatus
	st.CurrentWaypoint = status.PhaseV2Charter
	st.UpdatePhase(status.PhaseV2Charter, status.PhaseStatusV2InProgress, "")
	if allPhasesComplete {
		st.WaypointHistory = nil
		for _, phase := range status.AllPhases() {
			st.UpdatePhase(phase, status.PhaseStatusV2Completed, status.OutcomeSuccess)
		}
		approved := true
		testsCreated := true
		st.FindWaypointHistory(status.WaypointV2Spec).StakeholderApproved = &approved
		st.FindWaypointHistory(status.WaypointV2Plan).TestsFeatureCreated = &testsCreated
		build := st.FindWaypointHistory(status.WaypointV2Build)
		build.ValidationStatus = status.ValidationStatusPassed
		build.DeploymentStatus = status.DeploymentStatusDeployed
		st.CurrentWaypoint = status.WaypointV2Retro
	}
	if projectStatus == status.StatusV2Completed {
		completedAt := time.Now()
		st.CompletionDate = &completedAt
	}
	if err := status.WriteV2ToDir(st, dir); err != nil {
		t.Fatal(err)
	}
}

func createChildProject(t *testing.T, parentDir, childName, projectStatus string, complete bool) string {
	t.Helper()

	tasksDir := filepath.Join(parentDir, status.TasksDirectoryName)
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatal(err)
	}

	childDir := filepath.Join(tasksDir, childName)
	if err := os.Mkdir(childDir, 0755); err != nil {
		t.Fatal(err)
	}

	createStatusFile(t, childDir, projectStatus, complete)

	return childDir
}

// Tests for IsProjectComplete

func TestIsProjectComplete(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
		wantErr  bool
	}{
		{
			name: "status completed",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "completed", true)
				return tmpDir
			},
			expected: true,
			wantErr:  false,
		},
		{
			name: "all phases completed",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", true)
				return tmpDir
			},
			expected: true,
			wantErr:  false,
		},
		{
			name: "some phases incomplete",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", false)
				return tmpDir
			},
			expected: false,
			wantErr:  false,
		},
		{
			name: "partial completed history is incomplete",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				st := status.NewStatusV2("test", status.ProjectTypeFeature, status.RiskLevelM)
				st.Status = status.StatusV2InProgress
				st.UpdatePhase(status.WaypointV2Charter, status.WaypointStatusV2Completed, status.OutcomeSuccess)
				if err := status.WriteV2ToDir(st, tmpDir); err != nil {
					t.Fatal(err)
				}
				return tmpDir
			},
			expected: false,
			wantErr:  false,
		},
		{
			name: "missing STATUS file",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				// No STATUS file created
				return tmpDir
			},
			expected: false,
			wantErr:  true,
		},
		{
			name: "completed with all children complete",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "completed", true)
				createChildProject(t, tmpDir, "child-a", "completed", true)
				createChildProject(t, tmpDir, "child-b", "completed", true)
				return tmpDir
			},
			expected: true,
			wantErr:  false,
		},
		{
			name: "completed but child incomplete",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "completed", true)
				createChildProject(t, tmpDir, "child-a", "completed", true)
				createChildProject(t, tmpDir, "child-b", "in-progress", false)
				return tmpDir
			},
			expected: false,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)

			result, err := IsProjectComplete(dir)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("IsProjectComplete() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAllRequiredWaypointsCompleteRejectsActiveSkippedPhase(t *testing.T) {
	st := status.NewStatusV2("lite-child", status.ProjectTypeFeature, status.RiskLevelXS)
	st.SkipPhases = []string{status.WaypointV2Design, status.WaypointV2Spec, status.WaypointV2Plan}
	for _, waypoint := range status.AllWaypointsV2Schema() {
		if !st.IsPhaseSkipped(waypoint) {
			st.UpdatePhase(waypoint, status.WaypointStatusV2Completed, status.OutcomeSuccess)
		}
	}
	st.UpdatePhase(status.WaypointV2Design, status.WaypointStatusV2InProgress, "")

	if allRequiredWaypointsComplete(st) {
		t.Fatal("active profile-skipped phase must keep a child project incomplete")
	}
}

// Tests for GetIncompleteChildren

func TestGetIncompleteChildren(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) string
		expectedCount int
		checkNames    []string
	}{
		{
			name: "no children",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", false)
				return tmpDir
			},
			expectedCount: 0,
		},
		{
			name: "all children complete",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", false)
				createChildProject(t, tmpDir, "child-a", "completed", true)
				createChildProject(t, tmpDir, "child-b", "completed", true)
				return tmpDir
			},
			expectedCount: 0,
		},
		{
			name: "all children incomplete",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", false)
				createChildProject(t, tmpDir, "child-a", "in-progress", false)
				createChildProject(t, tmpDir, "child-b", "in-progress", false)
				return tmpDir
			},
			expectedCount: 2,
			checkNames:    []string{"tasks/child-a", "tasks/child-b"},
		},
		{
			name: "mixed status",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", false)
				createChildProject(t, tmpDir, "child-a", "completed", true)
				createChildProject(t, tmpDir, "child-b", "in-progress", false)
				createChildProject(t, tmpDir, "child-c", "completed", true)
				return tmpDir
			},
			expectedCount: 1,
			checkNames:    []string{"tasks/child-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)

			incomplete, err := GetIncompleteChildren(dir)
			if err != nil {
				t.Fatalf("GetIncompleteChildren() error = %v", err)
			}

			if len(incomplete) != tt.expectedCount {
				t.Errorf("GetIncompleteChildren() returned %d children, want %d", len(incomplete), tt.expectedCount)
			}

			// Check expected names
			if tt.checkNames != nil {
				incompleteMap := make(map[string]bool)
				for _, child := range incomplete {
					incompleteMap[child] = true
				}

				for _, expected := range tt.checkNames {
					if !incompleteMap[expected] {
						t.Errorf("Expected incomplete child %q not found in results", expected)
					}
				}
			}
		})
	}
}

// Tests for CheckChildrenComplete

func TestCheckChildrenComplete(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
		errMsg  string
	}{
		{
			name: "no children - passes",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", false)
				return tmpDir
			},
			wantErr: false,
		},
		{
			name: "all children complete - passes",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", false)
				createChildProject(t, tmpDir, "child-a", "completed", true)
				createChildProject(t, tmpDir, "child-b", "completed", true)
				return tmpDir
			},
			wantErr: false,
		},
		{
			name: "some children incomplete - fails",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", false)
				createChildProject(t, tmpDir, "child-a", "completed", true)
				createChildProject(t, tmpDir, "child-b", "in-progress", false)
				return tmpDir
			},
			wantErr: true,
			errMsg:  "child projects not complete",
		},
		{
			name: "all children incomplete - fails",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, "in-progress", false)
				createChildProject(t, tmpDir, "child-a", "in-progress", false)
				createChildProject(t, tmpDir, "child-b", "in-progress", false)
				return tmpDir
			},
			wantErr: true,
			errMsg:  "child projects not complete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)

			err := CheckChildrenComplete(dir)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Error message %q does not contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// Integration tests

func TestCheckChildrenComplete_RecursiveValidation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create parent
	createStatusFile(t, tmpDir, "in-progress", false)

	// Create child
	childDir := createChildProject(t, tmpDir, "child", "completed", true)

	// Create grandchild (incomplete)
	createChildProject(t, childDir, "grandchild", "in-progress", false)

	// Child should not be considered complete because grandchild is incomplete
	complete, err := IsProjectComplete(childDir)
	if err != nil {
		t.Fatalf("IsProjectComplete() error = %v", err)
	}

	if complete {
		t.Error("Child should not be complete when grandchild is incomplete")
	}

	// Therefore parent's CheckChildrenComplete should fail
	err = CheckChildrenComplete(tmpDir)
	if err == nil {
		t.Error("Expected error (child incomplete due to grandchild), got nil")
	}
}

func TestCheckChildrenComplete_ThreeLevelNesting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create parent
	createStatusFile(t, tmpDir, "in-progress", false)

	// Create child
	childDir := createChildProject(t, tmpDir, "child", "completed", true)

	// Create grandchild (complete)
	createChildProject(t, childDir, "grandchild", "completed", true)

	// All should be complete
	err := CheckChildrenComplete(tmpDir)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestCheckChildrenComplete_MaxDepthExceeded(t *testing.T) {
	tmpDir := t.TempDir()
	createStatusFile(t, tmpDir, "in-progress", false)

	// Create excessive nesting (MaxNestingDepth + 1)
	current := tmpDir
	for i := 0; i <= status.MaxNestingDepth+1; i++ {
		current = createChildProject(t, current, "child", "completed", true)
	}

	// Should error on max depth
	err := CheckChildrenComplete(tmpDir)
	if err == nil {
		t.Error("Expected max depth error, got nil")
	}

	if !errors.Is(err, status.ErrMaxDepthExceeded) {
		t.Errorf("Expected ErrMaxDepthExceeded, got: %v", err)
	}
}

func TestCheckChildrenComplete_ErrorMessage(t *testing.T) {
	tmpDir := t.TempDir()
	createStatusFile(t, tmpDir, "in-progress", false)

	createChildProject(t, tmpDir, "child-a", "completed", true)
	createChildProject(t, tmpDir, "child-b", "in-progress", false)
	createChildProject(t, tmpDir, "child-c", "in-progress", false)

	err := CheckChildrenComplete(tmpDir)
	if err == nil {
		t.Fatal("Expected error, got nil")
		return
	}

	errMsg := err.Error()

	// Should mention both incomplete children
	if !strings.Contains(errMsg, "tasks/child-b") {
		t.Error("Error message should mention child-b")
	}

	if !strings.Contains(errMsg, "tasks/child-c") {
		t.Error("Error message should mention child-c")
	}

	// Should NOT mention complete child
	if strings.Contains(errMsg, "tasks/child-a") {
		t.Error("Error message should not mention child-a (complete)")
	}
}

// Edge case tests

func TestIsProjectComplete_EmptyStatusFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty STATUS file
	err := os.WriteFile(filepath.Join(tmpDir, status.StatusFilename), []byte(""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = IsProjectComplete(tmpDir)
	if err == nil {
		t.Error("Expected error for empty STATUS file")
	}
}

func TestIsProjectComplete_CorruptedStatusFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid YAML
	content := `this is not valid YAML: {{{`
	err := os.WriteFile(filepath.Join(tmpDir, status.StatusFilename), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = IsProjectComplete(tmpDir)
	if err == nil {
		t.Error("Expected error for corrupted STATUS file")
	}
}

func TestCheckChildrenComplete_PermissionDenied(t *testing.T) {
	tmpDir := t.TempDir()
	createStatusFile(t, tmpDir, "in-progress", false)

	// Create child
	childDir := createChildProject(t, tmpDir, "child", "completed", true)

	// Remove read permission from child's STATUS file
	statusPath := filepath.Join(childDir, status.StatusFilename)
	err := os.Chmod(statusPath, 0000)
	if err != nil {
		t.Skip("Cannot change permissions on this system")
	}
	defer os.Chmod(statusPath, 0644) // Cleanup

	// Should handle permission error gracefully
	err = CheckChildrenComplete(tmpDir)
	if err == nil {
		t.Error("Expected error when child STATUS is unreadable")
	}
}
