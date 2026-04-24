package validator

import (
	"os"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestNewValidator(t *testing.T) {
	s := &status.Status{
		CurrentPhase: "D1",
	}

	v := NewValidator(s)

	if v == nil {
		t.Fatal("NewValidator returned nil")
	}

	if v.status != s {
		t.Error("NewValidator did not set status correctly")
	}
}

func TestCanStartPhase(t *testing.T) {
	tests := []struct {
		name        string
		status      *status.Status
		phaseName   string
		expectError bool
		errorMsg    string
	}{
		{
			name: "first phase W0 - no prerequisites",
			status: &status.Status{
				CurrentPhase: "",
				Phases:       []status.Phase{},
			},
			phaseName:   "W0",
			expectError: false,
		},
		{
			name: "D1 when W0 completed",
			status: &status.Status{
				CurrentPhase: "W0",
				Phases: []status.Phase{
					{Name: "W0", Status: status.PhaseStatusCompleted},
				},
			},
			phaseName:   "D1",
			expectError: false,
		},
		{
			name: "D2 when D1 not completed",
			status: &status.Status{
				CurrentPhase: "D1",
				Phases: []status.Phase{
					{Name: "W0", Status: status.PhaseStatusCompleted},
					{Name: "D1", Status: status.PhaseStatusInProgress},
				},
			},
			phaseName:   "D2",
			expectError: true,
			errorMsg:    "previous phase D1 is not completed",
		},
		{
			name: "phase already in progress",
			status: &status.Status{
				CurrentPhase: "D2",
				Phases: []status.Phase{
					{Name: "D1", Status: status.PhaseStatusCompleted},
					{Name: "D2", Status: status.PhaseStatusInProgress},
				},
			},
			phaseName:   "D2",
			expectError: true,
			errorMsg:    "phase is already in progress",
		},
		{
			name: "phase already completed",
			status: &status.Status{
				CurrentPhase: "D3",
				Phases: []status.Phase{
					{Name: "D1", Status: status.PhaseStatusCompleted},
					{Name: "D2", Status: status.PhaseStatusCompleted},
				},
			},
			phaseName:   "D2",
			expectError: true,
			errorMsg:    "phase is already completed",
		},
		{
			name: "invalid phase name",
			status: &status.Status{
				CurrentPhase: "D1",
				Phases:       []status.Phase{},
			},
			phaseName:   "X99",
			expectError: true,
			errorMsg:    "phase does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(tt.status)
			err := v.CanStartPhase(tt.phaseName, "/tmp/test-project")

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr))))
}

func TestCanCompletePhase(t *testing.T) {
	tests := []struct {
		name        string
		status      *status.Status
		phaseName   string
		expectError bool
		errorMsg    string
	}{
		{
			name: "complete phase in progress",
			status: &status.Status{
				CurrentPhase: "D1",
				Phases: []status.Phase{
					{Name: "D1", Status: status.PhaseStatusInProgress},
				},
			},
			phaseName:   "D1",
			expectError: false,
		},
		{
			name: "complete phase not started",
			status: &status.Status{
				CurrentPhase: "",
				Phases:       []status.Phase{},
			},
			phaseName:   "D1",
			expectError: true,
			errorMsg:    "phase was never started",
		},
		{
			name: "complete already completed phase",
			status: &status.Status{
				CurrentPhase: "D2",
				Phases: []status.Phase{
					{Name: "D1", Status: status.PhaseStatusCompleted},
				},
			},
			phaseName:   "D1",
			expectError: true,
			errorMsg:    "phase is already completed",
		},
		{
			name: "complete invalid phase",
			status: &status.Status{
				CurrentPhase: "D1",
				Phases:       []status.Phase{},
			},
			phaseName:   "X99",
			expectError: true,
			errorMsg:    "phase does not exist",
		},
		{
			name: "complete pending phase",
			status: &status.Status{
				CurrentPhase: "D1",
				Phases: []status.Phase{
					{Name: "D1", Status: status.PhaseStatusPending},
				},
			},
			phaseName:   "D1",
			expectError: true,
			errorMsg:    "phase is not in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp project directory for deliverable validation
			tmpDir := t.TempDir()

			// For tests that expect success, create a minimal valid deliverable
			if !tt.expectError {
				// Create engram file
				engramContent := "# Test engram content\n"
				engramPath := tmpDir + "/test-engram.md"
				os.WriteFile(engramPath, []byte(engramContent), 0600)

				// Calculate hash of engram
				hash, _ := calculatePhaseEngramHash(engramPath)

				// Create deliverable file with valid frontmatter
				deliverableContent := `---
phase: "` + tt.phaseName + `"
phase_name: "Test Phase"
wayfinder_session_id: "test-session"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "` + hash + `"
phase_engram_path: "` + engramPath + `"
---

# Test deliverable content (>100 bytes to pass size validation)
This is a test deliverable with enough content to pass the 100-byte minimum size requirement.
`
				os.WriteFile(tmpDir+"/"+tt.phaseName+"-test.md", []byte(deliverableContent), 0600)
			}

			v := NewValidator(tt.status)
			err := v.CanCompletePhase(tt.phaseName, tmpDir, "")

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestCanRewindTo(t *testing.T) {
	tests := []struct {
		name        string
		status      *status.Status
		targetPhase string
		expectError bool
		errorMsg    string
	}{
		{
			name: "rewind to earlier completed phase",
			status: &status.Status{
				CurrentPhase: "D4",
				Phases: []status.Phase{
					{Name: "D1", Status: status.PhaseStatusCompleted},
					{Name: "D2", Status: status.PhaseStatusCompleted},
					{Name: "D3", Status: status.PhaseStatusCompleted},
					{Name: "D4", Status: status.PhaseStatusInProgress},
				},
			},
			targetPhase: "D2",
			expectError: false,
		},
		{
			name: "rewind to phase that was never started",
			status: &status.Status{
				CurrentPhase: "D4",
				Phases: []status.Phase{
					{Name: "D4", Status: status.PhaseStatusInProgress},
				},
			},
			targetPhase: "D2",
			expectError: true,
			errorMsg:    "target phase was never started",
		},
		{
			name: "rewind to non-completed phase",
			status: &status.Status{
				CurrentPhase: "D4",
				Phases: []status.Phase{
					{Name: "D2", Status: status.PhaseStatusInProgress},
					{Name: "D4", Status: status.PhaseStatusInProgress},
				},
			},
			targetPhase: "D2",
			expectError: true,
			errorMsg:    "target phase is not completed",
		},
		{
			name: "rewind to current phase (not actually a rewind)",
			status: &status.Status{
				CurrentPhase: "D3",
				Phases: []status.Phase{
					{Name: "D3", Status: status.PhaseStatusCompleted},
				},
			},
			targetPhase: "D3",
			expectError: true,
			errorMsg:    "target phase is not before current phase",
		},
		{
			name: "rewind to later phase (forward, not backward)",
			status: &status.Status{
				CurrentPhase: "D2",
				Phases: []status.Phase{
					{Name: "D2", Status: status.PhaseStatusInProgress},
					{Name: "D4", Status: status.PhaseStatusCompleted},
				},
			},
			targetPhase: "D4",
			expectError: true,
			errorMsg:    "target phase is not before current phase",
		},
		{
			name: "rewind to invalid phase",
			status: &status.Status{
				CurrentPhase: "D3",
				Phases:       []status.Phase{},
			},
			targetPhase: "X99",
			expectError: true,
			errorMsg:    "phase does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(tt.status)
			err := v.CanRewindTo(tt.targetPhase)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := NewValidationError(
		"start D2",
		"phase D1 is not completed",
		"Complete D1 first",
	)

	expected := "cannot start D2: phase D1 is not completed. Fix: Complete D1 first"
	if err.Error() != expected {
		t.Errorf("ValidationError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestValidationError_NoFix(t *testing.T) {
	err := NewValidationError(
		"start D2",
		"phase D1 is not completed",
		"",
	)

	expected := "cannot start D2: phase D1 is not completed"
	if err.Error() != expected {
		t.Errorf("ValidationError.Error() = %q, want %q", err.Error(), expected)
	}
}
