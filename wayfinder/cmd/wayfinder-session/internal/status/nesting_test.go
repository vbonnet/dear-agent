package status

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test helpers

func createStatusFile(t *testing.T, dir string, status string) {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Second)
	projectStatus := NewStatusV2(filepath.Base(dir), ProjectTypeFeature, RiskLevelM)
	projectStatus.Status = status
	projectStatus.CreatedAt = now
	projectStatus.UpdatedAt = now
	projectStatus.WaypointHistory = []WaypointHistory{{
		Name:      WaypointV2Charter,
		Status:    WaypointStatusV2InProgress,
		StartedAt: now,
	}}
	if status == StatusV2Completed {
		projectStatus.CurrentWaypoint = WaypointV2Retro
		projectStatus.CompletionDate = &now
		projectStatus.WaypointHistory = completedWaypointHistory(now)
	}
	if err := WriteV2ToDir(projectStatus, dir); err != nil {
		t.Fatal(err)
	}
}

func completedWaypointHistory(now time.Time) []WaypointHistory {
	history := make([]WaypointHistory, 0, len(AllWaypointsV2Schema()))
	for _, waypoint := range AllWaypointsV2Schema() {
		history = append(history, WaypointHistory{
			Name:        waypoint,
			Status:      WaypointStatusV2Completed,
			StartedAt:   now,
			CompletedAt: &now,
		})
	}
	return history
}

func createChildProject(t *testing.T, parentDir, childName, status string) string {
	t.Helper()

	tasksDir := filepath.Join(parentDir, TasksDirectoryName)
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatal(err)
	}

	childDir := filepath.Join(tasksDir, childName)
	if err := os.Mkdir(childDir, 0755); err != nil {
		t.Fatal(err)
	}

	createStatusFile(t, childDir, status)

	return childDir
}

// Tests for HasChildren

func TestHasChildren(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, tmpDir string)
		expected bool
	}{
		{
			name: "directory with tasks containing projects",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
				createChildProject(t, tmpDir, "child-a", StatusV2InProgress)
			},
			expected: true,
		},
		{
			name: "directory with tasks but no STATUS files",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
				tasksDir := filepath.Join(tmpDir, TasksDirectoryName)
				os.MkdirAll(filepath.Join(tasksDir, "child-a"), 0755)
				// No STATUS file created
			},
			expected: false,
		},
		{
			name: "directory without tasks",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
			},
			expected: false,
		},
		{
			name: "empty directory",
			setup: func(t *testing.T, tmpDir string) {
				// No setup
			},
			expected: false,
		},
		{
			name: "multiple children",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
				createChildProject(t, tmpDir, "child-a", StatusV2Completed)
				createChildProject(t, tmpDir, "child-b", StatusV2InProgress)
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(t, tmpDir)

			result := HasChildren(tmpDir)
			if result != tt.expected {
				t.Errorf("HasChildren() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Tests for ListChildren

func TestListChildren(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, tmpDir string)
		expectedCount int
		expectedNames []string
	}{
		{
			name: "multiple children",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
				createChildProject(t, tmpDir, "child-a", StatusV2Completed)
				createChildProject(t, tmpDir, "child-b", StatusV2InProgress)
				createChildProject(t, tmpDir, "child-c", StatusV2Completed)
			},
			expectedCount: 3,
			expectedNames: []string{"child-a", "child-b", "child-c"},
		},
		{
			name: "single child",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
				createChildProject(t, tmpDir, "only-child", StatusV2InProgress)
			},
			expectedCount: 1,
			expectedNames: []string{"only-child"},
		},
		{
			name: "no children",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
			},
			expectedCount: 0,
			expectedNames: []string{},
		},
		{
			name: "tasks dir exists but empty",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
				os.Mkdir(filepath.Join(tmpDir, TasksDirectoryName), 0755)
			},
			expectedCount: 0,
			expectedNames: []string{},
		},
		{
			name: "children without STATUS files excluded",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
				createChildProject(t, tmpDir, "valid-child", StatusV2InProgress)

				// Create child without STATUS file
				tasksDir := filepath.Join(tmpDir, TasksDirectoryName)
				os.MkdirAll(filepath.Join(tasksDir, "invalid-child"), 0755)
			},
			expectedCount: 1,
			expectedNames: []string{"valid-child"},
		},
		{
			name: "non-directory entries ignored",
			setup: func(t *testing.T, tmpDir string) {
				createStatusFile(t, tmpDir, StatusV2InProgress)
				createChildProject(t, tmpDir, "valid-child", StatusV2InProgress)

				// Create file in tasks/ directory
				tasksDir := filepath.Join(tmpDir, TasksDirectoryName)
				os.WriteFile(filepath.Join(tasksDir, "README.md"), []byte("test"), 0644)
			},
			expectedCount: 1,
			expectedNames: []string{"valid-child"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(t, tmpDir)

			children, err := ListChildren(tmpDir)
			if err != nil {
				t.Fatalf("ListChildren() error = %v", err)
			}

			if len(children) != tt.expectedCount {
				t.Errorf("ListChildren() returned %d children, want %d", len(children), tt.expectedCount)
			}

			// Check expected names
			childMap := make(map[string]bool)
			for _, child := range children {
				childMap[child] = true
			}

			for _, expected := range tt.expectedNames {
				if !childMap[expected] {
					t.Errorf("Expected child %q not found in results", expected)
				}
			}
		})
	}
}

func TestListChildren_SymlinkRejection(t *testing.T) {
	tmpDir := t.TempDir()
	createStatusFile(t, tmpDir, StatusV2InProgress)

	tasksDir := filepath.Join(tmpDir, TasksDirectoryName)
	os.Mkdir(tasksDir, 0755)

	// Create valid child
	createChildProject(t, tmpDir, "valid-child", StatusV2InProgress)

	// Create symlink in tasks/
	symlinkPath := filepath.Join(tasksDir, "symlink-child")
	targetPath := filepath.Join(tmpDir, "external")
	os.Mkdir(targetPath, 0755)
	createStatusFile(t, targetPath, StatusV2InProgress)

	err := os.Symlink(targetPath, symlinkPath)
	if err != nil {
		t.Skip("Cannot create symlinks on this system")
	}

	children, err := ListChildren(tmpDir)
	if err != nil {
		t.Fatalf("ListChildren() error = %v", err)
	}

	// Should only have valid-child, not symlink
	if len(children) != 1 {
		t.Errorf("ListChildren() returned %d children, want 1 (symlinks should be rejected)", len(children))
	}

	if children[0] != "valid-child" {
		t.Errorf("Expected only valid-child, got %v", children)
	}
}

// Tests for HasParent

func TestHasParent(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string // Returns directory to test
		expected bool
	}{
		{
			name: "project in tasks directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				return createChildProject(t, tmpDir, "child", StatusV2InProgress)
			},
			expected: true,
		},
		{
			name: "top-level project",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				return tmpDir
			},
			expected: false,
		},
		{
			name: "deep nesting (grandchild has parent)",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				childDir := createChildProject(t, tmpDir, "child", StatusV2InProgress)
				return createChildProject(t, childDir, "grandchild", StatusV2InProgress)
			},
			expected: true,
		},
		{
			name: "directory named tasks but no parent STATUS",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				// Create parent without STATUS file
				parentDir := filepath.Join(tmpDir, "parent")
				os.Mkdir(parentDir, 0755)

				tasksDir := filepath.Join(parentDir, TasksDirectoryName)
				os.Mkdir(tasksDir, 0755)

				childDir := filepath.Join(tasksDir, "child")
				os.Mkdir(childDir, 0755)
				createStatusFile(t, childDir, StatusV2InProgress)

				return childDir
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)

			result := HasParent(dir)
			if result != tt.expected {
				t.Errorf("HasParent(%q) = %v, want %v", dir, result, tt.expected)
			}
		})
	}
}

// Tests for GetParentPath

func TestGetParentPath(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T) (testDir string, expectedParent string)
		expectEmpty    bool
		validateParent bool
	}{
		{
			name: "child project returns parent path",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				childDir := createChildProject(t, tmpDir, "child", StatusV2InProgress)
				return childDir, tmpDir
			},
			expectEmpty:    false,
			validateParent: true,
		},
		{
			name: "top-level returns empty string",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				return tmpDir, ""
			},
			expectEmpty:    true,
			validateParent: false,
		},
		{
			name: "grandchild returns child's path",
			setup: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				childDir := createChildProject(t, tmpDir, "child", StatusV2InProgress)
				grandchildDir := createChildProject(t, childDir, "grandchild", StatusV2InProgress)
				return grandchildDir, childDir
			},
			expectEmpty:    false,
			validateParent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir, expectedParent := tt.setup(t)

			result := GetParentPath(testDir)

			if tt.expectEmpty {
				if result != "" {
					t.Errorf("GetParentPath() = %q, want empty string", result)
				}
			} else {
				if result == "" {
					t.Error("GetParentPath() returned empty string, expected parent path")
				}

				if tt.validateParent && result != expectedParent {
					t.Errorf("GetParentPath() = %q, want %q", result, expectedParent)
				}
			}
		})
	}
}

// Tests for IsChildProject

func TestIsChildProject(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "absolute path with tasks parent",
			path:     "/tmp/test/wf/parent/tasks/child",
			expected: true,
		},
		{
			name:     "relative path with tasks parent",
			path:     "../tasks/myproject",
			expected: true,
		},
		{
			name:     "standalone project",
			path:     "/tmp/test/wf/standalone",
			expected: false,
		},
		{
			name:     "project with tasks in name but not parent",
			path:     "/tmp/test/wf/my-tasks-project",
			expected: false,
		},
		{
			name:     "current directory",
			path:     ".",
			expected: false,
		},
		{
			name:     "tasks as grandparent",
			path:     "/tmp/test/wf/parent/tasks/child/subdir",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsChildProject(tt.path)
			if result != tt.expected {
				t.Errorf("IsChildProject(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

// Tests for GetNestingLevel

func TestGetNestingLevel(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) string
		expectedLevel int
	}{
		{
			name: "top-level project",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				return tmpDir
			},
			expectedLevel: 0,
		},
		{
			name: "child project (level 1)",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				return createChildProject(t, tmpDir, "child", StatusV2InProgress)
			},
			expectedLevel: 1,
		},
		{
			name: "grandchild project (level 2)",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				childDir := createChildProject(t, tmpDir, "child", StatusV2InProgress)
				return createChildProject(t, childDir, "grandchild", StatusV2InProgress)
			},
			expectedLevel: 2,
		},
		{
			name: "three levels deep",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				createStatusFile(t, tmpDir, StatusV2InProgress)
				childDir := createChildProject(t, tmpDir, "child", StatusV2InProgress)
				grandchildDir := createChildProject(t, childDir, "grandchild", StatusV2InProgress)
				return createChildProject(t, grandchildDir, "greatgrandchild", StatusV2InProgress)
			},
			expectedLevel: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)

			level := GetNestingLevel(dir)
			if level != tt.expectedLevel {
				t.Errorf("GetNestingLevel() = %d, want %d", level, tt.expectedLevel)
			}
		})
	}
}

// Tests for ValidateNestingDepth

func TestValidateNestingDepth(t *testing.T) {
	t.Run("within limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		createStatusFile(t, tmpDir, StatusV2InProgress)

		// Create 5 levels (well within limit)
		current := tmpDir
		for i := 0; i < 5; i++ {
			current = createChildProject(t, current, "child", StatusV2InProgress)
		}

		err := ValidateNestingDepth(current)
		if err != nil {
			t.Errorf("ValidateNestingDepth() error = %v, want nil", err)
		}
	})

	t.Run("at limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		createStatusFile(t, tmpDir, StatusV2InProgress)

		// Create exactly MaxNestingDepth levels
		current := tmpDir
		for i := 0; i < MaxNestingDepth; i++ {
			current = createChildProject(t, current, "child", StatusV2InProgress)
		}

		err := ValidateNestingDepth(current)
		if err != nil {
			t.Errorf("ValidateNestingDepth() at limit error = %v, want nil", err)
		}
	})

	t.Run("exceeds limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		createStatusFile(t, tmpDir, StatusV2InProgress)

		// Create MaxNestingDepth + 1 levels
		current := tmpDir
		for i := 0; i < MaxNestingDepth+1; i++ {
			current = createChildProject(t, current, "child", StatusV2InProgress)
		}

		err := ValidateNestingDepth(current)
		if !errors.Is(err, ErrMaxDepthExceeded) {
			t.Errorf("ValidateNestingDepth() error = %v, want %v", err, ErrMaxDepthExceeded)
		}
	})
}

// Edge case tests

func TestHasChildren_PermissionDenied(t *testing.T) {
	tmpDir := t.TempDir()
	createStatusFile(t, tmpDir, StatusV2InProgress)

	tasksDir := filepath.Join(tmpDir, TasksDirectoryName)
	os.Mkdir(tasksDir, 0755)

	// Remove read permission
	err := os.Chmod(tasksDir, 0000)
	if err != nil {
		t.Skip("Cannot change permissions on this system")
	}
	defer os.Chmod(tasksDir, 0755) // Cleanup

	// Should return false, not panic
	result := HasChildren(tmpDir)
	if result {
		t.Error("HasChildren() should return false when cannot read tasks/ directory")
	}
}

func TestListChildren_PermissionDenied(t *testing.T) {
	tmpDir := t.TempDir()
	createStatusFile(t, tmpDir, StatusV2InProgress)

	tasksDir := filepath.Join(tmpDir, TasksDirectoryName)
	os.Mkdir(tasksDir, 0755)

	// Remove read permission
	err := os.Chmod(tasksDir, 0000)
	if err != nil {
		t.Skip("Cannot change permissions on this system")
	}
	defer os.Chmod(tasksDir, 0755) // Cleanup

	// Should return error
	_, err = ListChildren(tmpDir)
	if err == nil {
		t.Error("ListChildren() should return error when cannot read tasks/ directory")
	}
}
