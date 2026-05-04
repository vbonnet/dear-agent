package configloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPersona(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		errContains string
		validate    func(*testing.T, *Persona)
	}{
		{
			name: "valid persona",
			content: `---
name: security-engineer
displayName: Security Engineer
version: 1.0.0
description: Security expert
focusAreas:
  - security
  - testing
---
Security persona content here.`,
			wantErr: false,
			validate: func(t *testing.T, p *Persona) {
				if p.Name != "security-engineer" {
					t.Errorf("Name = %q, want %q", p.Name, "security-engineer")
				}
				if p.DisplayName != "Security Engineer" {
					t.Errorf("DisplayName = %q, want %q", p.DisplayName, "Security Engineer")
				}
				if p.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", p.Version, "1.0.0")
				}
				if p.Content != "Security persona content here." {
					t.Errorf("Content = %q, want %q", p.Content, "Security persona content here.")
				}
				if p.Tier != "tier2" {
					t.Errorf("Tier = %q, want %q (default)", p.Tier, "tier2")
				}
				if p.Maturity != "stable" {
					t.Errorf("Maturity = %q, want %q (default)", p.Maturity, "stable")
				}
			},
		},
		{
			name: "persona with all fields",
			content: `---
name: llm-context-expert
displayName: LLM Context Expert
version: 2.1.3
description: Context optimization expert
expertise:
  - context-management
  - token-optimization
severityLevels:
  - critical
  - high
focusAreas:
  - performance
  - optimization
gitHistoryAccess: true
tier: tier1
maturity: experimental
---
Advanced context management.`,
			wantErr: false,
			validate: func(t *testing.T, p *Persona) {
				if p.Name != "llm-context-expert" {
					t.Errorf("Name = %q", p.Name)
				}
				if len(p.Expertise) != 2 {
					t.Errorf("Expertise length = %d, want 2", len(p.Expertise))
				}
				if len(p.SeverityLevels) != 2 {
					t.Errorf("SeverityLevels length = %d, want 2", len(p.SeverityLevels))
				}
				if !p.GitHistoryAccess {
					t.Error("GitHistoryAccess = false, want true")
				}
				if p.Tier != "tier1" {
					t.Errorf("Tier = %q, want tier1", p.Tier)
				}
				if p.Maturity != "experimental" {
					t.Errorf("Maturity = %q, want experimental", p.Maturity)
				}
			},
		},
		{
			name:        "missing frontmatter",
			content:     "Just content without frontmatter",
			wantErr:     true,
			errContains: "missing frontmatter",
		},
		{
			name: "invalid yaml",
			content: `---
name: invalid
  bad indentation: true
---
Content`,
			wantErr:     true,
			errContains: "unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.ai.md")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Load persona
			persona, err := LoadPersona(tmpFile)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadPersona() error = nil, want error containing %q", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("LoadPersona() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("LoadPersona() unexpected error: %v", err)
			}

			// Run validation function
			if tt.validate != nil {
				tt.validate(t, persona)
			}
		})
	}
}

func TestPersona_Validate(t *testing.T) {
	tests := []struct {
		name        string
		persona     Persona
		wantErr     bool
		errContains string
	}{
		{
			name: "valid persona",
			persona: Persona{
				Name:        "test-persona",
				DisplayName: "Test Persona",
				Version:     "1.0.0",
				Description: "Test description",
				FocusAreas:  []string{"testing"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			persona: Persona{
				DisplayName: "Test",
				Version:     "1.0.0",
				Description: "Test",
				FocusAreas:  []string{"test"},
			},
			wantErr:     true,
			errContains: "name",
		},
		{
			name: "invalid name format (uppercase)",
			persona: Persona{
				Name:        "Test-Persona",
				DisplayName: "Test",
				Version:     "1.0.0",
				Description: "Test",
				FocusAreas:  []string{"test"},
			},
			wantErr:     true,
			errContains: "name format",
		},
		{
			name: "invalid name format (underscore)",
			persona: Persona{
				Name:        "test_persona",
				DisplayName: "Test",
				Version:     "1.0.0",
				Description: "Test",
				FocusAreas:  []string{"test"},
			},
			wantErr:     true,
			errContains: "name format",
		},
		{
			name: "invalid version format",
			persona: Persona{
				Name:        "test-persona",
				DisplayName: "Test",
				Version:     "v1.0.0",
				Description: "Test",
				FocusAreas:  []string{"test"},
			},
			wantErr:     true,
			errContains: "version format",
		},
		{
			name: "invalid severity level",
			persona: Persona{
				Name:           "test-persona",
				DisplayName:    "Test",
				Version:        "1.0.0",
				Description:    "Test",
				FocusAreas:     []string{"test"},
				SeverityLevels: []string{"critical", "invalid"},
			},
			wantErr:     true,
			errContains: "severity level",
		},
		{
			name: "empty focus areas",
			persona: Persona{
				Name:        "test-persona",
				DisplayName: "Test",
				Version:     "1.0.0",
				Description: "Test",
				FocusAreas:  []string{},
			},
			wantErr:     true,
			errContains: "focusAreas",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.persona.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() error = nil, want error containing %q", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPersona_IsExperimental(t *testing.T) {
	tests := []struct {
		name     string
		maturity string
		want     bool
	}{
		{"experimental persona", "experimental", true},
		{"stable persona", "stable", false},
		{"empty maturity", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Persona{Maturity: tt.maturity}
			if got := p.IsExperimental(); got != tt.want {
				t.Errorf("IsExperimental() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPersona_IsStable(t *testing.T) {
	tests := []struct {
		name     string
		maturity string
		want     bool
	}{
		{"stable persona", "stable", true},
		{"experimental persona", "experimental", false},
		{"empty maturity", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Persona{Maturity: tt.maturity}
			if got := p.IsStable(); got != tt.want {
				t.Errorf("IsStable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPersona_ToMap(t *testing.T) {
	p := Persona{
		Name:        "test",
		DisplayName: "Test Persona",
		Version:     "1.0.0",
		Tier:        "tier1",
		Maturity:    "stable",
	}

	m := p.ToMap()

	if m["name"] != "test" {
		t.Errorf("ToMap()[name] = %v, want test", m["name"])
	}
	if m["tier"] != "tier1" {
		t.Errorf("ToMap()[tier] = %v, want tier1", m["tier"])
	}
}

func TestListPersonas(t *testing.T) {
	// Create temp library directory
	tmpDir := t.TempDir()

	// Create test personas
	personas := map[string]string{
		"persona1.ai.md": `---
name: persona1
displayName: Persona One
version: 1.0.0
description: First persona
focusAreas:
  - testing
---
Content 1`,
		"persona2.ai.md": `---
name: persona2
displayName: Persona Two
version: 2.0.0
description: Second persona
focusAreas:
  - review
---
Content 2`,
		"ignore.why.md":    "This should be ignored",
		"not-a-persona.md": "Also ignored",
	}

	for filename, content := range personas {
		path := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Test listing
	opts := PersonaLoadOptions{
		LibraryPath: tmpDir,
		Recursive:   false,
	}

	result, err := ListPersonas(opts)
	if err != nil {
		t.Fatalf("ListPersonas() unexpected error: %v", err)
	}

	// Should find 2 personas
	if len(result) != 2 {
		t.Errorf("ListPersonas() returned %d personas, want 2", len(result))
	}

	// Check persona1
	if p, ok := result["persona1"]; ok {
		if p.DisplayName != "Persona One" {
			t.Errorf("persona1.DisplayName = %q, want %q", p.DisplayName, "Persona One")
		}
	} else {
		t.Error("persona1 not found in results")
	}

	// Check persona2
	if p, ok := result["persona2"]; ok {
		if p.Version != "2.0.0" {
			t.Errorf("persona2.Version = %q, want %q", p.Version, "2.0.0")
		}
	} else {
		t.Error("persona2 not found in results")
	}
}

func TestListPersonas_Recursive(t *testing.T) {
	// Create temp library with subdirectories
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "core")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create persona in root
	root := `---
name: root-persona
displayName: Root
version: 1.0.0
description: Root persona
focusAreas:
  - root
---
Root content`
	if err := os.WriteFile(filepath.Join(tmpDir, "root-persona.ai.md"), []byte(root), 0644); err != nil {
		t.Fatalf("Failed to create root persona: %v", err)
	}

	// Create persona in subdirectory
	sub := `---
name: sub-persona
displayName: Sub
version: 1.0.0
description: Sub persona
focusAreas:
  - sub
---
Sub content`
	if err := os.WriteFile(filepath.Join(subDir, "sub-persona.ai.md"), []byte(sub), 0644); err != nil {
		t.Fatalf("Failed to create sub persona: %v", err)
	}

	// Test recursive listing
	opts := PersonaLoadOptions{
		LibraryPath: tmpDir,
		Recursive:   true,
	}

	result, err := ListPersonas(opts)
	if err != nil {
		t.Fatalf("ListPersonas() unexpected error: %v", err)
	}

	// Should find both personas
	if len(result) != 2 {
		t.Errorf("ListPersonas() returned %d personas, want 2", len(result))
	}

	if _, ok := result["root-persona"]; !ok {
		t.Error("root-persona not found")
	}
	if _, ok := result["sub-persona"]; !ok {
		t.Error("sub-persona not found")
	}
}

func TestDefaultPersonaOptions(t *testing.T) {
	opts := DefaultPersonaOptions()

	if !opts.Recursive {
		t.Error("DefaultPersonaOptions().Recursive = false, want true")
	}
	if !opts.ValidateSchema {
		t.Error("DefaultPersonaOptions().ValidateSchema = false, want true")
	}
	if opts.LibraryPath != "" {
		t.Errorf("DefaultPersonaOptions().LibraryPath = %q, want empty string", opts.LibraryPath)
	}
}

func TestLoadPersonaByName(t *testing.T) {
	// Create temp library directory
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "security")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create test persona in subdirectory
	personaContent := `---
name: security-expert
displayName: Security Expert
version: 1.2.3
description: Security specialist
focusAreas:
  - security
  - testing
---
Security expert content.`
	personaPath := filepath.Join(subDir, "security-expert.ai.md")
	if err := os.WriteFile(personaPath, []byte(personaContent), 0644); err != nil {
		t.Fatalf("Failed to create persona file: %v", err)
	}

	tests := []struct {
		name        string
		personaName string
		opts        PersonaLoadOptions
		wantErr     bool
		errContains string
		validate    func(*testing.T, *Persona)
	}{
		{
			name:        "find persona recursively",
			personaName: "security-expert",
			opts: PersonaLoadOptions{
				LibraryPath: tmpDir,
				Recursive:   true,
			},
			wantErr: false,
			validate: func(t *testing.T, p *Persona) {
				if p.Name != "security-expert" {
					t.Errorf("Name = %q, want security-expert", p.Name)
				}
				if p.DisplayName != "Security Expert" {
					t.Errorf("DisplayName = %q, want Security Expert", p.DisplayName)
				}
			},
		},
		{
			name:        "persona not found when non-recursive",
			personaName: "security-expert",
			opts: PersonaLoadOptions{
				LibraryPath: tmpDir,
				Recursive:   false,
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "persona not found - wrong name",
			personaName: "nonexistent",
			opts: PersonaLoadOptions{
				LibraryPath: tmpDir,
				Recursive:   true,
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "empty library path",
			personaName: "test",
			opts: PersonaLoadOptions{
				LibraryPath: "",
			},
			wantErr:     true,
			errContains: "library path not specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			persona, err := LoadPersonaByName(tt.personaName, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadPersonaByName() error = nil, want error containing %q", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("LoadPersonaByName() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("LoadPersonaByName() unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, persona)
			}
		})
	}
}

func TestLoadPersonaByName_NonRecursive(t *testing.T) {
	// Create temp library directory
	tmpDir := t.TempDir()

	// Create persona in root directory
	personaContent := `---
name: root-persona
displayName: Root Persona
version: 1.0.0
description: Root persona
focusAreas:
  - testing
---
Root content.`
	personaPath := filepath.Join(tmpDir, "root-persona.ai.md")
	if err := os.WriteFile(personaPath, []byte(personaContent), 0644); err != nil {
		t.Fatalf("Failed to create persona file: %v", err)
	}

	opts := PersonaLoadOptions{
		LibraryPath: tmpDir,
		Recursive:   false,
	}

	persona, err := LoadPersonaByName("root-persona", opts)
	if err != nil {
		t.Fatalf("LoadPersonaByName() unexpected error: %v", err)
	}

	if persona.Name != "root-persona" {
		t.Errorf("Name = %q, want root-persona", persona.Name)
	}
}

func TestListPersonas_EmptyLibraryPath(t *testing.T) {
	opts := PersonaLoadOptions{
		LibraryPath: "",
	}

	_, err := ListPersonas(opts)
	if err == nil {
		t.Error("ListPersonas() error = nil, want error for empty library path")
	}
	if !strings.Contains(err.Error(), "library path not specified") {
		t.Errorf("ListPersonas() error = %q, want error containing 'library path not specified'", err.Error())
	}
}

func TestListPersonas_NonRecursive(t *testing.T) {
	// Create temp library directory
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Create persona in root
	rootContent := `---
name: root
displayName: Root
version: 1.0.0
description: Root
focusAreas:
  - root
---
Root`
	if err := os.WriteFile(filepath.Join(tmpDir, "root.ai.md"), []byte(rootContent), 0644); err != nil {
		t.Fatalf("Failed to create root persona: %v", err)
	}

	// Create persona in subdirectory (should not be found in non-recursive mode)
	subContent := `---
name: sub
displayName: Sub
version: 1.0.0
description: Sub
focusAreas:
  - sub
---
Sub`
	if err := os.WriteFile(filepath.Join(subDir, "sub.ai.md"), []byte(subContent), 0644); err != nil {
		t.Fatalf("Failed to create sub persona: %v", err)
	}

	opts := PersonaLoadOptions{
		LibraryPath: tmpDir,
		Recursive:   false,
	}

	result, err := ListPersonas(opts)
	if err != nil {
		t.Fatalf("ListPersonas() unexpected error: %v", err)
	}

	// Should only find root persona
	if len(result) != 1 {
		t.Errorf("ListPersonas() returned %d personas, want 1", len(result))
	}

	if _, ok := result["root"]; !ok {
		t.Error("root persona not found")
	}
	if _, ok := result["sub"]; ok {
		t.Error("sub persona should not be found in non-recursive mode")
	}
}

func TestPersona_Validate_AllFields(t *testing.T) {
	tests := []struct {
		name        string
		persona     Persona
		wantErr     bool
		errContains string
	}{
		{
			name: "missing displayName",
			persona: Persona{
				Name:        "test",
				Version:     "1.0.0",
				Description: "Test",
				FocusAreas:  []string{"test"},
			},
			wantErr:     true,
			errContains: "displayName",
		},
		{
			name: "missing description",
			persona: Persona{
				Name:        "test",
				DisplayName: "Test",
				Version:     "1.0.0",
				FocusAreas:  []string{"test"},
			},
			wantErr:     true,
			errContains: "description",
		},
		{
			name: "valid severity levels",
			persona: Persona{
				Name:           "test",
				DisplayName:    "Test",
				Version:        "1.0.0",
				Description:    "Test",
				FocusAreas:     []string{"test"},
				SeverityLevels: []string{"critical", "high", "medium", "low", "info"},
			},
			wantErr: false,
		},
		{
			name: "missing version",
			persona: Persona{
				Name:        "test",
				DisplayName: "Test",
				Description: "Test",
				FocusAreas:  []string{"test"},
			},
			wantErr:     true,
			errContains: "version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.persona.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() error = nil, want error containing %q", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestListPersonas_WithBadFile(t *testing.T) {
	// Create temp library directory
	tmpDir := t.TempDir()

	// Create valid persona
	validContent := `---
name: valid
displayName: Valid
version: 1.0.0
description: Valid
focusAreas:
  - test
---
Valid`
	if err := os.WriteFile(filepath.Join(tmpDir, "valid.ai.md"), []byte(validContent), 0644); err != nil {
		t.Fatalf("Failed to create valid persona: %v", err)
	}

	// Create invalid persona (should be skipped with warning)
	invalidContent := `---
invalid yaml: [ unclosed
---
Invalid`
	if err := os.WriteFile(filepath.Join(tmpDir, "invalid.ai.md"), []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to create invalid persona: %v", err)
	}

	opts := PersonaLoadOptions{
		LibraryPath: tmpDir,
		Recursive:   false,
	}

	result, err := ListPersonas(opts)
	if err != nil {
		t.Fatalf("ListPersonas() unexpected error: %v", err)
	}

	// Should only find valid persona (invalid skipped)
	if len(result) != 1 {
		t.Errorf("ListPersonas() returned %d personas, want 1", len(result))
	}

	if _, ok := result["valid"]; !ok {
		t.Error("valid persona not found")
	}
}
