// Package personas provides persona data models and loading for engram.
package personas

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Persona represents a persona loaded from an .ai.md file
type Persona struct {
	Name             string   `yaml:"name" json:"name"`
	DisplayName      string   `yaml:"displayName" json:"displayName"`
	Version          string   `yaml:"version" json:"version"`
	Description      string   `yaml:"description" json:"description"`
	Expertise        []string `yaml:"expertise" json:"expertise"`
	SeverityLevels   []string `yaml:"severityLevels" json:"severityLevels"`
	FocusAreas       []string `yaml:"focusAreas" json:"focusAreas"`
	GitHistoryAccess bool     `yaml:"gitHistoryAccess" json:"gitHistoryAccess"`
	Tier             string   `yaml:"tier" json:"tier"`
	Maturity         string   `yaml:"maturity" json:"maturity"`
	Content          string   `yaml:"-" json:"-"`
	SourcePath       string   `yaml:"-" json:"-"`
}

// LoadFromFile loads a persona from an .ai.md file
func LoadFromFile(path string) (*Persona, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	pattern := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)
	matches := pattern.FindSubmatch(content)
	if len(matches) < 3 {
		return nil, fmt.Errorf("file %s is missing YAML frontmatter", path)
	}
	frontmatterYAML := matches[1]
	markdownContent := matches[2]
	var persona Persona
	err = yaml.Unmarshal(frontmatterYAML, &persona)
	if err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter in %s: %w", path, err)
	}
	if persona.Version == "" {
		persona.Version = "1.0.0"
	}
	if persona.Tier == "" {
		persona.Tier = "tier2"
	}
	if persona.Maturity == "" {
		persona.Maturity = "stable"
	}
	if persona.Expertise == nil {
		persona.Expertise = []string{}
	}
	if persona.SeverityLevels == nil {
		persona.SeverityLevels = []string{}
	}
	if persona.FocusAreas == nil {
		persona.FocusAreas = []string{}
	}
	persona.Content = string(markdownContent)
	persona.SourcePath = path
	return &persona, nil
}

// IsExperimental checks if persona is experimental
func (p *Persona) IsExperimental() bool {
	return p.Maturity == "experimental"
}

// IsStable checks if persona is stable
func (p *Persona) IsStable() bool {
	return p.Maturity == "stable"
}

// ToMap converts persona to map
func (p *Persona) ToMap() map[string]any {
	return map[string]any{
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

// Loader loads personas from the persona library
type Loader struct {
	libraryPath string
}

// NewLoader creates a new PersonaLoader
func NewLoader(libraryPath string) (*Loader, error) {
	if libraryPath == "" {
		return nil, fmt.Errorf("library path cannot be empty")
	}
	return &Loader{libraryPath: libraryPath}, nil
}

// Load loads a persona by name
func (l *Loader) Load(personaName string) (*Persona, error) {
	pattern := filepath.Join(l.libraryPath, "**", personaName+".ai.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob error: %w", err)
	}
	if len(matches) == 0 {
		matches, err = l.rglob(l.libraryPath, personaName+".ai.md")
		if err != nil {
			return nil, fmt.Errorf("search error: %w", err)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("persona '%s' not found in library %s", personaName, l.libraryPath)
	}
	return LoadFromFile(matches[0])
}

// ListPersonas lists all personas in the library
func (l *Loader) ListPersonas() ([]*Persona, error) {
	var personas []*Persona
	matches, err := l.rglob(l.libraryPath, "*.ai.md")
	if err != nil {
		return nil, fmt.Errorf("search error: %w", err)
	}
	for _, personaFile := range matches {
		persona, err := LoadFromFile(personaFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to load %s: %v\n", personaFile, err)
			continue
		}
		personas = append(personas, persona)
	}
	return personas, nil
}

// ListExperimentalPersonas lists all experimental personas
func (l *Loader) ListExperimentalPersonas() ([]*Persona, error) {
	allPersonas, err := l.ListPersonas()
	if err != nil {
		return nil, err
	}
	var experimental []*Persona
	for _, p := range allPersonas {
		if p.IsExperimental() {
			experimental = append(experimental, p)
		}
	}
	return experimental, nil
}

// ListStablePersonas lists all stable personas
func (l *Loader) ListStablePersonas() ([]*Persona, error) {
	allPersonas, err := l.ListPersonas()
	if err != nil {
		return nil, err
	}
	var stable []*Persona
	for _, p := range allPersonas {
		if p.IsStable() {
			stable = append(stable, p)
		}
	}
	return stable, nil
}

func (l *Loader) rglob(root, pattern string) ([]string, error) {
	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil {
			return err
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}
