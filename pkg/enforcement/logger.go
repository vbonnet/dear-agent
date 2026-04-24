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
	PatternID   string
	PatternType string // Pattern file type (bash, beads, git)
	Command     string
	SessionID   string
	AgentType   string
	Timestamp   time.Time

	// Optional fields
	TaskCategory       string
	ConversationLength int
	Tags               []string
	EngramVersion      string
	EngramHash         string
}

// patternTypeMap maps pattern IDs to violation type categories.
var patternTypeMap = map[string]string{
	"cd-command":             "cd_usage",
	"cd-chaining":            "cd_usage",
	"cd-semicolon-chain":     "cd_usage",
	"subshell-cd":            "cd_usage",
	"for-loop":               "for_loops",
	"while-loop":             "for_loops",
	"command-chaining":       "chained_commands",
	"double-ampersand-chain": "chained_commands",
	"command-separator":      "chained_commands",
	"semicolon-chain":        "chained_commands",
	"git-add-broad":          "git_violations",
	"git-add-all":            "git_violations",
	"git-worktree-branch":    "git_violations",
	"file-operations":        "bash_over_tools",
	"text-processing":        "bash_over_tools",
	"cat-file-read":          "bash_over_tools",
	"grep-search":            "bash_over_tools",
	"find-file-search":       "bash_over_tools",
	"echo-redirect":          "bash_over_tools",
	"cat-heredoc":            "bash_over_tools",
}

// ViolationType returns the violation category for a pattern ID.
func ViolationType(patternID string) string {
	if t, ok := patternTypeMap[patternID]; ok {
		return t
	}
	return "bash_over_tools"
}

// FileViolation writes a violation file to the violations directory.
// Returns the path to the created violation file.
func FileViolation(violation ViolationData, violationsDir string, pattern *Pattern) (string, error) {
	if violation.PatternID == "" || violation.PatternType == "" || violation.Command == "" {
		return "", fmt.Errorf("missing required fields: pattern_id, pattern_type, and command are required")
	}
	if pattern == nil {
		return "", fmt.Errorf("pattern cannot be nil")
	}

	hash := sha256.Sum256([]byte(violation.Command))
	commandHash := fmt.Sprintf("%x", hash[:])[:8]

	typeDir := filepath.Join(violationsDir, violation.PatternType)
	if err := os.MkdirAll(typeDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create violations directory: %w", err)
	}

	timestamp := violation.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	dateStr := timestamp.Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s-%s.md", dateStr, violation.PatternID, commandHash)
	outPath := filepath.Join(typeDir, filename)

	violationType := ViolationType(violation.PatternID)
	severity := pattern.Severity
	if severity == "" {
		severity = "medium"
	}

	content := buildViolationContent(violation, pattern, violationType, severity, filename, timestamp)

	if err := os.WriteFile(outPath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("failed to write violation file: %w", err)
	}

	return outPath, nil
}

func buildViolationContent(violation ViolationData, pattern *Pattern, violationType, severity, filename string, timestamp time.Time) string {
	var content strings.Builder

	content.WriteString("---\n")
	fmt.Fprintf(&content, "id: %s\n", strings.TrimSuffix(filename, ".md"))
	fmt.Fprintf(&content, "date: %s\n", timestamp.Format(time.RFC3339))
	fmt.Fprintf(&content, "type: %s\n", violationType)
	fmt.Fprintf(&content, "severity: %s\n", severity)
	content.WriteString("tier: \"3_astrocyte\"\n")
	fmt.Fprintf(&content, "pattern_id: %s\n", violation.PatternID)
	fmt.Fprintf(&content, "pattern_type: %s\n", violation.PatternType)
	fmt.Fprintf(&content, "session_id: %s\n", violation.SessionID)
	fmt.Fprintf(&content, "agent_type: %s\n", violation.AgentType)
	fmt.Fprintf(&content, "command: %s\n", violation.Command)

	if violation.TaskCategory != "" {
		fmt.Fprintf(&content, "task_category: %s\n", violation.TaskCategory)
	}
	if violation.ConversationLength > 0 {
		fmt.Fprintf(&content, "conversation_length: %d\n", violation.ConversationLength)
	}
	if len(violation.Tags) > 0 {
		content.WriteString("tags:\n")
		for _, tag := range violation.Tags {
			fmt.Fprintf(&content, "  - %s\n", tag)
		}
	}
	if violation.EngramVersion != "" {
		fmt.Fprintf(&content, "engram_version: %s\n", violation.EngramVersion)
	}
	if violation.EngramHash != "" {
		fmt.Fprintf(&content, "engram_hash: %s\n", violation.EngramHash)
	}

	content.WriteString("---\n\n")

	fmt.Fprintf(&content, "# Violation Report: %s\n\n", violation.PatternID)

	content.WriteString("## Context\n\n")
	fmt.Fprintf(&content, "Agent attempted to use a bash command that violated the %s anti-pattern database.\n", violation.PatternType)
	content.WriteString("The enforcement layer detected this violation and filed this report.\n\n")
	fmt.Fprintf(&content, "Session: %s\n", violation.SessionID)
	fmt.Fprintf(&content, "Agent type: %s\n\n", violation.AgentType)

	content.WriteString("## Violation Details\n\n")
	fmt.Fprintf(&content, "- **Command attempted**: `%s`\n", violation.Command)
	fmt.Fprintf(&content, "- **Pattern matched**: %s (%s)\n", violation.PatternID, violation.PatternType)
	fmt.Fprintf(&content, "- **Reason**: %s\n", pattern.Reason)
	fmt.Fprintf(&content, "- **Correct approach**: %s\n\n", pattern.Alternative)

	content.WriteString("## Why It Happened\n\n")
	content.WriteString("This violation indicates that:\n")
	content.WriteString("1. The agent bypassed or ignored Tier 1 instructions in .ai.md engrams\n")
	content.WriteString("2. The PreTool hook (Tier 2) did not catch the violation (may need updating)\n")
	content.WriteString("3. The violation persisted long enough to trigger monitoring\n\n")

	content.WriteString("## Recovery\n\n")
	fmt.Fprintf(&content, "- Reason: %s\n", pattern.Reason)
	fmt.Fprintf(&content, "- Alternative: %s\n\n", pattern.Alternative)

	content.WriteString("## Proposed Fix\n\n")
	content.WriteString("Review and strengthen Tier 1 instructions.\n")
	content.WriteString("Consider adding this pattern to PreTool hook validation if not already present.\n")

	return content.String()
}
