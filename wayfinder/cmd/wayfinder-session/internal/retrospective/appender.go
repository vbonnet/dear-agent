package retrospective

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RetroFilename is the canonical retrospective deliverable filename.
const RetroFilename = "RETRO-retrospective.md"

// AppendToRetro formats RewindEventData as markdown and appends to the RETRO deliverable.
//
// Uses O_APPEND flag for concurrent-safe writes (atomic at OS level).
// Creates file if it doesn't exist (should exist from wayfinder session start, but fail-gracefully).
func AppendToRetro(projectDir string, data *RewindEventData) error {
	retroPath := filepath.Join(projectDir, RetroFilename)

	// Format markdown entry
	entry := formatRewindEntry(data)

	// Open file with O_APPEND (concurrent-safe atomic writes)
	// Create if doesn't exist (0644 permissions)
	file, err := os.OpenFile(retroPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", RetroFilename, err)
	}
	defer func() { _ = file.Close() }()

	// Write markdown entry
	if _, err := file.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to %s: %w", RetroFilename, err)
	}

	return nil
}

// formatRewindEntry formats RewindEventData as markdown section
//
// Template structure:
// ## Rewind: {from} → {to} (magnitude {N})
// **Timestamp**: {ISO8601}
// **Reason**: {reason}
// **Learnings**: {learnings}
// **Context**:
// - Git: {branch}@{commit} (uncommitted: {yes/no})
// - Deliverables: {list}
// - Completed phases: {list}
func formatRewindEntry(data *RewindEventData) string {
	var sb strings.Builder

	// Section header
	fmt.Fprintf(&sb, "\n---\n\n## Rewind: %s → %s (magnitude %d)\n\n",
		data.FromPhase, data.ToPhase, data.Magnitude)

	// Timestamp (ISO8601 format)
	fmt.Fprintf(&sb, "**Timestamp**: %s\n\n",
		data.Timestamp.Format(time.RFC3339))

	// Reason (always present for magnitude 1+)
	if data.Reason != "" {
		fmt.Fprintf(&sb, "**Reason**: %s\n\n", data.Reason)
	} else {
		sb.WriteString("**Reason**: _(not provided)_\n\n")
	}

	// Learnings (optional)
	if data.Learnings != "" {
		fmt.Fprintf(&sb, "**Learnings**: %s\n\n", data.Learnings)
	}

	// Context snapshot
	sb.WriteString("**Context**:\n")

	// Git context
	if data.Context.Git.Error != "" {
		fmt.Fprintf(&sb, "- Git: _(error: %s)_\n", data.Context.Git.Error)
	} else {
		uncommitted := "no"
		if data.Context.Git.UncommittedChanges {
			uncommitted = "yes"
		}
		fmt.Fprintf(&sb, "- Git: %s@%s (uncommitted: %s)\n",
			data.Context.Git.Branch, data.Context.Git.Commit, uncommitted)
	}

	// Deliverables
	if len(data.Context.Deliverables) > 0 {
		fmt.Fprintf(&sb, "- Deliverables: %s\n",
			strings.Join(data.Context.Deliverables, ", "))
	} else {
		sb.WriteString("- Deliverables: _(none)_\n")
	}

	// Completed phases
	if len(data.Context.PhaseState.CompletedPhases) > 0 {
		fmt.Fprintf(&sb, "- Completed phases: %s\n",
			strings.Join(data.Context.PhaseState.CompletedPhases, ", "))
	} else {
		sb.WriteString("- Completed phases: _(none)_\n")
	}

	sb.WriteString("\n")

	return sb.String()
}
