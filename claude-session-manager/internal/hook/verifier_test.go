package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/plugin"
)

// Mock plugin for testing
type mockTaskManager struct {
	tasks []plugin.Task
}

func (m *mockTaskManager) Metadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{Name: "mock", Version: "1.0.0"}
}

func (m *mockTaskManager) GetTasks(sessionDir string) ([]plugin.Task, error) {
	return m.tasks, nil
}

func (m *mockTaskManager) GetPhaseProgress(sessionDir string) ([]plugin.PhaseStats, error) {
	return nil, nil
}

func (m *mockTaskManager) SupportsSession(sessionDir string) bool {
	return true
}

func TestVerifyROADMAP_NoMismatches(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	roadmapPath := filepath.Join(tempDir, "ROADMAP.md")

	roadmapContent := `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | Task 1 | 1 day | ✅ COMPLETE |
| ` + "`oss-task-2`" + ` | Task 2 | 1 day | 📋 PLANNED |
`

	if err := os.WriteFile(roadmapPath, []byte(roadmapContent), 0644); err != nil {
		t.Fatalf("Failed to write ROADMAP: %v", err)
	}

	// Create mock task manager with matching tasks
	mockTM := &mockTaskManager{
		tasks: []plugin.Task{
			{
				ID:        "oss-task-1",
				Status:    "completed",
				Phase:     "phase-0",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			{
				ID:        "oss-task-2",
				Status:    "pending",
				Phase:     "phase-0",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}

	verifier := NewVerifier(mockTM)
	result, err := verifier.VerifyROADMAP(roadmapPath, tempDir)

	if err != nil {
		t.Fatalf("VerifyROADMAP() error: %v", err)
	}

	if !result.Passed {
		t.Errorf("VerifyROADMAP() failed, expected to pass")
	}

	if len(result.Mismatches) != 0 {
		t.Errorf("VerifyROADMAP() found %d mismatches, want 0", len(result.Mismatches))
	}
}

func TestVerifyROADMAP_StatusMismatch(t *testing.T) {
	tempDir := t.TempDir()
	roadmapPath := filepath.Join(tempDir, "ROADMAP.md")

	roadmapContent := `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | Task 1 | 1 day | ✅ COMPLETE |
`

	if err := os.WriteFile(roadmapPath, []byte(roadmapContent), 0644); err != nil {
		t.Fatalf("Failed to write ROADMAP: %v", err)
	}

	// Task manager shows task as "pending" but ROADMAP shows "completed"
	mockTM := &mockTaskManager{
		tasks: []plugin.Task{
			{
				ID:     "oss-task-1",
				Status: "pending", // Mismatch!
				Phase:  "phase-0",
			},
		},
	}

	verifier := NewVerifier(mockTM)
	result, err := verifier.VerifyROADMAP(roadmapPath, tempDir)

	if err != nil {
		t.Fatalf("VerifyROADMAP() error: %v", err)
	}

	if result.Passed {
		t.Errorf("VerifyROADMAP() passed, expected to fail (status mismatch)")
	}

	if len(result.Mismatches) != 1 {
		t.Fatalf("VerifyROADMAP() found %d mismatches, want 1", len(result.Mismatches))
	}

	mismatch := result.Mismatches[0]
	if mismatch.BeadID != "oss-task-1" {
		t.Errorf("Mismatch.BeadID = %q, want %q", mismatch.BeadID, "oss-task-1")
	}
	if mismatch.ROADMAPStatus != "completed" {
		t.Errorf("Mismatch.ROADMAPStatus = %q, want %q", mismatch.ROADMAPStatus, "completed")
	}
	if mismatch.TaskStatus != "pending" {
		t.Errorf("Mismatch.TaskStatus = %q, want %q", mismatch.TaskStatus, "pending")
	}
}

func TestVerifyROADMAP_TaskNotFound(t *testing.T) {
	tempDir := t.TempDir()
	roadmapPath := filepath.Join(tempDir, "ROADMAP.md")

	roadmapContent := `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | Task 1 | 1 day | ✅ COMPLETE |
`

	if err := os.WriteFile(roadmapPath, []byte(roadmapContent), 0644); err != nil {
		t.Fatalf("Failed to write ROADMAP: %v", err)
	}

	// Task manager has no tasks (task not found)
	mockTM := &mockTaskManager{
		tasks: []plugin.Task{}, // Empty!
	}

	verifier := NewVerifier(mockTM)
	result, err := verifier.VerifyROADMAP(roadmapPath, tempDir)

	if err != nil {
		t.Fatalf("VerifyROADMAP() error: %v", err)
	}

	if result.Passed {
		t.Errorf("VerifyROADMAP() passed, expected to fail (task not found)")
	}

	if len(result.Mismatches) != 1 {
		t.Fatalf("VerifyROADMAP() found %d mismatches, want 1", len(result.Mismatches))
	}

	mismatch := result.Mismatches[0]
	if mismatch.TaskStatus != "not_found" {
		t.Errorf("Mismatch.TaskStatus = %q, want %q", mismatch.TaskStatus, "not_found")
	}
}

func TestVerifyROADMAP_PendingTaskNotFound_OK(t *testing.T) {
	tempDir := t.TempDir()
	roadmapPath := filepath.Join(tempDir, "ROADMAP.md")

	roadmapContent := `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | Task 1 | 1 day | 📋 PLANNED |
`

	if err := os.WriteFile(roadmapPath, []byte(roadmapContent), 0644); err != nil {
		t.Fatalf("Failed to write ROADMAP: %v", err)
	}

	// Task manager has no tasks, but ROADMAP shows "pending" - this is OK
	mockTM := &mockTaskManager{
		tasks: []plugin.Task{},
	}

	verifier := NewVerifier(mockTM)
	result, err := verifier.VerifyROADMAP(roadmapPath, tempDir)

	if err != nil {
		t.Fatalf("VerifyROADMAP() error: %v", err)
	}

	if !result.Passed {
		t.Errorf("VerifyROADMAP() failed, expected to pass (pending tasks can be missing)")
	}

	if len(result.Mismatches) != 0 {
		t.Errorf("VerifyROADMAP() found %d mismatches, want 0", len(result.Mismatches))
	}
}

func TestVerifyROADMAP_MultipleMismatches(t *testing.T) {
	tempDir := t.TempDir()
	roadmapPath := filepath.Join(tempDir, "ROADMAP.md")

	roadmapContent := `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | Task 1 | 1 day | ✅ COMPLETE |
| ` + "`oss-task-2`" + ` | Task 2 | 1 day | 📋 PLANNED |
| ` + "`oss-task-3`" + ` | Task 3 | 1 day | ✅ COMPLETE |
`

	if err := os.WriteFile(roadmapPath, []byte(roadmapContent), 0644); err != nil {
		t.Fatalf("Failed to write ROADMAP: %v", err)
	}

	// Multiple mismatches
	mockTM := &mockTaskManager{
		tasks: []plugin.Task{
			{ID: "oss-task-1", Status: "in_progress"}, // Mismatch
			{ID: "oss-task-2", Status: "completed"},   // Mismatch
			// oss-task-3 not found - Mismatch
		},
	}

	verifier := NewVerifier(mockTM)
	result, err := verifier.VerifyROADMAP(roadmapPath, tempDir)

	if err != nil {
		t.Fatalf("VerifyROADMAP() error: %v", err)
	}

	if result.Passed {
		t.Errorf("VerifyROADMAP() passed, expected to fail")
	}

	if len(result.Mismatches) != 3 {
		t.Errorf("VerifyROADMAP() found %d mismatches, want 3", len(result.Mismatches))
	}
}

func TestFormatError_SingleMismatch(t *testing.T) {
	result := &VerificationResult{
		Passed: false,
		Mismatches: []Mismatch{
			{
				BeadID:        "oss-task-1",
				ROADMAPStatus: "completed",
				TaskStatus:    "pending",
				ROADMAPRaw:    "✅ COMPLETE",
				Line:          42,
			},
		},
	}

	output := FormatError(result, "beads")

	// Check for key components
	if !strings.Contains(output, "❌") {
		t.Error("Error message missing error emoji")
	}
	if !strings.Contains(output, "oss-task-1") {
		t.Error("Error message missing bead ID")
	}
	if !strings.Contains(output, "line 42") {
		t.Error("Error message missing line number")
	}
	if !strings.Contains(output, "bd close oss-task-1") {
		t.Error("Error message missing fix command for beads")
	}
	if !strings.Contains(output, "git commit --no-verify") {
		t.Error("Error message missing bypass option")
	}
	if !strings.Contains(output, "1 inconsistency") {
		t.Error("Error message should say '1 inconsistency' (singular)")
	}
}

func TestFormatError_MultipleMismatches(t *testing.T) {
	result := &VerificationResult{
		Passed: false,
		Mismatches: []Mismatch{
			{BeadID: "oss-task-1", ROADMAPStatus: "completed", TaskStatus: "pending", Line: 10},
			{BeadID: "oss-task-2", ROADMAPStatus: "completed", TaskStatus: "not_found", Line: 15},
		},
	}

	output := FormatError(result, "claude-tasks")

	if !strings.Contains(output, "2 inconsistencies") {
		t.Error("Error message should say '2 inconsistencies' (plural)")
	}
	if !strings.Contains(output, "oss-task-1") {
		t.Error("Error message missing first bead")
	}
	if !strings.Contains(output, "oss-task-2") {
		t.Error("Error message missing second bead")
	}
	if !strings.Contains(output, "TaskUpdate") {
		t.Error("Error message missing Claude tasks fix command")
	}
}

func TestFormatError_PassedResult(t *testing.T) {
	result := &VerificationResult{
		Passed:     true,
		Mismatches: []Mismatch{},
	}

	output := FormatError(result, "beads")

	if output != "" {
		t.Errorf("FormatError() for passed result = %q, want empty string", output)
	}
}

func TestFormatError_TaskNotFound(t *testing.T) {
	result := &VerificationResult{
		Passed: false,
		Mismatches: []Mismatch{
			{
				BeadID:        "oss-missing",
				ROADMAPStatus: "completed",
				TaskStatus:    "not_found",
				ROADMAPRaw:    "✅ COMPLETE",
				Line:          100,
			},
		},
	}

	output := FormatError(result, "beads")

	if !strings.Contains(output, "doesn't exist") {
		t.Error("Error message should mention task doesn't exist")
	}
}
