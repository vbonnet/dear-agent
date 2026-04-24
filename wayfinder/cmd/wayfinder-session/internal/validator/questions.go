package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validatePhaseQuestions validates that all critical questions have been answered
// before allowing phase completion.
//
// Performs three validation checks:
//  1. Scans all .md files for unresolved [NEEDS_CLARIFICATION_BY_PHASE: ...] markers
//  2. For D4/S7 phases: Verifies all assumption checklist items are checked
//  3. Checks for pending AskUserQuestion state file (optional hook-based tracking)
//
// Returns ValidationError if any check fails, nil if validation passes.
func validatePhaseQuestions(projectDir string, phaseName string) error {
	// Layer 1: Check for unresolved clarification markers
	count, files, err := countClarificationMarkers(projectDir)
	if err != nil {
		return fmt.Errorf("failed to check clarification markers: %w", err)
	}
	if count > 0 {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("Found %d unresolved clarification marker(s) in: %s",
				count, strings.Join(files, ", ")),
			"Resolve questions using AskUserQuestion tool, then remove markers",
		)
	}

	// Layer 2: Check assumption verification checklists (D4/S7 only)
	if phaseName == "D4" || phaseName == "S7" || phaseName == "SPEC" || phaseName == "SETUP" {
		count, file, err := countUncheckedAssumptions(phaseName, projectDir)
		if err != nil {
			return fmt.Errorf("failed to check assumptions: %w", err)
		}
		if count > 0 {
			return NewValidationError(
				"complete "+phaseName,
				fmt.Sprintf("Found %d unchecked assumption(s) in %s", count, file),
				"Verify each assumption using AskUserQuestion tool, then check items",
			)
		}
	}

	// Layer 3: Check for pending questions from hooks (optional)
	count, questionPhase, err := getPendingQuestions(projectDir)
	if err != nil {
		return fmt.Errorf("failed to check pending questions: %w", err)
	}
	if count > 0 {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("Found %d pending question(s) from phase %s", count, questionPhase),
			"Answer pending questions, or delete .wayfinder-questions.json to reset",
		)
	}

	return nil
}

// countClarificationMarkers scans all .md files in projectDir for the pattern
// [NEEDS_CLARIFICATION_BY_PHASE: ...] and returns the count and list of files
// containing markers.
func countClarificationMarkers(projectDir string) (int, []string, error) {
	// Get list of .md files in projectDir (not recursive)
	files, err := filepath.Glob(filepath.Join(projectDir, "*.md"))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to list markdown files: %w", err)
	}

	var markerCount int
	var filesWithMarkers []string
	markerPattern := "[NEEDS_CLARIFICATION_BY_PHASE:"

	// Scan each file for the pattern
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to read %s: %w", file, err)
		}

		count := strings.Count(string(content), markerPattern)
		if count > 0 {
			markerCount += count
			filesWithMarkers = append(filesWithMarkers, filepath.Base(file))
		}
	}

	return markerCount, filesWithMarkers, nil
}

// countUncheckedAssumptions counts unchecked items in D4 or S7 assumption
// verification checklists.
func countUncheckedAssumptions(phaseName string, projectDir string) (int, string, error) {
	// Determine target file based on phase
	var filename string
	switch phaseName {
	case "D4", "SPEC":
		filename = phaseName + "-solution-requirements.md"
	case "S7", "SETUP":
		filename = phaseName + "-plan.md"
	default:
		// Not D4/SPEC or S7/SETUP - no assumption checklist required
		return 0, "", nil
	}

	// Read the file
	filePath := filepath.Join(projectDir, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - graceful degradation
			return 0, "", nil
		}
		return 0, "", fmt.Errorf("failed to read %s: %w", filename, err)
	}

	// Find "## Assumption Verification Checklist" section
	lines := strings.Split(string(content), "\n")
	inSection := false
	uncheckedCount := 0

	for _, line := range lines {
		// Start of target section
		if strings.Contains(line, "## Assumption Verification Checklist") {
			inSection = true
			continue
		}

		// End of section (next ## header)
		if inSection && strings.HasPrefix(line, "## ") {
			break
		}

		// Count unchecked items: "- [ ]"
		if inSection {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [ ]") {
				uncheckedCount++
			}
		}
	}

	if uncheckedCount == 0 {
		return 0, "", nil
	}

	return uncheckedCount, filename, nil
}

// questionState represents the structure of .wayfinder-questions.json
// This file is OPTIONAL and created by AskUserQuestion hooks.
type questionState struct {
	Questions []questionRecord `json:"questions"`
}

type questionRecord struct {
	Phase    string `json:"phase"`
	Question string `json:"question"`
	Status   string `json:"status"` // "pending" or "answered"
	AskedAt  string `json:"asked_at,omitempty"`
}

// getPendingQuestions checks for the presence of a .wayfinder-questions.json
// state file created by the AskUserQuestion hook.
func getPendingQuestions(projectDir string) (int, string, error) {
	// Check if state file exists
	stateFilePath := filepath.Join(projectDir, ".wayfinder-questions.json")
	content, err := os.ReadFile(stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No state file = graceful degradation
			return 0, "", nil
		}
		return 0, "", fmt.Errorf("failed to read state file: %w", err)
	}

	// Parse JSON
	var state questionState
	if err := json.Unmarshal(content, &state); err != nil {
		return 0, "", fmt.Errorf("failed to parse state file: %w", err)
	}

	// Count pending questions
	var pendingCount int
	var firstPendingPhase string
	for _, q := range state.Questions {
		if q.Status == "pending" {
			pendingCount++
			if firstPendingPhase == "" {
				firstPendingPhase = q.Phase
			}
		}
	}

	return pendingCount, firstPendingPhase, nil
}
