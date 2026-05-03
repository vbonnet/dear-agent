package reflection

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewRetrospectiveParser verifies parser initialization
func TestNewRetrospectiveParser(t *testing.T) {
	parser := NewRetrospectiveParser()
	if parser == nil {
		t.Fatal("NewRetrospectiveParser() returned nil")
	}

	if parser.improvementHeaderPattern == nil {
		t.Error("improvementHeaderPattern not initialized")
	}
	if parser.challengesHeaderPattern == nil {
		t.Error("challengesHeaderPattern not initialized")
	}
	if parser.bulletPattern == nil {
		t.Error("bulletPattern not initialized")
	}
}

// TestParseFile_WithTechnicalChallenges verifies parsing retrospective with failures
func TestParseFile_WithTechnicalChallenges(t *testing.T) {
	// Create temporary retrospective file
	tmpDir := t.TempDir()

	retrospectiveContent := `---
phase: S11
title: Test Retrospective
---

# S11: Retrospective

## What Went Well

- Feature implemented successfully
- Tests passed on first try

---

## What Could Improve

**Technical Challenges:**
- Database timeout issues during peak load
- API rate limiting not properly handled
- Syntax error in configuration file took 2 hours to debug

**Process Breakdowns:**
- Code review delayed by 3 days

---

## Lessons Learned

- Always load test before production
`

	retrospectivePath := filepath.Join(tmpDir, "S11-retrospective.md")
	err := os.WriteFile(retrospectivePath, []byte(retrospectiveContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Parse the file
	parser := NewRetrospectiveParser()
	learnings, err := parser.ParseFile(retrospectivePath)
	if err != nil {
		t.Fatalf("ParseFile() failed: %v", err)
	}

	// Verify we extracted 3 technical challenges
	expectedCount := 3
	if len(learnings) != expectedCount {
		t.Errorf("ParseFile() extracted %d learnings, want %d", len(learnings), expectedCount)
	}

	// Verify first learning
	if len(learnings) > 0 {
		if !strings.Contains(learnings[0].Description, "Database timeout") {
			t.Errorf("First learning = %q, want to contain 'Database timeout'", learnings[0].Description)
		}
		if learnings[0].Source != "retrospective" {
			t.Errorf("Learning source = %q, want 'retrospective'", learnings[0].Source)
		}
	}

	// Verify all learnings are non-empty
	for i, learning := range learnings {
		if learning.Description == "" {
			t.Errorf("Learning %d has empty description", i)
		}
	}
}

// TestParseFile_NoTechnicalChallenges verifies handling empty section
func TestParseFile_NoTechnicalChallenges(t *testing.T) {
	tmpDir := t.TempDir()

	retrospectiveContent := `---
phase: S11
---

# S11: Retrospective

## What Went Well

- Everything worked perfectly

## What Could Improve

**Technical Challenges:**
- [What technical problems emerged?]
- [How could we have anticipated them?]

---
`

	retrospectivePath := filepath.Join(tmpDir, "S11-retrospective.md")
	err := os.WriteFile(retrospectivePath, []byte(retrospectiveContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	parser := NewRetrospectiveParser()
	learnings, err := parser.ParseFile(retrospectivePath)
	if err != nil {
		t.Fatalf("ParseFile() failed: %v", err)
	}

	// Should extract 0 learnings (template placeholders ignored)
	if len(learnings) != 0 {
		t.Errorf("ParseFile() extracted %d learnings from template, want 0", len(learnings))
	}
}

// TestParseFile_NumberedList verifies parsing numbered lists
func TestParseFile_NumberedList(t *testing.T) {
	tmpDir := t.TempDir()

	retrospectiveContent := `## What Could Improve

**Technical Challenges:**
1. First failure with timeout
2. Second failure with permissions
3. Third failure with syntax error
`

	retrospectivePath := filepath.Join(tmpDir, "S11-retrospective.md")
	err := os.WriteFile(retrospectivePath, []byte(retrospectiveContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	parser := NewRetrospectiveParser()
	learnings, err := parser.ParseFile(retrospectivePath)
	if err != nil {
		t.Fatalf("ParseFile() failed: %v", err)
	}

	// Should extract 3 learnings
	if len(learnings) != 3 {
		t.Errorf("ParseFile() extracted %d learnings, want 3", len(learnings))
	}

	// Verify numbered items parsed correctly
	if len(learnings) > 0 {
		if !strings.Contains(learnings[0].Description, "First failure") {
			t.Errorf("First learning = %q, want to contain 'First failure'", learnings[0].Description)
		}
	}
}

// TestParseFile_AlternativeHeaders verifies different header variations
func TestParseFile_AlternativeHeaders(t *testing.T) {
	testCases := []struct {
		name    string
		header  string
		wantHit bool
	}{
		{"what_went_wrong", "## What Went Wrong", true},
		{"what_could_improve", "## What Could Improve", true},
		{"challenges", "## Challenges", true},
		{"problems", "### Problems", true},
		{"issues", "### Issues", true},
		{"unrelated", "## Success Metrics", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			content := tc.header + `

**Technical Challenges:**
- Test failure item
`

			path := filepath.Join(tmpDir, "test.md")
			err := os.WriteFile(path, []byte(content), 0644)
			if err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			parser := NewRetrospectiveParser()
			learnings, err := parser.ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile() failed: %v", err)
			}

			if tc.wantHit && len(learnings) == 0 {
				t.Errorf("Header %q should match but extracted 0 learnings", tc.header)
			}
			if !tc.wantHit && len(learnings) > 0 {
				t.Errorf("Header %q should not match but extracted %d learnings", tc.header, len(learnings))
			}
		})
	}
}

// TestParseFile_NonexistentFile verifies error handling
func TestParseFile_NonexistentFile(t *testing.T) {
	parser := NewRetrospectiveParser()
	_, err := parser.ParseFile("/nonexistent/file.md")
	if err == nil {
		t.Error("ParseFile() should return error for nonexistent file")
	}
}

// TestConvertToReflection verifies conversion to reflection
func TestConvertToReflection(t *testing.T) {
	parser := NewRetrospectiveParser()

	learning := &FailureLearning{
		Description: "Database timeout during peak load",
		Source:      "retrospective",
	}

	sessionID := "test-session-12345678"
	reflection := parser.ConvertToReflection(learning, sessionID)

	// Verify reflection fields
	if reflection.SessionID != sessionID {
		t.Errorf("Reflection.SessionID = %q, want %q", reflection.SessionID, sessionID)
	}

	if reflection.Trigger.Type != TriggerRepeatedFailureToSuccess {
		t.Errorf("Reflection.Trigger.Type = %v, want %v",
			reflection.Trigger.Type, TriggerRepeatedFailureToSuccess)
	}

	if reflection.Trigger.Description != learning.Description {
		t.Errorf("Reflection.Trigger.Description = %q, want %q",
			reflection.Trigger.Description, learning.Description)
	}

	if reflection.Learning != learning.Description {
		t.Errorf("Reflection.Learning = %q, want %q",
			reflection.Learning, learning.Description)
	}

	// Verify retrospective tag
	hasRetrospectiveTag := false
	for _, tag := range reflection.Tags {
		if tag == "retrospective" {
			hasRetrospectiveTag = true
			break
		}
	}
	if !hasRetrospectiveTag {
		t.Error("Reflection should have 'retrospective' tag")
	}
}

// TestExtractAndConvert verifies end-to-end workflow
func TestExtractAndConvert(t *testing.T) {
	tmpDir := t.TempDir()

	retrospectiveContent := `## What Could Improve

**Technical Challenges:**
- Timeout in API call
- Permission denied on file access
`

	retrospectivePath := filepath.Join(tmpDir, "S11-retrospective.md")
	err := os.WriteFile(retrospectivePath, []byte(retrospectiveContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	parser := NewRetrospectiveParser()
	reflections, err := parser.ExtractAndConvert(retrospectivePath, "test-session-abc")
	if err != nil {
		t.Fatalf("ExtractAndConvert() failed: %v", err)
	}

	// Should create 2 reflections
	if len(reflections) != 2 {
		t.Errorf("ExtractAndConvert() created %d reflections, want 2", len(reflections))
	}

	// Verify first reflection
	if len(reflections) > 0 {
		r := reflections[0]
		if r.SessionID != "test-session-abc" {
			t.Errorf("Reflection.SessionID = %q, want 'test-session-abc'", r.SessionID)
		}
		if !strings.Contains(r.Learning, "Timeout") {
			t.Errorf("Reflection.Learning = %q, want to contain 'Timeout'", r.Learning)
		}
	}
}

// TestParseContent_DirectScanner tests parsing from scanner
func TestParseContent_DirectScanner(t *testing.T) {
	content := `## What Could Improve

**Technical Challenges:**
- Test failure 1
- Test failure 2
`

	scanner := bufio.NewScanner(strings.NewReader(content))
	parser := NewRetrospectiveParser()

	learnings, err := parser.parseContent(scanner)
	if err != nil {
		t.Fatalf("parseContent() failed: %v", err)
	}

	if len(learnings) != 2 {
		t.Errorf("parseContent() extracted %d learnings, want 2", len(learnings))
	}
}

// TestParseFile_RealWorldExample tests against realistic retrospective
func TestParseFile_RealWorldExample(t *testing.T) {
	tmpDir := t.TempDir()

	// Realistic retrospective with multiple sections
	retrospectiveContent := `---
phase: S11
title: Living Retrospective - Fix authentication bug
---

# S11: Retrospective

## What Went Well

**Team Collaboration:**
- Quick turnaround on code review
- Good communication throughout

**Technical Decisions:**
- Chose JWT for auth tokens - worked well
- Redis for session storage - fast and reliable

---

## What Could Improve

**Technical Challenges:**
- Authentication timeout after 30 minutes caused user frustration
- Rate limiting logic had off-by-one error, blocked legitimate users
- Database connection pool exhausted during load test

**Process Breakdowns:**
- Missing test coverage for edge cases
- Documentation not updated before deployment

**Communication Gaps:**
- Stakeholder not informed of breaking API changes

---

## Lessons Learned

**About the Problem Domain:**
- Always consider session expiry UX
- Rate limiting needs careful testing

**About Our Tools:**
- Redis cluster setup is complex
- Need better monitoring for connection pools
`

	retrospectivePath := filepath.Join(tmpDir, "S11-retrospective.md")
	err := os.WriteFile(retrospectivePath, []byte(retrospectiveContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	parser := NewRetrospectiveParser()
	learnings, err := parser.ParseFile(retrospectivePath)
	if err != nil {
		t.Fatalf("ParseFile() failed: %v", err)
	}

	// Should extract exactly 3 technical challenges
	if len(learnings) != 3 {
		t.Errorf("ParseFile() extracted %d learnings, want 3", len(learnings))
		for i, l := range learnings {
			t.Logf("Learning %d: %s", i, l.Description)
		}
	}

	// Verify specific challenges extracted
	descriptions := make([]string, len(learnings))
	for i, l := range learnings {
		descriptions[i] = l.Description
	}

	expectedSubstrings := []string{"timeout", "Rate limiting", "connection pool"}
	for _, expected := range expectedSubstrings {
		found := false
		for _, desc := range descriptions {
			if strings.Contains(desc, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find learning containing %q, but it's missing", expected)
		}
	}
}
