package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultThreshold = 6000
	defaultSkipTools = "Read,Agent"
	summaryPrefix    = 200
)

// ResponseMasker is a PostToolUse hook that compresses large tool outputs.
// Full output is archived to disk; the context window gets a compact summary + pointer.
type ResponseMasker struct {
	sessionID  string
	toolName   string
	toolResult string
	threshold  int
	skipTools  []string
	archiveDir string
	debug      bool
}

// NewResponseMasker creates a ResponseMasker from environment variables.
func NewResponseMasker() *ResponseMasker {
	threshold := defaultThreshold
	if v := os.Getenv("AGM_RESPONSE_MASK_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			threshold = n
		}
	}

	skipRaw := defaultSkipTools
	if v := os.Getenv("AGM_RESPONSE_MASK_SKIP"); v != "" {
		skipRaw = v
	}
	var skipTools []string
	for _, s := range strings.Split(skipRaw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			skipTools = append(skipTools, s)
		}
	}

	sessionID := os.Getenv("CLAUDE_SESSION_ID")
	archiveDir := filepath.Join("/tmp", "agm", sessionID)

	return &ResponseMasker{
		sessionID:  sessionID,
		toolName:   os.Getenv("CLAUDE_TOOL_NAME"),
		toolResult: os.Getenv("CLAUDE_TOOL_RESULT"),
		threshold:  threshold,
		skipTools:  skipTools,
		archiveDir: archiveDir,
		debug:      os.Getenv("AGM_HOOK_DEBUG") == "1",
	}
}

func (m *ResponseMasker) log(level, message string) {
	if m.debug || level == "ERROR" {
		timestamp := time.Now().Format(time.RFC3339)
		fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", timestamp, level, message)
	}
}

// shouldSkip returns true if the tool is in the skip list.
func (m *ResponseMasker) shouldSkip() bool {
	for _, skip := range m.skipTools {
		if strings.EqualFold(m.toolName, skip) {
			return true
		}
	}
	return false
}

// shouldMask returns true if the tool result exceeds the threshold.
func (m *ResponseMasker) shouldMask() bool {
	return len(m.toolResult) > m.threshold
}

// nextArchiveN finds the next sequence number for archive files.
func (m *ResponseMasker) nextArchiveN() int {
	prefix := fmt.Sprintf("%s-%s-", m.sessionID, m.toolName)
	entries, err := os.ReadDir(m.archiveDir)
	if err != nil {
		return 0
	}
	highest := -1
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".txt") {
			numStr := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".txt")
			if n, err := strconv.Atoi(numStr); err == nil && n > highest {
				highest = n
			}
		}
	}
	return highest + 1
}

// archive writes the full tool result to disk and returns the file path.
func (m *ResponseMasker) archive() (string, error) {
	if err := os.MkdirAll(m.archiveDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", m.archiveDir, err)
	}

	n := m.nextArchiveN()
	filename := fmt.Sprintf("%s-%s-%d.txt", m.sessionID, m.toolName, n)
	path := filepath.Join(m.archiveDir, filename)

	if err := os.WriteFile(path, []byte(m.toolResult), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	return path, nil
}

// formatSummary builds the compact replacement string.
func (m *ResponseMasker) formatSummary(archivePath string) string {
	originalSize := len(m.toolResult)
	estimatedTokens := originalSize / 4

	preview := m.toolResult
	if len(preview) > summaryPrefix {
		preview = preview[:summaryPrefix]
	}
	// Strip trailing partial lines from preview
	if idx := strings.LastIndex(preview, "\n"); idx > 0 {
		preview = preview[:idx]
	}

	return fmt.Sprintf("[Archived: %s] Summary: %s... (%d chars, %d tokens)",
		archivePath, preview, originalSize, estimatedTokens)
}

// Run executes the hook logic.
func (m *ResponseMasker) Run() {
	m.log("INFO", fmt.Sprintf("Response masking hook started (tool=%s, result_len=%d)", m.toolName, len(m.toolResult)))

	if m.shouldSkip() {
		m.log("INFO", fmt.Sprintf("Skipping tool %s (in skip list)", m.toolName))
		return
	}

	if !m.shouldMask() {
		m.log("INFO", fmt.Sprintf("Below threshold (%d < %d), no masking", len(m.toolResult), m.threshold))
		return
	}

	archivePath, err := m.archive()
	if err != nil {
		m.log("ERROR", fmt.Sprintf("Archive failed: %v (failing open)", err))
		return // fail open
	}

	summary := m.formatSummary(archivePath)
	fmt.Print(summary)

	m.log("INFO", fmt.Sprintf("Masked %d chars -> %d char summary, archived to %s", len(m.toolResult), len(summary), archivePath))
}

func main() {
	masker := NewResponseMasker()
	masker.Run()
}
