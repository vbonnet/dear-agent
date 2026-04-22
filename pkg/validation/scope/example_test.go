package scope_test

import (
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/validation/scope"
)

// Example demonstrates basic usage of the scope validator
func Example() {
	// Create parser and validator
	parser := scope.NewParser()
	validator := scope.NewValidator(parser)

	// Sample D3 document with anti-patterns
	markdown := `
# DESIGN: Approach Decision

## Decision Matrix

Comparison table

## Chosen Approach

We chose approach A.

## Risk Assessment

Low risk

## Acceptance Criteria

- Users can do X
- System supports Y
`

	// Validate the document
	result := validator.Validate(scope.PhaseDesign, markdown, nil)

	// Print results
	fmt.Printf("Passed: %v\n", result.Passed)
	fmt.Printf("Errors: %d\n", len(result.Errors))
	fmt.Printf("Warnings: %d\n", len(result.Warnings))

	if len(result.Errors) > 0 {
		fmt.Printf("First error: %s\n", result.Errors[0].Message)
	}

	// Output:
	// Passed: false
	// Errors: 1
	// Warnings: 1
	// First error: Section "Acceptance Criteria" belongs in SPEC, not DESIGN
}

// Example_formatReport demonstrates report formatting
func Example_formatReport() {
	parser := scope.NewParser()
	validator := scope.NewValidator(parser)

	markdown := `
# DESIGN: Approach Decision

## Summary

Valid document.
`

	result := validator.Validate(scope.PhaseDesign, markdown, nil)
	report := validator.FormatReport(result)

	fmt.Println(report)
}

// Example_override demonstrates validation override
func Example_override() {
	parser := scope.NewParser()
	validator := scope.NewValidator(parser)

	markdown := `
# DESIGN: Approach Decision

## Decision Matrix

Comparison

## Chosen Approach

Selected

## Risk Assessment

Low risk

## Acceptance Criteria

Content here
`

	// Validate with override
	result := validator.Validate(scope.PhaseDesign, markdown, &scope.ValidationOptions{
		Override: true,
	})

	fmt.Printf("Passed (with override): %v\n", result.Passed)
	fmt.Printf("Errors still reported: %d\n", len(result.Errors))

	// Output:
	// Passed (with override): true
	// Errors still reported: 1
}
