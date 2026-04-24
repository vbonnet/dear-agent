package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		errContains string
		validate    func(*testing.T, *DeliverableFrontmatter)
	}{
		{
			name: "valid frontmatter",
			content: `---
phase: "D1"
phase_name: "Problem Validation"
wayfinder_session_id: "test-session-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "sha256:abc123"
phase_engram_path: "~/engrams/d1-problem-validation.ai.md"
---

# D1: Problem Validation

Content here.
`,
			wantErr: false,
			validate: func(t *testing.T, fm *DeliverableFrontmatter) {
				if fm.Phase != "D1" {
					t.Errorf("Phase = %q, want %q", fm.Phase, "D1")
				}
				if fm.PhaseName != "Problem Validation" {
					t.Errorf("PhaseName = %q, want %q", fm.PhaseName, "Problem Validation")
				}
				if fm.WayfinderSessionID != "test-session-123" {
					t.Errorf("WayfinderSessionID = %q, want %q", fm.WayfinderSessionID, "test-session-123")
				}
				if fm.CreatedAt != "2026-01-05T12:00:00Z" {
					t.Errorf("CreatedAt = %q, want %q", fm.CreatedAt, "2026-01-05T12:00:00Z")
				}
				if fm.PhaseEngramHash != "sha256:abc123" {
					t.Errorf("PhaseEngramHash = %q, want %q", fm.PhaseEngramHash, "sha256:abc123")
				}
				if fm.PhaseEngramPath != "~/engrams/d1-problem-validation.ai.md" {
					t.Errorf("PhaseEngramPath = %q, want %q", fm.PhaseEngramPath, "~/engrams/d1-problem-validation.ai.md")
				}
			},
		},
		{
			name: "missing frontmatter - no opening delimiter",
			content: `# D1: Problem Validation

Content without frontmatter.
`,
			wantErr:     true,
			errContains: "does not start with YAML frontmatter delimiter",
		},
		{
			name: "missing closing delimiter",
			content: `---
phase: "D1"
phase_name: "Problem Validation"

# No closing delimiter above
`,
			wantErr:     true,
			errContains: "no closing frontmatter delimiter",
		},
		{
			name: "invalid YAML",
			content: `---
phase: "D1"
phase_name: [unclosed array
created_at: "2026-01-05T12:00:00Z"
---

Content
`,
			wantErr:     true,
			errContains: "failed to parse YAML frontmatter",
		},
		{
			name: "missing required field - phase",
			content: `---
phase_name: "Problem Validation"
wayfinder_session_id: "test-session-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "sha256:abc123"
phase_engram_path: "~/engrams/d1.ai.md"
---

Content
`,
			wantErr:     true,
			errContains: "missing required frontmatter fields: phase",
		},
		{
			name: "missing multiple required fields",
			content: `---
phase: "D1"
created_at: "2026-01-05T12:00:00Z"
---

Content
`,
			wantErr:     true,
			errContains: "missing required frontmatter fields:",
		},
		{
			name:        "empty file",
			content:     ``,
			wantErr:     true,
			errContains: "does not start with YAML frontmatter delimiter",
		},
		{
			name: "frontmatter with extra fields (strict mode)",
			content: `---
phase: "D1"
phase_name: "Problem Validation"
wayfinder_session_id: "test-session-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "sha256:abc123"
phase_engram_path: "~/engrams/d1.ai.md"
unknown_field: "should trigger error"
---

Content
`,
			wantErr:     true,
			errContains: "field unknown_field not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.md")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0600); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Extract frontmatter
			fm, err := extractFrontmatter(tmpFile)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("extractFrontmatter() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("extractFrontmatter() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("extractFrontmatter() unexpected error = %v", err)
				return
			}

			// Run validation if provided
			if tt.validate != nil {
				tt.validate(t, fm)
			}
		})
	}
}

func TestExtractFrontmatter_FileNotFound(t *testing.T) {
	_, err := extractFrontmatter("/nonexistent/file.md")
	if err == nil {
		t.Error("extractFrontmatter() expected error for nonexistent file, got nil")
	}
	if !contains(err.Error(), "failed to read file") {
		t.Errorf("extractFrontmatter() error = %q, want substring %q", err.Error(), "failed to read file")
	}
}

func TestExtractFrontmatterContent(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid frontmatter",
			content: `---
build_command: go build
build_artifacts:
  - bin/app
---

# Architecture

Content here.`,
			want:    "build_command: go build\nbuild_artifacts:\n  - bin/app",
			wantErr: false,
		},
		{
			name: "no frontmatter",
			content: `# Architecture

Content without frontmatter.`,
			want:    "",
			wantErr: false,
		},
		{
			name: "empty frontmatter",
			content: `---
---

# Content`,
			want:    "",
			wantErr: false,
		},
		{
			name: "unclosed frontmatter",
			content: `---
build_command: go build

# Missing closing delimiter`,
			want:        "",
			wantErr:     true,
			errContains: "frontmatter not closed",
		},
		{
			name: "frontmatter in middle of document",
			content: `# Header

---
This is not frontmatter
---`,
			want:    "This is not frontmatter",
			wantErr: false,
		},
		{
			name: "whitespace around delimiters",
			content: `   ---
build_command: go build
   ---

Content`,
			want:    "build_command: go build",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractFrontmatterContent(tt.content)

			if tt.wantErr {
				if err == nil {
					t.Errorf("extractFrontmatterContent() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("extractFrontmatterContent() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("extractFrontmatterContent() unexpected error = %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("extractFrontmatterContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseArchitectureFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		wantNil     bool
		wantErr     bool
		errContains string
		validate    func(*testing.T, *ArchitectureFrontmatter)
	}{
		{
			name: "valid frontmatter with all fields",
			fileContent: `---
build_command: go build -o bin/app
build_artifacts:
  - bin/app
  - dist/config.yaml
test_command: go test ./...
---

# Architecture`,
			wantNil: false,
			wantErr: false,
			validate: func(t *testing.T, fm *ArchitectureFrontmatter) {
				if fm.BuildCommand != "go build -o bin/app" {
					t.Errorf("BuildCommand = %q, want %q", fm.BuildCommand, "go build -o bin/app")
				}
				if len(fm.BuildArtifacts) != 2 {
					t.Errorf("BuildArtifacts length = %d, want 2", len(fm.BuildArtifacts))
				}
				if fm.TestCommand != "go test ./..." {
					t.Errorf("TestCommand = %q, want %q", fm.TestCommand, "go test ./...")
				}
			},
		},
		{
			name: "partial frontmatter - only build_command",
			fileContent: `---
build_command: make build
---

Content`,
			wantNil: false,
			wantErr: false,
			validate: func(t *testing.T, fm *ArchitectureFrontmatter) {
				if fm.BuildCommand != "make build" {
					t.Errorf("BuildCommand = %q, want %q", fm.BuildCommand, "make build")
				}
				if len(fm.BuildArtifacts) != 0 {
					t.Errorf("BuildArtifacts should be empty, got %v", fm.BuildArtifacts)
				}
			},
		},
		{
			name:        "no frontmatter - returns empty struct",
			fileContent: `# Architecture\n\nNo frontmatter here.`,
			wantNil:     false,
			wantErr:     false,
			validate: func(t *testing.T, fm *ArchitectureFrontmatter) {
				if fm.BuildCommand != "" {
					t.Errorf("BuildCommand should be empty, got %q", fm.BuildCommand)
				}
			},
		},
		{
			name: "malformed YAML",
			fileContent: `---
build_command: [unclosed array
---

Content`,
			wantNil:     false,
			wantErr:     true,
			errContains: "failed to parse ARCHITECTURE.md frontmatter",
		},
		{
			name: "unclosed frontmatter",
			fileContent: `---
build_command: go build

Missing closing delimiter`,
			wantNil:     false,
			wantErr:     true,
			errContains: "failed to extract frontmatter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with ARCHITECTURE.md
			tmpDir := t.TempDir()
			archPath := filepath.Join(tmpDir, "ARCHITECTURE.md")
			if err := os.WriteFile(archPath, []byte(tt.fileContent), 0600); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			got, err := parseArchitectureFrontmatter(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseArchitectureFrontmatter() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("parseArchitectureFrontmatter() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("parseArchitectureFrontmatter() unexpected error = %v", err)
				return
			}

			if tt.wantNil && got != nil {
				t.Errorf("parseArchitectureFrontmatter() = %v, want nil", got)
				return
			}

			if !tt.wantNil && got == nil {
				t.Error("parseArchitectureFrontmatter() = nil, want non-nil")
				return
			}

			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}

func TestParseArchitectureFrontmatter_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't create ARCHITECTURE.md

	got, err := parseArchitectureFrontmatter(tmpDir)
	if err != nil {
		t.Errorf("parseArchitectureFrontmatter() should return nil for missing file, got error: %v", err)
	}
	if got != nil {
		t.Errorf("parseArchitectureFrontmatter() = %v, want nil for missing file", got)
	}
}

func TestParseTestPlanFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		wantNil     bool
		wantErr     bool
		errContains string
		validate    func(*testing.T, *TestPlanFrontmatter)
	}{
		{
			name: "valid frontmatter with all fields",
			fileContent: `---
coverage_threshold: 80
test_command: pytest --cov
skip_coverage_check: false
---

# Test Plan`,
			wantNil: false,
			wantErr: false,
			validate: func(t *testing.T, fm *TestPlanFrontmatter) {
				if fm.CoverageThreshold != 80 {
					t.Errorf("CoverageThreshold = %d, want 80", fm.CoverageThreshold)
				}
				if fm.TestCommand != "pytest --cov" {
					t.Errorf("TestCommand = %q, want %q", fm.TestCommand, "pytest --cov")
				}
				if fm.SkipCoverageCheck != false {
					t.Errorf("SkipCoverageCheck = %v, want false", fm.SkipCoverageCheck)
				}
			},
		},
		{
			name: "skip coverage check enabled",
			fileContent: `---
skip_coverage_check: true
---

Content`,
			wantNil: false,
			wantErr: false,
			validate: func(t *testing.T, fm *TestPlanFrontmatter) {
				if fm.SkipCoverageCheck != true {
					t.Errorf("SkipCoverageCheck = %v, want true", fm.SkipCoverageCheck)
				}
				if fm.CoverageThreshold != 0 {
					t.Errorf("CoverageThreshold = %d, want 0", fm.CoverageThreshold)
				}
			},
		},
		{
			name:        "no frontmatter - returns empty struct",
			fileContent: `# Test Plan\n\nNo frontmatter.`,
			wantNil:     false,
			wantErr:     false,
			validate: func(t *testing.T, fm *TestPlanFrontmatter) {
				if fm.CoverageThreshold != 0 {
					t.Errorf("CoverageThreshold should be 0, got %d", fm.CoverageThreshold)
				}
			},
		},
		{
			name: "malformed YAML",
			fileContent: `---
coverage_threshold: not_a_number
---

Content`,
			wantNil:     false,
			wantErr:     true,
			errContains: "failed to parse TEST_PLAN.md frontmatter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with TEST_PLAN.md
			tmpDir := t.TempDir()
			testPlanPath := filepath.Join(tmpDir, "TEST_PLAN.md")
			if err := os.WriteFile(testPlanPath, []byte(tt.fileContent), 0600); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			got, err := parseTestPlanFrontmatter(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseTestPlanFrontmatter() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("parseTestPlanFrontmatter() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("parseTestPlanFrontmatter() unexpected error = %v", err)
				return
			}

			if tt.wantNil && got != nil {
				t.Errorf("parseTestPlanFrontmatter() = %v, want nil", got)
				return
			}

			if !tt.wantNil && got == nil {
				t.Error("parseTestPlanFrontmatter() = nil, want non-nil")
				return
			}

			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}

func TestParseTestPlanFrontmatter_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't create TEST_PLAN.md

	got, err := parseTestPlanFrontmatter(tmpDir)
	if err != nil {
		t.Errorf("parseTestPlanFrontmatter() should return nil for missing file, got error: %v", err)
	}
	if got != nil {
		t.Errorf("parseTestPlanFrontmatter() = %v, want nil for missing file", got)
	}
}
