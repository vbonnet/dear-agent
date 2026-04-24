package w0

import (
	"fmt"
	"regexp"
	"strings"
)

// SynthesisInput holds the input for charter synthesis.
type SynthesisInput struct {
	Q1Answer string // Problem description
	Q2Answer string // Impact/why it matters
	Q3Answer string // Proposed approach
	Q4Answer string // Constraints
	Date     string // Charter creation date (defaults to today)
}

// CharterValidationResult holds the result of charter validation.
type CharterValidationResult struct {
	Valid          bool
	Error          string
	Warning        string
	MissingSection []string
	Action         string // "retry_synthesis", "ask_clarification", "proceed"
}

// GenerateSynthesisPrompt generates the synthesis prompt for creating a W0 charter.
func GenerateSynthesisPrompt(input SynthesisInput) string {
	return fmt.Sprintf(`You are helping create a W0 Project Charter for a wayfinder project.

Context from conversation:
- Q1 (Problem): %s
- Q2 (Impact): %s
- Q3 (Approach): %s
- Q4 (Constraints): %s

Generate a W0 charter following this step-by-step process:

**Step 1: Identify Core Problem**
Combine Q1 (what's broken/missing) with Q2 (why it matters) into a clear problem statement.
- Make it specific and concrete
- Include impact/urgency from Q2

**Step 2: Derive Success Criteria**
Flip the problem from Q2 into measurable outcomes.
- Example: "40%% abandonment" -> "Reduce abandonment to <25%%"
- Make criteria observable and testable

**Step 3: Structure Solution Approach**
Take Q3 approach and expand slightly while staying high-level.
- Focus on "what" not "how"
- Detailed design comes in S6, not W0

**Step 4: Define Scope Boundaries**
Separate V1 from deferred based on Q3 and conversation.
- In scope: What's mentioned in Q3
- Out of scope: Enhancements not mentioned or explicitly deferred

**Step 5: Capture Constraints**
List constraints from Q4, or note "No specific constraints identified" if none.

**Step 6: Validate Completeness**
Check all required sections present:
- Problem Statement
- Proposed Solution
- Success Criteria
- Scope (in/out)

Now generate a W0 charter for the current conversation using the same structured approach:

Q1: %s
Q2: %s
Q3: %s
Q4: %s

Follow the 6-step process above.`,
		input.Q1Answer, input.Q2Answer, input.Q3Answer, input.Q4Answer,
		input.Q1Answer, input.Q2Answer, input.Q3Answer, input.Q4Answer)
}

// ValidateCharter validates a generated W0 charter.
func ValidateCharter(charter string) CharterValidationResult {
	requiredSections := []string{
		"Problem Statement",
		"Proposed Solution",
		"Success Criteria",
		"Scope",
	}

	var missing []string
	for _, section := range requiredSections {
		if !strings.Contains(charter, section) {
			missing = append(missing, section)
		}
	}

	if len(missing) > 0 {
		return CharterValidationResult{
			Error:          fmt.Sprintf("Missing required sections: %s", strings.Join(missing, ", ")),
			MissingSection: missing,
			Action:         "retry_synthesis",
		}
	}

	lowPriority := regexp.MustCompile(`(?i)low priority`).MatchString(charter)
	urgent := regexp.MustCompile(`(?i)urgent|critical|high priority`).MatchString(charter)

	if lowPriority && urgent {
		return CharterValidationResult{
			Warning: "Detected contradiction: charter mentions both \"low priority\" and \"urgent/high priority\"",
			Action:  "ask_clarification",
		}
	}

	if !strings.Contains(charter, "TBD") {
		return CharterValidationResult{
			Valid:   true,
			Warning: "Charter may need \"TBD\" markers for effort/priority (to be determined in later phases)",
			Action:  "proceed",
		}
	}

	return CharterValidationResult{Valid: true, Action: "proceed"}
}

// ExtractCharterTitle extracts the charter title from generated content.
func ExtractCharterTitle(charter string) string {
	re := regexp.MustCompile(`(?m)^#\s+W0 Project Charter:\s+(.+)$`)
	matches := re.FindStringSubmatch(charter)
	if matches == nil {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

// CharterSections holds extracted sections from a charter.
type CharterSections struct {
	ProblemStatement string
	ProposedSolution string
	SuccessCriteria  string
	Scope            string
	Constraints      string
}

// ExtractCharterSections extracts sections from a charter for quality assessment.
func ExtractCharterSections(charter string) CharterSections {
	var sections CharterSections

	extractSection := func(name string) string {
		re := regexp.MustCompile(`(?s)## ` + regexp.QuoteMeta(name) + `\s+(.*?)(?:##|$)`)
		matches := re.FindStringSubmatch(charter)
		if matches != nil {
			return strings.TrimSpace(matches[1])
		}
		return ""
	}

	sections.ProblemStatement = extractSection("Problem Statement")
	sections.ProposedSolution = extractSection("Proposed Solution")
	sections.SuccessCriteria = extractSection("Success Criteria")
	sections.Scope = extractSection("Scope")
	sections.Constraints = extractSection("Constraints")

	return sections
}
