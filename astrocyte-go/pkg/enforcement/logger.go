package enforcement

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ViolationData contains all the information needed to file a violation.
type ViolationData struct {
	// Required fields
	PatternID    string    // Pattern ID from pattern database
	PatternType  string    // Pattern file type (bash, beads, git)
	Command      string    // Exact command that violated
	SessionID    string    // Session name where violation occurred
	AgentType    string    // Type of agent (general-purpose, explore, etc.)
	Timestamp    time.Time // When the violation occurred

	// Optional fields
	TaskCategory         string   // Task being attempted
	ConversationLength   int      // Number of messages/turns before violation
	Tags                 []string // Freeform tags for categorization
	EngramVersion        string   // Version of engram at time of violation
	EngramHash           string   // SHA256 hash of engram content
}

// FileViolation writes a violation file to the violations directory.
// The file follows the schema defined in ~/src/engram/violations/SCHEMA.yaml
//
// File structure:
// - YAML frontmatter with required and optional fields
// - Markdown sections: Context, Violation Details, Why It Happened, Recovery, Proposed Fix
//
// Returns the path to the created violation file, or an error if filing failed.
func FileViolation(violation ViolationData, violationsDir string, pattern *Pattern) (string, error) {
	// Validate required fields
	if violation.PatternID == "" || violation.PatternType == "" || violation.Command == "" {
		return "", fmt.Errorf("missing required fields: pattern_id, pattern_type, and command are required")
	}
	if pattern == nil {
		return "", fmt.Errorf("pattern cannot be nil")
	}

	// Generate short hash from command (8 chars like Python version)
	hash := sha256.Sum256([]byte(violation.Command))
	commandHash := fmt.Sprintf("%x", hash[:])[:8]

	// Create violations subdirectory if needed
	typeDir := filepath.Join(violationsDir, violation.PatternType)
	if err := os.MkdirAll(typeDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create violations directory: %w", err)
	}

	// Generate filename: YYYY-MM-DD-{pattern-id}-{short-hash}.md
	timestamp := violation.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	dateStr := timestamp.Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s-%s.md", dateStr, violation.PatternID, commandHash)
	filepath := filepath.Join(typeDir, filename)

	// Map pattern_id to violation type (matches Python Astrocyte)
	typeMap := map[string]string{
		"cd-chaining":             "cd_usage",
		"cd-semicolon-chain":      "cd_usage",
		"for-loop":                "for_loops",
		"while-loop":              "for_loops",
		"double-ampersand-chain":  "chained_commands",
		"semicolon-chain":         "chained_commands",
		"git-add-all":             "git_violations",
		"git-worktree-branch":     "git_violations",
		"cat-file-read":           "bash_over_tools",
		"grep-search":             "bash_over_tools",
		"find-file-search":        "bash_over_tools",
		"echo-redirect":           "bash_over_tools",
		"cat-heredoc":             "bash_over_tools",
	}
	violationType := typeMap[violation.PatternID]
	if violationType == "" {
		violationType = "bash_over_tools"
	}

	// Determine severity from pattern
	severity := pattern.Severity
	if severity == "" {
		severity = "medium"
	}

	// Build file content
	content := buildViolationContent(violation, pattern, violationType, severity, filename, timestamp)

	// Write violation file
	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write violation file: %w", err)
	}

	return filepath, nil
}

// buildViolationContent constructs the complete violation file content
// with YAML frontmatter and markdown sections.
func buildViolationContent(violation ViolationData, pattern *Pattern, violationType, severity, filename string, timestamp time.Time) string {
	var content strings.Builder

	// YAML frontmatter
	content.WriteString("---\n")
	content.WriteString(fmt.Sprintf("id: %s\n", strings.TrimSuffix(filename, ".md")))
	content.WriteString(fmt.Sprintf("date: %s\n", timestamp.Format(time.RFC3339)))
	content.WriteString(fmt.Sprintf("type: %s\n", violationType))
	content.WriteString(fmt.Sprintf("severity: %s\n", severity))
	content.WriteString("tier: \"3_astrocyte\"\n")
	content.WriteString(fmt.Sprintf("pattern_id: %s\n", violation.PatternID))
	content.WriteString(fmt.Sprintf("pattern_type: %s\n", violation.PatternType))
	content.WriteString(fmt.Sprintf("session_id: %s\n", violation.SessionID))
	content.WriteString(fmt.Sprintf("agent_type: %s\n", violation.AgentType))
	content.WriteString(fmt.Sprintf("command: %s\n", violation.Command))

	// Optional fields
	if violation.TaskCategory != "" {
		content.WriteString(fmt.Sprintf("task_category: %s\n", violation.TaskCategory))
	}
	if violation.ConversationLength > 0 {
		content.WriteString(fmt.Sprintf("conversation_length: %d\n", violation.ConversationLength))
	}
	if len(violation.Tags) > 0 {
		content.WriteString("tags:\n")
		for _, tag := range violation.Tags {
			content.WriteString(fmt.Sprintf("  - %s\n", tag))
		}
	}
	if violation.EngramVersion != "" {
		content.WriteString(fmt.Sprintf("engram_version: %s\n", violation.EngramVersion))
	}
	if violation.EngramHash != "" {
		content.WriteString(fmt.Sprintf("engram_hash: %s\n", violation.EngramHash))
	}

	content.WriteString("---\n\n")

	// Markdown body - matches Python Astrocyte format

	// Title
	content.WriteString(fmt.Sprintf("# Violation Report: %s\n\n", violation.PatternID))

	// Context section
	content.WriteString("## Context\n\n")
	content.WriteString(fmt.Sprintf("Agent attempted to use a bash command that violated the %s anti-pattern database.\n", violation.PatternType))
	content.WriteString("The Astrocyte daemon detected this violation during session monitoring and filed this report.\n\n")
	content.WriteString(fmt.Sprintf("Session: %s\n", violation.SessionID))
	content.WriteString(fmt.Sprintf("Agent type: %s\n\n", violation.AgentType))

	// Violation Details section
	content.WriteString("## Violation Details\n\n")
	content.WriteString(fmt.Sprintf("- **Command attempted**: `%s`\n", violation.Command))
	content.WriteString(fmt.Sprintf("- **Pattern matched**: %s (%s)\n", violation.PatternID, violation.PatternType))
	content.WriteString(fmt.Sprintf("- **Reason**: %s\n", pattern.Reason))
	content.WriteString(fmt.Sprintf("- **Correct approach**: %s\n\n", pattern.Alternative))

	// Why It Happened section
	content.WriteString("## Why It Happened\n\n")
	content.WriteString("This violation was detected by Astrocyte (Tier 3 enforcement). It indicates that:\n")
	content.WriteString("1. The agent bypassed or ignored Tier 1 instructions in .ai.md engrams\n")
	content.WriteString("2. The PreTool hook (Tier 2) did not catch the violation (may need updating)\n")
	content.WriteString("3. The violation persisted long enough to trigger Astrocyte monitoring\n\n")
	content.WriteString("Root cause likely: Instruction gap in engrams or agent mental model override.\n\n")

	// Recovery section
	content.WriteString("## Recovery\n\n")
	content.WriteString("Astrocyte sent rejection message via `agm send reject` with the following guidance:\n")
	content.WriteString(fmt.Sprintf("- Reason: %s\n", pattern.Reason))
	content.WriteString(fmt.Sprintf("- Alternative: %s\n\n", pattern.Alternative))

	// Proposed Fix section
	content.WriteString("## Proposed Fix\n\n")
	content.WriteString("Review and strengthen Tier 1 instructions in bash-command-simplification.ai.md.\n")
	content.WriteString("Consider adding this pattern to PreTool hook validation if not already present.\n")

	return content.String()
}
