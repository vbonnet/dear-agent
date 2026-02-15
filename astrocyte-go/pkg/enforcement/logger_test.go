package enforcement

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestFileViolation(t *testing.T) {
	// Create temporary directory for test violations
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		violation   ViolationData
		pattern     *Pattern
		wantErr     bool
		errContains string
	}{
		{
			name: "valid violation with all required fields",
			violation: ViolationData{
				PatternID:   "cd-chaining",
				PatternType: "bash",
				Command:     "cd /repo && git push",
				SessionID:   "test-session",
				AgentType:   "general-purpose",
				Timestamp:   time.Date(2026, 2, 15, 10, 30, 0, 0, time.UTC),
			},
			pattern: &Pattern{
				ID:          "cd-chaining",
				Reason:      "Command chaining with cd",
				Alternative: "Use tool-specific -C flag",
				Severity:    "high",
			},
			wantErr: false,
		},
		{
			name: "violation with optional fields",
			violation: ViolationData{
				PatternID:          "cat-file-read",
				PatternType:        "bash",
				Command:            "cat file.txt",
				SessionID:          "test-session-2",
				AgentType:          "explore",
				TaskCategory:       "file_reading",
				ConversationLength: 15,
				Tags:               []string{"cat", "bash", "tools"},
				EngramVersion:      "v1.6",
				EngramHash:         "sha256:abc123",
				Timestamp:          time.Date(2026, 2, 15, 11, 0, 0, 0, time.UTC),
			},
			pattern: &Pattern{
				ID:          "cat-file-read",
				Reason:      "Using cat to read files",
				Alternative: "Use Read tool",
				Severity:    "high",
			},
			wantErr: false,
		},
		{
			name: "missing pattern_id",
			violation: ViolationData{
				PatternType: "bash",
				Command:     "cd /repo && git push",
				SessionID:   "test-session",
				AgentType:   "general-purpose",
			},
			pattern:     &Pattern{ID: "test"},
			wantErr:     true,
			errContains: "missing required fields",
		},
		{
			name: "missing pattern_type",
			violation: ViolationData{
				PatternID: "cd-chaining",
				Command:   "cd /repo && git push",
				SessionID: "test-session",
				AgentType: "general-purpose",
			},
			pattern:     &Pattern{ID: "cd-chaining"},
			wantErr:     true,
			errContains: "missing required fields",
		},
		{
			name: "missing command",
			violation: ViolationData{
				PatternID:   "cd-chaining",
				PatternType: "bash",
				SessionID:   "test-session",
				AgentType:   "general-purpose",
			},
			pattern:     &Pattern{ID: "cd-chaining"},
			wantErr:     true,
			errContains: "missing required fields",
		},
		{
			name: "nil pattern",
			violation: ViolationData{
				PatternID:   "cd-chaining",
				PatternType: "bash",
				Command:     "cd /repo && git push",
				SessionID:   "test-session",
				AgentType:   "general-purpose",
			},
			pattern:     nil,
			wantErr:     true,
			errContains: "pattern cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filepath, err := FileViolation(tt.violation, tmpDir, tt.pattern)

			if tt.wantErr {
				if err == nil {
					t.Errorf("FileViolation() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("FileViolation() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("FileViolation() unexpected error: %v", err)
				return
			}

			// Verify file was created
			if _, err := os.Stat(filepath); os.IsNotExist(err) {
				t.Errorf("FileViolation() did not create file at %s", filepath)
			}
		})
	}
}

func TestFileViolationContent(t *testing.T) {
	tmpDir := t.TempDir()

	violation := ViolationData{
		PatternID:          "cd-chaining",
		PatternType:        "bash",
		Command:            "cd /repo && git push",
		SessionID:          "test-session",
		AgentType:          "general-purpose",
		TaskCategory:       "version_control",
		ConversationLength: 10,
		Tags:               []string{"cd", "git"},
		EngramVersion:      "v1.6",
		EngramHash:         "sha256:test123",
		Timestamp:          time.Date(2026, 2, 15, 10, 30, 45, 0, time.UTC),
	}

	pattern := &Pattern{
		ID:          "cd-chaining",
		Reason:      "Command chaining with cd",
		Alternative: "Use tool-specific -C flag (e.g., git -C /path)",
		Severity:    "high",
	}

	filepath, err := FileViolation(violation, tmpDir, pattern)
	if err != nil {
		t.Fatalf("FileViolation() failed: %v", err)
	}

	// Read the file content
	content, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("Failed to read violation file: %v", err)
	}

	contentStr := string(content)

	// Test YAML frontmatter
	t.Run("yaml_frontmatter", func(t *testing.T) {
		expectedFields := []string{
			"---",
			"date: 2026-02-15T10:30:45Z",
			"type: cd_usage",
			"severity: high",
			"tier: \"3_astrocyte\"",
			"pattern_id: cd-chaining",
			"pattern_type: bash",
			"session_id: test-session",
			"agent_type: general-purpose",
			"command: cd /repo && git push",
			"task_category: version_control",
			"conversation_length: 10",
			"tags:",
			"  - cd",
			"  - git",
			"engram_version: v1.6",
			"engram_hash: sha256:test123",
		}

		for _, field := range expectedFields {
			if !strings.Contains(contentStr, field) {
				t.Errorf("Missing expected field in frontmatter: %q", field)
			}
		}
	})

	// Test markdown sections
	t.Run("markdown_sections", func(t *testing.T) {
		requiredSections := []string{
			"# Violation Report: cd-chaining",
			"## Context",
			"## Violation Details",
			"## Why It Happened",
			"## Recovery",
			"## Proposed Fix",
		}

		for _, section := range requiredSections {
			if !strings.Contains(contentStr, section) {
				t.Errorf("Missing required section: %q", section)
			}
		}
	})

	// Test violation details
	t.Run("violation_details", func(t *testing.T) {
		expectedDetails := []string{
			"- **Command attempted**: `cd /repo && git push`",
			"- **Pattern matched**: cd-chaining (bash)",
			"- **Reason**: Command chaining with cd",
			"- **Correct approach**: Use tool-specific -C flag",
		}

		for _, detail := range expectedDetails {
			if !strings.Contains(contentStr, detail) {
				t.Errorf("Missing expected detail: %q", detail)
			}
		}
	})

	// Test context information
	t.Run("context_information", func(t *testing.T) {
		expectedContext := []string{
			"Session: test-session",
			"Agent type: general-purpose",
			"The Astrocyte daemon detected this violation",
		}

		for _, ctx := range expectedContext {
			if !strings.Contains(contentStr, ctx) {
				t.Errorf("Missing expected context: %q", ctx)
			}
		}
	})
}

func TestFileViolationFilename(t *testing.T) {
	tmpDir := t.TempDir()

	violation := ViolationData{
		PatternID:   "cd-chaining",
		PatternType: "bash",
		Command:     "cd /repo && git push",
		SessionID:   "test-session",
		AgentType:   "general-purpose",
		Timestamp:   time.Date(2026, 2, 15, 10, 30, 0, 0, time.UTC),
	}

	pattern := &Pattern{
		ID:       "cd-chaining",
		Severity: "high",
	}

	filepath, err := FileViolation(violation, tmpDir, pattern)
	if err != nil {
		t.Fatalf("FileViolation() failed: %v", err)
	}

	filename := filepath[strings.LastIndex(filepath, "/")+1:]

	// Check filename format: YYYY-MM-DD-{pattern-id}-{hash}.md
	if !strings.HasPrefix(filename, "2026-02-15-cd-chaining-") {
		t.Errorf("Filename has incorrect format: %s", filename)
	}

	if !strings.HasSuffix(filename, ".md") {
		t.Errorf("Filename should end with .md: %s", filename)
	}

	// Check that hash is 8 characters
	parts := strings.Split(filename, "-")
	if len(parts) < 4 {
		t.Errorf("Filename missing hash component: %s", filename)
	} else {
		hashPart := strings.TrimSuffix(parts[len(parts)-1], ".md")
		if len(hashPart) != 8 {
			t.Errorf("Hash should be 8 characters, got %d: %s", len(hashPart), hashPart)
		}
	}
}

func TestFileViolationDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	violation := ViolationData{
		PatternID:   "cd-chaining",
		PatternType: "bash",
		Command:     "cd /repo && git push",
		SessionID:   "test-session",
		AgentType:   "general-purpose",
	}

	pattern := &Pattern{
		ID:       "cd-chaining",
		Severity: "high",
	}

	filepath, err := FileViolation(violation, tmpDir, pattern)
	if err != nil {
		t.Fatalf("FileViolation() failed: %v", err)
	}

	// Check that file is in bash subdirectory
	expectedDir := tmpDir + "/bash"
	if !strings.HasPrefix(filepath, expectedDir) {
		t.Errorf("File should be in bash subdirectory, got: %s", filepath)
	}

	// Verify directory was created
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("bash subdirectory was not created")
	}
}

func TestFileViolationTypeMapping(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name             string
		patternID        string
		expectedTypeInYAML string
	}{
		{"cd-chaining", "cd-chaining", "type: cd_usage"},
		{"cd-semicolon-chain", "cd-semicolon-chain", "type: cd_usage"},
		{"for-loop", "for-loop", "type: for_loops"},
		{"while-loop", "while-loop", "type: for_loops"},
		{"double-ampersand-chain", "double-ampersand-chain", "type: chained_commands"},
		{"semicolon-chain", "semicolon-chain", "type: chained_commands"},
		{"git-add-all", "git-add-all", "type: git_violations"},
		{"cat-file-read", "cat-file-read", "type: bash_over_tools"},
		{"grep-search", "grep-search", "type: bash_over_tools"},
		{"unknown-pattern", "unknown-pattern", "type: bash_over_tools"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violation := ViolationData{
				PatternID:   tt.patternID,
				PatternType: "bash",
				Command:     "test command",
				SessionID:   "test-session",
				AgentType:   "general-purpose",
			}

			pattern := &Pattern{
				ID:       tt.patternID,
				Severity: "medium",
			}

			filepath, err := FileViolation(violation, tmpDir, pattern)
			if err != nil {
				t.Fatalf("FileViolation() failed: %v", err)
			}

			content, err := os.ReadFile(filepath)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}

			if !strings.Contains(string(content), tt.expectedTypeInYAML) {
				t.Errorf("Expected %q in content, not found", tt.expectedTypeInYAML)
			}
		})
	}
}

func TestFileViolationDefaultTimestamp(t *testing.T) {
	tmpDir := t.TempDir()

	violation := ViolationData{
		PatternID:   "cd-chaining",
		PatternType: "bash",
		Command:     "cd /repo && git push",
		SessionID:   "test-session",
		AgentType:   "general-purpose",
		// Timestamp is zero value (not set)
	}

	pattern := &Pattern{
		ID:       "cd-chaining",
		Severity: "high",
	}

	before := time.Now()
	filepath, err := FileViolation(violation, tmpDir, pattern)
	after := time.Now()

	if err != nil {
		t.Fatalf("FileViolation() failed: %v", err)
	}

	// Check that filename contains today's date
	filename := filepath[strings.LastIndex(filepath, "/")+1:]
	todayStr := before.Format("2006-01-02")
	if !strings.HasPrefix(filename, todayStr) {
		t.Errorf("Expected filename to start with %s, got: %s", todayStr, filename)
	}

	// Read content and verify date is in reasonable range
	content, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Extract date from YAML frontmatter
	contentStr := string(content)
	if !strings.Contains(contentStr, "date: "+before.Format("2006-01-02")) {
		if !strings.Contains(contentStr, "date: "+after.Format("2006-01-02")) {
			t.Errorf("Date in file should be today's date")
		}
	}
}

func TestFileViolationDefaultSeverity(t *testing.T) {
	tmpDir := t.TempDir()

	violation := ViolationData{
		PatternID:   "test-pattern",
		PatternType: "bash",
		Command:     "test command",
		SessionID:   "test-session",
		AgentType:   "general-purpose",
	}

	// Pattern with no severity specified
	pattern := &Pattern{
		ID: "test-pattern",
	}

	filepath, err := FileViolation(violation, tmpDir, pattern)
	if err != nil {
		t.Fatalf("FileViolation() failed: %v", err)
	}

	content, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Should default to "medium"
	if !strings.Contains(string(content), "severity: medium") {
		t.Errorf("Expected default severity 'medium', not found in content")
	}
}

func TestFileViolationSubdirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()

	patternTypes := []string{"bash", "beads", "git"}

	for _, patternType := range patternTypes {
		t.Run(patternType, func(t *testing.T) {
			violation := ViolationData{
				PatternID:   "test-pattern",
				PatternType: patternType,
				Command:     "test command",
				SessionID:   "test-session",
				AgentType:   "general-purpose",
			}

			pattern := &Pattern{
				ID:       "test-pattern",
				Severity: "medium",
			}

			filepath, err := FileViolation(violation, tmpDir, pattern)
			if err != nil {
				t.Fatalf("FileViolation() failed: %v", err)
			}

			// Verify subdirectory exists
			expectedDir := filepath[:strings.LastIndex(filepath, "/")]
			expectedSubdir := tmpDir + "/" + patternType

			if expectedDir != expectedSubdir {
				t.Errorf("Expected file in %s, got %s", expectedSubdir, expectedDir)
			}

			if _, err := os.Stat(expectedSubdir); os.IsNotExist(err) {
				t.Errorf("Subdirectory %s was not created", expectedSubdir)
			}
		})
	}
}
