// Package validator provides content validation for .ai.md files.
//
// This package implements comprehensive validation checks including:
//   - Metadata accuracy (token counts)
//   - Frontmatter completeness
//   - Core file structure (NEVER/ALWAYS/REMINDER)
//   - Token bloat detection
//   - Progressive disclosure compliance
//   - Token budget enforcement
package validator

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/internal/tokens/tokenizers"
	configloader "github.com/vbonnet/dear-agent/pkg/config-loader"
	"gopkg.in/yaml.v3"
)

// Required frontmatter fields for .ai.md files
var requiredFields = []string{"schema", "type", "status", "created", "tokens"}

// ContentValidationError represents a validation error or warning for content files.
type ContentValidationError struct {
	Filepath string // Relative path to the file
	Check    string // Check name (e.g., "frontmatter", "tokens")
	Severity string // "error" or "warning"
	Message  string // Detailed error message
}

// String returns a formatted error message.
func (e ContentValidationError) String() string {
	symbol := "❌"
	if e.Severity == "warning" {
		symbol = "⚠️"
	}
	return fmt.Sprintf("%s [%s] %s\n   %s", symbol, e.Check, e.Filepath, e.Message)
}

// ContentValidator validates .ai.md content files.
type ContentValidator struct {
	contentDir   string
	autoFix      bool
	coreFiles    map[string]bool
	tokenBudgets map[string]int
	tokenizer    tokenizers.Tokenizer
	errors       []ContentValidationError
	warnings     []ContentValidationError
	fixesApplied []string
}

// NewContentValidator creates a new content validator.
//
// Parameters:
//   - contentDir: Path to content directory
//   - autoFix: If true, automatically fix token count mismatches
//
// Returns error if:
//   - Content directory doesn't exist
//   - Tokenizer initialization fails
func NewContentValidator(contentDir string, autoFix bool) (*ContentValidator, error) {
	// Verify content directory exists
	if _, err := os.Stat(contentDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("content directory not found: %s", contentDir)
	}

	// Get absolute path
	absPath, err := filepath.Abs(contentDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve content directory path: %w", err)
	}

	// Initialize tiktoken tokenizer
	tok := tokenizers.NewTiktokenTokenizer()

	v := &ContentValidator{
		contentDir:   absPath,
		autoFix:      autoFix,
		coreFiles:    make(map[string]bool),
		tokenBudgets: make(map[string]int),
		tokenizer:    tok,
		errors:       make([]ContentValidationError, 0),
		warnings:     make([]ContentValidationError, 0),
		fixesApplied: make([]string, 0),
	}

	// Load configuration files
	if err := v.loadCoreFiles(); err != nil {
		// Non-fatal: core files list is optional
		// Continue with empty set
	}

	if err := v.loadTokenBudgets(); err != nil {
		// Non-fatal: token budgets are optional
		// Continue with empty budgets
	}

	return v, nil
}

// loadCoreFiles loads the list of core files requiring strict structure validation.
func (v *ContentValidator) loadCoreFiles() error {
	coreFilesPath := filepath.Join(filepath.Dir(v.contentDir), "validation", "core-files.txt")

	data, err := os.ReadFile(coreFilesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist - not an error
		}
		return fmt.Errorf("failed to read core files list: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		v.coreFiles[line] = true
	}

	return nil
}

// loadTokenBudgets loads type-specific token budgets.
func (v *ContentValidator) loadTokenBudgets() error {
	budgetsPath := filepath.Join(filepath.Dir(v.contentDir), "validation", "token-budgets.json")

	data, err := os.ReadFile(budgetsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist - not an error
		}
		return fmt.Errorf("failed to read token budgets: %w", err)
	}

	var config struct {
		Budgets map[string]int `json:"budgets"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse token budgets: %w", err)
	}

	v.tokenBudgets = config.Budgets
	return nil
}

// ValidateFile runs all validation checks on a single file.
func (v *ContentValidator) ValidateFile(filepath string) error {
	// Get relative path for reporting
	relPath, err := v.getRelativePath(filepath)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %w", err)
	}

	// Read file content
	content, err := os.ReadFile(filepath)
	if err != nil {
		v.errors = append(v.errors, ContentValidationError{
			Filepath: relPath,
			Check:    "read",
			Severity: "error",
			Message:  fmt.Sprintf("Failed to read file: %v", err),
		})
		return nil // Don't stop validation, continue with other files
	}

	// Parse frontmatter
	fm, body, err := configloader.ParseFrontmatter(string(content))
	if err != nil {
		v.errors = append(v.errors, ContentValidationError{
			Filepath: relPath,
			Check:    "parsing",
			Severity: "error",
			Message:  fmt.Sprintf("Failed to parse frontmatter: %v", err),
		})
		return nil
	}

	// Parse frontmatter YAML
	var metadata map[string]interface{}
	if err := yaml.Unmarshal([]byte(fm), &metadata); err != nil {
		v.errors = append(v.errors, ContentValidationError{
			Filepath: relPath,
			Check:    "parsing",
			Severity: "error",
			Message:  fmt.Sprintf("Invalid YAML in frontmatter: %v", err),
		})
		return nil
	}

	// Run validation checks

	// Check 1: Frontmatter completeness (P0 - blocking)
	v.checkFrontmatterCompleteness(relPath, metadata)

	// Check 2: Token count accuracy (P0 - auto-fix)
	v.checkTokenAccuracy(relPath, filepath, metadata, body)

	// Check 3: Core file structure (P0 - blocking for core files)
	if v.coreFiles[relPath] {
		v.checkCoreStructure(relPath, string(content))
	}

	// Check 4: Token bloat detection (P1 - warning)
	v.checkTokenBloat(relPath, filepath, metadata)

	// Check 5: Progressive disclosure (P1 - warning)
	v.checkProgressiveDisclosure(relPath, filepath, metadata, body)

	// Check 6: Token budget (P1 - warning)
	v.checkTokenBudget(relPath, metadata)

	return nil
}

// ValidateAll validates all .ai.md files in the content directory or specific files.
//
// Parameters:
//   - files: Optional list of specific files to validate (empty = validate all)
//
// Returns:
//   - errorCount: Number of errors found
//   - warningCount: Number of warnings found
//   - error: Fatal error that prevented validation
func (v *ContentValidator) ValidateAll(files []string) (int, int, error) {
	var filesToValidate []string

	if len(files) > 0 {
		// Validate specific files
		filesToValidate = files
	} else {
		// Find all .ai.md files
		var err error
		filesToValidate, err = v.findAllAiMdFiles()
		if err != nil {
			return 0, 0, fmt.Errorf("failed to find .ai.md files: %w", err)
		}
	}

	// Validate each file
	for _, file := range filesToValidate {
		if err := v.ValidateFile(file); err != nil {
			// Log error but continue with other files
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	return len(v.errors), len(v.warnings), nil
}

// findAllAiMdFiles finds all .ai.md files in the content directory.
func (v *ContentValidator) findAllAiMdFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(v.contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".ai.md") {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// getRelativePath returns the path relative to the content directory.
func (v *ContentValidator) getRelativePath(absPath string) (string, error) {
	relPath, err := filepath.Rel(v.contentDir, absPath)
	if err != nil {
		// If we can't get relative path, use the filename
		return filepath.Base(absPath), nil
	}
	return relPath, nil
}

// GetErrors returns all validation errors.
func (v *ContentValidator) GetErrors() []ContentValidationError {
	return v.errors
}

// GetWarnings returns all validation warnings.
func (v *ContentValidator) GetWarnings() []ContentValidationError {
	return v.warnings
}

// GetFixesApplied returns list of auto-fix messages.
func (v *ContentValidator) GetFixesApplied() []string {
	return v.fixesApplied
}

// checkFrontmatterCompleteness verifies all required fields are present.
func (v *ContentValidator) checkFrontmatterCompleteness(filepath string, metadata map[string]interface{}) {
	var missing []string

	for _, field := range requiredFields {
		if _, ok := metadata[field]; !ok {
			missing = append(missing, field)
		}
	}

	if len(missing) > 0 {
		v.errors = append(v.errors, ContentValidationError{
			Filepath: filepath,
			Check:    "frontmatter",
			Severity: "error",
			Message: fmt.Sprintf("Missing required fields: %s\n   Required: %s\n   Template: howtos/howto-template.ai.md",
				strings.Join(missing, ", "),
				strings.Join(requiredFields, ", ")),
		})
	}
}

// checkTokenAccuracy verifies declared token count matches actual count.
func (v *ContentValidator) checkTokenAccuracy(relPath, absPath string, metadata map[string]interface{}, content string) {
	tokensVal, ok := metadata["tokens"]
	if !ok {
		return // Caught by frontmatter check
	}

	// Handle both int and float64 (YAML numbers can be either)
	var declaredTokens int
	switch val := tokensVal.(type) {
	case int:
		declaredTokens = val
	case float64:
		declaredTokens = int(val)
	default:
		v.errors = append(v.errors, ContentValidationError{
			Filepath: relPath,
			Check:    "tokens",
			Severity: "error",
			Message:  fmt.Sprintf("Invalid token count type: %T (expected int)", tokensVal),
		})
		return
	}

	// Count actual tokens
	actualTokens, err := v.tokenizer.Count(content)
	if err != nil {
		v.errors = append(v.errors, ContentValidationError{
			Filepath: relPath,
			Check:    "tokens",
			Severity: "error",
			Message:  fmt.Sprintf("Failed to count tokens: %v", err),
		})
		return
	}

	if declaredTokens != actualTokens {
		if v.autoFix {
			// Auto-fix: Update token count
			if err := v.updateTokenCount(absPath, actualTokens); err != nil {
				v.errors = append(v.errors, ContentValidationError{
					Filepath: relPath,
					Check:    "tokens",
					Severity: "error",
					Message:  fmt.Sprintf("Failed to auto-fix token count: %v", err),
				})
			} else {
				v.fixesApplied = append(v.fixesApplied,
					fmt.Sprintf("Auto-updated %s: %d → %d tokens",
						relPath, declaredTokens, actualTokens))
			}
		} else {
			v.errors = append(v.errors, ContentValidationError{
				Filepath: relPath,
				Check:    "tokens",
				Severity: "error",
				Message: fmt.Sprintf("Token count mismatch: declared %d, actual %d\n   Run with --fix to auto-update",
					declaredTokens, actualTokens),
			})
		}
	}
}

// updateTokenCount updates the token count in a file's frontmatter.
func (v *ContentValidator) updateTokenCount(filepath string, newCount int) error {
	// Read file
	content, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse frontmatter
	fm, body, err := configloader.ParseFrontmatter(string(content))
	if err != nil {
		return fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Parse YAML
	var metadata map[string]interface{}
	if err := yaml.Unmarshal([]byte(fm), &metadata); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Update token count
	metadata["tokens"] = newCount

	// Re-serialize YAML
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(metadata); err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}
	encoder.Close()

	// Reconstruct file
	newContent := fmt.Sprintf("---\n%s---\n%s", buf.String(), body)

	// Write back
	if err := os.WriteFile(filepath, []byte(newContent), 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// checkCoreStructure verifies core files have NEVER/ALWAYS/REMINDER sections.
func (v *ContentValidator) checkCoreStructure(filepath, content string) {
	requiredSections := map[string]*regexp.Regexp{
		"## NEVER":    regexp.MustCompile(`(?m)^## NEVER\s*$`),
		"## ALWAYS":   regexp.MustCompile(`(?m)^## ALWAYS\s*$`),
		"## REMINDER": regexp.MustCompile(`(?m)^## REMINDER\s*$`),
	}

	var missing []string
	for section, pattern := range requiredSections {
		if !pattern.MatchString(content) {
			missing = append(missing, section)
		}
	}

	if len(missing) > 0 {
		v.errors = append(v.errors, ContentValidationError{
			Filepath: filepath,
			Check:    "structure",
			Severity: "error",
			Message: fmt.Sprintf("Core file missing required sections: %s\n   Core guidance files must have:\n     ## NEVER\n     ## ALWAYS\n     ## REMINDER\n   See: docs/guidance-structure.md",
				strings.Join(missing, ", ")),
		})
	}
}

// checkTokenBloat detects significant token count increases via git history.
func (v *ContentValidator) checkTokenBloat(relPath, absPath string, metadata map[string]interface{}) {
	// Get current token count
	tokensVal, ok := metadata["tokens"]
	if !ok {
		return
	}

	var currentTokens int
	switch val := tokensVal.(type) {
	case int:
		currentTokens = val
	case float64:
		currentTokens = int(val)
	default:
		return
	}

	// Get previous token count from git
	prevTokens, err := v.getPreviousTokenCount(relPath)
	if err != nil {
		// File not in git or git unavailable - skip check
		return
	}

	if prevTokens > 0 {
		increase := currentTokens - prevTokens
		percentIncrease := float64(increase) / float64(prevTokens) * 100

		// Warn on >10% increase
		if percentIncrease > 10 {
			v.warnings = append(v.warnings, ContentValidationError{
				Filepath: relPath,
				Check:    "bloat",
				Severity: "warning",
				Message: fmt.Sprintf("Token count increased by %.1f%% (%d → %d tokens, +%d)\n   This file may be loaded frequently. Consider optimizing before merge.\n   Optimization guide: docs/token-optimization.md",
					percentIncrease, prevTokens, currentTokens, increase),
			})
		}
	}
}

// getPreviousTokenCount retrieves token count from git history.
func (v *ContentValidator) getPreviousTokenCount(relPath string) (int, error) {
	// Run: git show HEAD:{relPath}
	cmd := exec.Command("git", "show", fmt.Sprintf("HEAD:%s", relPath))
	cmd.Dir = v.contentDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, err
	}

	// Parse frontmatter from git output
	fm, _, err := configloader.ParseFrontmatter(string(output))
	if err != nil {
		return 0, err
	}

	// Parse YAML
	var metadata map[string]interface{}
	if err := yaml.Unmarshal([]byte(fm), &metadata); err != nil {
		return 0, err
	}

	tokensVal, ok := metadata["tokens"]
	if !ok {
		return 0, fmt.Errorf("no token count in previous version")
	}

	// Handle both int and float64
	switch val := tokensVal.(type) {
	case int:
		return val, nil
	case float64:
		return int(val), nil
	default:
		return 0, fmt.Errorf("invalid token type: %T", tokensVal)
	}
}

// checkProgressiveDisclosure verifies large files have companion files.
func (v *ContentValidator) checkProgressiveDisclosure(relPath, absPath string, metadata map[string]interface{}, content string) {
	tokensVal, ok := metadata["tokens"]
	if !ok {
		return
	}

	var tokens int
	switch val := tokensVal.(type) {
	case int:
		tokens = val
	case float64:
		tokens = int(val)
	default:
		return
	}

	if tokens <= 1000 {
		return // Under threshold
	}

	// Check for companion files
	basePath := strings.TrimSuffix(absPath, ".ai.md")
	companionMD := basePath + ".md"
	companionWhy := basePath + ".why.md"

	var existingCompanions []string
	if _, err := os.Stat(companionMD); err == nil {
		existingCompanions = append(existingCompanions, filepath.Base(companionMD))
	}
	if _, err := os.Stat(companionWhy); err == nil {
		existingCompanions = append(existingCompanions, filepath.Base(companionWhy))
	}

	if len(existingCompanions) == 0 {
		v.warnings = append(v.warnings, ContentValidationError{
			Filepath: relPath,
			Check:    "progressive-disclosure",
			Severity: "warning",
			Message: fmt.Sprintf("Large file (%d tokens) has no companion files\n   Consider creating companion files for progressive disclosure:\n     - %s (detailed explanation)\n     - %s (rationale and context)\n   Guide: docs/companion-file-guide.md",
				tokens, filepath.Base(companionMD), filepath.Base(companionWhy)),
		})
		return
	}

	// Check if .ai.md references companions
	var missingRefs []string
	for _, companion := range existingCompanions {
		if !strings.Contains(content, companion) {
			missingRefs = append(missingRefs, companion)
		}
	}

	if len(missingRefs) > 0 {
		v.warnings = append(v.warnings, ContentValidationError{
			Filepath: relPath,
			Check:    "progressive-disclosure",
			Severity: "warning",
			Message: fmt.Sprintf("Large file has companions but doesn't reference them: %s\n   Add references to companion files in .ai.md",
				strings.Join(missingRefs, ", ")),
		})
	}
}

// checkTokenBudget verifies file doesn't exceed type-specific budget.
func (v *ContentValidator) checkTokenBudget(filepath string, metadata map[string]interface{}) {
	fileType, ok := metadata["type"].(string)
	if !ok {
		return
	}

	budget, ok := v.tokenBudgets[fileType]
	if !ok {
		return // No budget defined for this type
	}

	tokensVal, ok := metadata["tokens"]
	if !ok {
		return
	}

	var tokens int
	switch val := tokensVal.(type) {
	case int:
		tokens = val
	case float64:
		tokens = int(val)
	default:
		return
	}

	if tokens > budget {
		overage := tokens - budget
		percentOver := float64(overage) / float64(budget) * 100

		v.warnings = append(v.warnings, ContentValidationError{
			Filepath: filepath,
			Check:    "budget",
			Severity: "warning",
			Message: fmt.Sprintf("Exceeds %s budget by %.0f%% (%d/%d tokens, +%d)\n   Consider:\n     - Moving examples to companion .md file\n     - Splitting into multiple topic files\n     - Using progressive disclosure pattern\n   Optimization guide: docs/token-optimization.md",
				fileType, percentOver, tokens, budget, overage),
		})
	}
}
