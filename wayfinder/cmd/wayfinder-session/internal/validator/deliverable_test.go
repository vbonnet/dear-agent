package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDeliverableExists(t *testing.T) {
	tests := []struct {
		name        string
		phaseName   string
		files       []string // Files to create in temp dir
		wantErr     bool
		errContains string
	}{
		{
			name:      "deliverable exists - single file",
			phaseName: "PROBLEM",
			files:     []string{"PROBLEM-validation.md"},
			wantErr:   false,
		},
		{
			name:      "deliverable exists - multiple matches",
			phaseName: "PLAN",
			files:     []string{"PLAN-research.md", "PLAN-additional-notes.md"},
			wantErr:   false,
		},
		{
			name:        "no deliverable found",
			phaseName:   "RESEARCH",
			files:       []string{"PROBLEM-validation.md"},
			wantErr:     true,
			errContains: "no deliverable file found matching pattern RESEARCH-*.md",
		},
		{
			name:      "CHARTER optional - not started",
			phaseName: "CHARTER",
			files:     []string{"PROBLEM-validation.md"},
			wantErr:   false, // CHARTER is optional, no error if missing
		},
		{
			name:      "CHARTER exists",
			phaseName: "CHARTER",
			files:     []string{"CHARTER-intake.md"},
			wantErr:   false,
		},
		{
			name:      "case sensitive matching",
			phaseName: "problem",
			files:     []string{"PROBLEM-validation.md"},
			wantErr:   true, // problem != PROBLEM
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()

			// Create test files
			for _, filename := range tt.files {
				filePath := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(filePath, []byte("test content"), 0600); err != nil {
					t.Fatalf("failed to create test file %s: %v", filename, err)
				}
			}

			// Validate
			err := validateDeliverableExists(tmpDir, tt.phaseName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateDeliverableExists() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validateDeliverableExists() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("validateDeliverableExists() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateDeliverableSize(t *testing.T) {
	tests := []struct {
		name        string
		phaseName   string
		content     string
		wantErr     bool
		errContains string
	}{
		{
			name:      "deliverable >= 100 bytes",
			phaseName: "PROBLEM",
			content:   string(make([]byte, 100)), // Exactly 100 bytes
			wantErr:   false,
		},
		{
			name:      "deliverable >> 100 bytes",
			phaseName: "PLAN",
			content:   string(make([]byte, 1000)), // 1000 bytes
			wantErr:   false,
		},
		{
			name:        "deliverable < 100 bytes - empty",
			phaseName:   "RESEARCH",
			content:     "",
			wantErr:     true,
			errContains: "is too small (0 bytes, minimum 100 bytes)",
		},
		{
			name:        "deliverable < 100 bytes - stub",
			phaseName:   "DESIGN",
			content:     "# DESIGN: Approach Decision\n\nTODO",
			wantErr:     true,
			errContains: "is too small",
		},
		{
			name:      "deliverable exactly 100 bytes",
			phaseName: "SPEC",
			content:   string(make([]byte, 100)),
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()

			// Create deliverable file
			filename := filepath.Join(tmpDir, tt.phaseName+"-test.md")
			if err := os.WriteFile(filename, []byte(tt.content), 0600); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Validate
			err := validateDeliverableSize(tmpDir, tt.phaseName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateDeliverableSize() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validateDeliverableSize() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("validateDeliverableSize() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateDeliverableSize_NoDeliverable(t *testing.T) {
	tmpDir := t.TempDir()
	// No deliverable file created

	// Should not error (validateDeliverableExists catches this case)
	err := validateDeliverableSize(tmpDir, "PROBLEM")
	if err != nil {
		t.Errorf("validateDeliverableSize() with no deliverable should return nil, got %v", err)
	}
}

func TestValidateS8Implementation(t *testing.T) {
	tests := []struct {
		name        string
		files       []string
		wantErr     bool
		errContains string
	}{
		{
			name:    "BUILD markdown deliverable exists",
			files:   []string{"BUILD-implementation.md"},
			wantErr: false,
		},
		{
			name:    "code file exists - Go",
			files:   []string{"main.go", "utils.go"},
			wantErr: false,
		},
		{
			name:    "code file exists - Python",
			files:   []string{"script.py"},
			wantErr: false,
		},
		{
			name:    "code file exists - JavaScript",
			files:   []string{"index.js", "app.tsx"},
			wantErr: false,
		},
		{
			name:    "both BUILD deliverable and code",
			files:   []string{"BUILD-implementation.md", "main.go"},
			wantErr: false,
		},
		{
			name:        "no BUILD deliverable or code",
			files:       []string{"PROBLEM-analysis.md", "README.md"},
			wantErr:     true,
			errContains: "no implementation artifacts found",
		},
		{
			name:        "only non-code files",
			files:       []string{"README.md", "notes.txt", "data.json"},
			wantErr:     true,
			errContains: "no implementation artifacts found",
		},
		{
			name:    "code in multiple languages",
			files:   []string{"main.cpp", "helper.py", "script.sh"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()

			// Create test files
			for _, filename := range tt.files {
				filePath := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(filePath, []byte("test content"), 0600); err != nil {
					t.Fatalf("failed to create test file %s: %v", filename, err)
				}
			}

			// Validate
			err := validateBuildImplementation(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateBuildImplementation() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validateBuildImplementation() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("validateBuildImplementation() unexpected error = %v", err)
			}
		})
	}
}
