package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/validator"
)

var (
	validateAll       bool
	validateType      string
	validateJSON      bool
	validateVerbose   bool
	validateFix       bool
	validateRecursive bool
)

// ValidatorType represents the type of validator to run
type ValidatorType string

// Recognized ValidatorType values.
const (
	ValidatorEngram           ValidatorType = "engram"
	ValidatorContent          ValidatorType = "content"
	ValidatorWayfinder        ValidatorType = "wayfinder"
	ValidatorLinkChecker      ValidatorType = "linkchecker"
	ValidatorYAMLTokenCounter ValidatorType = "yamltokencounter"
	ValidatorRetrospective    ValidatorType = "retrospective"
)

// ValidationResult holds results from any validator
type ValidationResult struct {
	ValidatorType ValidatorType
	FilePath      string
	Errors        []ValidationError
	Warnings      []ValidationWarning
	FixesApplied  []string
}

// ValidationError represents a validation error
type ValidationError struct {
	FilePath string
	Line     int
	Type     string
	Message  string
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	FilePath string
	Line     int
	Type     string
	Message  string
}

// ValidationSummary holds aggregate results
type ValidationSummary struct {
	TotalFiles     int
	FilesValidated int
	ErrorCount     int
	WarningCount   int
	FixesApplied   int
	Results        []ValidationResult
}

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate engram files (unified validator for all types)",
	Long: `Validate engram files with auto-detection or explicit validator selection.

VALIDATORS:
  engram          - Validate .ai.md engram files (frontmatter, anti-patterns)
  content         - Validate .ai.md content files (tokens, structure, budgets)
  wayfinder       - Validate wayfinder-artifact.yaml files (phase schemas)
  linkchecker     - Check internal .ai.md links
  yamltokencounter - Count tokens in YAML frontmatter
  retrospective   - Validate S11 retrospective files

AUTO-DETECTION:
  The validator auto-detects file type from filename and content:
    - *.ai.md                  → engram validator
    - core/**/*.ai.md          → content validator
    - *-retrospective.md       → retrospective validator
    - wayfinder-artifact.yaml  → wayfinder validator
    - *.yaml, *.yml            → yamltokencounter

USAGE:
  # Auto-detect and validate a single file
  engram validate file.ai.md

  # Validate all files in current directory
  engram validate --all

  # Validate with specific validator type
  engram validate --type=content core/file.ai.md

  # Auto-fix token counts (content validator only)
  engram validate --fix --type=content core/

  # JSON output format
  engram validate --json file.ai.md

  # Verbose output
  engram validate --verbose --all

EXIT CODES:
  0 - Success (no errors)
  1 - Validation errors found
  2 - Runtime error (file not found, etc.)

FLAGS:
  --all              Validate all files in current directory
  --type=<validator> Specify validator type explicitly
  --json             Output in JSON format
  --verbose          Show detailed output
  --fix              Auto-fix issues (content validator token counts)
  --recursive        Validate recursively (deprecated, use --all)

EXAMPLES:
  # Validate single engram file
  engram validate prompts/example.ai.md

  # Validate all files with auto-detection
  engram validate --all

  # Validate and fix content files
  engram validate --type=content --fix core/

  # Check all links in .ai.md files
  engram validate --type=linkchecker --all

  # Validate wayfinder artifact
  engram validate --type=wayfinder wayfinder-artifact.yaml

  # Validate retrospective with JSON output
  engram validate --json project-retrospective.md
`,
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)

	validateCmd.Flags().BoolVar(&validateAll, "all", false, "Validate all files in current directory")
	validateCmd.Flags().StringVar(&validateType, "type", "", "Validator type (engram|content|wayfinder|linkchecker|yamltokencounter|retrospective)")
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Output in JSON format")
	validateCmd.Flags().BoolVar(&validateVerbose, "verbose", false, "Show detailed output")
	validateCmd.Flags().BoolVar(&validateFix, "fix", false, "Auto-fix issues (content validator token counts)")
	validateCmd.Flags().BoolVarP(&validateRecursive, "recursive", "r", false, "Validate recursively (deprecated, use --all)")
}

func runValidate(cmd *cobra.Command, args []string) error {
	// Handle deprecated --recursive flag
	if validateRecursive {
		validateAll = true
	}

	// Determine files to validate
	var filesToValidate []string
	var err error

	if validateAll {
		// Find all relevant files in current directory
		filesToValidate, err = findAllValidatableFiles()
		if err != nil {
			return fmt.Errorf("failed to find files: %w", err)
		}
		if len(filesToValidate) == 0 {
			fmt.Fprintf(os.Stderr, "No validatable files found in current directory\n")
			return nil
		}
	} else {
		// Validate specific file(s)
		if len(args) == 0 {
			return fmt.Errorf("no file specified (use --all to validate all files)")
		}

		target := args[0]
		info, err := os.Stat(target)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file or directory not found: %s", target)
			}
			return fmt.Errorf("failed to access %s: %w", target, err)
		}

		if info.IsDir() {
			// Validate all files in directory
			filesToValidate, err = findAllValidatableFilesInDir(target)
			if err != nil {
				return fmt.Errorf("failed to find files in directory: %w", err)
			}
			if len(filesToValidate) == 0 {
				return fmt.Errorf("no validatable files found in directory: %s", target)
			}
		} else {
			filesToValidate = []string{target}
		}
	}

	// Run validation
	summary, err := validateFiles(filesToValidate)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Output results
	if validateJSON {
		return outputValidationJSON(summary)
	}
	return outputValidationText(summary)
}

// validateFiles validates all files and returns summary
func validateFiles(files []string) (*ValidationSummary, error) {
	summary := &ValidationSummary{
		TotalFiles: len(files),
		Results:    make([]ValidationResult, 0),
	}

	for _, file := range files {
		// Detect validator type if not specified
		validatorType := ValidatorType(validateType)
		if validateType == "" {
			validatorType = detectValidatorType(file)
			if validatorType == "" {
				if validateVerbose {
					fmt.Fprintf(os.Stderr, "Skipping %s: unknown file type\n", file)
				}
				continue
			}
		}

		// Run appropriate validator
		result, err := runValidatorForFile(file, validatorType)
		if err != nil {
			// Non-fatal: log error but continue with other files
			fmt.Fprintf(os.Stderr, "Error validating %s: %v\n", file, err)
			continue
		}

		summary.FilesValidated++
		summary.ErrorCount += len(result.Errors)
		summary.WarningCount += len(result.Warnings)
		summary.FixesApplied += len(result.FixesApplied)
		summary.Results = append(summary.Results, result)
	}

	return summary, nil
}

// detectValidatorType auto-detects the appropriate validator from filename
func detectValidatorType(filePath string) ValidatorType {
	filename := filepath.Base(filePath)
	ext := filepath.Ext(filePath)

	// Retrospective files
	if strings.HasSuffix(filename, "-retrospective.md") {
		return ValidatorRetrospective
	}

	// Wayfinder artifacts
	if strings.Contains(filename, "wayfinder-artifact") && (ext == ".yaml" || ext == ".yml") {
		return ValidatorWayfinder
	}

	// YAML files (token counter)
	if ext == ".yaml" || ext == ".yml" {
		return ValidatorYAMLTokenCounter
	}

	// .ai.md files
	if ext == ".md" && strings.HasSuffix(filename, ".ai.md") {
		// Check if in core/ directory (content validator)
		if strings.Contains(filePath, "/core/") || strings.Contains(filePath, "\\core\\") {
			return ValidatorContent
		}
		// Default to engram validator
		return ValidatorEngram
	}

	return ""
}

// runValidatorForFile runs the appropriate validator for a file
func runValidatorForFile(filePath string, validatorType ValidatorType) (ValidationResult, error) {
	result := ValidationResult{
		ValidatorType: validatorType,
		FilePath:      filePath,
		Errors:        make([]ValidationError, 0),
		Warnings:      make([]ValidationWarning, 0),
		FixesApplied:  make([]string, 0),
	}

	switch validatorType {
	case ValidatorEngram:
		return runEngramValidator(filePath)
	case ValidatorContent:
		return runContentValidator(filePath)
	case ValidatorWayfinder:
		return runWayfinderValidator(filePath)
	case ValidatorLinkChecker:
		return runLinkChecker(filePath)
	case ValidatorYAMLTokenCounter:
		return runYAMLTokenCounter(filePath)
	case ValidatorRetrospective:
		return runRetrospectiveValidator(filePath)
	default:
		return result, fmt.Errorf("unknown validator type: %s", validatorType)
	}
}

// runEngramValidator runs the engram validator
func runEngramValidator(filePath string) (ValidationResult, error) {
	result := ValidationResult{
		ValidatorType: ValidatorEngram,
		FilePath:      filePath,
		Errors:        make([]ValidationError, 0),
		Warnings:      make([]ValidationWarning, 0),
	}

	v := validator.New(filePath)
	errs := v.Validate()

	for _, err := range errs {
		result.Errors = append(result.Errors, ValidationError{
			FilePath: err.FilePath,
			Line:     err.Line,
			Type:     err.ErrorType,
			Message:  err.Message,
		})
	}

	return result, nil
}

// runContentValidator runs the content validator
func runContentValidator(filePath string) (ValidationResult, error) {
	result := ValidationResult{
		ValidatorType: ValidatorContent,
		FilePath:      filePath,
		Errors:        make([]ValidationError, 0),
		Warnings:      make([]ValidationWarning, 0),
		FixesApplied:  make([]string, 0),
	}

	// Determine content directory
	contentDir := filepath.Dir(filePath)
	if strings.HasSuffix(contentDir, "/core") || strings.HasSuffix(contentDir, "\\core") {
		// Already in core directory
	} else if strings.Contains(filePath, "/core/") || strings.Contains(filePath, "\\core\\") {
		// Extract core directory
		parts := strings.Split(filePath, string(filepath.Separator))
		for i, part := range parts {
			if part == "core" && i > 0 {
				contentDir = strings.Join(parts[:i+1], string(filepath.Separator))
				break
			}
		}
	}

	v, err := validator.NewContentValidator(contentDir, validateFix)
	if err != nil {
		return result, fmt.Errorf("failed to create content validator: %w", err)
	}

	if err := v.ValidateFile(filePath); err != nil {
		return result, fmt.Errorf("validation failed: %w", err)
	}

	// Convert errors
	for _, e := range v.GetErrors() {
		result.Errors = append(result.Errors, ValidationError{
			FilePath: e.Filepath,
			Line:     0, // Content validator doesn't provide line numbers
			Type:     e.Check,
			Message:  e.Message,
		})
	}

	// Convert warnings
	for _, w := range v.GetWarnings() {
		result.Warnings = append(result.Warnings, ValidationWarning{
			FilePath: w.Filepath,
			Line:     0,
			Type:     w.Check,
			Message:  w.Message,
		})
	}

	// Fixes applied
	result.FixesApplied = v.GetFixesApplied()

	return result, nil
}

// runWayfinderValidator runs the wayfinder validator
func runWayfinderValidator(filePath string) (ValidationResult, error) {
	result := ValidationResult{
		ValidatorType: ValidatorWayfinder,
		FilePath:      filePath,
		Errors:        make([]ValidationError, 0),
		Warnings:      make([]ValidationWarning, 0),
	}

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return result, fmt.Errorf("failed to read file: %w", err)
	}

	// Extract frontmatter
	frontmatter, err := validator.ExtractFrontmatter(string(content))
	if err != nil {
		return result, fmt.Errorf("failed to extract frontmatter: %w", err)
	}

	// Validate
	errs := validator.ValidateArtifact(frontmatter)
	for _, e := range errs {
		result.Errors = append(result.Errors, ValidationError{
			FilePath: filePath,
			Line:     0,
			Type:     e.Field,
			Message:  e.Message,
		})
	}

	return result, nil
}

// runLinkChecker runs the link checker
func runLinkChecker(filePath string) (ValidationResult, error) {
	result := ValidationResult{
		ValidatorType: ValidatorLinkChecker,
		FilePath:      filePath,
		Errors:        make([]ValidationError, 0),
		Warnings:      make([]ValidationWarning, 0),
	}

	// Determine content directory
	contentDir := filepath.Dir(filePath)

	lc, err := validator.NewLinkChecker(contentDir)
	if err != nil {
		return result, fmt.Errorf("failed to create link checker: %w", err)
	}

	if err := lc.CheckFile(filePath); err != nil {
		return result, fmt.Errorf("link check failed: %w", err)
	}

	// Convert broken links to errors
	for _, link := range lc.GetBrokenLinks() {
		result.Errors = append(result.Errors, ValidationError{
			FilePath: link.FilePath,
			Line:     0,
			Type:     "broken_link",
			Message:  fmt.Sprintf("Broken link: %s (%s)", link.LinkPath, link.LinkText),
		})
	}

	return result, nil
}

// runYAMLTokenCounter runs the YAML token counter
func runYAMLTokenCounter(filePath string) (ValidationResult, error) {
	result := ValidationResult{
		ValidatorType: ValidatorYAMLTokenCounter,
		FilePath:      filePath,
		Errors:        make([]ValidationError, 0),
		Warnings:      make([]ValidationWarning, 0),
	}

	counter := validator.NewYAMLTokenCounter(validator.CounterOptions{
		FrontmatterOnly: true,
		Offline:         false,
	})

	countResult, err := counter.CountFile(filePath)
	if err != nil {
		return result, fmt.Errorf("failed to count tokens: %w", err)
	}

	// This is informational, not an error
	if validateVerbose {
		fmt.Printf("%s: %d tokens (method: %s)\n",
			filePath, countResult.FrontmatterTokens, countResult.Method)
	}

	return result, nil
}

// runRetrospectiveValidator runs the retrospective validator
func runRetrospectiveValidator(filePath string) (ValidationResult, error) {
	result := ValidationResult{
		ValidatorType: ValidatorRetrospective,
		FilePath:      filePath,
		Errors:        make([]ValidationError, 0),
		Warnings:      make([]ValidationWarning, 0),
	}

	v := validator.NewRetrospectiveValidator(filePath)
	errs, warnings, err := v.Validate()
	if err != nil {
		return result, fmt.Errorf("validation failed: %w", err)
	}

	// Convert errors
	for _, e := range errs {
		result.Errors = append(result.Errors, ValidationError{
			FilePath: filePath,
			Line:     0,
			Type:     e.Field,
			Message:  e.Message,
		})
	}

	// Convert warnings
	for _, w := range warnings {
		result.Warnings = append(result.Warnings, ValidationWarning{
			FilePath: filePath,
			Line:     0,
			Type:     w.Field,
			Message:  w.Message,
		})
	}

	return result, nil
}

// findAllValidatableFiles finds all files in current directory
func findAllValidatableFiles() ([]string, error) {
	return findAllValidatableFilesInDir(".")
}

// findAllValidatableFilesInDir finds all files in a directory
func findAllValidatableFilesInDir(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Check if file is validatable
		if detectValidatorType(path) != "" {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// outputValidationJSON outputs results in JSON format
func outputValidationJSON(summary *ValidationSummary) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	if summary.ErrorCount > 0 {
		os.Exit(1)
	}
	return nil
}

// outputValidationText outputs results in text format
func outputValidationText(summary *ValidationSummary) error {
	if len(summary.Results) == 0 {
		fmt.Println("No files validated")
		return nil
	}

	// Print results for each file
	for _, result := range summary.Results {
		if len(result.Errors) == 0 && len(result.Warnings) == 0 && len(result.FixesApplied) == 0 {
			if validateVerbose {
				fmt.Printf("✓ %s (%s)\n", result.FilePath, result.ValidatorType)
			}
			continue
		}

		fmt.Printf("\n%s (%s):\n", result.FilePath, result.ValidatorType)

		// Print errors
		for _, err := range result.Errors {
			if err.Line > 0 {
				fmt.Printf("  ❌ Line %d [%s]: %s\n", err.Line, err.Type, err.Message)
			} else {
				fmt.Printf("  ❌ [%s]: %s\n", err.Type, err.Message)
			}
		}

		// Print warnings
		for _, warn := range result.Warnings {
			if warn.Line > 0 {
				fmt.Printf("  ⚠️  Line %d [%s]: %s\n", warn.Line, warn.Type, warn.Message)
			} else {
				fmt.Printf("  ⚠️  [%s]: %s\n", warn.Type, warn.Message)
			}
		}

		// Print fixes
		for _, fix := range result.FixesApplied {
			fmt.Printf("  ✓ %s\n", fix)
		}
	}

	// Print summary
	fmt.Printf("\n")
	fmt.Printf("Summary:\n")
	fmt.Printf("  Files scanned: %d\n", summary.TotalFiles)
	fmt.Printf("  Files validated: %d\n", summary.FilesValidated)
	fmt.Printf("  Errors: %d\n", summary.ErrorCount)
	fmt.Printf("  Warnings: %d\n", summary.WarningCount)
	if summary.FixesApplied > 0 {
		fmt.Printf("  Fixes applied: %d\n", summary.FixesApplied)
	}

	// Print deprecation notice if Python validators detected
	printDeprecationNoticeIfNeeded()

	if summary.ErrorCount > 0 {
		os.Exit(1)
	}
	return nil
}

// printDeprecationNoticeIfNeeded prints deprecation notice for old Python validators
func printDeprecationNoticeIfNeeded() {
	// Check for Python validator files
	pythonValidators := []string{
		"scripts/validate_engram.py",
		"scripts/validate_content.py",
		"scripts/validate_wayfinder.py",
		"scripts/linkchecker.py",
		"scripts/yaml_token_counter.py",
		"scripts/validate_retrospective.py",
	}

	found := false
	for _, pyFile := range pythonValidators {
		if _, err := os.Stat(pyFile); err == nil {
			found = true
			break
		}
	}

	if found {
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "⚠️  DEPRECATION NOTICE:\n")
		fmt.Fprintf(os.Stderr, "   Old Python validators detected in scripts/\n")
		fmt.Fprintf(os.Stderr, "   Please migrate to unified CLI: engram validate\n")
		fmt.Fprintf(os.Stderr, "   See: docs/migration-guide.md\n")
		fmt.Fprintf(os.Stderr, "\n")
	}
}
