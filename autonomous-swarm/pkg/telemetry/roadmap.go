package telemetry

import (
	"fmt"
	"os"
	"strings"

	"github.com/vbonnet/ai-tools/autonomous-swarm/pkg/taskqueue"
)

const (
	// MaxRoadmapTokens is the maximum token budget for ROADMAP.md
	// Estimated at ~4 chars per token
	MaxRoadmapTokens = 1500
	MaxRoadmapChars  = MaxRoadmapTokens * 4
)

// GenerateRoadmap creates a human-readable roadmap from the task queue
func GenerateRoadmap(queueFilePath string, outputPath string) error {
	// Parse task queue
	queue, err := taskqueue.ParseTaskQueue(queueFilePath)
	if err != nil {
		return fmt.Errorf("failed to parse task queue: %w", err)
	}

	// Generate markdown content
	content := generateRoadmapMarkdown(queue)

	// Enforce token budget (truncate if necessary)
	if len(content) > MaxRoadmapChars {
		content = truncateWithEllipsis(content, MaxRoadmapChars)
	}

	// Write to temp file
	tmpPath := outputPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath) // Cleanup on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// generateRoadmapMarkdown creates markdown content from task queue
func generateRoadmapMarkdown(queue *taskqueue.TaskQueue) string {
	var sb strings.Builder

	sb.WriteString("# Task Roadmap\n\n")
	sb.WriteString(fmt.Sprintf("**Last Updated**: %s\n\n", queue.LastUpdated.Format("2006-01-02 15:04:05")))

	// Ready section (count only)
	sb.WriteString("## Ready\n\n")
	sb.WriteString(fmt.Sprintf("**Count**: %d beads ready for execution\n\n", len(queue.Ready)))

	// In Progress section (list)
	sb.WriteString("## In Progress\n\n")
	if len(queue.InProgress) == 0 {
		sb.WriteString("*No beads currently in progress*\n\n")
	} else {
		for _, bead := range queue.InProgress {
			sessionInfo := ""
			if bead.Metadata.SessionName != "" {
				sessionInfo = fmt.Sprintf(" (session: %s, iteration: %d)", bead.Metadata.SessionName, bead.Metadata.Iterations)
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s%s\n", bead.ID, bead.Title, sessionInfo))
		}
		sb.WriteString("\n")
	}

	// Blocked section (list with reasons)
	sb.WriteString("## Blocked\n\n")
	if len(queue.Blocked) == 0 {
		sb.WriteString("*No blocked beads*\n\n")
	} else {
		for _, bead := range queue.Blocked {
			reason := "unspecified"
			if bead.Metadata.EscalationReason != "" {
				reason = bead.Metadata.EscalationReason
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n  - Reason: %s\n", bead.ID, bead.Title, reason))
		}
		sb.WriteString("\n")
	}

	// Completed section (count only)
	sb.WriteString("## Completed\n\n")
	sb.WriteString(fmt.Sprintf("**Count**: %d beads completed\n\n", len(queue.Completed)))

	// Progress summary
	total := len(queue.Ready) + len(queue.InProgress) + len(queue.Blocked) + len(queue.Completed)
	if total > 0 {
		percentage := (len(queue.Completed) * 100) / total
		sb.WriteString("---\n\n")
		sb.WriteString(fmt.Sprintf("**Progress**: %d/%d (%d%%) completed\n", len(queue.Completed), total, percentage))
	}

	return sb.String()
}

// truncateWithEllipsis truncates content to max length with "..." suffix
func truncateWithEllipsis(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}

	// Reserve space for ellipsis
	truncated := content[:maxLen-4]

	// Try to truncate at last newline to avoid cutting mid-sentence
	if lastNewline := strings.LastIndex(truncated, "\n"); lastNewline > maxLen/2 {
		truncated = truncated[:lastNewline]
	}

	return truncated + "\n\n..."
}
