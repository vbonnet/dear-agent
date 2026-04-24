package validator

// Exported accessors for the analyzer tool.
// These allow internal/analyzer to inspect patterns without duplicating definitions.
// Kept in a separate file to avoid conflicts with the code-generated patterns.go.

// PatternCount returns the number of forbidden patterns.
func PatternCount() int { return len(forbiddenPatterns) }

// PatternName returns the human-readable name for pattern at index i.
func PatternName(i int) string { return patternNames[i] }

// PatternRegex returns the regex string for pattern at index i.
func PatternRegex(i int) string { return forbiddenPatterns[i].String() }

// PatternRemediation returns the remediation advice for pattern at index i.
func PatternRemediation(i int) string { return remediations[i] }
