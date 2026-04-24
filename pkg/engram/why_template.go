package engram

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GenerateWhyTemplate creates .why.md template for engram
func GenerateWhyTemplate(engramName string) string {
	today := time.Now().Format("2006-01-02")

	return fmt.Sprintf(`---
rationale_for: "%s"
decision_date: "%s"
decided_by: "TODO"
review_cycle: "quarterly"
status: "active"
superseded_by: ""
---

## Problem Statement

TODO: What problem does this engram solve?

- What pain points exist?
- What is the business/technical impact?
- Quantifiable metrics (if applicable)

## Decision Criteria

What requirements must the solution meet?

1. **Criterion 1**: TODO
2. **Criterion 2**: TODO
3. **Criterion 3**: TODO

## Alternatives Considered

### Option A: TODO
**Approach**: TODO

**Pros**:
- TODO

**Cons**:
- TODO

**Verdict**: ❌ Rejected | ✅ Selected

### Option B: TODO
**Approach**: TODO

**Pros**:
- TODO

**Cons**:
- TODO

**Verdict**: ❌ Rejected | ✅ Selected

## Decision

**Selected**: TODO

**Rationale**:
TODO: Why was this option selected?

1. Reason 1
2. Reason 2
3. Reason 3

## Success Metrics

How do we know this engram is successful?

- [ ] Metric 1: TODO
- [ ] Metric 2: TODO
- [ ] Metric 3: TODO

## Review Schedule

- **Next Review**: TODO (YYYY-MM-DD)
- **Review Triggers**:
  - Quarterly review (if review_cycle: quarterly)
  - When success metrics fail
  - When technology/requirements change
`, engramName, today)
}

// CreateWhyFileIfMissing generates .why.md if it doesn't exist
func CreateWhyFileIfMissing(aiMdPath string) error {
	whyPath := strings.TrimSuffix(aiMdPath, ".ai.md") + ".why.md"

	// Check if already exists
	if _, err := os.Stat(whyPath); err == nil {
		return nil // Already exists, skip
	}

	// Extract engram name
	basename := filepath.Base(strings.TrimSuffix(aiMdPath, ".ai.md"))

	// Generate template
	template := GenerateWhyTemplate(basename)

	// Write file
	if err := os.WriteFile(whyPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("failed to write .why.md: %w", err)
	}

	return nil
}
