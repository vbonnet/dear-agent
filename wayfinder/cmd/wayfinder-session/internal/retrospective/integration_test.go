package retrospective

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Integration tests for end-to-end retrospective logging workflows
// These tests exercise the full rewind retrospective logging flow

// TestIntegration_Magnitude0_NoLogging tests that magnitude 0 rewinds skip logging
func TestIntegration_Magnitude0_NoLogging(t *testing.T) {
	tmpDir := t.TempDir()

	// Create WAYFINDER-STATUS.md
	createStatusFile(t, tmpDir, "S7")

	// Rewind S7→S7 (magnitude 0)
	flags := RewindFlags{NoPrompt: true}
	err := LogRewindEvent(tmpDir, "S7", "S7", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify S11 was NOT created (magnitude 0 skips logging)
	s11Path := filepath.Join(tmpDir, S11Filename)
	if _, err := os.Stat(s11Path); !os.IsNotExist(err) {
		t.Errorf("S11 file should not exist for magnitude 0 rewind")
	}

	// Verify HISTORY was NOT created
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")
	if _, err := os.Stat(historyPath); !os.IsNotExist(err) {
		t.Errorf("HISTORY file should not exist for magnitude 0 rewind")
	}
}

// TestIntegration_Magnitude1_WithFlags tests rewind with pre-provided flags
func TestIntegration_Magnitude1_WithFlags(t *testing.T) {
	tmpDir := t.TempDir()

	// Create WAYFINDER-STATUS.md
	createStatusFile(t, tmpDir, "S7")

	// Rewind S7→S6 with --reason and --learnings flags
	flags := RewindFlags{
		Reason:    "Design was overcomplicated",
		Learnings: "Simpler approaches work better",
	}

	err := LogRewindEvent(tmpDir, "S7", "S6", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify S11 exists and contains expected content
	s11Path := filepath.Join(tmpDir, S11Filename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read S11 file: %v", err)
	}

	s11Str := string(s11Content)

	// Validate S11 content
	if !strings.Contains(s11Str, "## Rewind: S7 → S6 (magnitude 1)") {
		t.Errorf("S11 missing rewind header")
	}
	if !strings.Contains(s11Str, "Design was overcomplicated") {
		t.Errorf("S11 missing reason")
	}
	if !strings.Contains(s11Str, "Simpler approaches work better") {
		t.Errorf("S11 missing learnings")
	}
	if !strings.Contains(s11Str, "**Context**:") {
		t.Errorf("S11 missing context section")
	}

	// Verify HISTORY exists
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")
	historyContent, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("Failed to read HISTORY file: %v", err)
	}

	historyStr := string(historyContent)
	if !strings.Contains(historyStr, "rewind.logged") {
		t.Errorf("HISTORY missing rewind.logged event")
	}
	if !strings.Contains(historyStr, "S6") {
		t.Errorf("HISTORY missing target phase S6")
	}
}

// TestIntegration_Magnitude3_LargeRewind tests large magnitude rewind
func TestIntegration_Magnitude3_LargeRewind(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "S7")

	// Rewind S7→D4 (magnitude 4)
	flags := RewindFlags{
		NoPrompt:  true,
		Reason:    "Major approach change needed",
		Learnings: "Requirements analysis was incomplete",
	}

	err := LogRewindEvent(tmpDir, "S7", "D4", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify S11
	s11Path := filepath.Join(tmpDir, S11Filename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read S11 file: %v", err)
	}

	s11Str := string(s11Content)

	// Should show correct magnitude
	if !strings.Contains(s11Str, "magnitude 4") {
		t.Errorf("S11 missing or wrong magnitude (expected 4)")
	}
}

// TestIntegration_ParallelDualLogging tests that both HISTORY and S11 are updated
func TestIntegration_ParallelDualLogging(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "S8")

	// Perform rewind
	flags := RewindFlags{
		Reason: "Test parallel logging",
	}

	err := LogRewindEvent(tmpDir, "S8", "S5", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify BOTH files exist
	s11Path := filepath.Join(tmpDir, S11Filename)
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")

	if _, err := os.Stat(s11Path); os.IsNotExist(err) {
		t.Errorf("S11 file was not created")
	}

	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		t.Errorf("HISTORY file was not created")
	}

	// Verify S11 content
	s11Content, _ := os.ReadFile(s11Path)
	if !strings.Contains(string(s11Content), "Test parallel logging") {
		t.Errorf("S11 missing expected content")
	}

	// Verify HISTORY content
	historyContent, _ := os.ReadFile(historyPath)
	if !strings.Contains(string(historyContent), "rewind.logged") {
		t.Errorf("HISTORY missing event")
	}
}

// TestIntegration_FailGracefully tests error handling (fail-gracefully design)
func TestIntegration_FailGracefully(t *testing.T) {
	tmpDir := t.TempDir()

	// Don't create STATUS file - should fail gracefully
	flags := RewindFlags{
		NoPrompt: true,
		Reason:   "Test error handling",
	}

	// Should NOT panic, should return nil (fail-gracefully)
	err := LogRewindEvent(tmpDir, "S7", "S5", flags)
	if err != nil {
		t.Errorf("LogRewindEvent should return nil on errors (fail-gracefully), got: %v", err)
	}

	// Should not create files if error occurred
	s11Path := filepath.Join(tmpDir, S11Filename)
	if info, err := os.Stat(s11Path); err == nil {
		// If file exists, it should be empty or minimal
		content, _ := os.ReadFile(s11Path)
		if len(content) > 100 {
			t.Errorf("S11 file should not have full content on error, got %d bytes", len(content))
		}
		_ = info
	}
}

// TestIntegration_NonInteractiveEnvironment tests --no-prompt flag behavior
func TestIntegration_NonInteractiveEnvironment(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "S6")

	// Simulate CI/CD with --no-prompt (no reason provided)
	flags := RewindFlags{
		NoPrompt: true,
		// No reason or learnings provided
	}

	err := LogRewindEvent(tmpDir, "S6", "S4", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Should create S11 even without reason
	s11Path := filepath.Join(tmpDir, S11Filename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read S11 file: %v", err)
	}

	s11Str := string(s11Content)

	// Should contain rewind header
	if !strings.Contains(s11Str, "## Rewind: S6 → S4") {
		t.Errorf("S11 missing rewind header")
	}

	// Should indicate no reason provided
	if !strings.Contains(s11Str, "_(not provided)_") {
		t.Errorf("S11 should indicate reason not provided")
	}
}

// TestIntegration_ContextCaptureCompleteness tests that context snapshot is complete
func TestIntegration_ContextCaptureCompleteness(t *testing.T) {
	tmpDir := t.TempDir()

	// Create git repository
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Skipf("Skipping test: git not available")
	}

	// Configure git
	exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run()

	// Create deliverable files
	deliverables := []string{
		"W0-PROJECT-CHARTER.md",
		"D1-problem-validation.md",
		"S6-design.md",
	}
	for _, deliverable := range deliverables {
		path := filepath.Join(tmpDir, deliverable)
		os.WriteFile(path, []byte("content"), 0644)
	}

	// Commit files
	exec.Command("git", "-C", tmpDir, "add", ".").Run()
	exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run()

	// Create STATUS file
	createStatusFile(t, tmpDir, "S7")

	// Perform rewind
	flags := RewindFlags{
		Reason: "Test context capture",
	}

	err := LogRewindEvent(tmpDir, "S7", "S6", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify S11 contains context
	s11Path := filepath.Join(tmpDir, S11Filename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read S11 file: %v", err)
	}

	s11Str := string(s11Content)

	// Should contain git context
	if !strings.Contains(s11Str, "Git:") {
		t.Errorf("S11 missing git context")
	}

	// Should contain deliverables
	if !strings.Contains(s11Str, "Deliverables:") {
		t.Errorf("S11 missing deliverables section")
	}

	// Should list some deliverables
	hasDeliverables := false
	for _, d := range deliverables {
		if strings.Contains(s11Str, d) {
			hasDeliverables = true
			break
		}
	}
	if !hasDeliverables {
		t.Errorf("S11 missing deliverable files")
	}

	// Should contain completed phases
	if !strings.Contains(s11Str, "Completed phases:") {
		t.Errorf("S11 missing completed phases section")
	}
}

// TestIntegration_S11MarkdownFormat tests S11 markdown is human-readable
func TestIntegration_S11MarkdownFormat(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "D3")

	// Create rewind with all fields
	flags := RewindFlags{
		Reason:    "Comprehensive formatting test",
		Learnings: "Multiple key learnings from this rewind",
	}

	err := LogRewindEvent(tmpDir, "D3", "D1", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Read S11
	s11Path := filepath.Join(tmpDir, S11Filename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read S11 file: %v", err)
	}

	s11Str := string(s11Content)

	// Validate markdown structure
	requiredSections := []string{
		"## Rewind:",          // Header
		"**Timestamp**:",      // Timestamp
		"**Reason**:",         // Reason
		"**Learnings**:",      // Learnings
		"**Context**:",        // Context section
		"- Git:",              // Git info
		"- Deliverables:",     // Deliverables
		"- Completed phases:", // Phases
		"magnitude",           // Magnitude mentioned
		"---",                 // Separator
	}

	for _, section := range requiredSections {
		if !strings.Contains(s11Str, section) {
			t.Errorf("S11 missing required section: %s", section)
		}
	}

	// Validate ISO8601 timestamp format (YYYY-MM-DDTHH:MM:SSZ)
	if !strings.Contains(s11Str, "T") || !strings.Contains(s11Str, "Z") {
		t.Errorf("S11 timestamp not in ISO8601 format")
	}
}

// Helper function to create WAYFINDER-STATUS.md file
func createStatusFile(t *testing.T, dir string, currentPhase string) {
	t.Helper()

	statusContent := `---
session_id: test-session-integration
current_phase: ` + currentPhase + `
phases:
  - name: W0
    status: completed
    started_at: "2024-01-01T10:00:00Z"
    completed_at: "2024-01-01T11:00:00Z"
  - name: D1
    status: completed
    started_at: "2024-01-01T12:00:00Z"
    completed_at: "2024-01-01T13:00:00Z"
  - name: D2
    status: completed
  - name: D3
    status: completed
  - name: D4
    status: completed
  - name: S4
    status: completed
  - name: S5
    status: completed
  - name: S6
    status: completed
  - name: S7
    status: in_progress
    started_at: "2024-01-07T10:00:00Z"
---
`

	statusPath := filepath.Join(dir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte(statusContent), 0644); err != nil {
		t.Fatalf("Failed to create STATUS file: %v", err)
	}
}
