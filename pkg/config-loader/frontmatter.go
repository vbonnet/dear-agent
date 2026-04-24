package configloader

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseFrontmatter extracts YAML frontmatter from markdown content.
//
// Expects content in the format:
//
//	---
//	key: value
//	---
//	Content here...
//
// Returns:
//   - frontmatter: YAML string (without delimiters)
//   - content: Remaining markdown content (trimmed)
//   - error: If frontmatter is missing or malformed
//
// Example:
//
//	fm, content, err := ParseFrontmatter(fileContent)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// fm = "key: value\n"
//	// content = "Content here..."
func ParseFrontmatter(content string) (frontmatter string, body string, err error) {
	// Check if content starts with frontmatter delimiter
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("missing frontmatter: content must start with '---'")
	}

	// Find frontmatter boundaries using regex with DOTALL flag
	// - Opening "---" at start
	// - YAML content (non-greedy, including newlines)
	// - Closing "---"
	// - Body content
	re := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)$`)
	matches := re.FindStringSubmatch(content)

	if matches == nil {
		return "", "", fmt.Errorf("invalid frontmatter format: expected '---\\n...\\n---\\n'")
	}

	// matches[0] = full match
	// matches[1] = frontmatter content
	// matches[2] = body content
	frontmatter = matches[1]
	body = strings.TrimSpace(matches[2])

	return frontmatter, body, nil
}

// ParseFrontmatterStrict is like ParseFrontmatter but also validates that
// the frontmatter is valid YAML (not just extracting it).
//
// This is useful when you want to fail fast on invalid YAML without
// needing to unmarshal into a specific struct.
//
// Example:
//
//	fm, content, err := ParseFrontmatterStrict(fileContent)
//	// err will be returned if YAML is invalid
func ParseFrontmatterStrict(content string) (frontmatter string, body string, err error) {
	fm, body, err := ParseFrontmatter(content)
	if err != nil {
		return "", "", err
	}

	// Validate YAML syntax by attempting to parse it
	var parsed interface{}
	if err := yaml.Unmarshal([]byte(fm), &parsed); err != nil {
		return "", "", fmt.Errorf("invalid YAML in frontmatter: %w", err)
	}

	return fm, body, nil
}

// HasFrontmatter checks if content contains frontmatter without parsing.
//
// Returns true if content starts with "---" delimiter.
// Useful for quick checks before parsing.
//
// Example:
//
//	if HasFrontmatter(content) {
//	    fm, body, _ := ParseFrontmatter(content)
//	    // ... process frontmatter
//	}
func HasFrontmatter(content string) bool {
	return strings.HasPrefix(content, "---\n")
}
