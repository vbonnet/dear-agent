package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestFileMigrator_MigrateS4ToD4(t *testing.T) {
	tests := []struct {
		name          string
		s4Files       map[string]string // filename -> content
		existingD4    string
		wantD4Content string
		wantErr       bool
	}{
		{
			name: "migrate single S4 file to new D4",
			s4Files: map[string]string{
				"S4-stakeholder-approval.md": "Approved by John Doe on 2026-02-15",
			},
			existingD4:    "",
			wantD4Content: "## Stakeholder Decisions",
		},
		{
			name: "migrate multiple S4 files",
			s4Files: map[string]string{
				"S4-stakeholder-1.md": "Stakeholder 1 feedback",
				"S4-stakeholder-2.md": "Stakeholder 2 approval",
			},
			existingD4:    "",
			wantD4Content: "S4-stakeholder-1",
		},
		{
			name: "append to existing D4",
			s4Files: map[string]string{
				"S4-approval.md": "Final approval",
			},
			existingD4:    "# D4 Requirements\n\nExisting content\n",
			wantD4Content: "Existing content",
		},
		{
			name:          "no S4 files - no changes",
			s4Files:       map[string]string{},
			existingD4:    "# D4 Requirements\n",
			wantD4Content: "# D4 Requirements",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()

			// Create S4 files
			for filename, content := range tt.s4Files {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create S4 file: %v", err)
				}
			}

			// Create existing D4 if specified
			if tt.existingD4 != "" {
				d4Path := filepath.Join(tmpDir, "D4-requirements.md")
				if err := os.WriteFile(d4Path, []byte(tt.existingD4), 0644); err != nil {
					t.Fatalf("failed to create D4 file: %v", err)
				}
			}

			// Run migration
			fm := NewFileMigrator(tmpDir)
			err := fm.migrateS4ToD4()

			if (err != nil) != tt.wantErr {
				t.Errorf("migrateS4ToD4() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify D4 content
			d4Path := filepath.Join(tmpDir, "D4-requirements.md")
			d4Content, err := os.ReadFile(d4Path)
			if err != nil && len(tt.s4Files) > 0 {
				t.Fatalf("D4 file should exist after migration: %v", err)
			}

			if len(tt.s4Files) > 0 && !strings.Contains(string(d4Content), tt.wantD4Content) {
				t.Errorf("D4 content does not contain expected text: %q\nGot: %s",
					tt.wantD4Content, string(d4Content))
			}
		})
	}
}

func TestFileMigrator_MigrateS5ToS6(t *testing.T) {
	tests := []struct {
		name          string
		s5Files       map[string]string
		existingS6    string
		wantS6Content string
		wantErr       bool
	}{
		{
			name: "migrate single S5 file to new S6",
			s5Files: map[string]string{
				"S5-research.md": "Research findings on technology X",
			},
			existingS6:    "",
			wantS6Content: "## Research Notes",
		},
		{
			name: "migrate multiple S5 files",
			s5Files: map[string]string{
				"S5-tech-research.md":   "Technology research",
				"S5-design-patterns.md": "Design pattern analysis",
			},
			existingS6:    "",
			wantS6Content: "S5-tech-research",
		},
		{
			name: "append to existing S6",
			s5Files: map[string]string{
				"S5-additional.md": "Additional research",
			},
			existingS6:    "# S6 Design\n\nExisting design\n",
			wantS6Content: "Existing design",
		},
		{
			name:          "no S5 files - no changes",
			s5Files:       map[string]string{},
			existingS6:    "# S6 Design\n",
			wantS6Content: "# S6 Design",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create S5 files
			for filename, content := range tt.s5Files {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create S5 file: %v", err)
				}
			}

			// Create existing S6 if specified
			if tt.existingS6 != "" {
				s6Path := filepath.Join(tmpDir, "S6-design.md")
				if err := os.WriteFile(s6Path, []byte(tt.existingS6), 0644); err != nil {
					t.Fatalf("failed to create S6 file: %v", err)
				}
			}

			// Run migration
			fm := NewFileMigrator(tmpDir)
			err := fm.migrateS5ToS6()

			if (err != nil) != tt.wantErr {
				t.Errorf("migrateS5ToS6() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify S6 content
			s6Path := filepath.Join(tmpDir, "S6-design.md")
			s6Content, err := os.ReadFile(s6Path)
			if err != nil && len(tt.s5Files) > 0 {
				t.Fatalf("S6 file should exist after migration: %v", err)
			}

			if len(tt.s5Files) > 0 && !strings.Contains(string(s6Content), tt.wantS6Content) {
				t.Errorf("S6 content does not contain expected text: %q\nGot: %s",
					tt.wantS6Content, string(s6Content))
			}
		})
	}
}

func TestFileMigrator_MigrateS8S9S10ToS8(t *testing.T) {
	tests := []struct {
		name               string
		s8Files            map[string]string
		s9Files            map[string]string
		s10Files           map[string]string
		wantS8BuildContent []string // multiple strings to check
		wantErr            bool
	}{
		{
			name: "migrate all three phases",
			s8Files: map[string]string{
				"S8-implementation.md": "Code implementation details",
			},
			s9Files: map[string]string{
				"S9-validation.md": "Validation results",
			},
			s10Files: map[string]string{
				"S10-deployment.md": "Deployment steps",
			},
			wantS8BuildContent: []string{
				"## Implementation (S8)",
				"## Validation (S9)",
				"## Deployment (S10)",
			},
		},
		{
			name: "migrate only S8 and S9",
			s8Files: map[string]string{
				"S8-code.md": "Code changes",
			},
			s9Files: map[string]string{
				"S9-tests.md": "Test results",
			},
			s10Files: map[string]string{},
			wantS8BuildContent: []string{
				"## Implementation (S8)",
				"## Validation (S9)",
			},
		},
		{
			name: "multiple files per phase",
			s8Files: map[string]string{
				"S8-backend.md":  "Backend code",
				"S8-frontend.md": "Frontend code",
			},
			s9Files:  map[string]string{},
			s10Files: map[string]string{},
			wantS8BuildContent: []string{
				"S8-backend",
				"S8-frontend",
			},
		},
		{
			name:               "no files to migrate",
			s8Files:            map[string]string{},
			s9Files:            map[string]string{},
			s10Files:           map[string]string{},
			wantS8BuildContent: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create S8 files
			for filename, content := range tt.s8Files {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create S8 file: %v", err)
				}
			}

			// Create S9 files
			for filename, content := range tt.s9Files {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create S9 file: %v", err)
				}
			}

			// Create S10 files
			for filename, content := range tt.s10Files {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create S10 file: %v", err)
				}
			}

			// Run migration
			fm := NewFileMigrator(tmpDir)
			err := fm.migrateS8S9S10ToS8()

			if (err != nil) != tt.wantErr {
				t.Errorf("migrateS8S9S10ToS8() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check if any files existed
			totalFiles := len(tt.s8Files) + len(tt.s9Files) + len(tt.s10Files)
			if totalFiles == 0 {
				// No files, S8-build.md should not exist
				s8BuildPath := filepath.Join(tmpDir, "S8-build.md")
				if _, err := os.Stat(s8BuildPath); err == nil {
					t.Error("S8-build.md should not exist when no files to migrate")
				}
				return
			}

			// Verify S8-build.md content
			s8BuildPath := filepath.Join(tmpDir, "S8-build.md")
			s8BuildContent, err := os.ReadFile(s8BuildPath)
			if err != nil {
				t.Fatalf("S8-build.md should exist after migration: %v", err)
			}

			for _, want := range tt.wantS8BuildContent {
				if !strings.Contains(string(s8BuildContent), want) {
					t.Errorf("S8-build.md does not contain expected text: %q\nGot: %s",
						want, string(s8BuildContent))
				}
			}
		})
	}
}

func TestFileMigrator_GenerateTestsOutlineIfNeeded(t *testing.T) {
	tests := []struct {
		name               string
		d4Content          string
		existingOutline    string
		wantOutlineCreated bool
		wantOutlineContent string
	}{
		{
			name: "generate outline from D4",
			d4Content: `# D4 Requirements
## Functional Requirements
- User can log in
- User can log out
- Sessions expire after 1 hour
`,
			existingOutline:    "",
			wantOutlineCreated: true,
			wantOutlineContent: "AC1",
		},
		{
			name:               "D4 exists but outline already exists",
			d4Content:          "# D4 Requirements",
			existingOutline:    "# Existing outline",
			wantOutlineCreated: false,
		},
		{
			name:               "D4 doesn't exist",
			d4Content:          "",
			existingOutline:    "",
			wantOutlineCreated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create D4 file if specified
			if tt.d4Content != "" {
				d4Path := filepath.Join(tmpDir, "D4-requirements.md")
				if err := os.WriteFile(d4Path, []byte(tt.d4Content), 0644); err != nil {
					t.Fatalf("failed to create D4 file: %v", err)
				}
			}

			// Create existing outline if specified
			if tt.existingOutline != "" {
				outlinePath := filepath.Join(tmpDir, "TESTS.outline")
				if err := os.WriteFile(outlinePath, []byte(tt.existingOutline), 0644); err != nil {
					t.Fatalf("failed to create outline file: %v", err)
				}
			}

			// Run generation
			fm := NewFileMigrator(tmpDir)
			err := fm.generateTestsOutlineIfNeeded()
			if err != nil {
				t.Fatalf("generateTestsOutlineIfNeeded() error = %v", err)
			}

			// Check if outline was created
			outlinePath := filepath.Join(tmpDir, "TESTS.outline")
			outlineContent, err := os.ReadFile(outlinePath)
			outlineExists := err == nil

			// Check if outline was created (didn't exist before, exists now)
			wasCreated := (tt.existingOutline == "") && outlineExists
			if wasCreated != tt.wantOutlineCreated {
				t.Errorf("outline created = %v, want %v (existed before: %v, exists now: %v)",
					wasCreated, tt.wantOutlineCreated, tt.existingOutline != "", outlineExists)
			}

			// Verify content if outline should be created
			if tt.wantOutlineCreated && !strings.Contains(string(outlineContent), tt.wantOutlineContent) {
				t.Errorf("outline does not contain expected text: %q\nGot: %s",
					tt.wantOutlineContent, string(outlineContent))
			}
		})
	}
}

func TestFileMigrator_GenerateTestsFeatureIfNeeded(t *testing.T) {
	tests := []struct {
		name               string
		s6Content          string
		existingFeature    string
		wantFeatureCreated bool
		wantFeatureContent string
	}{
		{
			name: "generate feature from S6",
			s6Content: `# S6 Design
## Implementation Plan
- Implement login API
- Add authentication middleware
`,
			existingFeature:    "",
			wantFeatureCreated: true,
			wantFeatureContent: "Feature:",
		},
		{
			name:               "S6 exists but feature already exists",
			s6Content:          "# S6 Design",
			existingFeature:    "Feature: existing",
			wantFeatureCreated: false,
		},
		{
			name:               "S6 doesn't exist",
			s6Content:          "",
			existingFeature:    "",
			wantFeatureCreated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create S6 file if specified
			if tt.s6Content != "" {
				s6Path := filepath.Join(tmpDir, "S6-design.md")
				if err := os.WriteFile(s6Path, []byte(tt.s6Content), 0644); err != nil {
					t.Fatalf("failed to create S6 file: %v", err)
				}
			}

			// Create existing feature if specified
			if tt.existingFeature != "" {
				featurePath := filepath.Join(tmpDir, "TESTS.feature")
				if err := os.WriteFile(featurePath, []byte(tt.existingFeature), 0644); err != nil {
					t.Fatalf("failed to create feature file: %v", err)
				}
			}

			// Run generation
			fm := NewFileMigrator(tmpDir)
			err := fm.generateTestsFeatureIfNeeded()
			if err != nil {
				t.Fatalf("generateTestsFeatureIfNeeded() error = %v", err)
			}

			// Check if feature was created
			featurePath := filepath.Join(tmpDir, "TESTS.feature")
			featureContent, err := os.ReadFile(featurePath)
			featureExists := err == nil

			// Check if feature was created (didn't exist before, exists now)
			wasCreated := (tt.existingFeature == "") && featureExists
			if wasCreated != tt.wantFeatureCreated {
				t.Errorf("feature created = %v, want %v (existed before: %v, exists now: %v)",
					wasCreated, tt.wantFeatureCreated, tt.existingFeature != "", featureExists)
			}

			// Verify content if feature should be created
			if tt.wantFeatureCreated && !strings.Contains(string(featureContent), tt.wantFeatureContent) {
				t.Errorf("feature does not contain expected text: %q\nGot: %s",
					tt.wantFeatureContent, string(featureContent))
			}
		})
	}
}

func TestFileMigrator_Cleanup(t *testing.T) {
	tests := []struct {
		name         string
		files        map[string]string
		wantBackedUp []string
		wantErr      bool
	}{
		{
			name: "backup S4 and S5 files",
			files: map[string]string{
				"S4-approval.md":     "approval",
				"S5-research.md":     "research",
				"D4-requirements.md": "should not be backed up",
			},
			wantBackedUp: []string{"S4-approval.md", "S5-research.md"},
		},
		{
			name: "backup S9 and S10 files",
			files: map[string]string{
				"S9-validation.md":  "validation",
				"S10-deployment.md": "deployment",
			},
			wantBackedUp: []string{"S9-validation.md", "S10-deployment.md"},
		},
		{
			name: "no files to backup",
			files: map[string]string{
				"D4-requirements.md": "requirements",
				"S6-design.md":       "design",
			},
			wantBackedUp: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create test files
			for filename, content := range tt.files {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
			}

			// Run cleanup
			fm := NewFileMigrator(tmpDir)
			err := fm.Cleanup()

			if (err != nil) != tt.wantErr {
				t.Errorf("Cleanup() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify backed up files
			backupDir := filepath.Join(tmpDir, ".wayfinder-v1-backup")
			for _, filename := range tt.wantBackedUp {
				backupPath := filepath.Join(backupDir, filename)
				if _, err := os.Stat(backupPath); os.IsNotExist(err) {
					t.Errorf("file %s should be backed up but doesn't exist", filename)
				}

				// Original should be gone
				originalPath := filepath.Join(tmpDir, filename)
				if _, err := os.Stat(originalPath); err == nil {
					t.Errorf("original file %s should be removed after backup", filename)
				}
			}

			// Verify files that should NOT be backed up still exist
			for filename := range tt.files {
				shouldBackup := false
				for _, backup := range tt.wantBackedUp {
					if backup == filename {
						shouldBackup = true
						break
					}
				}

				if !shouldBackup {
					originalPath := filepath.Join(tmpDir, filename)
					if _, err := os.Stat(originalPath); os.IsNotExist(err) {
						t.Errorf("file %s should not be backed up but was removed", filename)
					}
				}
			}
		})
	}
}

func TestFileMigrator_MigrateFiles_Integration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a complete V1 project structure
	files := map[string]string{
		"S4-stakeholder-approval.md": "Approved by stakeholders",
		"S5-tech-research.md":        "Technology research findings",
		"S8-implementation.md":       "Implementation details",
		"S9-validation.md":           "Validation results",
		"S10-deployment.md":          "Deployment procedure",
	}

	for filename, content := range files {
		path := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	// Also create D4 with requirements for outline generation
	d4Content := `# D4 Requirements
## Functional Requirements
- Feature 1
- Feature 2
## Acceptance Criteria
- AC1: System works
- AC2: Tests pass
`
	d4Path := filepath.Join(tmpDir, "D4-requirements.md")
	if err := os.WriteFile(d4Path, []byte(d4Content), 0644); err != nil {
		t.Fatalf("failed to create D4: %v", err)
	}

	// Run full migration
	fm := NewFileMigrator(tmpDir)
	v2Status := status.NewStatusV2("test-project", "feature", "M")
	err := fm.MigrateFiles(v2Status)
	if err != nil {
		t.Fatalf("MigrateFiles() error = %v", err)
	}

	// Verify all expected outputs
	expectedFiles := []string{
		"D4-requirements.md", // Should contain stakeholder section
		"S6-design.md",       // Should contain research section
		"S8-build.md",        // Should contain unified S8/S9/S10
		"TESTS.outline",      // Should be generated
		"TESTS.feature",      // Should be generated
	}

	for _, filename := range expectedFiles {
		path := filepath.Join(tmpDir, filename)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("expected file %s not found: %v", filename, err)
			continue
		}

		// Basic content checks
		switch filename {
		case "D4-requirements.md":
			if !strings.Contains(string(content), "Stakeholder Decisions") {
				t.Error("D4 should contain Stakeholder Decisions section")
			}
		case "S6-design.md":
			if !strings.Contains(string(content), "Research Notes") {
				t.Error("S6 should contain Research Notes section")
			}
		case "S8-build.md":
			if !strings.Contains(string(content), "BUILD Loop") {
				t.Error("S8 should contain BUILD Loop section")
			}
		case "TESTS.outline":
			if !strings.Contains(string(content), "AC") {
				t.Error("TESTS.outline should contain acceptance criteria")
			}
		case "TESTS.feature":
			if !strings.Contains(string(content), "Feature:") {
				t.Error("TESTS.feature should contain Feature definition")
			}
		}
	}
}

func TestFileMigrator_GenerateOutlineFromD4(t *testing.T) {
	tests := []struct {
		name      string
		d4Content string
		wantACs   int
	}{
		{
			name: "extract from bullet points",
			d4Content: `# Requirements
- User can log in
- User can log out
- Session timeout after 1 hour`,
			wantACs: 3,
		},
		{
			name: "extract from numbered list",
			d4Content: `# Requirements
1. System must handle 1000 requests/sec
2. Response time < 100ms
3. 99.9% uptime`,
			wantACs: 3,
		},
		{
			name: "no requirements - use template",
			d4Content: `# Some other content
Nothing about requirements here`,
			wantACs: 3, // Default template has 3 ACs
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := NewFileMigrator("")
			outline := fm.generateOutlineFromD4(tt.d4Content)

			// Count ACs
			acCount := strings.Count(outline, "**AC")
			if acCount < tt.wantACs {
				t.Errorf("generateOutlineFromD4() has %d ACs, want at least %d\nOutline: %s",
					acCount, tt.wantACs, outline)
			}
		})
	}
}

func TestFileMigrator_GenerateFeatureFromS6(t *testing.T) {
	fm := NewFileMigrator("")
	feature := fm.generateFeatureFromS6("Sample S6 content")

	// Verify feature file structure
	requiredElements := []string{
		"Feature:",
		"Scenario:",
		"Given",
		"When",
		"Then",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(feature, elem) {
			t.Errorf("generateFeatureFromS6() missing required element: %s\nFeature: %s",
				elem, feature)
		}
	}
}

func TestNewFileMigrator(t *testing.T) {
	projectDir := "/tmp/test-project"
	fm := NewFileMigrator(projectDir)

	if fm.projectDir != projectDir {
		t.Errorf("NewFileMigrator() projectDir = %s, want %s", fm.projectDir, projectDir)
	}
}
