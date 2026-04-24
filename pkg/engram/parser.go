package engram

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Parser handles parsing .ai.md engram files
type Parser struct{}

// NewParser creates a new engram parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses an engram file and returns an Engram
func (p *Parser) Parse(path string) (*Engram, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read engram file: %w", err)
	}

	return p.ParseBytes(path, data)
}

// ParseBytes parses engram content from bytes
func (p *Parser) ParseBytes(path string, data []byte) (*Engram, error) {
	// Split frontmatter and content
	frontmatter, content, err := p.splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	// Parse frontmatter YAML
	var fm Frontmatter
	if err := yaml.Unmarshal(frontmatter, &fm); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Apply defaults for missing metadata fields (backward compatibility)
	if fm.EncodingStrength == 0.0 {
		fm.EncodingStrength = 1.0 // Default neutral strength
	}

	// RetrievalCount defaults to 0 (zero value is correct)

	// Initialize CreatedAt from file mtime if missing (for legacy engrams)
	if fm.CreatedAt.IsZero() {
		info, err := os.Stat(path)
		if err == nil {
			fm.CreatedAt = info.ModTime()
		}
		// If stat fails, leave as zero (will be set on first tracking update)
	}

	// LastAccessed defaults to zero value (never accessed) - no initialization needed

	return &Engram{
		Path:        path,
		Frontmatter: fm,
		Content:     string(content),
	}, nil
}

// splitFrontmatter splits a markdown file into frontmatter and content
// Expects format:
// ---
// frontmatter: here
// ---
// content here
func (p *Parser) splitFrontmatter(data []byte) (frontmatter, content []byte, err error) {
	// Must start with ---
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, nil, fmt.Errorf("missing frontmatter delimiter")
	}

	// Find closing ---
	rest := data[4:] // Skip opening ---\n
	idx := bytes.Index(rest, []byte("\n---\n"))
	if idx == -1 {
		return nil, nil, fmt.Errorf("missing closing frontmatter delimiter")
	}

	frontmatter = rest[:idx]
	content = rest[idx+5:] // Skip \n---\n

	return frontmatter, content, nil
}
