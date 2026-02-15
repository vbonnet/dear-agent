package enforcement

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEndToEndViolationFlow tests the complete violation detection and filing workflow:
// 1. Load patterns from database
// 2. Detect violation in content
// 3. Generate rejection message
// 4. File violation to disk
func TestEndToEndViolationFlow(t *testing.T) {
	// Create temporary violations directory
	tmpDir := t.TempDir()

	// Step 1: Load patterns from the real pattern database
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Skipping integration test: cannot get home directory: %v", err)
	}

	patternsPath := filepath.Join(home, "src", "ws", "oss", "repos", "engram", "patterns", "bash-anti-patterns.yaml")
	if _, err := os.Stat(patternsPath); os.IsNotExist(err) {
		t.Skipf("Skipping integration test: pattern database not found at %s", patternsPath)
	}

	db, err := LoadPatterns(patternsPath)
	if err != nil {
		t.Fatalf("Failed to load patterns: %v", err)
	}

	if len(db.Patterns) == 0 {
		t.Fatalf("No patterns loaded from database")
	}

	// Step 2: Create detector and detect violation
	detector, err := NewDetector(db)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	testContent := "cd /home/user/repo && git push origin main"
	pattern, err := detector.Detect(testContent)
	if err != nil {
		t.Fatalf("Detection failed: %v", err)
	}

	if pattern == nil {
		t.Fatalf("Expected to detect cd-chaining violation, got nil")
	}

	if pattern.ID != "cd-chaining" {
		t.Errorf("Expected pattern ID 'cd-chaining', got '%s'", pattern.ID)
	}

	// Step 3: Generate rejection message
	message := GenerateRejectionMessage(pattern, testContent)

	if !strings.Contains(message, "cd-chaining") {
		t.Errorf("Rejection message missing pattern ID")
	}

	if !strings.Contains(message, "Command chaining with cd") {
		t.Errorf("Rejection message missing reason")
	}

	if !strings.Contains(message, testContent) {
		t.Errorf("Rejection message missing command")
	}

	// Step 4: File violation
	violation := ViolationData{
		PatternID:          pattern.ID,
		PatternType:        "bash",
		Command:            testContent,
		SessionID:          "integration-test-session",
		AgentType:          "general-purpose",
		TaskCategory:       "version_control",
		ConversationLength: 5,
		Tags:               []string{"cd", "git", "chaining"},
		Timestamp:          time.Now(),
	}

	violationPath, err := FileViolation(violation, tmpDir, pattern)
	if err != nil {
		t.Fatalf("Failed to file violation: %v", err)
	}

	// Step 5: Verify violation file
	if _, err := os.Stat(violationPath); os.IsNotExist(err) {
		t.Fatalf("Violation file was not created at %s", violationPath)
	}

	// Read and verify violation file content
	content, err := os.ReadFile(violationPath)
	if err != nil {
		t.Fatalf("Failed to read violation file: %v", err)
	}

	contentStr := string(content)

	// Verify YAML frontmatter
	expectedFrontmatter := []string{
		"---",
		"pattern_id: cd-chaining",
		"pattern_type: bash",
		"session_id: integration-test-session",
		"agent_type: general-purpose",
		"command: cd /home/user/repo && git push origin main",
		"tier: \"3_astrocyte\"",
		"type: cd_usage",
		"severity: high",
	}

	for _, expected := range expectedFrontmatter {
		if !strings.Contains(contentStr, expected) {
			t.Errorf("Violation file missing expected frontmatter: %q", expected)
		}
	}

	// Verify markdown sections
	expectedSections := []string{
		"# Violation Report: cd-chaining",
		"## Context",
		"## Violation Details",
		"## Why It Happened",
		"## Recovery",
		"## Proposed Fix",
	}

	for _, section := range expectedSections {
		if !strings.Contains(contentStr, section) {
			t.Errorf("Violation file missing required section: %q", section)
		}
	}

	// Verify violation details match pattern
	if !strings.Contains(contentStr, pattern.Reason) {
		t.Errorf("Violation file missing pattern reason: %q", pattern.Reason)
	}

	if !strings.Contains(contentStr, pattern.Alternative) {
		t.Errorf("Violation file missing pattern alternative: %q", pattern.Alternative)
	}

	t.Logf("End-to-end test successful. Violation filed to: %s", violationPath)
}

// TestMultipleViolationDetectionAndFiling tests handling multiple violations in sequence
func TestMultipleViolationDetectionAndFiling(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple pattern database for testing
	db := &PatternDatabase{
		Patterns: []Pattern{
			{
				ID:          "cd-chaining",
				Regex:       `cd\s+[^\s]+\s+&&`,
				Reason:      "Command chaining with cd",
				Alternative: "Use tool-specific -C flag",
				Severity:    "high",
			},
			{
				ID:          "cat-file-read",
				Regex:       `^cat\s+[^\|]+$`,
				Reason:      "Using cat to read files",
				Alternative: "Use Read tool",
				Severity:    "high",
			},
			{
				ID:          "grep-search",
				Regex:       `grep\s+`,
				Reason:      "Using bash grep",
				Alternative: "Use Grep tool",
				Severity:    "high",
			},
		},
	}

	detector, err := NewDetector(db)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	testCases := []struct {
		content     string
		expectedID  string
		sessionID   string
	}{
		{
			content:     "cd /repo && git push",
			expectedID:  "cd-chaining",
			sessionID:   "session-1",
		},
		{
			content:     "cat config.yaml",
			expectedID:  "cat-file-read",
			sessionID:   "session-2",
		},
		{
			content:     "grep TODO *.go",
			expectedID:  "grep-search",
			sessionID:   "session-3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expectedID, func(t *testing.T) {
			// Detect
			pattern, err := detector.Detect(tc.content)
			if err != nil {
				t.Fatalf("Detection failed: %v", err)
			}

			if pattern == nil {
				t.Fatalf("Expected to detect %s, got nil", tc.expectedID)
			}

			if pattern.ID != tc.expectedID {
				t.Errorf("Expected pattern %s, got %s", tc.expectedID, pattern.ID)
			}

			// Generate message
			message := GenerateRejectionMessage(pattern, tc.content)
			if !strings.Contains(message, pattern.ID) {
				t.Errorf("Message missing pattern ID")
			}

			// File violation
			violation := ViolationData{
				PatternID:   pattern.ID,
				PatternType: "bash",
				Command:     tc.content,
				SessionID:   tc.sessionID,
				AgentType:   "general-purpose",
			}

			path, err := FileViolation(violation, tmpDir, pattern)
			if err != nil {
				t.Fatalf("Failed to file violation: %v", err)
			}

			// Verify file exists
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("Violation file not created: %s", path)
			}
		})
	}

	// Verify all files were created in bash subdirectory
	bashDir := filepath.Join(tmpDir, "bash")
	files, err := os.ReadDir(bashDir)
	if err != nil {
		t.Fatalf("Failed to read bash directory: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("Expected 3 violation files, got %d", len(files))
	}
}

// TestDetectionWithRealPatternDatabase tests detection using actual pattern database
func TestDetectionWithRealPatternDatabase(t *testing.T) {
	// Try to load the real pattern database
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Skipping test: cannot get home directory: %v", err)
	}

	patternsPath := filepath.Join(home, "src", "ws", "oss", "repos", "engram", "patterns", "bash-anti-patterns.yaml")
	if _, err := os.Stat(patternsPath); os.IsNotExist(err) {
		t.Skipf("Skipping test: pattern database not found at %s", patternsPath)
	}

	db, err := LoadPatterns(patternsPath)
	if err != nil {
		t.Fatalf("Failed to load patterns: %v", err)
	}

	detector, err := NewDetector(db)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}

	// Test various commands
	testCases := []struct {
		name        string
		content     string
		shouldMatch bool
		expectedID  string
	}{
		{
			name:        "cd with &&",
			content:     "cd /repo && git status",
			shouldMatch: true,
			expectedID:  "cd-chaining",
		},
		{
			name:        "cd with semicolon",
			content:     "cd /repo; ls",
			shouldMatch: true,
			expectedID:  "cd-semicolon-chain",
		},
		{
			name:        "cat file",
			content:     "cat README.md",
			shouldMatch: true,
			expectedID:  "cat-file-read",
		},
		{
			name:        "grep search",
			content:     "grep TODO file.txt",
			shouldMatch: true,
			expectedID:  "grep-search",
		},
		{
			name:        "find command",
			content:     "find . -name '*.go'",
			shouldMatch: true,
			expectedID:  "find-file-search",
		},
		{
			name:        "valid git command",
			content:     "git -C /repo push",
			shouldMatch: false,
		},
		{
			name:        "valid npm command",
			content:     "npm install",
			shouldMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pattern, err := detector.Detect(tc.content)
			if err != nil {
				t.Fatalf("Detection failed: %v", err)
			}

			if tc.shouldMatch {
				if pattern == nil {
					t.Errorf("Expected to detect violation, got nil")
				} else if pattern.ID != tc.expectedID {
					t.Errorf("Expected pattern %s, got %s", tc.expectedID, pattern.ID)
				}
			} else {
				if pattern != nil {
					t.Errorf("Expected no violation, got pattern %s", pattern.ID)
				}
			}
		})
	}
}

// TestViolationFilingWithVariousPatternTypes tests filing violations for different pattern types
func TestViolationFilingWithVariousPatternTypes(t *testing.T) {
	tmpDir := t.TempDir()

	patternTypes := []struct {
		patternType string
		patternID   string
		command     string
	}{
		{"bash", "cd-chaining", "cd /repo && git push"},
		{"beads", "bead-violation", "invalid bead command"},
		{"git", "git-violation", "git command violation"},
	}

	for _, pt := range patternTypes {
		t.Run(pt.patternType, func(t *testing.T) {
			pattern := &Pattern{
				ID:          pt.patternID,
				Reason:      "Test reason",
				Alternative: "Test alternative",
				Severity:    "medium",
			}

			violation := ViolationData{
				PatternID:   pt.patternID,
				PatternType: pt.patternType,
				Command:     pt.command,
				SessionID:   "test-session",
				AgentType:   "general-purpose",
			}

			path, err := FileViolation(violation, tmpDir, pattern)
			if err != nil {
				t.Fatalf("Failed to file violation: %v", err)
			}

			// Verify file is in correct subdirectory
			expectedSubdir := filepath.Join(tmpDir, pt.patternType)
			if !strings.HasPrefix(path, expectedSubdir) {
				t.Errorf("Expected file in %s, got %s", expectedSubdir, path)
			}

			// Verify subdirectory exists
			if _, err := os.Stat(expectedSubdir); os.IsNotExist(err) {
				t.Errorf("Subdirectory %s was not created", expectedSubdir)
			}

			// Verify file content
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}

			if !strings.Contains(string(content), "pattern_type: "+pt.patternType) {
				t.Errorf("File missing correct pattern_type")
			}
		})
	}
}

// TestRejectionMessageGeneration tests message generation with various patterns
func TestRejectionMessageGeneration(t *testing.T) {
	patterns := []Pattern{
		{
			ID:          "cd-chaining",
			Reason:      "Command chaining with cd",
			Alternative: "Use tool-specific -C flag",
			Severity:    "high",
			Tier1Example: "❌ BAD: cd /repo && git push\n✅ GOOD: git -C /repo push",
		},
		{
			ID:          "cat-file-read",
			Reason:      "Using cat to read files",
			Alternative: "Use Read tool",
			Severity:    "high",
			Examples:    []string{"cat file.txt", "cat /path/to/file"},
		},
	}

	for _, pattern := range patterns {
		t.Run(pattern.ID, func(t *testing.T) {
			command := "test command"

			// Test basic message
			msg := GenerateRejectionMessage(&pattern, command)
			if !strings.Contains(msg, pattern.ID) {
				t.Errorf("Message missing pattern ID")
			}
			if !strings.Contains(msg, pattern.Reason) {
				t.Errorf("Message missing reason")
			}
			if !strings.Contains(msg, pattern.Alternative) {
				t.Errorf("Message missing alternative")
			}

			// Test short message
			shortMsg := GenerateShortRejectionMessage(&pattern)
			if !strings.Contains(shortMsg, pattern.ID) {
				t.Errorf("Short message missing pattern ID")
			}

			// Test message with severity
			severityMsg := GenerateRejectionMessageWithSeverity(&pattern, command)
			if !strings.Contains(severityMsg, "HIGH") {
				t.Errorf("Severity message missing severity level")
			}
		})
	}
}
