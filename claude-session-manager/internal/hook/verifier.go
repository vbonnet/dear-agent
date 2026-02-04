package hook

import (
	"fmt"
	"strings"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/plugin"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/roadmap"
)

// Mismatch represents a discrepancy between ROADMAP.md and task manager
type Mismatch struct {
	BeadID         string // Bead ID with mismatch
	ROADMAPStatus  string // Status in ROADMAP.md (normalized)
	TaskStatus     string // Status in task manager (normalized)
	ROADMAPRaw     string // Raw status from ROADMAP.md (e.g., "✅ COMPLETE")
	Line           int    // Line number in ROADMAP.md
}

// VerificationResult contains the results of ROADMAP verification
type VerificationResult struct {
	Mismatches []Mismatch
	Passed     bool
}

// Verifier checks ROADMAP.md consistency with task manager
type Verifier struct {
	plugin plugin.TaskManagerPlugin
}

// NewVerifier creates a new ROADMAP verifier
func NewVerifier(p plugin.TaskManagerPlugin) *Verifier {
	return &Verifier{plugin: p}
}

// VerifyROADMAP checks if ROADMAP.md matches task manager state
// Returns verification result with any mismatches found
func (v *Verifier) VerifyROADMAP(roadmapPath, sessionDir string) (*VerificationResult, error) {
	// Parse ROADMAP.md
	beads, err := roadmap.ParseROADMAP(roadmapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ROADMAP.md: %w", err)
	}

	// Get tasks from task manager
	tasks, err := v.plugin.GetTasks(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks from task manager: %w", err)
	}

	// Build task lookup map
	taskMap := make(map[string]plugin.Task)
	for _, task := range tasks {
		taskMap[task.ID] = task
	}

	// Check for mismatches
	var mismatches []Mismatch
	for _, bead := range beads {
		task, exists := taskMap[bead.ID]

		// Case 1: Bead in ROADMAP but not in task manager
		if !exists {
			// Only flag as mismatch if ROADMAP shows completed
			// (it's OK for pending tasks to not exist in task manager yet)
			if bead.Status == "completed" {
				mismatches = append(mismatches, Mismatch{
					BeadID:        bead.ID,
					ROADMAPStatus: bead.Status,
					TaskStatus:    "not_found",
					ROADMAPRaw:    bead.RawStatus,
					Line:          bead.LineNumber,
				})
			}
			continue
		}

		// Case 2: Status mismatch between ROADMAP and task manager
		if bead.Status != task.Status {
			mismatches = append(mismatches, Mismatch{
				BeadID:        bead.ID,
				ROADMAPStatus: bead.Status,
				TaskStatus:    task.Status,
				ROADMAPRaw:    bead.RawStatus,
				Line:          bead.LineNumber,
			})
		}
	}

	return &VerificationResult{
		Mismatches: mismatches,
		Passed:     len(mismatches) == 0,
	}, nil
}

// FormatError generates a user-friendly error message for mismatches
func FormatError(result *VerificationResult, taskManagerName string) string {
	if result.Passed {
		return ""
	}

	var b strings.Builder

	b.WriteString("❌ ROADMAP.md / Task Manager Mismatch Detected\n\n")
	b.WriteString(fmt.Sprintf("Found %d inconsistenc", len(result.Mismatches)))
	if len(result.Mismatches) == 1 {
		b.WriteString("y:\n")
	} else {
		b.WriteString("ies:\n")
	}

	for _, m := range result.Mismatches {
		b.WriteString(fmt.Sprintf("  • %s (line %d): ", m.BeadID, m.Line))

		if m.TaskStatus == "not_found" {
			b.WriteString(fmt.Sprintf("ROADMAP shows '%s' but task doesn't exist\n", m.ROADMAPRaw))
		} else {
			b.WriteString(fmt.Sprintf("ROADMAP shows '%s' but task is '%s'\n",
				m.ROADMAPStatus, m.TaskStatus))
		}
	}

	b.WriteString("\nFix options:\n")
	b.WriteString("  1. Update task status to match ROADMAP:\n")

	// Generate fix commands based on task manager type
	switch taskManagerName {
	case "beads":
		for _, m := range result.Mismatches {
			if m.ROADMAPStatus == "completed" {
				b.WriteString(fmt.Sprintf("     bd close %s\n", m.BeadID))
			}
		}
	case "claude-tasks":
		b.WriteString("     TaskUpdate taskId=\"<id>\" status=\"completed\"\n")
	default:
		b.WriteString("     (use your task manager's close/complete command)\n")
	}

	b.WriteString("\n  2. Update ROADMAP.md status to match task manager\n")
	b.WriteString("\n  3. Bypass verification (not recommended):\n")
	b.WriteString("     git commit --no-verify\n")
	b.WriteString("\nVerification failed. Commit blocked.\n")

	return b.String()
}
