package roadmap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateBeadID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid simple ID",
			id:      "oss-task-123",
			wantErr: false,
		},
		{
			name:    "valid ID with hyphens",
			id:      "oss-roadmap-parser-v2",
			wantErr: false,
		},
		{
			name:    "valid ID with numbers",
			id:      "oss-test123",
			wantErr: false,
		},
		{
			name:    "invalid prefix",
			id:      "task-123",
			wantErr: true,
			errMsg:  "must start with 'oss-'",
		},
		{
			name:    "uppercase letters",
			id:      "oss-Task-ABC",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "special characters",
			id:      "oss-task_123",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "too short",
			id:      "oss-",
			wantErr: true,
			errMsg:  "too short",
		},
		{
			name:    "minimal valid",
			id:      "oss-a",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBeadID(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateBeadID(%q) expected error, got nil", tt.id)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateBeadID(%q) error = %v, want error containing %q", tt.id, err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateBeadID(%q) unexpected error: %v", tt.id, err)
				}
			}
		})
	}
}

func TestValidateROADMAP(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantErrors int
		checkMsgs  []string
	}{
		{
			name: "valid ROADMAP",
			content: `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | First task | 1 day | ✅ COMPLETE |
| ` + "`oss-task-2`" + ` | Second task | 2 hours | 📋 PLANNED |
`,
			wantErrors: 0,
		},
		{
			name: "duplicate bead IDs",
			content: `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-dup`" + ` | First | 1 day | ✅ COMPLETE |
| ` + "`oss-dup`" + ` | Second | 2 hours | 📋 PLANNED |
`,
			wantErrors: 1,
			checkMsgs:  []string{"Duplicate bead ID"},
		},
		{
			name: "invalid status",
			content: `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` | Task | 1 day | INVALID_STATUS |
`,
			wantErrors: 1,
			checkMsgs:  []string{"Invalid status"},
		},
		{
			name: "orphan beads (no phase)",
			content: `# Test ROADMAP

Some intro text

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-orphan`" + ` | Orphan task | 1 day | ✅ COMPLETE |
`,
			wantErrors: 1,
			checkMsgs:  []string{"not assigned to any phase"},
		},
		{
			name: "empty description",
			content: `# Test ROADMAP

## Phase 0: Setup

| Bead ID | Description | Effort | Status |
|---------|-------------|--------|--------|
| ` + "`oss-task-1`" + ` |  | 1 day | ✅ COMPLETE |
`,
			wantErrors: 1,
			checkMsgs:  []string{"empty description"},
		},
		{
			name: "multiple errors",
			content: `# Test ROADMAP

| ` + "`oss-orphan1`" + ` | Orphan | 1 day | ✅ |

## Phase 0: Setup

| ` + "`oss-dup`" + ` | First | 1 day | ✅ |
| ` + "`oss-dup`" + ` | Second | 1 day | ✅ |
| ` + "`oss-empty`" + ` |  | 1 day | ✅ |
| ` + "`oss-bad-status`" + ` | Bad | 1 day | WRONG |
`,
			wantErrors: 4, // 1 orphan + 1 duplicate + 1 empty desc + 1 invalid status
			checkMsgs:  []string{"not assigned to any phase", "Duplicate", "empty description", "Invalid status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp ROADMAP file
			tempDir := t.TempDir()
			roadmapPath := filepath.Join(tempDir, "ROADMAP.md")
			if err := os.WriteFile(roadmapPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to create test ROADMAP: %v", err)
			}

			errors := ValidateROADMAP(roadmapPath)

			if len(errors) != tt.wantErrors {
				t.Errorf("ValidateROADMAP() returned %d errors, want %d", len(errors), tt.wantErrors)
				for i, err := range errors {
					t.Logf("  Error %d: %s", i+1, err.Message)
				}
			}

			// Check for expected error messages
			for _, checkMsg := range tt.checkMsgs {
				found := false
				for _, err := range errors {
					if contains(err.Message, checkMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing %q, but not found in errors", checkMsg)
				}
			}
		})
	}
}

func TestValidateROADMAP_FileNotFound(t *testing.T) {
	errors := ValidateROADMAP("/nonexistent/ROADMAP.md")
	if len(errors) == 0 {
		t.Error("ValidateROADMAP() with non-existent file should return errors")
	}
	if !contains(errors[0].Message, "Failed to parse") {
		t.Errorf("Expected parse error, got: %s", errors[0].Message)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && s[:len(substr)] == substr) ||
		(len(s) > len(substr) && (s[len(s)-len(substr):] == substr ||
		findSubstring(s, substr))))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
