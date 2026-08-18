package retrospective

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
	wayfinderstatus "github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// Integration tests for end-to-end retrospective logging workflows
// These tests exercise the full rewind retrospective logging flow

// TestIntegration_Magnitude0_LogsSamePhaseReplay proves that an accepted
// same-phase rewind leaves complete, exactly-once trace evidence.
func TestIntegration_Magnitude0_LogsSamePhaseReplay(t *testing.T) {
	tmpDir := t.TempDir()

	// Create WAYFINDER-STATUS.md
	createStatusFile(t, tmpDir, "RETRO")

	// Rewind RETRO→RETRO (magnitude 0)
	flags := RewindFlags{
		NoPrompt:  true,
		Reason:    "replay RETRO",
		Learnings: "keep the trace complete",
	}
	err := LogRewindEvent(tmpDir, "RETRO", "RETRO", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify RETRO contains exactly one same-phase rewind block.
	s11Path := filepath.Join(tmpDir, RetroFilename)
	retro, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("read RETRO: %v", err)
	}
	if got := strings.Count(string(retro), "## Rewind: RETRO → RETRO (magnitude 0)"); got != 1 {
		t.Errorf("same-phase RETRO blocks = %d, want 1", got)
	}
	for _, want := range []string{"**Reason**: " + flags.Reason, "**Learnings**: " + flags.Learnings} {
		if !strings.Contains(string(retro), want) {
			t.Errorf("same-phase RETRO missing %q:\n%s", want, retro)
		}
	}

	// Verify HISTORY contains exactly one canonical event.
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.jsonl")
	history, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if got := strings.Count(string(history), "rewind.logged"); got != 1 {
		t.Errorf("same-phase history events = %d, want 1", got)
	}
	for _, want := range []string{`"reason":"` + flags.Reason + `"`, `"learnings":"` + flags.Learnings + `"`} {
		if !strings.Contains(string(history), want) {
			t.Errorf("same-phase history missing %q:\n%s", want, history)
		}
	}
}

// TestIntegration_Magnitude1_WithFlags tests rewind with pre-provided flags
func TestIntegration_Magnitude1_WithFlags(t *testing.T) {
	tmpDir := t.TempDir()

	// Create WAYFINDER-STATUS.md
	createStatusFile(t, tmpDir, "RETRO")

	// Rewind RETRO→BUILD with --reason and --learnings flags
	flags := RewindFlags{
		Reason:    "Design was overcomplicated",
		Learnings: "Simpler approaches work better",
	}

	err := LogRewindEvent(tmpDir, "RETRO", "BUILD", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify RETRO exists and contains expected content
	s11Path := filepath.Join(tmpDir, RetroFilename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read RETRO file: %v", err)
	}

	s11Str := string(s11Content)

	// Validate RETRO content
	if !strings.Contains(s11Str, "## Rewind: RETRO → BUILD (magnitude 1)") {
		t.Errorf("RETRO missing rewind header")
	}
	if !strings.Contains(s11Str, "Design was overcomplicated") {
		t.Errorf("RETRO missing reason")
	}
	if !strings.Contains(s11Str, "Simpler approaches work better") {
		t.Errorf("RETRO missing learnings")
	}
	if !strings.Contains(s11Str, "**Context**:") {
		t.Errorf("RETRO missing context section")
	}

	// Verify HISTORY exists
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.jsonl")
	historyContent, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("Failed to read HISTORY file: %v", err)
	}

	historyStr := string(historyContent)
	if !strings.Contains(historyStr, "rewind.logged") {
		t.Errorf("HISTORY missing rewind.logged event")
	}
	if !strings.Contains(historyStr, "BUILD") {
		t.Errorf("HISTORY missing target phase BUILD")
	}
}

// TestIntegration_Magnitude3_LargeRewind tests large magnitude rewind
func TestIntegration_Magnitude3_LargeRewind(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "RETRO")

	// Rewind RETRO→SPEC (magnitude 4)
	flags := RewindFlags{
		NoPrompt:  true,
		Reason:    "Major approach change needed",
		Learnings: "Requirements analysis was incomplete",
	}

	err := LogRewindEvent(tmpDir, "RETRO", "SPEC", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify RETRO
	s11Path := filepath.Join(tmpDir, RetroFilename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read RETRO file: %v", err)
	}

	s11Str := string(s11Content)

	// Should show correct magnitude
	if !strings.Contains(s11Str, "magnitude 4") {
		t.Errorf("RETRO missing or wrong magnitude (expected 4)")
	}
}

// TestIntegration_ParallelDualLogging tests that both HISTORY and RETRO are updated
func TestIntegration_ParallelDualLogging(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "BUILD")

	// Perform rewind
	flags := RewindFlags{
		Reason: "Test parallel logging",
	}

	err := LogRewindEvent(tmpDir, "BUILD", "SETUP", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify BOTH files exist
	s11Path := filepath.Join(tmpDir, RetroFilename)
	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.jsonl")

	if _, err := os.Stat(s11Path); os.IsNotExist(err) {
		t.Errorf("RETRO file was not created")
	}

	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		t.Errorf("HISTORY file was not created")
	}

	// Verify RETRO content
	s11Content, _ := os.ReadFile(s11Path)
	if !strings.Contains(string(s11Content), "Test parallel logging") {
		t.Errorf("RETRO missing expected content")
	}

	// Verify HISTORY content
	historyContent, _ := os.ReadFile(historyPath)
	if !strings.Contains(string(historyContent), "rewind.logged") {
		t.Errorf("HISTORY missing event")
	}
}

// TestIntegration_MissingStatusReturnsError proves required trace persistence
// failures are visible to the command instead of being mistaken for success.
func TestIntegration_MissingStatusReturnsError(t *testing.T) {
	tmpDir := t.TempDir()

	// Don't create STATUS file.
	flags := RewindFlags{
		NoPrompt: true,
		Reason:   "Test error handling",
	}

	err := LogRewindEvent(tmpDir, "RETRO", "SETUP", flags)
	if err == nil || !strings.Contains(err.Error(), "read rewind status") {
		t.Errorf("LogRewindEvent error = %v, want missing-status error", err)
	}

	// Should not create files if error occurred
	s11Path := filepath.Join(tmpDir, RetroFilename)
	if info, err := os.Stat(s11Path); err == nil {
		// If file exists, it should be empty or minimal
		content, _ := os.ReadFile(s11Path)
		if len(content) > 100 {
			t.Errorf("RETRO file should not have full content on error, got %d bytes", len(content))
		}
		_ = info
	}
}

// TestIntegration_NonInteractiveEnvironment tests --no-prompt flag behavior
func TestIntegration_NonInteractiveEnvironment(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "BUILD")

	// Simulate CI/CD with --no-prompt (no reason provided)
	flags := RewindFlags{
		NoPrompt: true,
		// No reason or learnings provided
	}

	err := LogRewindEvent(tmpDir, "BUILD", "PLAN", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Should create RETRO even without reason
	s11Path := filepath.Join(tmpDir, RetroFilename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read RETRO file: %v", err)
	}

	s11Str := string(s11Content)

	// Should contain rewind header
	if !strings.Contains(s11Str, "## Rewind: BUILD → PLAN") {
		t.Errorf("RETRO missing rewind header")
	}

	// Should indicate no reason provided
	if !strings.Contains(s11Str, "_(not provided)_") {
		t.Errorf("RETRO should indicate reason not provided")
	}
}

// TestIntegration_ContextCaptureCompleteness tests that context snapshot is complete
func TestIntegration_ContextCaptureCompleteness(t *testing.T) {
	tmpDir := t.TempDir()

	// Create git repository (the sandbox also supplies the commit identity)
	if err := gittest.Command(t, tmpDir, "init", tmpDir).Run(); err != nil {
		t.Skipf("Skipping test: git not available")
	}

	// Create deliverable files
	deliverables := []string{
		"CHARTER-PROJECT-CHARTER.md",
		"PROBLEM-problem-validation.md",
		"BUILD-design.md",
	}
	for _, deliverable := range deliverables {
		path := filepath.Join(tmpDir, deliverable)
		os.WriteFile(path, []byte("content"), 0644)
	}

	// Commit files
	gittest.Command(t, tmpDir, "add", ".").Run()
	gittest.Command(t, tmpDir, "commit", "-m", "Initial commit").Run()

	// Create STATUS file
	createStatusFile(t, tmpDir, "RETRO")

	// Perform rewind
	flags := RewindFlags{
		Reason: "Test context capture",
	}

	err := LogRewindEvent(tmpDir, "RETRO", "BUILD", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Verify RETRO contains context
	s11Path := filepath.Join(tmpDir, RetroFilename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read RETRO file: %v", err)
	}

	s11Str := string(s11Content)

	// Should contain git context
	if !strings.Contains(s11Str, "Git:") {
		t.Errorf("RETRO missing git context")
	}

	// Should contain deliverables
	if !strings.Contains(s11Str, "Deliverables:") {
		t.Errorf("RETRO missing deliverables section")
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
		t.Errorf("RETRO missing deliverable files")
	}

	// Should contain completed phases
	if !strings.Contains(s11Str, "Completed phases:") {
		t.Errorf("RETRO missing completed phases section")
	}
}

// TestIntegration_RETROMarkdownFormat tests RETRO markdown is human-readable
func TestIntegration_RETROMarkdownFormat(t *testing.T) {
	tmpDir := t.TempDir()

	createStatusFile(t, tmpDir, "DESIGN")

	// Create rewind with all fields
	flags := RewindFlags{
		Reason:    "Comprehensive formatting test",
		Learnings: "Multiple key learnings from this rewind",
	}

	err := LogRewindEvent(tmpDir, "DESIGN", "PROBLEM", flags)
	if err != nil {
		t.Fatalf("LogRewindEvent failed: %v", err)
	}

	// Read RETRO
	s11Path := filepath.Join(tmpDir, RetroFilename)
	s11Content, err := os.ReadFile(s11Path)
	if err != nil {
		t.Fatalf("Failed to read RETRO file: %v", err)
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
			t.Errorf("RETRO missing required section: %s", section)
		}
	}

	// Validate ISO8601 timestamp format (YYYY-MM-DDTHH:MM:SSZ)
	if !strings.Contains(s11Str, "T") || !strings.Contains(s11Str, "Z") {
		t.Errorf("RETRO timestamp not in ISO8601 format")
	}
}

// Helper function to create a canonical WAYFINDER-STATUS.md file.
func createStatusFile(t *testing.T, dir string, currentWaypoint string) {
	t.Helper()
	now := time.Date(2024, time.January, 7, 10, 0, 0, 0, time.UTC)
	projectStatus := &wayfinderstatus.StatusV2{
		SchemaVersion:   wayfinderstatus.SchemaVersion,
		ProjectName:     "test-session-integration",
		ProjectType:     wayfinderstatus.ProjectTypeFeature,
		RiskLevel:       wayfinderstatus.RiskLevelS,
		CurrentWaypoint: currentWaypoint,
		Status:          wayfinderstatus.StatusV2InProgress,
		CreatedAt:       now.Add(-time.Hour),
		UpdatedAt:       now,
	}
	for _, waypoint := range wayfinderstatus.AllWaypointsV2Schema() {
		entry := wayfinderstatus.WaypointHistory{Name: waypoint, StartedAt: now}
		if waypoint == currentWaypoint {
			entry.Status = wayfinderstatus.WaypointStatusV2InProgress
			projectStatus.WaypointHistory = append(projectStatus.WaypointHistory, entry)
			break
		}
		entry.Status = wayfinderstatus.WaypointStatusV2Completed
		entry.CompletedAt = &now
		projectStatus.WaypointHistory = append(projectStatus.WaypointHistory, entry)
	}
	statusPath := filepath.Join(dir, "WAYFINDER-STATUS.md")
	if err := wayfinderstatus.WriteV2(projectStatus, statusPath); err != nil {
		t.Fatalf("Failed to create STATUS file: %v", err)
	}
}
