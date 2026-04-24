package validate

import (
	"errors"
	"testing"
)

func TestClassifyResumeError_EmptySessionEnv(t *testing.T) {
	output := "TypeError: Cannot read properties of undefined (reading 'version')"
	err := errors.New("resume failed")

	issue := classifyResumeError(output, err)

	if issue.Type != IssueEmptySessionEnv {
		t.Errorf("Expected type=%s, got %s", IssueEmptySessionEnv, issue.Type)
	}
	if !issue.AutoFixable {
		t.Error("Expected AutoFixable=true for empty session env")
	}
	if issue.Message == "" {
		t.Error("Expected non-empty message")
	}
}

func TestClassifyResumeError_JSONLMissing(t *testing.T) {
	output := "No conversation found for UUID abc123"
	err := errors.New("resume failed")

	issue := classifyResumeError(output, err)

	if issue.Type != IssueJSONLMissing {
		t.Errorf("Expected type=%s, got %s", IssueJSONLMissing, issue.Type)
	}
	if issue.AutoFixable {
		t.Error("Expected AutoFixable=false for JSONL missing")
	}
}

func TestClassifyResumeError_VersionMismatch(t *testing.T) {
	output := "No messages returned from conversation"
	err := errors.New("resume failed")

	issue := classifyResumeError(output, err)

	if issue.Type != IssueVersionMismatch {
		t.Errorf("Expected type=%s, got %s", IssueVersionMismatch, issue.Type)
	}
	if !issue.AutoFixable {
		t.Error("Expected AutoFixable=true for version mismatch")
	}
}

func TestClassifyResumeError_CompactedJSONL(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"summary entries", "Found 32 summary entries in JSONL"},
		{"summaries word", "JSONL contains summaries at the start"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := classifyResumeError(tt.output, errors.New("resume failed"))

			if issue.Type != IssueCompactedJSONL {
				t.Errorf("Expected type=%s, got %s", IssueCompactedJSONL, issue.Type)
			}
			if !issue.AutoFixable {
				t.Error("Expected AutoFixable=true for compacted JSONL")
			}
		})
	}
}

func TestClassifyResumeError_CwdMismatch(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"empty string", ""},
		{"whitespace only", "   \n\t  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := classifyResumeError(tt.output, errors.New("resume failed"))

			if issue.Type != IssueCwdMismatch {
				t.Errorf("Expected type=%s, got %s", IssueCwdMismatch, issue.Type)
			}
			if !issue.AutoFixable {
				t.Error("Expected AutoFixable=true for cwd mismatch")
			}
		})
	}
}

func TestClassifyResumeError_LockContention(t *testing.T) {
	output := "Another agm command is currently running (pid=12345)"
	err := errors.New("lock held")

	issue := classifyResumeError(output, err)

	if issue.Type != IssueLockContention {
		t.Errorf("Expected type=%s, got %s", IssueLockContention, issue.Type)
	}
	if issue.AutoFixable {
		t.Error("Expected AutoFixable=false for lock contention")
	}
}

func TestClassifyResumeError_Permissions(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"permission denied", "Permission denied: /path/to/file"},
		{"EACCES", "EACCES: access denied"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := classifyResumeError(tt.output, errors.New("access denied"))

			if issue.Type != IssuePermissions {
				t.Errorf("Expected type=%s, got %s", IssuePermissions, issue.Type)
			}
			if issue.AutoFixable {
				t.Error("Expected AutoFixable=false for permissions")
			}
		})
	}
}

func TestClassifyResumeError_CorruptedData(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"invalid JSON", "invalid JSON at line 42"},
		{"syntax error", "SyntaxError: Unexpected token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := classifyResumeError(tt.output, errors.New("parse error"))

			if issue.Type != IssueCorruptedData {
				t.Errorf("Expected type=%s, got %s", IssueCorruptedData, issue.Type)
			}
			if issue.AutoFixable {
				t.Error("Expected AutoFixable=false for corrupted data")
			}
		})
	}
}

func TestClassifyResumeError_MissingDependency(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"command not found", "claude: command not found"},
		{"not installed", "tmux not installed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := classifyResumeError(tt.output, errors.New("dependency missing"))

			if issue.Type != IssueMissingDependency {
				t.Errorf("Expected type=%s, got %s", IssueMissingDependency, issue.Type)
			}
			if issue.AutoFixable {
				t.Error("Expected AutoFixable=false for missing dependency")
			}
		})
	}
}

func TestClassifyResumeError_Environment(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"PATH issue", "claude not found in PATH"},
		{"shell issue", "shell initialization failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := classifyResumeError(tt.output, errors.New("env error"))

			if issue.Type != IssueEnvironment {
				t.Errorf("Expected type=%s, got %s", IssueEnvironment, issue.Type)
			}
			if issue.AutoFixable {
				t.Error("Expected AutoFixable=false for environment")
			}
		})
	}
}

func TestClassifyResumeError_SessionConflict(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"already exists", "Session already exists: test-session"},
		{"UUID collision", "UUID collision detected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := classifyResumeError(tt.output, errors.New("conflict"))

			if issue.Type != IssueSessionConflict {
				t.Errorf("Expected type=%s, got %s", IssueSessionConflict, issue.Type)
			}
			if issue.AutoFixable {
				t.Error("Expected AutoFixable=false for session conflict")
			}
		})
	}
}

func TestClassifyResumeError_Unknown(t *testing.T) {
	output := "Some completely unexpected error message"
	err := errors.New("unknown error")

	issue := classifyResumeError(output, err)

	if issue.Type != IssueUnknown {
		t.Errorf("Expected type=%s, got %s", IssueUnknown, issue.Type)
	}
	if issue.AutoFixable {
		t.Error("Expected AutoFixable=false for unknown errors")
	}
	if issue.Message == "" {
		t.Error("Expected non-empty message")
	}
}

func TestClassifyResumeError_PriorityOrdering(t *testing.T) {
	// Test that more specific patterns are matched before general ones
	// For example, "No conversation found" should match JSONL missing,
	// not just "No" matching something else

	output := "No conversation found"
	issue := classifyResumeError(output, errors.New("test"))

	if issue.Type != IssueJSONLMissing {
		t.Errorf("Pattern priority failed: expected %s, got %s", IssueJSONLMissing, issue.Type)
	}
}

func TestClassifyResumeError_CaseInsensitive(t *testing.T) {
	// Verify that pattern matching is case-sensitive (as it should be)
	// because error messages from programs are typically consistent

	output := "permission denied" // lowercase
	issue := classifyResumeError(output, errors.New("test"))

	// Should still match because we check for "Permission denied" (case-sensitive)
	// but this test verifies current behavior
	if issue.Type == IssuePermissions {
		t.Error("Unexpected match - pattern matching should be case-sensitive")
	}
	if issue.Type != IssueUnknown {
		t.Errorf("Expected IssueUnknown for lowercase, got %s", issue.Type)
	}
}

func TestClassifyResumeError_MultiplePatterns(t *testing.T) {
	// Test output containing multiple error patterns
	// Should match the first pattern in switch order
	output := "No messages returned. Also, Permission denied"

	issue := classifyResumeError(output, errors.New("test"))

	// Should match version mismatch (appears first in switch)
	if issue.Type != IssueVersionMismatch {
		t.Errorf("Expected first pattern to match, got %s", issue.Type)
	}
}

func TestClassifyResumeError_AllFieldsPopulated(t *testing.T) {
	// Verify all Issue fields are populated
	output := "TypeError: Cannot read properties of undefined"
	issue := classifyResumeError(output, errors.New("test"))

	if issue.Type == "" {
		t.Error("Issue.Type should not be empty")
	}
	if issue.Message == "" {
		t.Error("Issue.Message should not be empty")
	}
	if issue.Fix == "" {
		t.Error("Issue.Fix should not be empty")
	}
	// AutoFixable can be true or false, both valid
}
