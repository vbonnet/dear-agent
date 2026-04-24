package configloader

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Persona represents a persona loaded from an .ai.md file.
type Persona struct {
	// Frontmatter fields
	Name             string   `yaml:"name"`
	DisplayName      string   `yaml:"displayName"`
	Version          string   `yaml:"version"`
	Description      string   `yaml:"description"`
	Expertise        []string `yaml:"expertise"`
	SeverityLevels   []string `yaml:"severityLevels"`
	FocusAreas       []string `yaml:"focusAreas"`
	GitHistoryAccess bool     `yaml:"gitHistoryAccess"`
	Tier             string   `yaml:"tier"`
	Maturity         string   `yaml:"maturity"`

	// Content (markdown body after frontmatter)
	Content string

	// Source file path (for debugging/telemetry)
	SourcePath string
}

// PersonaLoadOptions configures persona loading behavior.
type PersonaLoadOptions struct {
	// LibraryPath is the root directory containing personas.
	// Defaults to engram/core/persona/library if not specified.
	LibraryPath string

	// Recursive enables searching in subdirectories.
	// Default: true
	Recursive bool

	// ValidateSchema enables strict schema validation.
	// Default: true
	ValidateSchema bool
}

// DefaultPersonaOptions returns default persona loading options.
func DefaultPersonaOptions() PersonaLoadOptions {
	return PersonaLoadOptions{
		Recursive:      true,
		ValidateSchema: true,
	}
}

// LoadPersona loads a persona from an .ai.md file.
//
// The file must have YAML frontmatter delimited by "---" markers:
//
//	---
//	name: security-engineer
//	displayName: Security Engineer
//	version: 1.0.0
//	---
//	Persona content here...
//
// Returns an error if:
//   - File cannot be read
//   - Frontmatter is missing or invalid YAML
//   - Required fields are missing (when ValidateSchema is true)
func LoadPersona(path string) (*Persona, error) {
	// Expand home directory
	expandedPath, err := ExpandHome(path)
	if err != nil {
		return nil, fmt.Errorf("expand path %q: %w", path, err)
	}

	// Read file
	data, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("read persona file %q: %w", expandedPath, err)
	}

	// Parse frontmatter and content
	frontmatter, content, err := ParseFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter in %q: %w", expandedPath, err)
	}

	// Unmarshal frontmatter into Persona struct
	var persona Persona
	if err := yaml.Unmarshal([]byte(frontmatter), &persona); err != nil {
		return nil, fmt.Errorf("unmarshal persona frontmatter from %q: %w", expandedPath, err)
	}

	persona.Content = content
	persona.SourcePath = expandedPath

	// Set defaults
	if persona.Tier == "" {
		persona.Tier = "tier2"
	}
	if persona.Maturity == "" {
		persona.Maturity = "stable"
	}

	return &persona, nil
}

// LoadPersonaByName loads a persona by name from a library directory.
//
// Searches for {name}.ai.md in the library path and subdirectories.
// Returns the first matching persona found.
//
// Example:
//
//	persona, err := LoadPersonaByName("security-engineer", opts)
//	// Searches for security-engineer.ai.md in library
func LoadPersonaByName(name string, opts PersonaLoadOptions) (*Persona, error) {
	if opts.LibraryPath == "" {
		return nil, fmt.Errorf("library path not specified")
	}

	expandedPath, err := ExpandHome(opts.LibraryPath)
	if err != nil {
		return nil, fmt.Errorf("expand library path: %w", err)
	}

	// Search for persona file
	filename := name + ".ai.md"
	var foundPath string

	if opts.Recursive {
		// Search recursively
		err := filepath.Walk(expandedPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && info.Name() == filename {
				foundPath = path
				return filepath.SkipAll // Stop searching
			}
			return nil
		})
		if err != nil && !errors.Is(err, filepath.SkipAll) {
			return nil, fmt.Errorf("search library %q: %w", expandedPath, err)
		}
	} else {
		// Search only in root directory
		path := filepath.Join(expandedPath, filename)
		if _, err := os.Stat(path); err == nil {
			foundPath = path
		}
	}

	if foundPath == "" {
		return nil, fmt.Errorf("persona %q not found in library %q", name, expandedPath)
	}

	return LoadPersona(foundPath)
}

// ListPersonas lists all personas in a library directory.
//
// Returns a map of persona name to Persona object.
// Skips files that fail to parse (logs warnings).
func ListPersonas(opts PersonaLoadOptions) (map[string]*Persona, error) {
	if opts.LibraryPath == "" {
		return nil, fmt.Errorf("library path not specified")
	}

	expandedPath, err := ExpandHome(opts.LibraryPath)
	if err != nil {
		return nil, fmt.Errorf("expand library path: %w", err)
	}

	personas := make(map[string]*Persona)

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-.ai.md files
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".ai.md") {
			return nil
		}

		// Skip .why.md files
		if strings.HasSuffix(info.Name(), ".why.md") {
			return nil
		}

		// Load persona
		persona, err := LoadPersona(path)
		if err != nil {
			// Log warning but continue (don't fail entire listing)
			fmt.Fprintf(os.Stderr, "Warning: failed to load persona from %s: %v\n", path, err)
			return nil
		}

		personas[persona.Name] = persona
		return nil
	}

	if opts.Recursive {
		err = filepath.Walk(expandedPath, walkFn)
	} else {
		// Non-recursive: only scan root directory
		entries, err := os.ReadDir(expandedPath)
		if err != nil {
			return nil, fmt.Errorf("read directory %q: %w", expandedPath, err)
		}
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			path := filepath.Join(expandedPath, entry.Name())
			walkFn(path, info, nil)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("scan library %q: %w", expandedPath, err)
	}

	return personas, nil
}

// IsExperimental returns true if the persona is experimental (candidate for beta testing).
func (p *Persona) IsExperimental() bool {
	return p.Maturity == "experimental"
}

// IsStable returns true if the persona is stable (production-ready).
func (p *Persona) IsStable() bool {
	return p.Maturity == "stable"
}

// Validate validates the persona against schema requirements.
//
// Checks:
//   - Required fields are present
//   - Name format (lowercase-kebab-case)
//   - Version format (semver x.y.z)
//   - Severity levels (valid values)
func (p *Persona) Validate() error {
	// Required fields
	if p.Name == "" {
		return fmt.Errorf("missing required field: name")
	}
	if p.DisplayName == "" {
		return fmt.Errorf("missing required field: displayName")
	}
	if p.Version == "" {
		return fmt.Errorf("missing required field: version")
	}
	if p.Description == "" {
		return fmt.Errorf("missing required field: description")
	}
	if len(p.FocusAreas) == 0 {
		return fmt.Errorf("missing required field: focusAreas (must be non-empty)")
	}

	// Name format validation (lowercase-kebab-case)
	nameRegex := regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	if !nameRegex.MatchString(p.Name) {
		return fmt.Errorf("invalid name format %q: must be lowercase-kebab-case (a-z, 0-9, hyphen)", p.Name)
	}

	// Version format validation (semver x.y.z)
	versionRegex := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	if !versionRegex.MatchString(p.Version) {
		return fmt.Errorf("invalid version format %q: must be semver (x.y.z)", p.Version)
	}

	// Severity levels validation
	validSeverities := map[string]bool{
		"critical": true,
		"high":     true,
		"medium":   true,
		"low":      true,
		"info":     true,
	}
	for _, severity := range p.SeverityLevels {
		if !validSeverities[severity] {
			return fmt.Errorf("invalid severity level %q: must be one of critical, high, medium, low, info", severity)
		}
	}

	return nil
}

// ToMap converts the persona to a map for serialization/telemetry.
func (p *Persona) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"name":             p.Name,
		"displayName":      p.DisplayName,
		"version":          p.Version,
		"maturity":         p.Maturity,
		"tier":             p.Tier,
		"description":      p.Description,
		"expertise":        p.Expertise,
		"severityLevels":   p.SeverityLevels,
		"focusAreas":       p.FocusAreas,
		"gitHistoryAccess": p.GitHistoryAccess,
	}
}
