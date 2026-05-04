// Package engram provides validation for .ai.md engram files.
//
// Detects anti-patterns and validates structure based on Agent Prompt Pattern Guide.
package engram

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationError represents a validation error found in an engram file.
type ValidationError struct {
	FilePath  string
	Line      *int // nil if error is not associated with a specific line
	ErrorType string
	Message   string
}

// Validator validates .ai.md engram files for anti-patterns and structure.
type Validator struct {
	filePath string
	errors   []ValidationError
}

// Required frontmatter fields and their types
var (
	requiredFrontmatterFields = map[string]interface{}{
		"type":        []string{"reference", "template", "workflow", "guide"},
		"title":       "string",
		"description": "string",
	}

	// Context reference patterns (Principle 1 violation)
	contextReferencePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:see|as mentioned|mentioned|discussed|described|refer to the|reference the)\s+(?:above|earlier|previously|before)\b`),
		regexp.MustCompile(`(?i)\b(?:previous|earlier)\s+(?:section|example|pattern|discussion)\b`),
		regexp.MustCompile(`(?i)\bas\s+discussed\b`),
		regexp.MustCompile(`(?i)\bthe\s+(?:pattern|example|approach)\s+(?:mentioned|described|discussed)\s+(?:above|earlier|previously)\b`),
	}

	// Vague verbs without specifics (Principle 2 violation)
	vagueVerbs = []string{
		"improve", "optimize", "fix", "enhance", "update", "refactor",
	}

	// Measurable criteria keywords (to check if vague verbs are made specific)
	measurableCriteriaKeywords = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\btarget:`),
		regexp.MustCompile(`\b\d+%`),                               // percentages like 80%
		regexp.MustCompile(`(?i)\b<\d+(?:ms|s|minutes?|hours?)\b`), // time bounds like <100ms
		regexp.MustCompile(`(?i)\badd\s+(?:try-catch|index|test|logging|error handling)`),
		regexp.MustCompile(`(?i)\bwith\s+(?:stack traces?|error messages?)`),
		regexp.MustCompile(`(?i)\bby\s+(?:adding|using|implementing)`),
		regexp.MustCompile(`(?i)\bmust\s+(?:be|have|include)`),
		regexp.MustCompile(`(?i)\bshould\s+(?:be|have|include)`),
	}

	// Task keywords (to detect tasks that need constraints)
	taskKeywords = []string{
		"implement", "create", "build", "generate", "write", "develop",
	}

	// Constraint keywords (to check if tasks have constraints)
	constraintKeywords = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\btoken budget:`),
		regexp.MustCompile(`(?i)\bfile limit:`),
		regexp.MustCompile(`(?i)\bmax\s+\d+\s+files?\b`),
		regexp.MustCompile(`(?i)\bscope:`),
		regexp.MustCompile(`(?i)\btime\s+bound:`),
		regexp.MustCompile(`(?i)\bcomplete\s+in\s+(?:single|one)`),
		regexp.MustCompile(`(?i)\bunder\s+\d+\s+tokens?\b`),
		regexp.MustCompile(`(?i)\bdon't\s+(?:touch|modify|change)`),
		regexp.MustCompile(`(?i)\bconstraints?:`),
	}

	// Principle keywords (to detect principles that need examples)
	principleKeywords = []string{
		"principle", "pattern", "guideline", "rule", "best practice",
	}
)

// NewValidator creates a new Validator for the given file path.
func NewValidator(filePath string) *Validator {
	return &Validator{
		filePath: filePath,
		errors:   []ValidationError{},
	}
}

// Validate runs all validations and returns a list of errors.
func (v *Validator) Validate() []ValidationError {
	v.errors = []ValidationError{}

	content, err := os.ReadFile(v.filePath)
	if err != nil {
		v.errors = append(v.errors, ValidationError{
			FilePath:  v.filePath,
			Line:      nil,
			ErrorType: "file_error",
			Message:   fmt.Sprintf("Failed to read file: %v", err),
		})
		return v.errors
	}

	contentStr := string(content)

	// Validate frontmatter
	v.validateFrontmatter(contentStr)

	// Validate content for anti-patterns
	v.detectContextReferences(contentStr)
	v.detectVagueVerbs(contentStr)
	v.detectMissingExamples(contentStr)
	v.detectMissingConstraints(contentStr)

	return v.errors
}

// validateFrontmatter validates YAML frontmatter structure and required fields.
//nolint:gocyclo // reason: linear field-by-field frontmatter validation
func (v *Validator) validateFrontmatter(content string) {
	// Extract frontmatter using regex
	frontmatterRegex := regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n`)
	matches := frontmatterRegex.FindStringSubmatch(content)

	if matches == nil {
		line := 1
		v.errors = append(v.errors, ValidationError{
			FilePath:  v.filePath,
			Line:      &line,
			ErrorType: "missing_frontmatter",
			Message:   "Missing YAML frontmatter (must start with ---)",
		})
		return
	}

	frontmatterYAML := matches[1]

	// Parse YAML
	var frontmatter map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &frontmatter); err != nil {
		line := 1
		v.errors = append(v.errors, ValidationError{
			FilePath:  v.filePath,
			Line:      &line,
			ErrorType: "invalid_frontmatter",
			Message:   fmt.Sprintf("Invalid YAML frontmatter: %v", err),
		})
		return
	}

	// Validate required fields
	for field, fieldType := range requiredFrontmatterFields {
		value, exists := frontmatter[field]
		if !exists {
			line := 1
			v.errors = append(v.errors, ValidationError{
				FilePath:  v.filePath,
				Line:      &line,
				ErrorType: "missing_field",
				Message:   fmt.Sprintf("Missing required frontmatter field: %s", field),
			})
			continue
		}

		// Validate type field (must be one of allowed values)
		if field == "type" {
			allowedTypes, ok := fieldType.([]string)
			if !ok {
				continue
			}
			typeStr, ok := value.(string)
			if !ok {
				line := 1
				v.errors = append(v.errors, ValidationError{
					FilePath:  v.filePath,
					Line:      &line,
					ErrorType: "invalid_type",
					Message:   "Type must be a string",
				})
				continue
			}

			isValid := false
			for _, allowed := range allowedTypes {
				if typeStr == allowed {
					isValid = true
					break
				}
			}
			if !isValid {
				line := 1
				v.errors = append(v.errors, ValidationError{
					FilePath:  v.filePath,
					Line:      &line,
					ErrorType: "invalid_type",
					Message:   fmt.Sprintf("Invalid type \"%s\". Must be one of: %s", typeStr, strings.Join(allowedTypes, ", ")),
				})
			}
		}

		// Validate title field (must be non-empty string)
		if field == "title" {
			titleStr, ok := value.(string)
			if !ok || strings.TrimSpace(titleStr) == "" {
				line := 1
				v.errors = append(v.errors, ValidationError{
					FilePath:  v.filePath,
					Line:      &line,
					ErrorType: "invalid_title",
					Message:   "Title must be a non-empty string",
				})
			}
		}

		// Validate description field (must be non-empty string, <200 chars)
		if field == "description" {
			descStr, ok := value.(string)
			if !ok || strings.TrimSpace(descStr) == "" {
				line := 1
				v.errors = append(v.errors, ValidationError{
					FilePath:  v.filePath,
					Line:      &line,
					ErrorType: "invalid_description",
					Message:   "Description must be a non-empty string",
				})
			} else if len(descStr) > 200 {
				line := 1
				v.errors = append(v.errors, ValidationError{
					FilePath:  v.filePath,
					Line:      &line,
					ErrorType: "description_too_long",
					Message:   fmt.Sprintf("Description too long (%d chars, max 200)", len(descStr)),
				})
			}
		}
	}
}

// detectContextReferences detects context references like 'see above', 'mentioned earlier'.
func (v *Validator) detectContextReferences(content string) {
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		// Skip frontmatter (first ~20 lines containing ---)
		if lineNum < 20 && strings.Contains(line, "---") {
			continue
		}

		for _, pattern := range contextReferencePatterns {
			if match := pattern.FindString(line); match != "" {
				lineNumInt := lineNum + 1 // Convert to 1-based indexing
				v.errors = append(v.errors, ValidationError{
					FilePath:  v.filePath,
					Line:      &lineNumInt,
					ErrorType: "context_reference",
					Message:   fmt.Sprintf("Context reference detected: \"%s\"", match),
				})
				break // Only report once per line
			}
		}
	}
}

// detectVagueVerbs detects vague verbs without measurable criteria.
func (v *Validator) detectVagueVerbs(content string) {
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		lineLower := strings.ToLower(line)

		// Check for vague verbs
		for _, verb := range vagueVerbs {
			verbPattern := regexp.MustCompile(`\b` + verb + `\b`)
			if verbPattern.MatchString(lineLower) {
				// Check if measurable criteria are present within 50 tokens
				// (approximate by checking current line and next 2 lines)
				endIdx := lineNum + 3
				if endIdx > len(lines) {
					endIdx = len(lines)
				}
				contextWindow := strings.Join(lines[lineNum:endIdx], "\n")

				hasCriteria := false
				for _, criteriaPattern := range measurableCriteriaKeywords {
					if criteriaPattern.MatchString(contextWindow) {
						hasCriteria = true
						break
					}
				}

				if !hasCriteria {
					lineNumInt := lineNum + 1
					v.errors = append(v.errors, ValidationError{
						FilePath:  v.filePath,
						Line:      &lineNumInt,
						ErrorType: "vague_verb",
						Message:   fmt.Sprintf("Vague verb \"%s\" without measurable criteria", verb),
					})
					break // Only report once per line
				}
			}
		}
	}
}

// detectMissingExamples detects principles/patterns without examples.
func (v *Validator) detectMissingExamples(content string) {
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		lineLower := strings.ToLower(line)

		// Check if line mentions a principle/pattern/guideline
		hasPrincipleKeyword := false
		for _, keyword := range principleKeywords {
			if strings.Contains(lineLower, keyword) {
				hasPrincipleKeyword = true
				break
			}
		}

		if hasPrincipleKeyword {
			// Check if there's an example within next 500 tokens (approx 20 lines)
			endIdx := lineNum + 21
			if endIdx > len(lines) {
				endIdx = len(lines)
			}
			contextWindow := strings.Join(lines[lineNum:endIdx], "\n")
			contextWindowLower := strings.ToLower(contextWindow)

			// Look for code blocks (```) or example sections
			hasExample := strings.Contains(contextWindow, "```") ||
				strings.Contains(contextWindowLower, "example:") ||
				strings.Contains(contextWindowLower, "good example") ||
				strings.Contains(contextWindowLower, "bad example")

			if !hasExample && strings.Contains(line, "##") { // Only flag if it's a header
				lineNumInt := lineNum + 1
				v.errors = append(v.errors, ValidationError{
					FilePath:  v.filePath,
					Line:      &lineNumInt,
					ErrorType: "missing_example",
					Message:   "Principle/pattern without example within 20 lines",
				})
			}
		}
	}
}

// detectMissingConstraints detects tasks without constraints.
func (v *Validator) detectMissingConstraints(content string) {
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		lineLower := strings.ToLower(line)

		// Check if line contains a task keyword
		hasTaskKeyword := false
		var taskVerb string
		for _, verb := range taskKeywords {
			verbPattern := regexp.MustCompile(`\b` + verb + `\b`)
			if verbPattern.MatchString(lineLower) {
				hasTaskKeyword = true
				taskVerb = verb
				break
			}
		}

		if hasTaskKeyword {
			// Check if there are constraints within next 10 lines
			endIdx := lineNum + 11
			if endIdx > len(lines) {
				endIdx = len(lines)
			}
			contextWindow := strings.Join(lines[lineNum:endIdx], "\n")

			hasConstraints := false
			for _, constraintPattern := range constraintKeywords {
				if constraintPattern.MatchString(contextWindow) {
					hasConstraints = true
					break
				}
			}

			if !hasConstraints {
				lineNumInt := lineNum + 1
				v.errors = append(v.errors, ValidationError{
					FilePath:  v.filePath,
					Line:      &lineNumInt,
					ErrorType: "missing_constraints",
					Message:   fmt.Sprintf("Task verb \"%s\" without constraints (scope, limits, bounds)", taskVerb),
				})
			}
		}
	}
}

// ValidateFile validates a single .ai.md file and returns errors.
func ValidateFile(filePath string) ([]ValidationError, error) {
	validator := NewValidator(filePath)
	errors := validator.Validate()
	return errors, nil
}

// ValidateDirectory validates all .ai.md files in directory and subdirectories.
func ValidateDirectory(directory string) (map[string][]ValidationError, error) {
	results := make(map[string][]ValidationError)

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".ai.md") {
			errors, _ := ValidateFile(path)
			results[path] = errors
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}
