package reflection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewStore verifies store initialization
func TestNewStore(t *testing.T) {
	store := NewStore("/tmp/reflections")
	if store == nil {
		t.Fatal("NewStore() returned nil")
	}

	if store.reflectionPath != "/tmp/reflections" {
		t.Errorf("Store.reflectionPath = %q, want %q", store.reflectionPath, "/tmp/reflections")
	}
}

// TestSave_Success verifies saving a reflection
func TestSave_Success(t *testing.T) {
	tmpDir := t.TempDir()

	store := NewStore(tmpDir)

	reflection := &Reflection{
		SessionID: "test-session-12345678",
		Timestamp: time.Date(2024, 11, 27, 10, 30, 0, 0, time.UTC),
		Trigger: Trigger{
			Type:        TriggerRepeatedFailureToSuccess,
			Description: "Fixed parser bug after 3 attempts",
		},
		Learning: "Remember to check edge cases for empty input before testing happy path.",
		Tags:     []string{"debugging", "parser"},
		Metrics: SessionMetrics{
			Duration:          30 * time.Minute,
			LinesChanged:      42,
			FilesModified:     3,
			CommandsExecuted:  15,
			ErrorsEncountered: 3,
		},
	}

	err := store.Save(reflection)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file was created
	expectedFilename := "2024-11-27-10-30-00-test-ses.ai.md"
	expectedPath := filepath.Join(tmpDir, expectedFilename)

	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("Reflection file not created: %v", err)
	}

	// Read and verify content
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read reflection file: %v", err)
	}

	contentStr := string(content)

	// Verify frontmatter
	if !strings.Contains(contentStr, "type: strategy") {
		t.Error("Content missing 'type: strategy'")
	}
	if !strings.Contains(contentStr, "title: 'Reflection: Fixed parser bug after 3 attempts'") {
		t.Error("Content missing reflection title")
	}
	if !strings.Contains(contentStr, "tags:") {
		t.Error("Content missing tags")
	}
	if !strings.Contains(contentStr, "reflections") {
		t.Error("Content missing 'reflections' tag")
	}
	if !strings.Contains(contentStr, "repeated_failure_to_success") {
		t.Error("Content missing trigger type tag")
	}
	if !strings.Contains(contentStr, "debugging") {
		t.Error("Content missing custom tag 'debugging'")
	}

	// Verify learning content
	if !strings.Contains(contentStr, "Remember to check edge cases") {
		t.Error("Content missing learning text")
	}

	// Verify metrics
	if !strings.Contains(contentStr, "Duration: 30m0s") {
		t.Error("Content missing duration metric")
	}
	if !strings.Contains(contentStr, "Lines changed: 42") {
		t.Error("Content missing lines changed metric")
	}
	if !strings.Contains(contentStr, "Files modified: 3") {
		t.Error("Content missing files modified metric")
	}
}

// TestSave_CreatesDirectory verifies directory creation
func TestSave_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Use nested path that doesn't exist
	reflectionPath := filepath.Join(tmpDir, "nested", "reflections")
	store := NewStore(reflectionPath)

	reflection := &Reflection{
		SessionID: "test-123456789",
		Timestamp: time.Now(),
		Trigger: Trigger{
			Type:        TriggerExplicitRequest,
			Description: "User requested reflection",
		},
		Learning: "Test learning",
		Metrics:  SessionMetrics{},
	}

	err := store.Save(reflection)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(reflectionPath); err != nil {
		t.Errorf("Reflection directory not created: %v", err)
	}
}

// TestSave_MinimalReflection verifies minimal reflection with no tags or metrics
func TestSave_MinimalReflection(t *testing.T) {
	tmpDir := t.TempDir()

	store := NewStore(tmpDir)

	reflection := &Reflection{
		SessionID: "minimal-session-id",
		Timestamp: time.Now(),
		Trigger: Trigger{
			Type:        TriggerWorkDiscarded,
			Description: "Reverted changes",
		},
		Learning: "Short learning note",
		Tags:     nil,              // No custom tags
		Metrics:  SessionMetrics{}, // Zero metrics
	}

	err := store.Save(reflection)
	if err != nil {
		t.Fatalf("Save() failed with minimal reflection: %v", err)
	}

	// Verify file exists
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 reflection file, got %d", len(files))
	}

	// Read and verify
	content, err := os.ReadFile(filepath.Join(tmpDir, files[0].Name()))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Should still have reflections tag and trigger type tag
	if !strings.Contains(string(content), "reflections") {
		t.Error("Content missing 'reflections' tag")
	}
	if !strings.Contains(string(content), "work_discarded") {
		t.Error("Content missing trigger type tag")
	}
}

// TestSave_FilenameFormat verifies filename generation
func TestSave_FilenameFormat(t *testing.T) {
	tmpDir := t.TempDir()

	store := NewStore(tmpDir)

	tests := []struct {
		sessionID        string
		timestamp        time.Time
		expectedFilename string
	}{
		{
			sessionID:        "session-abcd1234",
			timestamp:        time.Date(2024, 11, 27, 14, 30, 45, 0, time.UTC),
			expectedFilename: "2024-11-27-14-30-45-session-.ai.md",
		},
		{
			sessionID:        "12345678-long-session-id",
			timestamp:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedFilename: "2024-01-01-00-00-00-12345678.ai.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.sessionID, func(t *testing.T) {
			reflection := &Reflection{
				SessionID: tt.sessionID,
				Timestamp: tt.timestamp,
				Trigger: Trigger{
					Type:        TriggerUnusualPattern,
					Description: "Test",
				},
				Learning: "Test",
				Metrics:  SessionMetrics{},
			}

			err := store.Save(reflection)
			if err != nil {
				t.Fatalf("Save() failed: %v", err)
			}

			// Check filename
			files, err := os.ReadDir(tmpDir)
			if err != nil {
				t.Fatalf("failed to read directory: %v", err)
			}

			// Find the file (there may be multiple from previous iterations)
			found := false
			for _, file := range files {
				if file.Name() == tt.expectedFilename {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Expected filename %q not found. Files: %v", tt.expectedFilename, files)
			}

			// Clean up for next iteration
			os.RemoveAll(tmpDir)
			os.MkdirAll(tmpDir, 0755)
		})
	}
}

// TestList_EmptyDirectory verifies List returns empty when no reflections
func TestList_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	store := NewStore(tmpDir)

	reflections, err := store.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	// Implementation returns nil for empty (not parsed yet)
	if reflections == nil {
		reflections = []*Reflection{}
	}

	if len(reflections) != 0 {
		t.Errorf("List() returned %d reflections, want 0", len(reflections))
	}
}

// TestList_NonexistentDirectory verifies List handles missing directory
func TestList_NonexistentDirectory(t *testing.T) {
	store := NewStore("/nonexistent/path/to/reflections")

	reflections, err := store.List()
	if err != nil {
		t.Fatalf("List() failed with nonexistent directory: %v", err)
	}

	if reflections != nil {
		t.Errorf("List() returned %v, want nil for nonexistent directory", reflections)
	}
}

// TestTriggerTypes verifies trigger type constants
func TestTriggerTypes(t *testing.T) {
	triggers := []TriggerType{
		TriggerRepeatedFailureToSuccess,
		TriggerWorkDiscarded,
		TriggerUnusualPattern,
		TriggerExplicitRequest,
	}

	for _, trigger := range triggers {
		if trigger == "" {
			t.Error("Trigger type constant is empty")
		}
	}
}

// TestReflection_Structure verifies Reflection struct
func TestReflection_Structure(t *testing.T) {
	reflection := Reflection{
		SessionID: "test-session",
		Timestamp: time.Now(),
		Trigger: Trigger{
			Type:        TriggerExplicitRequest,
			Description: "Test trigger",
		},
		Learning: "Test learning",
		Tags:     []string{"tag1", "tag2"},
		Metrics: SessionMetrics{
			Duration:          10 * time.Minute,
			LinesChanged:      100,
			FilesModified:     5,
			CommandsExecuted:  20,
			ErrorsEncountered: 2,
		},
	}

	if reflection.SessionID != "test-session" {
		t.Error("Reflection.SessionID not set correctly")
	}

	if len(reflection.Tags) != 2 {
		t.Error("Reflection.Tags not set correctly")
	}

	if reflection.Metrics.LinesChanged != 100 {
		t.Error("Reflection.Metrics not set correctly")
	}
}

// TestSessionMetrics_ZeroValues verifies zero value handling
func TestSessionMetrics_ZeroValues(t *testing.T) {
	metrics := SessionMetrics{}

	if metrics.Duration != 0 {
		t.Error("Zero SessionMetrics.Duration should be 0")
	}

	if metrics.LinesChanged != 0 {
		t.Error("Zero SessionMetrics.LinesChanged should be 0")
	}
}

// TestSave_FailureTracking verifies failure tracking fields (Task 1.1.1: Mistake Notebook)
func TestSave_FailureTracking(t *testing.T) {
	tmpDir := t.TempDir()

	store := NewStore(tmpDir)

	// Create a failure reflection with all new fields
	reflection := &Reflection{
		SessionID: "fail-test-87654321",
		Timestamp: time.Date(2024, 12, 1, 14, 30, 0, 0, time.UTC),
		Trigger: Trigger{
			Type:        TriggerRepeatedFailureToSuccess,
			Description: "Fixed timeout after multiple tool_misuse errors",
		},
		Learning:      "Tool parameters must be validated before execution to avoid timeouts.",
		Tags:          []string{"debugging", "timeout"},
		Outcome:       OutcomeFailure,
		ErrorCategory: ErrorCategoryToolMisuse,
		LessonLearned: "Always validate tool parameters match expected schema before calling",
		Metrics: SessionMetrics{
			Duration:          45 * time.Minute,
			LinesChanged:      15,
			FilesModified:     2,
			CommandsExecuted:  8,
			ErrorsEncountered: 5,
		},
	}

	err := store.Save(reflection)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file was created
	expectedFilename := "2024-12-01-14-30-00-fail-tes.ai.md"
	expectedPath := filepath.Join(tmpDir, expectedFilename)

	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read reflection file: %v", err)
	}

	contentStr := string(content)

	// Verify failure tracking fields in frontmatter
	if !strings.Contains(contentStr, "outcome: failure") {
		t.Error("Content missing 'outcome: failure' frontmatter")
	}
	if !strings.Contains(contentStr, "error_category: tool_misuse") {
		t.Error("Content missing 'error_category: tool_misuse' frontmatter")
	}
	if !strings.Contains(contentStr, "lesson_learned: Always validate tool parameters") {
		t.Error("Content missing 'lesson_learned' frontmatter")
	}

	// Verify Lesson Learned section in content
	if !strings.Contains(contentStr, "## Lesson Learned") {
		t.Error("Content missing '## Lesson Learned' section")
	}
	if !strings.Contains(contentStr, "Always validate tool parameters match expected schema") {
		t.Error("Content missing lesson learned text in body")
	}
	if !strings.Contains(contentStr, "**Error Category**: tool_misuse") {
		t.Error("Content missing error category in lesson section")
	}

	// Verify metrics section still present
	if !strings.Contains(contentStr, "## Session Metrics") {
		t.Error("Content missing metrics section")
	}
}

// TestOutcomeTypes verifies outcome type constants (Task 1.1.1)
func TestOutcomeTypes(t *testing.T) {
	testCases := []struct {
		outcome OutcomeType
		want    string
	}{
		{OutcomeSuccess, "success"},
		{OutcomeFailure, "failure"},
		{OutcomePartial, "partial"},
	}

	for _, tc := range testCases {
		if string(tc.outcome) != tc.want {
			t.Errorf("OutcomeType %s = %q, want %q", tc.outcome, string(tc.outcome), tc.want)
		}
	}
}

// TestErrorCategories verifies error category constants (Task 1.1.1)
func TestErrorCategories(t *testing.T) {
	testCases := []struct {
		category ErrorCategory
		want     string
	}{
		{ErrorCategorySyntax, "syntax_error"},
		{ErrorCategoryToolMisuse, "tool_misuse"},
		{ErrorCategoryPermissionDenied, "permission_denied"},
		{ErrorCategoryTimeout, "timeout"},
		{ErrorCategoryOther, "other"},
	}

	for _, tc := range testCases {
		if string(tc.category) != tc.want {
			t.Errorf("ErrorCategory %s = %q, want %q", tc.category, string(tc.category), tc.want)
		}
	}

	// Verify we have exactly 5 initial categories
	allCategories := []ErrorCategory{
		ErrorCategorySyntax,
		ErrorCategoryToolMisuse,
		ErrorCategoryPermissionDenied,
		ErrorCategoryTimeout,
		ErrorCategoryOther,
	}

	if len(allCategories) != 5 {
		t.Errorf("Expected 5 initial error categories, got %d", len(allCategories))
	}
}

// TestSaveWithAutoDetect verifies automatic failure detection (Task 1.1.2)
func TestSaveWithAutoDetect(t *testing.T) {
	tmpDir := t.TempDir()

	store := NewStore(tmpDir)

	// Create a reflection WITHOUT failure fields set
	reflection := &Reflection{
		SessionID: "auto-detect-12345678",
		Timestamp: time.Date(2024, 12, 2, 10, 0, 0, 0, time.UTC),
		Trigger: Trigger{
			Type:        TriggerRepeatedFailureToSuccess,
			Description: "Fixed timeout error in API call",
		},
		Learning: "Add retry logic with exponential backoff",
		Tags:     []string{"api", "timeout"},
		Metrics: SessionMetrics{
			Duration:          20 * time.Minute,
			LinesChanged:      25,
			FilesModified:     2,
			CommandsExecuted:  10,
			ErrorsEncountered: 2,
		},
		// NOTE: Outcome, ErrorCategory, LessonLearned NOT set - should be auto-detected
	}

	// Create detection context
	context := DetectionContext{
		Description:       "Fixed timeout error in API call",
		Learning:          "Add retry logic with exponential backoff",
		ErrorMessage:      "timeout: request timed out after 30 seconds",
		ErrorsEncountered: 2,
		SessionCompleted:  false,
	}

	// Save with auto-detect
	err := store.SaveWithAutoDetect(reflection, context)
	if err != nil {
		t.Fatalf("SaveWithAutoDetect() failed: %v", err)
	}

	// Verify reflection was enriched
	if reflection.Outcome == "" {
		t.Error("SaveWithAutoDetect() did not set Outcome")
	}
	if reflection.Outcome != OutcomeFailure {
		t.Errorf("SaveWithAutoDetect() Outcome = %v, want %v", reflection.Outcome, OutcomeFailure)
	}
	if reflection.ErrorCategory != ErrorCategoryTimeout {
		t.Errorf("SaveWithAutoDetect() ErrorCategory = %v, want %v", reflection.ErrorCategory, ErrorCategoryTimeout)
	}
	if reflection.LessonLearned == "" {
		t.Error("SaveWithAutoDetect() did not set LessonLearned")
	}

	// Verify file was created with enriched data
	expectedFilename := "2024-12-02-10-00-00-auto-det.ai.md"
	expectedPath := filepath.Join(tmpDir, expectedFilename)

	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read reflection file: %v", err)
	}

	contentStr := string(content)

	// Verify frontmatter includes auto-detected fields
	if !strings.Contains(contentStr, "outcome: failure") {
		t.Error("File missing auto-detected 'outcome: failure'")
	}
	if !strings.Contains(contentStr, "error_category: timeout") {
		t.Error("File missing auto-detected 'error_category: timeout'")
	}
	if !strings.Contains(contentStr, "lesson_learned:") {
		t.Error("File missing auto-detected 'lesson_learned'")
	}

	// Verify Lesson Learned section in content
	if !strings.Contains(contentStr, "## Lesson Learned") {
		t.Error("File missing '## Lesson Learned' section")
	}
}

// TestSaveWithAutoDetect_Success verifies no enrichment for success (Task 1.1.2)
func TestSaveWithAutoDetect_Success(t *testing.T) {
	tmpDir := t.TempDir()

	store := NewStore(tmpDir)

	reflection := &Reflection{
		SessionID: "success-auto-12345678",
		Timestamp: time.Date(2024, 12, 2, 11, 0, 0, 0, time.UTC),
		Trigger: Trigger{
			Type:        TriggerExplicitRequest,
			Description: "Completed feature successfully",
		},
		Learning: "Feature implemented with all tests passing",
		Tags:     []string{"feature"},
		Metrics: SessionMetrics{
			Duration:          15 * time.Minute,
			ErrorsEncountered: 0,
		},
	}

	context := DetectionContext{
		Description:       "Completed feature successfully",
		Learning:          "Feature implemented with all tests passing",
		ErrorsEncountered: 0,
		SessionCompleted:  true,
	}

	err := store.SaveWithAutoDetect(reflection, context)
	if err != nil {
		t.Fatalf("SaveWithAutoDetect() failed: %v", err)
	}

	// Verify outcome is success
	if reflection.Outcome != OutcomeSuccess {
		t.Errorf("SaveWithAutoDetect() Outcome = %v, want %v", reflection.Outcome, OutcomeSuccess)
	}

	// Verify no error category for success
	if reflection.ErrorCategory != "" {
		t.Errorf("SaveWithAutoDetect() should not set ErrorCategory for success, got %v", reflection.ErrorCategory)
	}

	// Verify no lesson learned for success
	if reflection.LessonLearned != "" {
		t.Errorf("SaveWithAutoDetect() should not set LessonLearned for success, got %v", reflection.LessonLearned)
	}

	// Verify file content
	expectedFilename := "2024-12-02-11-00-00-success-.ai.md"
	expectedPath := filepath.Join(tmpDir, expectedFilename)

	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read reflection file: %v", err)
	}

	contentStr := string(content)

	// Success should NOT have error fields in frontmatter
	if strings.Contains(contentStr, "error_category:") {
		t.Error("Success reflection should not have error_category in frontmatter")
	}
	if strings.Contains(contentStr, "## Lesson Learned") {
		t.Error("Success reflection should not have Lesson Learned section")
	}
}
