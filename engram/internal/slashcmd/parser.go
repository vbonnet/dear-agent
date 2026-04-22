package slashcmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parameter represents a command parameter definition
type Parameter struct {
	Name         string                 `yaml:"name"`
	Type         string                 `yaml:"type"` // string, choice, boolean, integer
	Required     bool                   `yaml:"required"`
	Description  string                 `yaml:"description"`
	Help         string                 `yaml:"help"`
	Autocomplete string                 `yaml:"autocomplete"`
	Choices      []Choice               `yaml:"choices"`
	Default      interface{}            `yaml:"default"`
	Validation   map[string]interface{} `yaml:"validation"`
}

// Choice represents a parameter choice
type Choice struct {
	Value       string `yaml:"value"`
	Description string `yaml:"description"`
}

// ValidationRule represents a validation rule
type ValidationRule struct {
	Rule    string   `yaml:"rule"`
	Params  []string `yaml:"params"`
	Message string   `yaml:"message"`
	Command string   `yaml:"command"`
}

// SlashCommand represents an enhanced slash command
type SlashCommand struct {
	Name         string                 `yaml:"name"`
	Description  string                 `yaml:"description"`
	ArgumentHint string                 `yaml:"argument-hint"`
	AllowedTools []string               `yaml:"allowed-tools"`
	Parameters   []Parameter            `yaml:"parameters"`
	Validation   map[string]interface{} `yaml:"validation"`
	Composition  map[string]interface{} `yaml:"composition"`
	Context      map[string]interface{} `yaml:"context"`
	Body         string                 // Markdown body (after frontmatter)
}

// ParseCommand parses a slash command file
func ParseCommand(path string) (*SlashCommand, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Split frontmatter and body
	cmd, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, err
	}

	// Set body content
	cmd.Body = body

	return cmd, nil
}

// parseFrontmatter extracts YAML frontmatter and markdown body
func parseFrontmatter(content []byte) (*SlashCommand, string, error) {
	lines := string(content)

	// Check for frontmatter delimiter (---)
	if !startsWith(lines, "---\n") {
		// No frontmatter, treat entire content as body
		return &SlashCommand{}, lines, nil
	}

	// Find end of frontmatter
	endIdx := indexOf(lines[4:], "\n---\n")
	if endIdx == -1 {
		return nil, "", fmt.Errorf("unclosed frontmatter block")
	}
	endIdx += 4 // Adjust for offset

	// Extract frontmatter YAML
	frontmatterYAML := lines[4:endIdx]
	body := lines[endIdx+5:] // Skip past "\n---\n"

	// Parse YAML
	var cmd SlashCommand
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &cmd); err != nil {
		return nil, "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	return &cmd, body, nil
}

// Helper functions
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func indexOf(s, substr string) int {
	idx := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return idx + i
		}
	}
	return -1
}

// AutocompleteProvider gets autocomplete values for a parameter
func (cmd *SlashCommand) AutocompleteProvider(paramName string) ([]string, error) {
	// Find parameter
	for _, param := range cmd.Parameters {
		if param.Name == paramName {
			// Static choices
			if len(param.Choices) > 0 {
				var values []string
				for _, choice := range param.Choices {
					values = append(values, choice.Value)
				}
				return values, nil
			}

			// Dynamic autocomplete (CLI command)
			if param.Autocomplete != "" {
				return executeAutocomplete(param.Autocomplete)
			}
		}
	}

	return []string{}, fmt.Errorf("parameter not found: %s", paramName)
}

// ValidateParams validates command parameters
func (cmd *SlashCommand) ValidateParams(params map[string]interface{}) []error {
	var errors []error

	// Check required parameters
	for _, param := range cmd.Parameters {
		if param.Required {
			if _, ok := params[param.Name]; !ok {
				errors = append(errors, &ValidationError{
					Param:   param.Name,
					Message: param.Name + " is required",
				})
			}
		}
	}

	return errors
}

// ValidationError represents a parameter validation error
type ValidationError struct {
	Param   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// executeAutocomplete runs an autocomplete command and returns its output
func executeAutocomplete(command string) ([]string, error) {
	// Split command into parts (simple shell parsing)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return []string{}, fmt.Errorf("empty autocomplete command")
	}

	// Execute command
	cmd := exec.Command(parts[0], parts[1:]...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return []string{}, fmt.Errorf("autocomplete command failed: %w", err)
	}

	// Parse output (one value per line)
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return []string{}, nil
	}

	lines := strings.Split(output, "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			values = append(values, line)
		}
	}

	return values, nil
}
