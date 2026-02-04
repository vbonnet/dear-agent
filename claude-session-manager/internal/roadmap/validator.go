package roadmap

import (
	"fmt"
	"strings"
)

// ValidateROADMAP validates ROADMAP.md structure and content
// Returns a list of validation errors found
func ValidateROADMAP(filePath string) []ValidationError {
	beads, err := ParseROADMAP(filePath)
	if err != nil {
		return []ValidationError{
			{Line: 0, Message: fmt.Sprintf("Failed to parse ROADMAP: %v", err)},
		}
	}

	var errors []ValidationError

	// Check for duplicate bead IDs
	seenIDs := make(map[string]int)
	for _, bead := range beads {
		if firstLine, exists := seenIDs[bead.ID]; exists {
			errors = append(errors, ValidationError{
				Line: bead.LineNumber,
				Message: fmt.Sprintf("Duplicate bead ID '%s' (first seen at line %d)",
					bead.ID, firstLine),
			})
		} else {
			seenIDs[bead.ID] = bead.LineNumber
		}
	}

	// Check for invalid status symbols
	validStatuses := map[string]bool{
		"completed":   true,
		"in_progress": true,
		"blocked":     true,
		"cancelled":   true,
		"pending":     true,
	}

	for _, bead := range beads {
		if !validStatuses[bead.Status] {
			errors = append(errors, ValidationError{
				Line: bead.LineNumber,
				Message: fmt.Sprintf("Invalid status '%s' for bead '%s' (expected: completed, in_progress, blocked, cancelled, or pending)",
					bead.Status, bead.ID),
			})
		}
	}

	// Check for beads not assigned to any phase
	orphanCount := 0
	var orphanIDs []string
	for _, bead := range beads {
		if bead.Phase == "phase-unknown" {
			orphanCount++
			orphanIDs = append(orphanIDs, bead.ID)
		}
	}

	if orphanCount > 0 {
		errors = append(errors, ValidationError{
			Line: 0,
			Message: fmt.Sprintf("%d bead(s) not assigned to any phase section: %s",
				orphanCount, strings.Join(orphanIDs, ", ")),
		})
	}

	// Check for empty bead descriptions
	for _, bead := range beads {
		if strings.TrimSpace(bead.Description) == "" {
			errors = append(errors, ValidationError{
				Line: bead.LineNumber,
				Message: fmt.Sprintf("Bead '%s' has empty description", bead.ID),
			})
		}
	}

	return errors
}

// ValidateBeadID checks if a bead ID follows the correct naming convention
func ValidateBeadID(id string) error {
	if !strings.HasPrefix(id, "oss-") {
		return fmt.Errorf("bead ID must start with 'oss-', got: %s", id)
	}

	// Check for valid characters (lowercase, digits, hyphens only)
	for _, ch := range id[4:] { // Skip "oss-" prefix
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
			return fmt.Errorf("bead ID contains invalid character '%c': %s", ch, id)
		}
	}

	if len(id) < 5 { // At least "oss-x"
		return fmt.Errorf("bead ID too short: %s", id)
	}

	return nil
}
