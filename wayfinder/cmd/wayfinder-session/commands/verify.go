package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/validator"
)

// VerifyCmd validates a phase file and adds signature if valid
var VerifyCmd = &cobra.Command{
	Use:   "verify [file]",
	Short: "Validate a phase file and sign if valid",
	Long: `Validate a phase file against Wayfinder rules and add a validation signature if valid.

The verify command:
- Runs all validation rules for the phase (size, frontmatter, content)
- If validation passes: adds a signature to the file's YAML frontmatter
- If validation fails: prints errors to stderr and exits with code 1

The signature includes:
- validated: true
- validated_at: timestamp
- validator_version: version string
- checksum: SHA256 hash of file content

Example:
  wayfinder-session verify D1.md
  wayfinder-session verify ./phases/S7-plan.md`,
	Args: cobra.ExactArgs(1),
	RunE: runVerify,
}

func runVerify(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// Resolve to absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", absPath)
	}

	// Extract phase name from filename (e.g., "D1.md" -> "D1")
	phaseName := extractPhaseName(filepath.Base(absPath))
	if phaseName == "" {
		return fmt.Errorf("invalid phase file name: must match pattern W0.md, D1.md, S4.md, etc.")
	}

	// Validate phase name is in AllPhases()
	validPhase := false
	for _, p := range status.AllPhases() {
		if p == phaseName {
			validPhase = true
			break
		}
	}
	if !validPhase {
		return fmt.Errorf("invalid phase: %s (valid phases: W0, D1-D4, S4-S11)", phaseName)
	}

	projectDir := filepath.Dir(absPath)

	// Try to read status from filesystem
	currentStatus, err := status.DetectFromFilesystem(projectDir)
	if err != nil {
		// Fallback to reading STATUS file
		currentStatus, err = status.ReadFrom(projectDir)
		if err != nil {
			// No status found - create minimal status for validation
			currentStatus = status.New(projectDir)
		}
	}

	// Create validator
	v := validator.NewValidator(currentStatus)

	// Run validation for this phase
	// Note: We validate as if we're completing the phase
	// This ensures all content requirements are met
	fmt.Fprintf(os.Stderr, "Validating %s...\n", phaseName)

	// Run phase-specific validation
	if err := v.CanCompletePhase(phaseName, projectDir, ""); err != nil {
		var validationErr *validator.ValidationError
		if errors.As(err, &validationErr) {
			fmt.Fprintf(os.Stderr, "\nValidation failed:\n")
			fmt.Fprintf(os.Stderr, "  Phase: %s\n", validationErr.Phase)
			fmt.Fprintf(os.Stderr, "  Reason: %s\n", validationErr.Reason)
			if validationErr.Fix != "" {
				fmt.Fprintf(os.Stderr, "  Fix: %s\n", validationErr.Fix)
			}
			return fmt.Errorf("validation failed")
		}
		return fmt.Errorf("validation failed: %w", err)
	}

	// Validation passed - add signature
	fmt.Fprintf(os.Stderr, "Validation passed. Adding signature...\n")
	if err := validator.AddSignature(absPath); err != nil {
		return fmt.Errorf("failed to add signature: %w", err)
	}

	fmt.Printf("✓ %s validated and signed\n", phaseName)
	return nil
}

// extractPhaseName extracts phase name from filename
// Examples: "D1.md" -> "D1", "S7-plan.md" -> "S7", "W0.md" -> "W0"
func extractPhaseName(filename string) string {
	// Remove extension
	base := filename
	if len(base) > 3 && base[len(base)-3:] == ".md" {
		base = base[:len(base)-3]
	}

	// Check for phase pattern at start: W0, D1-D4, S4-S11
	if len(base) < 2 {
		return ""
	}

	// Extract first character (W, D, or S) and number
	prefix := base[0:1]
	if prefix != "W" && prefix != "D" && prefix != "S" {
		return ""
	}

	// Extract number (one or two digits)
	numStr := ""
	for i := 1; i < len(base) && i < 3; i++ {
		if base[i] >= '0' && base[i] <= '9' {
			numStr += string(base[i])
		} else {
			break
		}
	}

	if numStr == "" {
		return ""
	}

	return prefix + numStr
}
