package earsbdd

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FeatureFile holds the Gherkin content for one SPEC.md source.
type FeatureFile struct {
	// SpecPath is the source SPEC.md that was processed.
	SpecPath string
	// FeatureName is the human-readable name derived from SpecPath.
	FeatureName string
	// Content is the rendered Gherkin text.
	Content string
}

// Generate converts a slice of requirements from a single SPEC.md into a
// Gherkin feature file stub. specPath is used to derive the feature name and
// the source comment; it should be the same value used when calling Extract.
func Generate(specPath string, reqs []Requirement) FeatureFile {
	if len(reqs) == 0 {
		return FeatureFile{SpecPath: specPath, FeatureName: featureName(specPath)}
	}

	var sb strings.Builder
	name := featureName(specPath)

	fmt.Fprintf(&sb, "# Generated from %s — do not edit by hand.\n", specPath)
	fmt.Fprintf(&sb, "# Re-generate with: ears-to-bdd %s\n", specPath)
	fmt.Fprintln(&sb, "#")
	fmt.Fprintln(&sb, "# These are scaffold stubs for the EARS requirements in that file.")
	fmt.Fprintln(&sb, "# Fill in concrete Given/When/Then steps before running the BDD suite.")
	fmt.Fprintln(&sb)
	fmt.Fprintf(&sb, "Feature: %s\n", name)

	for _, req := range reqs {
		fmt.Fprintln(&sb)
		if req.ID != "" {
			fmt.Fprintf(&sb, "  @ears @%s\n", req.ID)
		} else {
			fmt.Fprintln(&sb, "  @ears")
		}
		// Scenario title: ID + truncated condition for readability.
		title := scenarioTitle(req)
		fmt.Fprintf(&sb, "  Scenario: %s\n", title)
		fmt.Fprintln(&sb, "    Given the system is configured")
		if req.Condition != "" {
			fmt.Fprintf(&sb, "    When %s\n", lcFirst(req.Condition))
		} else {
			fmt.Fprintln(&sb, "    When an operation occurs")
		}
		if req.Action != "" {
			fmt.Fprintf(&sb, "    Then the system shall %s\n", lcFirst(req.Action))
		} else {
			fmt.Fprintf(&sb, "    Then the requirement is satisfied\n")
		}
	}

	return FeatureFile{
		SpecPath:    specPath,
		FeatureName: name,
		Content:     sb.String(),
	}
}

// featureName derives a human-readable feature name from the SPEC.md path.
// "internal/fsguard/SPEC.md" → "internal/fsguard — Write Policy"
// Falls back to the directory name when no known mapping exists.
func featureName(specPath string) string {
	dir := filepath.Dir(specPath)
	// Use the two-level path relative to a known anchor if possible.
	parts := strings.Split(filepath.ToSlash(dir), "/")
	switch {
	case len(parts) >= 2:
		return strings.Join(parts[len(parts)-2:], "/")
	case len(parts) == 1:
		return parts[0]
	default:
		return specPath
	}
}

// scenarioTitle builds a short, readable Gherkin scenario title.
func scenarioTitle(req Requirement) string {
	base := req.FullText
	if len(base) > 80 {
		base = base[:77] + "..."
	}
	if req.ID != "" {
		return req.ID + " " + base
	}
	return base
}

// lcFirst lower-cases the first letter of s, leaving the rest unchanged.
// EARS conditions often start with "When", which we keep as-is since Gherkin
// expects lower-case after the step keyword.
func lcFirst(s string) string {
	if s == "" {
		return s
	}
	// If starts with a keyword like "When", "While", "Where", "If", "The" —
	// strip it so the Gherkin step keyword handles that word.
	lower := strings.ToLower(s)
	for _, kw := range []string{"when ", "while ", "where ", "if "} {
		if strings.HasPrefix(lower, kw) {
			s = s[len(kw):]
			break
		}
	}
	if s == "" {
		return s
	}
	runes := []rune(s)
	return strings.ToLower(string(runes[0])) + string(runes[1:])
}
