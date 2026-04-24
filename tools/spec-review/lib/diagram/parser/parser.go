// Package parser provides multi-format diagram parsing (D2, Structurizr, Mermaid)
package parser

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/tools/spec-review/lib/diagram/c4model"
)

// Parser parses diagrams from different formats into C4 model structures.
type Parser struct {
	Format string
}

// New creates a new diagram parser for the specified format.
func New(format string) *Parser {
	return &Parser{Format: format}
}

// Parse parses a diagram file into C4 model Diagram structure.
func (p *Parser) Parse(filePath string) (*c4model.Diagram, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read diagram: %w", err)
	}

	switch p.Format {
	case "d2":
		return p.parseD2(string(content))
	case "structurizr":
		return p.parseStructurizr(string(content))
	case "mermaid":
		return p.parseMermaid(string(content))
	default:
		return nil, fmt.Errorf("unsupported format: %s", p.Format)
	}
}

// parseD2 parses D2 diagram syntax into C4 model.
func (p *Parser) parseD2(content string) (*c4model.Diagram, error) {
	diagram := &c4model.Diagram{
		Elements:      []*c4model.Element{},
		Relationships: []*c4model.Relationship{},
	}

	scanner := bufio.NewScanner(strings.NewReader(content))

	// D2 patterns
	personPattern := regexp.MustCompile(`^(\w+):\s*\{\s*shape:\s*person`)
	elementPattern := regexp.MustCompile(`^(\w+):\s*\{`)
	labelPattern := regexp.MustCompile(`label:\s*"([^"]+)"`)
	relationPattern := regexp.MustCompile(`^(\w+)\s*->\s*(\w+):\s*"([^"]*)"`)

	elementMap := make(map[string]*c4model.Element)
	var currentElement *c4model.Element
	inBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Check for person element
		if personPattern.MatchString(line) {
			matches := personPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Type: c4model.TypePerson,
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
			currentElement = elem
			inBlock = true
			continue
		}

		// Check for general element
		if elementPattern.MatchString(line) {
			matches := elementPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Type: c4model.TypeSoftwareSystem, // Default assumption
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
			currentElement = elem
			inBlock = true
			continue
		}

		// Parse label
		if inBlock && labelPattern.MatchString(line) {
			matches := labelPattern.FindStringSubmatch(line)
			if currentElement != nil {
				currentElement.Name = matches[1]
			}
		}

		// Parse relationship
		if relationPattern.MatchString(line) {
			matches := relationPattern.FindStringSubmatch(line)
			sourceID := matches[1]
			targetID := matches[2]
			desc := matches[3]

			source := elementMap[sourceID]
			target := elementMap[targetID]

			if source != nil && target != nil {
				rel := &c4model.Relationship{
					Source:      source,
					Destination: target,
					Type:        c4model.RelUses,
					Description: desc,
				}
				diagram.Relationships = append(diagram.Relationships, rel)
			}
		}

		// End of block
		if line == "}" {
			inBlock = false
			currentElement = nil
		}
	}

	// Auto-detect C4 level based on element types
	diagram.Level = detectC4Level(diagram.Elements)

	return diagram, nil
}

// parseStructurizr parses Structurizr DSL into C4 model.
func (p *Parser) parseStructurizr(content string) (*c4model.Diagram, error) {
	diagram := &c4model.Diagram{
		Elements:      []*c4model.Element{},
		Relationships: []*c4model.Relationship{},
	}

	scanner := bufio.NewScanner(strings.NewReader(content))

	// Structurizr patterns
	personPattern := regexp.MustCompile(`(\w+)\s*=\s*person\s+"([^"]+)"`)
	systemPattern := regexp.MustCompile(`(\w+)\s*=\s*softwareSystem\s+"([^"]+)"`)
	containerPattern := regexp.MustCompile(`(\w+)\s*=\s*container\s+"([^"]+)"`)
	componentPattern := regexp.MustCompile(`(\w+)\s*=\s*component\s+"([^"]+)"`)
	relationPattern := regexp.MustCompile(`(\w+)\s*->\s*(\w+)\s+"([^"]*)"`)

	elementMap := make(map[string]*c4model.Element)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Parse person
		if personPattern.MatchString(line) {
			matches := personPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Name: matches[2],
				Type: c4model.TypePerson,
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
		}

		// Parse software system
		if systemPattern.MatchString(line) {
			matches := systemPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Name: matches[2],
				Type: c4model.TypeSoftwareSystem,
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
		}

		// Parse container
		if containerPattern.MatchString(line) {
			matches := containerPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Name: matches[2],
				Type: c4model.TypeContainer,
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
		}

		// Parse component
		if componentPattern.MatchString(line) {
			matches := componentPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Name: matches[2],
				Type: c4model.TypeComponent,
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
		}

		// Parse relationship
		if relationPattern.MatchString(line) {
			matches := relationPattern.FindStringSubmatch(line)
			sourceID := matches[1]
			targetID := matches[2]
			desc := matches[3]

			source := elementMap[sourceID]
			target := elementMap[targetID]

			if source != nil && target != nil {
				rel := &c4model.Relationship{
					Source:      source,
					Destination: target,
					Type:        c4model.RelUses,
					Description: desc,
				}
				diagram.Relationships = append(diagram.Relationships, rel)
			}
		}
	}

	// Auto-detect C4 level
	diagram.Level = detectC4Level(diagram.Elements)

	return diagram, nil
}

// parseMermaid parses Mermaid C4 diagram syntax into C4 model.
func (p *Parser) parseMermaid(content string) (*c4model.Diagram, error) {
	diagram := &c4model.Diagram{
		Elements:      []*c4model.Element{},
		Relationships: []*c4model.Relationship{},
	}

	scanner := bufio.NewScanner(strings.NewReader(content))

	// Mermaid C4 patterns
	personPattern := regexp.MustCompile(`Person\((\w+),\s*"([^"]+)"`)
	systemPattern := regexp.MustCompile(`System\((\w+),\s*"([^"]+)"`)
	containerPattern := regexp.MustCompile(`Container\((\w+),\s*"([^"]+)"`)
	componentPattern := regexp.MustCompile(`Component\((\w+),\s*"([^"]+)"`)
	relationPattern := regexp.MustCompile(`Rel\((\w+),\s*(\w+),\s*"([^"]*)"`)

	elementMap := make(map[string]*c4model.Element)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Parse person
		if personPattern.MatchString(line) {
			matches := personPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Name: matches[2],
				Type: c4model.TypePerson,
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
		}

		// Parse system
		if systemPattern.MatchString(line) {
			matches := systemPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Name: matches[2],
				Type: c4model.TypeSoftwareSystem,
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
		}

		// Parse container
		if containerPattern.MatchString(line) {
			matches := containerPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Name: matches[2],
				Type: c4model.TypeContainer,
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
		}

		// Parse component
		if componentPattern.MatchString(line) {
			matches := componentPattern.FindStringSubmatch(line)
			elem := &c4model.Element{
				ID:   matches[1],
				Name: matches[2],
				Type: c4model.TypeComponent,
			}
			diagram.Elements = append(diagram.Elements, elem)
			elementMap[elem.ID] = elem
		}

		// Parse relationship
		if relationPattern.MatchString(line) {
			matches := relationPattern.FindStringSubmatch(line)
			sourceID := matches[1]
			targetID := matches[2]
			desc := matches[3]

			source := elementMap[sourceID]
			target := elementMap[targetID]

			if source != nil && target != nil {
				rel := &c4model.Relationship{
					Source:      source,
					Destination: target,
					Type:        c4model.RelUses,
					Description: desc,
				}
				diagram.Relationships = append(diagram.Relationships, rel)
			}
		}
	}

	// Auto-detect C4 level
	diagram.Level = detectC4Level(diagram.Elements)

	return diagram, nil
}

// detectC4Level determines the C4 level based on element types present.
func detectC4Level(elements []*c4model.Element) c4model.Level {
	hasComponents := false
	hasContainers := false
	hasSystems := false

	for _, elem := range elements {
		switch elem.Type {
		case c4model.TypeComponent:
			hasComponents = true
		case c4model.TypeContainer, c4model.TypeDatabase, c4model.TypeQueue:
			hasContainers = true
		case c4model.TypeSoftwareSystem, c4model.TypeExternalSystem:
			hasSystems = true
		case c4model.TypePerson:
			// Persons exist at context level but don't determine the level
		case c4model.TypeClass, c4model.TypeInterface:
			// Class/Interface types don't map to C4 levels
		}
	}

	if hasComponents {
		return c4model.LevelComponent
	}
	if hasContainers {
		return c4model.LevelContainer
	}
	if hasSystems {
		return c4model.LevelContext
	}

	return c4model.LevelContext // Default to Context
}

// DetectFormat detects diagram format from file extension or content.
func DetectFormat(filePath string) string {
	if strings.HasSuffix(filePath, ".d2") {
		return "d2"
	}
	if strings.HasSuffix(filePath, ".dsl") {
		return "structurizr"
	}
	if strings.HasSuffix(filePath, ".mmd") || strings.HasSuffix(filePath, ".mermaid") {
		return "mermaid"
	}

	// Try content-based detection
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "unknown"
	}

	contentStr := string(content)
	if strings.Contains(contentStr, "workspace {") {
		return "structurizr"
	}
	if strings.Contains(contentStr, "C4Context") || strings.Contains(contentStr, "C4Container") {
		return "mermaid"
	}
	if strings.Contains(contentStr, "shape: person") || strings.Contains(contentStr, "direction:") {
		return "d2"
	}

	return "unknown"
}
