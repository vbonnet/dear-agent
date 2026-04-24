// Package cli provides validation utilities for CLI input validation.
//
// The validation package offers a comprehensive set of validation functions
// for common CLI input patterns including:
//
//   - Enum validation (format, shell type, tier, etc.)
//   - Numeric range validation (floats and integers)
//   - Path validation (with existence checks)
//   - String validation (non-empty, namespace format)
//   - Logical validation (at least one field required)
//
// All validation functions return nil for valid input and a descriptive
// error for invalid input. The errors use the EngramError type to provide
// helpful suggestions and related commands.
//
// # Usage Examples
//
// Validate an enum value:
//
//	err := cli.ValidateEnum("format", "json", []string{"json", "text", "table"})
//	if err != nil {
//	    return err
//	}
//
// Validate a numeric range:
//
//	err := cli.ValidateRange("importance", 0.5, 0, 1)
//	if err != nil {
//	    return err
//	}
//
// Validate a path exists:
//
//	err := cli.ValidatePathExists("config", "/path/to/config", false)
//	if err != nil {
//	    return err
//	}
//
// Validate output format:
//
//	err := cli.ValidateOutputFormat(format, cli.FormatJSON, cli.FormatText)
//	if err != nil {
//	    return err
//	}
//
// Validate at least one field is provided:
//
//	fields := map[string]string{
//	    "set-content": updateContent,
//	    "set-type":    updateType,
//	}
//	err := cli.ValidateAtLeastOne(fields, "update field")
//	if err != nil {
//	    return err
//	}
package cli
