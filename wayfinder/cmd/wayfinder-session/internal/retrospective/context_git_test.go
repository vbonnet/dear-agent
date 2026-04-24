package retrospective

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestCaptureContext_GitSuccess tests successful git context capture
func TestCaptureContext_GitSuccess(t *testing.T) {
	// Create a real git repository for testing
	tmpDir := t.TempDir()

	// Initialize git repo
	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		t.Skipf("Skipping test: git not available or init failed: %v", err)
	}

	// Configure git (required for commit)
	configCmds := [][]string{
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, cmdArgs := range configCmds {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Skipf("Skipping test: git config failed: %v", err)
		}
	}

	// Create a test file and commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	addCmd := exec.Command("git", "add", "test.txt")
	addCmd.Dir = tmpDir
	if err := addCmd.Run(); err != nil {
		t.Skipf("Skipping test: git add failed: %v", err)
	}

	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = tmpDir
	if err := commitCmd.Run(); err != nil {
		t.Skipf("Skipping test: git commit failed: %v", err)
	}

	// Create minimal status
	st := &status.Status{
		CurrentPhase: "S7",
		Phases: []status.Phase{
			{Name: "W0", Status: status.PhaseStatusCompleted},
			{Name: "S7", Status: status.PhaseStatusInProgress},
		},
	}

	// Capture context (should succeed with real git repo)
	snapshot := CaptureContext(tmpDir, st)

	// Verify git context was captured
	if snapshot.Git.Branch == "" {
		t.Errorf("Expected branch name, got empty string")
	}

	if snapshot.Git.Commit == "" {
		t.Errorf("Expected commit SHA, got empty string")
	}

	// Should not have error since repo is valid
	if snapshot.Git.Error != "" {
		t.Errorf("Expected no git error, got: %s", snapshot.Git.Error)
	}

	// Verify phase state
	if snapshot.PhaseState.CurrentPhase != "S7" {
		t.Errorf("Expected current phase 'S7', got: %s", snapshot.PhaseState.CurrentPhase)
	}
}

// TestCaptureContext_GitUncommittedChanges tests detection of uncommitted changes
func TestCaptureContext_GitUncommittedChanges(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo
	initCmd := exec.Command("git", "init")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		t.Skipf("Skipping test: git not available: %v", err)
	}

	// Configure git
	configCmds := [][]string{
		{"git", "config", "user.email", "test@example.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, cmdArgs := range configCmds {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Skipf("Skipping test: git config failed: %v", err)
		}
	}

	// Create and commit initial file
	testFile := filepath.Join(tmpDir, "committed.txt")
	if err := os.WriteFile(testFile, []byte("committed"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	addCmd := exec.Command("git", "add", "committed.txt")
	addCmd.Dir = tmpDir
	if err := addCmd.Run(); err != nil {
		t.Skipf("Skipping test: git add failed: %v", err)
	}

	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = tmpDir
	if err := commitCmd.Run(); err != nil {
		t.Skipf("Skipping test: git commit failed: %v", err)
	}

	// Create uncommitted change
	uncommittedFile := filepath.Join(tmpDir, "uncommitted.txt")
	if err := os.WriteFile(uncommittedFile, []byte("uncommitted"), 0644); err != nil {
		t.Fatalf("Failed to create uncommitted file: %v", err)
	}

	// Capture context
	st := &status.Status{CurrentPhase: "S7"}
	snapshot := CaptureContext(tmpDir, st)

	// Should detect uncommitted changes
	if !snapshot.Git.UncommittedChanges {
		t.Errorf("Expected uncommitted changes to be detected, got false")
	}
}

// TestCaptureContext_Concurrent tests parallel context capture
func TestCaptureContext_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()

	st := &status.Status{
		CurrentPhase: "D3",
		Phases: []status.Phase{
			{Name: "W0", Status: status.PhaseStatusCompleted},
			{Name: "D1", Status: status.PhaseStatusCompleted},
			{Name: "D2", Status: status.PhaseStatusCompleted},
			{Name: "D3", Status: status.PhaseStatusInProgress},
		},
	}

	// Call CaptureContext multiple times concurrently (stress test)
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			snapshot := CaptureContext(tmpDir, st)
			// Should not panic
			if snapshot.PhaseState.CurrentPhase != "D3" {
				t.Errorf("Concurrent capture corrupted phase state")
			}
			done <- true
		}()
	}

	// Wait for all goroutines with timeout
	timeout := time.After(2 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-done:
			// Success
		case <-timeout:
			t.Fatal("Concurrent capture timed out")
		}
	}
}

// TestCaptureDeliverables_RealFiles tests deliverable detection with actual files
func TestCaptureDeliverables_RealFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create phase deliverable files
	deliverableFiles := []string{
		"W0-PROJECT-CHARTER.md",
		"D1-problem-validation.md",
		"D2-user-research.md",
		"S6-design.md",
		"S7-plan.md",
		"README.md", // Not a phase deliverable
	}

	for _, fileName := range deliverableFiles {
		filePath := filepath.Join(tmpDir, fileName)
		if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", fileName, err)
		}
	}

	// Capture deliverables
	deliverables, err := captureDeliverables(tmpDir)
	if err != nil {
		t.Fatalf("captureDeliverables failed: %v", err)
	}

	// Should find 5 phase deliverables (not README.md)
	if len(deliverables) != 5 {
		t.Errorf("Expected 5 deliverables, got %d: %v", len(deliverables), deliverables)
	}

	// Verify specific deliverables
	expectedDeliverables := map[string]bool{
		"W0-PROJECT-CHARTER.md":    true,
		"D1-problem-validation.md": true,
		"D2-user-research.md":      true,
		"S6-design.md":             true,
		"S7-plan.md":               true,
	}

	for _, deliverable := range deliverables {
		if !expectedDeliverables[deliverable] {
			t.Errorf("Unexpected deliverable: %s", deliverable)
		}
	}
}
