package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

// GetValidator returns the shared validator instance with custom validations
func GetValidator() *validator.Validate {
	if validate == nil {
		validate = validator.New()
		// Register custom validation tags for struct-based validation
		validate.RegisterValidation("pathexists", func(fl validator.FieldLevel) bool {
			return pathExistsCheck(fl.Field().String(), false)
		})
		validate.RegisterValidation("pathexistsdir", func(fl validator.FieldLevel) bool {
			return pathExistsCheck(fl.Field().String(), true) && pathIsDirCheck(fl.Field().String())
		})
		validate.RegisterValidation("namespace", func(fl validator.FieldLevel) bool {
			return namespaceValid(fl.Field().String())
		})
	}
	return validate
}

func pathExistsCheck(path string, checkEmpty bool) bool {
	if path == "" {
		return !checkEmpty
	}
	if strings.Contains(path, "..") {
		return false
	}
	exp := expandPathVal(path)
	_, err := os.Stat(exp)
	return err == nil
}

func pathIsDirCheck(path string) bool {
	info, err := os.Stat(expandPathVal(path))
	return err == nil && info.IsDir()
}

func namespaceValid(ns string) bool {
	if ns == "" || len(ns) > MaxNamespaceLength {
		return false
	}
	parts := strings.Split(ns, ",")
	if len(parts) > MaxComponents {
		return false
	}
	for _, p := range parts {
		if t := strings.TrimSpace(p); t == "" || len(t) > MaxComponentLength {
			return false
		}
	}
	return true
}

func expandPathVal(path string) string {
	exp := os.ExpandEnv(path)
	if strings.HasPrefix(exp, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			exp = filepath.Join(home, exp[2:])
		}
	}
	return exp
}

// ValidateStruct validates a struct using go-playground/validator tags
func ValidateStruct(s interface{}) error {
	if err := GetValidator().Struct(s); err != nil {
		var errs validator.ValidationErrors
		if errors.As(err, &errs) {
			e := errs[0]
			msg := fmt.Sprintf("Invalid %s", e.Field())
			if e.Tag() == "oneof" {
				msg = fmt.Sprintf("%s must be one of: %s", e.Field(), e.Param())
			} else if e.Tag() == "required" {
				msg = fmt.Sprintf("%s is required", e.Field())
			}
			return &EngramError{Symbol: "✗", Message: msg}
		}
		return err
	}
	return nil
}

// Simplified validation functions

// ValidateEnum returns an error if value is non-empty and not in allowed.
func ValidateEnum(field string, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, opt := range allowed {
		if value == opt {
			return nil
		}
	}
	return InvalidInputError(field, value, strings.Join(allowed, "|"))
}

// ValidateEnumRequired returns an error if value is empty or not in allowed.
func ValidateEnumRequired(field string, value string, allowed []string) error {
	if value == "" {
		return fmt.Errorf("%s is required (must be one of: %s)", field, strings.Join(allowed, "|"))
	}
	return ValidateEnum(field, value, allowed)
}

// ValidateRange returns an error if value is outside [minVal, maxVal].
func ValidateRange(field string, value float64, minVal float64, maxVal float64) error {
	if value < minVal || value > maxVal {
		return InvalidInputError(field, fmt.Sprintf("%.2f", value), fmt.Sprintf("%.2f to %.2f", minVal, maxVal))
	}
	return nil
}

// ValidateRangeInt returns an error if value is outside [minVal, maxVal].
func ValidateRangeInt(field string, value int, minVal int, maxVal int) error {
	if value < minVal || value > maxVal {
		return InvalidInputError(field, fmt.Sprintf("%d", value), fmt.Sprintf("%d to %d", minVal, maxVal))
	}
	return nil
}

// ValidatePositive returns an error if value is negative.
func ValidatePositive(field string, value int) error {
	if value < 0 {
		return InvalidInputError(field, fmt.Sprintf("%d", value), "must be >= 0")
	}
	return nil
}

// ValidateNonEmpty returns an error if value is empty after trimming whitespace.
func ValidateNonEmpty(field string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s cannot be empty", field)
	}
	return nil
}

// ValidateAtLeastOne returns an error if all fields are empty after trimming whitespace.
func ValidateAtLeastOne(fields map[string]string, description string) error {
	for _, value := range fields {
		if strings.TrimSpace(value) != "" {
			return nil
		}
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, "--"+name)
	}
	return &EngramError{
		Symbol:      "✗",
		Message:     fmt.Sprintf("At least one %s is required", description),
		Suggestions: []string{fmt.Sprintf("Provide one of: %s", strings.Join(names, ", "))},
	}
}

// ValidateNamespace returns an error if namespace is empty or violates component limits.
func ValidateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}
	return ValidateNamespaceComponents(namespace, MaxComponents, MaxComponentLength)
}

// ValidatePathExists returns an error if a non-empty path does not exist or has the wrong type.
func ValidatePathExists(field string, path string, expectDir bool) error {
	if path == "" {
		return nil
	}
	if err := ValidateNoTraversal(field, path); err != nil {
		return err
	}
	exp := expandPathVal(path)
	info, err := os.Stat(exp)
	if err != nil {
		msg := fmt.Sprintf("Path does not exist: %s", path)
		if !os.IsNotExist(err) {
			msg = fmt.Sprintf("Cannot access path: %s", path)
		}
		return &EngramError{Symbol: "✗", Message: msg, Suggestions: []string{"Verify the path is correct"}}
	}
	if expectDir && !info.IsDir() {
		return &EngramError{Symbol: "✗", Message: fmt.Sprintf("Expected directory, found file: %s", path)}
	}
	if !expectDir && info.IsDir() {
		return &EngramError{Symbol: "✗", Message: fmt.Sprintf("Expected file, found directory: %s", path)}
	}
	return nil
}

// ValidatePathExistsRequired returns an error if path is empty or fails ValidatePathExists.
func ValidatePathExistsRequired(field string, path string, expectDir bool) error {
	if path == "" {
		typ := "file"
		if expectDir {
			typ = "directory"
		}
		return fmt.Errorf("%s is required (must be a valid %s path)", field, typ)
	}
	return ValidatePathExists(field, path, expectDir)
}

// Type definitions

// OutputFormat names a CLI output rendering format.
type OutputFormat string

// Supported output format values for OutputFormat.
const (
	FormatJSON     OutputFormat = "json"
	FormatText     OutputFormat = "text"
	FormatTable    OutputFormat = "table"
	FormatMarkdown OutputFormat = "markdown"
	FormatCSV      OutputFormat = "csv"
	FormatPaths    OutputFormat = "paths"
)

// ValidateOutputFormat returns an error if format is non-empty and not in allowed.
func ValidateOutputFormat(format string, allowed ...OutputFormat) error {
	if format == "" {
		return nil
	}
	allowedStr := make([]string, len(allowed))
	for i, f := range allowed {
		allowedStr[i] = string(f)
	}
	return ValidateEnum("format", format, allowedStr)
}

// ShellType names a supported shell for completion installation.
type ShellType string

// Supported shell type values for ShellType.
const (
	ShellBash       ShellType = "bash"
	ShellZsh        ShellType = "zsh"
	ShellFish       ShellType = "fish"
	ShellPowerShell ShellType = "powershell"
)

// ValidateShellType returns an error if shell is empty or not a supported ShellType.
func ValidateShellType(shell string) error {
	return ValidateEnumRequired("shell", shell, []string{
		string(ShellBash), string(ShellZsh), string(ShellFish), string(ShellPowerShell),
	})
}

// TierType names a knowledge tier (user, team, company, etc.).
type TierType string

// Supported tier values for TierType.
const (
	TierUser    TierType = "user"
	TierTeam    TierType = "team"
	TierCompany TierType = "company"
	TierCore    TierType = "core"
	TierAll     TierType = "all"
)

// ValidateTier returns an error if tier is non-empty and not a supported TierType.
func ValidateTier(tier string) error {
	return ValidateEnum("tier", tier, []string{
		string(TierUser), string(TierTeam), string(TierCompany), string(TierCore), string(TierAll),
	})
}
