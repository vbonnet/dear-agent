// Package engram provides validation for .ai.md engram files.
//
// This package validates engram files against the Agent Prompt Pattern Guide,
// detecting anti-patterns and enforcing structure requirements.
//
// # Key Features
//
//   - Frontmatter validation (required fields, types, lengths)
//   - Context reference detection (Principle 1: Context Embedding)
//   - Vague verb detection (Principle 2: Specificity)
//   - Missing example detection (Principle 3: Examples Required)
//   - Missing constraint detection (Principle 4: Task Constraints)
//
// # Usage
//
// Validate a single file:
//
//	errors, err := engram.ValidateFile("path/to/file.ai.md")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, e := range errors {
//	    fmt.Printf("%s:%d [%s] %s\n", e.FilePath, *e.Line, e.ErrorType, e.Message)
//	}
//
// Validate a directory:
//
//	results, err := engram.ValidateDirectory("path/to/engrams")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for filePath, errors := range results {
//	    fmt.Printf("\n%s:\n", filePath)
//	    for _, e := range errors {
//	        fmt.Printf("  Line %d [%s]: %s\n", *e.Line, e.ErrorType, e.Message)
//	    }
//	}
//
// Use the Validator directly for more control:
//
//	validator := engram.NewValidator("path/to/file.ai.md")
//	errors := validator.Validate()
//
// # Error Types
//
//   - file_error: Cannot read file
//   - missing_frontmatter: No YAML frontmatter found
//   - invalid_frontmatter: Malformed YAML
//   - missing_field: Required frontmatter field missing
//   - invalid_type: Type field has invalid value
//   - invalid_title: Title field is invalid
//   - invalid_description: Description field is invalid
//   - description_too_long: Description exceeds 200 characters
//   - context_reference: Context reference detected (e.g., "mentioned above")
//   - vague_verb: Vague verb without measurable criteria
//   - missing_example: Principle/pattern without example
//   - missing_constraints: Task without constraints
//
// # Performance
//
// This Go implementation provides 15-40x speedup over the Python original:
//   - Python: ~50ms per file (regex-heavy, interpreted)
//   - Go: ~1-3ms per file (compiled, optimized regex)
//
// # Frontmatter Requirements
//
// All .ai.md files must have YAML frontmatter with:
//   - type: one of [reference, template, workflow, guide]
//   - title: non-empty string
//   - description: non-empty string, max 200 characters
//
// Example:
//
//	---
//	type: reference
//	title: Repository Pattern
//	description: Separate data access logic using repositories
//	---
package engram
