// Package validator provides validation for .ai.md engram files.
//
// It validates engram files based on the Agent Prompt Pattern Guide,
// checking for structural issues and anti-patterns.
package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// EngramValidationError represents a validation error found in an engram file.
type EngramValidationError struct {
	FilePath  string
	Line      int // 0 if not applicable
	ErrorType string
	Message   string
}

// EngramValidator validates .ai.md engram files for anti-patterns and structure.
type EngramValidator struct {
	filePath string
	errors   []EngramValidationError
}

// Frontmatter represents the YAML frontmatter structure.
type Frontmatter struct {
	Type        string `yaml:"type"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
}

// Validation rule types
const (
	ErrorTypeMissingFrontmatter = "missing_frontmatter"
	ErrorTypeInvalidFrontmatter = "invalid_frontmatter"
	ErrorTypeMissingField       = "missing_field"
	ErrorTypeInvalidType        = "invalid_type"
	ErrorTypeInvalidTitle       = "invalid_title"
	ErrorTypeInvalidDescription = "invalid_description"
	ErrorTypeDescriptionTooLong = "description_too_long"
	ErrorTypeContextReference   = "context_reference"
	ErrorTypeVagueVerb          = "vague_verb"
	ErrorTypeMissingExample     = "missing_example"
	ErrorTypeMissingConstraints = "missing_constraints"
	ErrorTypeFileError          = "file_error"
)

// Valid frontmatter types
var validTypes = []string{"reference", "template", "workflow", "guide"}

// Context reference patterns (Principle 1 violation)
var contextReferencePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:see|as mentioned|mentioned|discussed|described|reference)\s+(?:above|earlier|previously|before)\b`),
	regexp.MustCompile(`(?i)\brefer to the\s+(?:above|earlier|previously|before|section|example|pattern|discussion)`),
	regexp.MustCompile(`(?i)\b(?:previous|earlier)\s+(?:section|example|pattern|discussion)\b`),
	regexp.MustCompile(`(?i)\bas\s+discussed\b`),
	regexp.MustCompile(`(?i)\bthe\s+(?:pattern|example|approach)\s+(?:mentioned|described|discussed)\s+(?:above|earlier|previously)\b`),
}

// Vague verbs without specifics (Principle 2 violation)
var vagueVerbs = []string{"improve", "optimize", "fix", "enhance", "update", "refactor"}

// Measurable criteria keywords (to check if vague verbs are made specific)
var measurableCriteriaPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\btarget:`),
	regexp.MustCompile(`\b\d+%`),                                       // percentages like 80%
	regexp.MustCompile(`(?i)(?:to\s+)?<\d+(?:ms|s|minutes?|hours?)\b`), // time bounds like <100ms or "to <100ms"
	regexp.MustCompile(`(?i)\badd\s+(?:try-catch|index|test|logging|error handling)`),
	regexp.MustCompile(`(?i)\bwith\s+(?:stack traces?|error messages?)`),
	regexp.MustCompile(`(?i)\bby\s+(?:adding|using|implementing)`),
	regexp.MustCompile(`(?i)\bmust\s+(?:be|have|include)`),
	regexp.MustCompile(`(?i)\bshould\s+(?:be|have|include)`),
}

// Task keywords (to detect tasks that need constraints)
var taskKeywords = []string{"implement", "create", "build", "generate", "write", "develop"}

// Constraint keywords (to check if tasks have constraints)
var constraintPatterns = []*regexp.Regexp{
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
var principleKeywords = []string{"principle", "pattern", "guideline", "rule", "best practice"}

// New creates a new EngramValidator for the specified file.
func New(filePath string) *EngramValidator {
	return &EngramValidator{
		filePath: filePath,
		errors:   []EngramValidationError{},
	}
}

// Validate runs all validation rules and returns the list of errors.
func (v *EngramValidator) Validate() []EngramValidationError {
	v.errors = []EngramValidationError{}

	content, err := os.ReadFile(v.filePath)
	if err != nil {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      0,
			ErrorType: ErrorTypeFileError,
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
func (v *EngramValidator) validateFrontmatter(content string) {
	// Extract frontmatter using regex
	frontmatterRegex := regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n`)
	matches := frontmatterRegex.FindStringSubmatch(content)

	if matches == nil {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      1,
			ErrorType: ErrorTypeMissingFrontmatter,
			Message:   "Missing YAML frontmatter (must start with ---)",
		})
		return
	}

	// Parse YAML
	var fm Frontmatter
	err := yaml.Unmarshal([]byte(matches[1]), &fm)
	if err != nil {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      1,
			ErrorType: ErrorTypeInvalidFrontmatter,
			Message:   fmt.Sprintf("Invalid YAML frontmatter: %v", err),
		})
		return
	}

	// Validate required fields
	// Check for missing fields first (field not in YAML at all)
	var rawMap map[string]interface{}
	yaml.Unmarshal([]byte(matches[1]), &rawMap)

	if _, exists := rawMap["type"]; !exists {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      1,
			ErrorType: ErrorTypeMissingField,
			Message:   "Missing required frontmatter field: type",
		})
	} else {
		v.validateType(fm.Type)
	}

	if _, exists := rawMap["title"]; !exists {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      1,
			ErrorType: ErrorTypeMissingField,
			Message:   "Missing required frontmatter field: title",
		})
	} else {
		v.validateTitle(fm.Title)
	}

	if _, exists := rawMap["description"]; !exists {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      1,
			ErrorType: ErrorTypeMissingField,
			Message:   "Missing required frontmatter field: description",
		})
	} else {
		v.validateDescription(fm.Description)
	}
}

// validateType validates the type field (assumes field exists).
func (v *EngramValidator) validateType(typeVal string) {
	valid := false
	for _, validType := range validTypes {
		if typeVal == validType {
			valid = true
			break
		}
	}

	if !valid {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      1,
			ErrorType: ErrorTypeInvalidType,
			Message:   fmt.Sprintf("Invalid type \"%s\". Must be one of: %s", typeVal, strings.Join(validTypes, ", ")),
		})
	}
}

// validateTitle validates the title field (assumes field exists).
func (v *EngramValidator) validateTitle(title string) {
	if title == "" || strings.TrimSpace(title) == "" {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      1,
			ErrorType: ErrorTypeInvalidTitle,
			Message:   "Title must be a non-empty string",
		})
	}
}

// validateDescription validates the description field (assumes field exists).
func (v *EngramValidator) validateDescription(description string) {
	if description == "" || strings.TrimSpace(description) == "" {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      1,
			ErrorType: ErrorTypeInvalidDescription,
			Message:   "Description must be a non-empty string",
		})
		return
	}

	if len(description) > 200 {
		v.errors = append(v.errors, EngramValidationError{
			FilePath:  v.filePath,
			Line:      1,
			ErrorType: ErrorTypeDescriptionTooLong,
			Message:   fmt.Sprintf("Description too long (%d chars, max 200)", len(description)),
		})
	}
}

// detectContextReferences detects context references like 'see above', 'mentioned earlier'.
func (v *EngramValidator) detectContextReferences(content string) {
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		// Skip frontmatter (approximate check)
		if lineNum < 20 && strings.Contains(line, "---") {
			continue
		}

		lineLower := strings.ToLower(line)

		for _, pattern := range contextReferencePatterns {
			if match := pattern.FindString(lineLower); match != "" {
				v.errors = append(v.errors, EngramValidationError{
					FilePath:  v.filePath,
					Line:      lineNum + 1,
					ErrorType: ErrorTypeContextReference,
					Message:   fmt.Sprintf("Context reference detected: \"%s\"", match),
				})
				break // Only report once per line
			}
		}
	}
}

// detectVagueVerbs detects vague verbs without measurable criteria.
func (v *EngramValidator) detectVagueVerbs(content string) {
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		lineLower := strings.ToLower(line)

		// Check for vague verbs
		for _, verb := range vagueVerbs {
			verbPattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(verb) + `\b`)
			if verbPattern.MatchString(lineLower) {
				// Check if measurable criteria are present within 50 tokens
				// (approximate by checking current line and next 2 lines)
				endIdx := lineNum + 3
				if endIdx > len(lines) {
					endIdx = len(lines)
				}
				contextWindow := strings.Join(lines[lineNum:endIdx], "\n")

				hasCriteria := false
				for _, criteriaPattern := range measurableCriteriaPatterns {
					if criteriaPattern.MatchString(contextWindow) {
						hasCriteria = true
						break
					}
				}

				if !hasCriteria {
					v.errors = append(v.errors, EngramValidationError{
						FilePath:  v.filePath,
						Line:      lineNum + 1,
						ErrorType: ErrorTypeVagueVerb,
						Message:   fmt.Sprintf("Vague verb \"%s\" without measurable criteria", verb),
					})
					break // Only report once per line
				}
			}
		}
	}
}

// detectMissingExamples detects principles/patterns without examples.
func (v *EngramValidator) detectMissingExamples(content string) {
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
			endIdx := lineNum + 20
			if endIdx > len(lines) {
				endIdx = len(lines)
			}
			contextWindow := strings.Join(lines[lineNum:endIdx], "\n")
			contextLower := strings.ToLower(contextWindow)

			// Look for code blocks (```) or example sections
			hasExample := strings.Contains(contextWindow, "```") ||
				strings.Contains(contextLower, "example:") ||
				strings.Contains(contextLower, "good example") ||
				strings.Contains(contextLower, "bad example")

			if !hasExample && strings.Contains(line, "##") { // Only flag if it's a header
				v.errors = append(v.errors, EngramValidationError{
					FilePath:  v.filePath,
					Line:      lineNum + 1,
					ErrorType: ErrorTypeMissingExample,
					Message:   "Principle/pattern without example within 20 lines",
				})
			}
		}
	}
}

// detectMissingConstraints detects tasks without constraints.
func (v *EngramValidator) detectMissingConstraints(content string) {
	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		lineLower := strings.ToLower(line)

		// Check if line contains a task keyword
		var taskVerb string
		for _, verb := range taskKeywords {
			verbPattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(verb) + `\b`)
			if verbPattern.MatchString(lineLower) {
				taskVerb = verb
				break
			}
		}

		if taskVerb != "" {
			// Check if there are constraints within next 10 lines
			endIdx := lineNum + 10
			if endIdx > len(lines) {
				endIdx = len(lines)
			}
			contextWindow := strings.Join(lines[lineNum:endIdx], "\n")

			hasConstraints := false
			for _, constraintPattern := range constraintPatterns {
				if constraintPattern.MatchString(contextWindow) {
					hasConstraints = true
					break
				}
			}

			if !hasConstraints {
				v.errors = append(v.errors, EngramValidationError{
					FilePath:  v.filePath,
					Line:      lineNum + 1,
					ErrorType: ErrorTypeMissingConstraints,
					Message:   fmt.Sprintf("Task verb \"%s\" without constraints (scope, limits, bounds)", taskVerb),
				})
			}
		}
	}
}

// ValidateFile validates a single .ai.md file and returns errors.
func ValidateFile(filePath string) ([]EngramValidationError, error) {
	validator := New(filePath)
	errors := validator.Validate()
	return errors, nil
}

// ValidateDirectory validates all .ai.md files in directory and subdirectories.
func ValidateDirectory(dirPath string) (map[string][]EngramValidationError, error) {
	results := make(map[string][]EngramValidationError)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
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
