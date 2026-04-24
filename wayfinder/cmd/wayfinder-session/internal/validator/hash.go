package validator

import (
	"fmt"
	"path/filepath"

	"github.com/vbonnet/dear-agent/pkg/hash"
)

// calculatePhaseEngramHash calculates SHA-256 hash of a phase engram file
// Returns hash in format "sha256:{hex_hash}"
// Returns error if:
// - Path expansion fails
// - File cannot be read
func calculatePhaseEngramHash(engramPath string) (string, error) {
	return hash.CalculateFileHash(engramPath)
}

// validateMethodologyFreshness validates that the deliverable was created using
// the current version of the phase methodology (anti-hallucination check)
// Compares phase_engram_hash from deliverable frontmatter against actual engram file hash
// Returns ValidationError if:
// - Deliverable has no frontmatter
// - Hash mismatch and no --reason override provided
// Returns nil if:
// - Hashes match (deliverable is fresh)
// - Hash mismatch but --reason override provided (logs warning, allows completion)
func validateMethodologyFreshness(projectDir, phaseName, hashMismatchReason string) error {
	// Find deliverable file
	pattern := filepath.Join(projectDir, fmt.Sprintf("%s-*.md", phaseName))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to search for deliverable: %w", err)
	}

	if len(matches) == 0 {
		// No deliverable found - this should be caught by validateDeliverableExists
		return nil
	}

	deliverablePath := matches[0]

	// Extract frontmatter from deliverable
	fm, err := extractFrontmatter(deliverablePath)
	if err != nil {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("deliverable %s has invalid or missing frontmatter: %v", filepath.Base(deliverablePath), err),
			"Add YAML frontmatter with required fields (phase, phase_name, wayfinder_session_id, created_at, phase_engram_hash, phase_engram_path)",
		)
	}

	// Calculate current hash of the phase engram
	currentHash, err := calculatePhaseEngramHash(fm.PhaseEngramPath)
	if err != nil {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to calculate hash of phase engram %s: %v", fm.PhaseEngramPath, err),
			"Ensure the phase_engram_path in frontmatter points to a valid engram file",
		)
	}

	// Compare hashes
	if fm.PhaseEngramHash != currentHash {
		// Hash mismatch - methodology has changed since deliverable was created
		if hashMismatchReason == "" {
			// No override reason provided - block completion
			return NewValidationError(
				"complete "+phaseName,
				fmt.Sprintf("deliverable was created with outdated methodology (hash mismatch: deliverable has %s, current is %s)",
					fm.PhaseEngramHash, currentHash),
				"The phase methodology has been updated since this deliverable was created. Either:\n"+
					"  1. Recreate the deliverable using the current methodology, OR\n"+
					"  2. Use --reason flag to override: complete-phase "+phaseName+" --reason \"Reviewed methodology changes, deliverable still valid\"",
			)
		}

		// Override reason provided - log warning but allow completion
		// TODO: Log hash mismatch to WAYFINDER-HISTORY.md (deferred to future iteration)
		// For now, just allow completion
		// In production, this would log: "Phase %s completed with hash mismatch override: %s", phaseName, hashMismatchReason
	}

	// Hashes match or override provided - validation passes
	return nil
}
