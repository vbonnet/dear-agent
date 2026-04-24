package compaction

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// PromptInput holds data used to generate a compaction prompt.
type PromptInput struct {
	SessionName string
	Project     string
	Purpose     string
	Tags        []string
	Notes       string
	Harness     string
	FocusText   string // from --focus flag
}

// GeneratePrompt builds a /compact command string with structured preservation instructions.
func GeneratePrompt(input *PromptInput) string {
	var parts []string

	if input.SessionName != "" || input.Project != "" {
		parts = append(parts, fmt.Sprintf("Session: %s, Project: %s", input.SessionName, input.Project))
	}
	if input.Purpose != "" {
		parts = append(parts, fmt.Sprintf("Purpose: %s", input.Purpose))
	}
	if len(input.Tags) > 0 {
		parts = append(parts, fmt.Sprintf("Tags: %s", strings.Join(input.Tags, ", ")))
	}
	if input.Notes != "" {
		parts = append(parts, fmt.Sprintf("Notes: %s", input.Notes))
	}
	if input.FocusText != "" {
		parts = append(parts, fmt.Sprintf("Focus: %s", input.FocusText))
	}

	if len(parts) == 0 {
		return "/compact"
	}

	var sb strings.Builder
	sb.WriteString("/compact Preserve the following context during compaction:\n")
	for _, p := range parts {
		sb.WriteString("- ")
		sb.WriteString(p)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// promptDir returns the compaction-prompts directory under baseDir.
func promptDir(baseDir string) string {
	return filepath.Join(baseDir, "compaction-prompts")
}

// promptFilePattern matches "<session>-compact-<N>.md"
var promptFilePattern = regexp.MustCompile(`-compact-(\d+)\.md$`)

// NextPromptNumber scans compaction-prompts for the next available number.
func NextPromptNumber(baseDir, sessionName string) (int, error) {
	dir := promptDir(baseDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, fmt.Errorf("read prompt dir: %w", err)
	}

	prefix := sessionName + "-compact-"
	maxN := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		matches := promptFilePattern.FindStringSubmatch(e.Name())
		if len(matches) == 2 {
			n, _ := strconv.Atoi(matches[1])
			if n > maxN {
				maxN = n
			}
		}
	}
	return maxN + 1, nil
}

// SavePrompt writes the compaction prompt to an audit trail file.
func SavePrompt(baseDir, sessionName string, promptNumber int, content string) (string, error) {
	dir := promptDir(baseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create prompt dir: %w", err)
	}
	filename := fmt.Sprintf("%s-compact-%d.md", sessionName, promptNumber)
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write prompt file: %w", err)
	}
	return path, nil
}
