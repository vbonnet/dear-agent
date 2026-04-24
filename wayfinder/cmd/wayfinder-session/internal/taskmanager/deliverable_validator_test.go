package taskmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestValidateDeliverables_AllExist(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	for _, name := range []string{"feature.go", "feature_test.go"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("package main"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	err := ValidateDeliverables(tmpDir, []string{"feature.go", "feature_test.go"})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateDeliverables_SomeMissing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only one file
	if err := os.WriteFile(filepath.Join(tmpDir, "exists.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := ValidateDeliverables(tmpDir, []string{"exists.go", "missing.go", "also_missing.go"})
	if err == nil {
		t.Fatal("expected error for missing files, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "missing.go") {
		t.Errorf("expected error to mention missing.go, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "also_missing.go") {
		t.Errorf("expected error to mention also_missing.go, got: %s", errMsg)
	}
	if strings.Contains(errMsg, "exists.go") {
		t.Errorf("error should not mention exists.go, got: %s", errMsg)
	}
}

func TestValidateDeliverables_EmptyList(t *testing.T) {
	err := ValidateDeliverables("/some/root", []string{})
	if err != nil {
		t.Errorf("expected nil for empty list, got: %v", err)
	}

	err = ValidateDeliverables("/some/root", nil)
	if err != nil {
		t.Errorf("expected nil for nil list, got: %v", err)
	}
}

func TestValidateDeliverables_AbsolutePaths(t *testing.T) {
	tmpDir := t.TempDir()

	absFile := filepath.Join(tmpDir, "abs.go")
	if err := os.WriteFile(absFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Absolute path that exists should pass
	err := ValidateDeliverables(tmpDir, []string{absFile})
	if err != nil {
		t.Errorf("expected no error for existing absolute path, got: %v", err)
	}

	// Absolute path that doesn't exist should fail
	err = ValidateDeliverables(tmpDir, []string{"/nonexistent/path/file.go"})
	if err == nil {
		t.Error("expected error for non-existent absolute path, got nil")
	}
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name     string
		root     string
		path     string
		expected string
	}{
		{
			name:     "relative path",
			root:     "/repo",
			path:     "src/main.go",
			expected: "/repo/src/main.go",
		},
		{
			name:     "absolute path",
			root:     "/repo",
			path:     "/absolute/file.go",
			expected: "/absolute/file.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolvePath(tt.root, tt.path)
			if result != tt.expected {
				t.Errorf("resolvePath(%q, %q) = %q, want %q", tt.root, tt.path, result, tt.expected)
			}
		})
	}
}

// createTestStatusFileWithRepoRoot creates a test status file with explicit repo root
func createTestStatusFileWithRepoRoot(t *testing.T) (string, *TaskManager) {
	t.Helper()

	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "WAYFINDER-STATUS.md")

	st := status.NewStatusV2("Test Project", status.ProjectTypeFeature, status.RiskLevelM)
	if err := status.WriteV2(st, statusFile); err != nil {
		t.Fatalf("failed to create test status file: %v", err)
	}

	return tempDir, NewWithRepoRoot(tempDir, tempDir)
}

func TestAddTask_BeadWithInvalidDeliverables(t *testing.T) {
	_, tm := createTestStatusFileWithRepoRoot(t)

	_, err := tm.AddTask("S8", "Task with bad deliverables", &TaskOptions{
		BeadID:       "bd-123",
		Deliverables: []string{"nonexistent.go", "also_missing.go"},
	})
	if err == nil {
		t.Fatal("expected error for non-existent deliverables with bead, got nil")
	}
	if !strings.Contains(err.Error(), "deliverable validation failed") {
		t.Errorf("expected 'deliverable validation failed' in error, got: %v", err)
	}
}

func TestAddTask_DeliverablesWithoutBead(t *testing.T) {
	_, tm := createTestStatusFileWithRepoRoot(t)

	// Deliverables without BeadID should skip validation (planned files)
	task, err := tm.AddTask("S8", "Planned task", &TaskOptions{
		Deliverables: []string{"planned_file.go", "future_test.go"},
	})
	if err != nil {
		t.Fatalf("expected no error for deliverables without bead, got: %v", err)
	}
	if len(task.Deliverables) != 2 {
		t.Errorf("expected 2 deliverables, got %d", len(task.Deliverables))
	}
}

func TestUpdateTask_BeadWithInvalidDeliverables(t *testing.T) {
	_, tm := createTestStatusFileWithRepoRoot(t)

	// Create a task first
	task, err := tm.AddTask("S8", "Test task", nil)
	if err != nil {
		t.Fatalf("failed to add task: %v", err)
	}

	// Update with BeadID + non-existent deliverables should fail
	_, err = tm.UpdateTask(task.ID, &UpdateOptions{
		BeadID:       "bd-456",
		Deliverables: []string{"ghost_file.go"},
	})
	if err == nil {
		t.Fatal("expected error for non-existent deliverables on update with bead, got nil")
	}
	if !strings.Contains(err.Error(), "deliverable validation failed") {
		t.Errorf("expected 'deliverable validation failed' in error, got: %v", err)
	}
}

func TestAddTask_BeadWithValidDeliverables(t *testing.T) {
	tmpDir, tm := createTestStatusFileWithRepoRoot(t)

	// Create real files
	if err := os.WriteFile(filepath.Join(tmpDir, "real.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	task, err := tm.AddTask("S8", "Task with valid deliverables", &TaskOptions{
		BeadID:       "bd-789",
		Deliverables: []string{"real.go"},
	})
	if err != nil {
		t.Fatalf("expected no error for valid deliverables with bead, got: %v", err)
	}
	if task.BeadID != "bd-789" {
		t.Errorf("expected bead ID bd-789, got %s", task.BeadID)
	}
}
