package validator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestIntegration_CanCompletePhase_EndToEnd tests the complete validation workflow
// from start to finish, exercising all validation layers:
// 1. Phase state validation
// 2. Deliverable file existence
// 3. Deliverable file size
// 4. S8 implementation artifacts (if applicable)
// 5. Methodology freshness (hash validation)
func TestIntegration_CanCompletePhase_EndToEnd(t *testing.T) {
	// Skip integration tests if ANTHROPIC_API_KEY not available (review-spec skill requires it)
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("Skipping integration test: ANTHROPIC_API_KEY not set")
	}

	tests := []struct {
		name               string
		phaseName          string
		setupProject       func(t *testing.T, projectDir string) string // Returns engram path
		setupStatus        func() *status.Status
		hashMismatchReason string
		wantErr            bool
		errContains        string
	}{
		{
			name:      "happy path - all validations pass",
			phaseName: "D1",
			setupProject: func(t *testing.T, projectDir string) string {
				engramPath := filepath.Join(projectDir, "d1-engram.md")
				hash := createEngramFile(t, engramPath, "# D1 Phase Methodology\n\nContent for D1 phase.\n")
				deliverableContent := `# D1: Problem Validation

## Problem Statement

This is a comprehensive deliverable with sufficient content to pass size validation.
We are validating the problem domain and ensuring all validation layers work correctly.

## Key Findings

1. Finding one
2. Finding two
3. Finding three
`
				createDeliverableFile(t, filepath.Join(projectDir, "D1-problem-validation.md"), "D1", "Problem Validation", engramPath, hash, deliverableContent)
				return engramPath
			},
			setupStatus: func() *status.Status {
				return createSimpleStatus("D1")
			},
			wantErr: false,
		},
		{
			name:      "fail - no deliverable file",
			phaseName: "D2",
			setupProject: func(t *testing.T, projectDir string) string {
				return ""
			},
			setupStatus: func() *status.Status {
				return createSimpleStatus("D2")
			},
			wantErr:     true,
			errContains: "no deliverable file found matching pattern D2-*.md",
		},
		{
			name:      "fail - deliverable too small",
			phaseName: "D3",
			setupProject: func(t *testing.T, projectDir string) string {
				createCodeFile(t, filepath.Join(projectDir, "D3-approach.md"), "# D3: Stub")
				return ""
			},
			setupStatus: func() *status.Status {
				return createSimpleStatus("D3")
			},
			wantErr:     true,
			errContains: "is too small",
		},
		{
			name:      "fail - hash mismatch without override",
			phaseName: "D4",
			setupProject: func(t *testing.T, projectDir string) string {
				engramPath := filepath.Join(projectDir, "d4-engram.md")
				createEngramFile(t, engramPath, "# D4 Phase Methodology\n\nCurrent version.\n")
				deliverableContent := `# D4: Solution Requirements

This deliverable was created with an outdated methodology version.
It has sufficient content to pass size validation, but the hash is wrong.

## Requirements

1. Requirement one
2. Requirement two
`
				createDeliverableFile(t, filepath.Join(projectDir, "D4-solution-requirements.md"),
					"D4", "Solution Requirements", engramPath, "sha256:outdatedhash123", deliverableContent)
				// Create SPEC.md for documentation quality gate
				specContent := "# SPEC - High Quality Documentation\n\nThis is a placeholder SPEC.md for testing.\n"
				createCodeFile(t, filepath.Join(projectDir, "SPEC.md"), specContent)
				return engramPath
			},
			setupStatus: func() *status.Status {
				return createSimpleStatus("D4")
			},
			hashMismatchReason: "", // No override
			wantErr:            true,
			errContains:        "outdated methodology (hash mismatch",
		},
		{
			name:      "success - hash mismatch with override",
			phaseName: "D4",
			setupProject: func(t *testing.T, projectDir string) string {
				engramPath := filepath.Join(projectDir, "d4-engram.md")
				createEngramFile(t, engramPath, "# D4 Phase Methodology\n\nCurrent version.\n")
				deliverableContent := `# D4: Solution Requirements

This deliverable was created with an outdated methodology version.
But we're overriding the validation with a reason.

## Requirements

1. Requirement one
2. Requirement two
`
				createDeliverableFile(t, filepath.Join(projectDir, "D4-solution-requirements.md"),
					"D4", "Solution Requirements", engramPath, "sha256:outdatedhash123", deliverableContent)
				// Create SPEC.md for documentation quality gate
				specContent := "# SPEC - High Quality Documentation\n\nThis is a placeholder SPEC.md for testing.\n"
				createCodeFile(t, filepath.Join(projectDir, "SPEC.md"), specContent)
				return engramPath
			},
			setupStatus: func() *status.Status {
				return createSimpleStatus("D4")
			},
			hashMismatchReason: "Reviewed methodology changes, deliverable still valid",
			wantErr:            false,
		},
		{
			name:      "success - S8 with code files",
			phaseName: "S8",
			setupProject: func(t *testing.T, projectDir string) string {
				// Create go.mod for valid Go module
				createCodeFile(t, filepath.Join(projectDir, "go.mod"), "module example.com/test\n\ngo 1.24\n")

				// Create code files instead of S8 deliverable
				createCodeFile(t, filepath.Join(projectDir, "main.go"), "package main\n\nfunc main() {}\n")
				createCodeFile(t, filepath.Join(projectDir, "utils.go"), "package main\n\nfunc helper() string { return \"test\" }\n")

				// Create test file (required by compilation gate)
				createCodeFile(t, filepath.Join(projectDir, "main_test.go"),
					"package main\n\nimport \"testing\"\n\nfunc TestHelper(t *testing.T) {\n\tif helper() != \"test\" {\n\t\tt.Error(\"helper failed\")\n\t}\n}\n")

				// Still need a deliverable (can be any name for S8)
				engramPath := filepath.Join(projectDir, "s8-engram.md")
				hash := createEngramFile(t, engramPath, "# S8 Implementation\n")

				deliverableContent := `# Implementation Notes

Implemented in Go files: main.go and utils.go.
`
				createDeliverableFile(t, filepath.Join(projectDir, "S8-notes.md"),
					"S8", "Implementation", engramPath, hash, deliverableContent)

				return engramPath
			},
			setupStatus: func() *status.Status {
				return createSimpleStatus("S8")
			},
			wantErr: false,
		},
		{
			name:      "fail - S8 with no code or deliverable",
			phaseName: "S8",
			setupProject: func(t *testing.T, projectDir string) string {
				// Create minimal S8 deliverable WITHOUT S8-*.md pattern and WITHOUT code files
				engramPath := filepath.Join(projectDir, "s8-engram.md")
				hash := createEngramFile(t, engramPath, "# S8 Implementation\n")

				// Create deliverable with non-S8 pattern (e.g., "notes.md")
				deliverableContent := `# Implementation Notes

No actual implementation artifacts.
`
				createDeliverableFile(t, filepath.Join(projectDir, "notes.md"),
					"S8", "Implementation", engramPath, hash, deliverableContent)

				return engramPath
			},
			setupStatus: func() *status.Status {
				return createSimpleStatus("S8")
			},
			wantErr:     true,
			errContains: "no deliverable file found matching pattern S8-*.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary project directory
			projectDir := t.TempDir()

			// Setup project files
			tt.setupProject(t, projectDir)

			// Setup status
			st := tt.setupStatus()

			// Create validator
			v := NewValidator(st)

			// Run validation
			err := v.CanCompletePhase(tt.phaseName, projectDir, tt.hashMismatchReason)

			// Check result
			if tt.wantErr {
				if err == nil {
					t.Errorf("CanCompletePhase() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("CanCompletePhase() error = %q, want substring %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("CanCompletePhase() unexpected error = %v", err)
				}
			}
		})
	}
}

// Test helper functions

// createEngramFile creates an engram file with content and returns its hash
func createEngramFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create engram: %v", err)
	}
	hash, _ := calculatePhaseEngramHash(path)
	return hash
}

// createDeliverableFile creates a deliverable file with frontmatter
func createDeliverableFile(t *testing.T, path, phase, phaseName, engramPath, hash, content string) {
	t.Helper()
	fullContent := `---
phase: "` + phase + `"
phase_name: "` + phaseName + `"
wayfinder_session_id: "integration-test-123"
created_at: "2026-01-05T14:00:00Z"
phase_engram_hash: "` + hash + `"
phase_engram_path: "` + engramPath + `"
---

` + content
	if err := os.WriteFile(path, []byte(fullContent), 0600); err != nil {
		t.Fatalf("failed to create deliverable: %v", err)
	}
}

// createCodeFile creates a code file with content
func createCodeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create code file: %v", err)
	}
}

// createSimpleStatus creates a status object with the given phase
func createSimpleStatus(phase string) *status.Status {
	return &status.Status{
		CurrentPhase: phase,
		Phases: []status.Phase{
			{Name: phase, Status: status.PhaseStatusInProgress},
		},
	}
}
