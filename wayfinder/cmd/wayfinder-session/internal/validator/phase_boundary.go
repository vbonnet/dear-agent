package validator

import (
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/git"
)

// forbiddenPhasesV1 defines v1 phases where code changes are not allowed (planning phases).
// These phases focus on discovery, design, and planning rather than implementation.
var forbiddenPhasesV1 = map[string]bool{
	"D1":       true, // Problem Validation (v1 legacy)
	"D2":       true, // Existing Solutions (v1 legacy)
	"D3":       true, // Approach Decision (v1 legacy)
	"D4":       true, // Solution Requirements (v1 legacy)
	"S4":       true, // Stakeholder Alignment (v1 legacy)
	"S5":       true, // Research (v1 legacy)
	"S6":       true, // Design (v1 legacy)
	"S7":       true, // Plan (v1 legacy)
	"PROBLEM":  true, // Discovery & Context (v2)
	"RESEARCH": true, // Investigation & Options (v2)
	"DESIGN":   true, // Architecture & Design Spec (v2)
	"SPEC":     true, // Solution Requirements (v2)
	"PLAN":     true, // Design (v2)
	"SETUP":    true, // Planning & Task Breakdown (v2)
}

// forbiddenPhasesV2 defines v2 phases where code changes are not allowed.
var forbiddenPhasesV2 = map[string]bool{
	"discovery.problem":      true,
	"discovery.solutions":    true,
	"discovery.approach":     true,
	"discovery.requirements": true,
	"definition":             true,
	"specification":          true,
	"design.tech-lead":       true,
	"design.security":        true,
	"design.qa":              true,
	"roadmap.planning":       true,
	"roadmap.breakdown":      true,
	"roadmap.dependencies":   true,
}

// isPhaseForbiddenForCode returns true if the phase is a planning phase
// where code changes are not allowed. Supports both v1 and v2 phase names.
func isPhaseForbiddenForCode(phaseName string) bool {
	// Check v1 phases
	if forbiddenPhasesV1[phaseName] {
		return true
	}
	// Check v2 phases
	return forbiddenPhasesV2[phaseName]
}

// validatePhaseBoundaries checks if current phase allows code changes.
// Returns ValidationError if code changes detected in planning phases.
//
// Algorithm:
//  1. Check if phase is forbidden for code (D1-D4, S4-S7)
//  2. If allowed phase (CHARTER, BUILD, RETRO) → skip check (optimization)
//  3. If forbidden → query git for modified source files
//  4. If violations found → return ValidationError with file list
//  5. If no violations → return nil
func (v *Validator) validatePhaseBoundaries(phaseName, projectDir string) error {
	// Optimization: Skip check for allowed phases
	if !isPhaseForbiddenForCode(phaseName) {
		return nil
	}

	// Check for modified source files
	gitIntegrator := git.New(projectDir)
	violatingFiles, err := gitIntegrator.GetModifiedSourceFiles(projectDir)
	if err != nil {
		return NewValidationError(
			phaseName,
			fmt.Sprintf("Git check failed: %v", err),
			"Ensure project is in a git repository",
		)
	}

	// If violations found, return error with file list
	if len(violatingFiles) > 0 {
		fileList := formatViolatingFiles(violatingFiles)
		return NewValidationError(
			phaseName,
			fmt.Sprintf("Code changes detected in planning phase\n\nViolating files:\n  %s", fileList),
			"Revert code changes or wait until BUILD (Implementation) phase.\nRun 'git status' to see all modified files.",
		)
	}

	return nil
}

// formatViolatingFiles formats file list for error message.
// Shows first 5 files, then "...and N more" if >5.
func formatViolatingFiles(files []string) string {
	if len(files) == 0 {
		return ""
	}

	const maxFiles = 5
	if len(files) <= maxFiles {
		return strings.Join(files, ", ")
	}

	shown := strings.Join(files[:maxFiles], ", ")
	remaining := len(files) - maxFiles
	return fmt.Sprintf("%s ...and %d more", shown, remaining)
}
