package context

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DetectFromClaude detects context usage from Claude Code CLI using current session.
func (d *Detector) DetectFromClaude() (*Usage, error) {
	sessionID := os.Getenv("CLAUDE_SESSION_ID")
	if sessionID == "" {
		return nil, fmt.Errorf("CLAUDE_SESSION_ID not set")
	}

	return d.DetectFromClaudeSession(sessionID)
}

// DetectFromClaudeSession detects context usage from a specific Claude session ID.
func (d *Detector) DetectFromClaudeSession(sessionID string) (*Usage, error) {
	// Try to extract from environment first (PostToolUse hook context)
	toolResult := os.Getenv("CLAUDE_TOOL_RESULT")
	if toolResult != "" {
		usage, err := d.extractFromSystemReminder(toolResult, sessionID)
		if err == nil {
			return usage, nil
		}
	}

	// Fallback: read conversation file
	return d.extractFromConversationFile(sessionID)
}

// extractFromSystemReminder parses token usage from Claude Code system reminders.
//
// Pattern: <system-reminder>Token usage: 42184/200000; 157816 remaining</system-reminder>
func (d *Detector) extractFromSystemReminder(text string, sessionID string) (*Usage, error) {
	pattern := regexp.MustCompile(`Token usage: (\d+)/(\d+);`)
	matches := pattern.FindStringSubmatch(text)

	if len(matches) < 3 {
		return nil, fmt.Errorf("token usage pattern not found in text")
	}

	used, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse used tokens: %w", err)
	}

	total, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, fmt.Errorf("failed to parse total tokens: %w", err)
	}

	// Try to detect model ID from system reminder
	modelID := d.extractModelIDFromText(text)
	if modelID == "" {
		modelID = "claude-sonnet-4.5" // Default fallback
	}

	percentage := float64(used) / float64(total) * 100.0

	return &Usage{
		TotalTokens:    total,
		UsedTokens:     used,
		PercentageUsed: percentage,
		LastUpdated:    time.Now(),
		Source:         "claude-cli",
		ModelID:        modelID,
		SessionID:      sessionID,
	}, nil
}

// extractModelIDFromText attempts to extract model ID from system reminder text.
//
// Pattern: "You are powered by the model named X. The exact model ID is Y."
func (d *Detector) extractModelIDFromText(text string) string {
	// Look for exact model ID pattern
	pattern := regexp.MustCompile(`The exact model ID is ([a-z0-9-]+)`)
	matches := pattern.FindStringSubmatch(text)

	if len(matches) >= 2 {
		// Extract model ID (e.g., "claude-sonnet-4-5@20250929")
		modelID := matches[1]

		// Strip @version suffix if present
		if idx := strings.Index(modelID, "@"); idx >= 0 {
			modelID = modelID[:idx]
		}

		return modelID
	}

	// Fallback: look for model name mention
	if strings.Contains(strings.ToLower(text), "sonnet 4.5") {
		return "claude-sonnet-4.5"
	}
	if strings.Contains(strings.ToLower(text), "sonnet 4.6") {
		return "claude-sonnet-4.6"
	}
	if strings.Contains(strings.ToLower(text), "opus 4.6") {
		return "claude-opus-4.6"
	}
	if strings.Contains(strings.ToLower(text), "haiku 4.5") {
		return "claude-haiku-4.5"
	}

	return ""
}

// extractFromConversationFile reads the Claude Code conversation JSONL file.
func (d *Detector) extractFromConversationFile(sessionID string) (*Usage, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Claude Code conversation path
	conversationPath := filepath.Join(home, ".claude", "projects", "-home-user-src", sessionID+".jsonl")

	// Check if file exists
	if _, err := os.Stat(conversationPath); os.IsNotExist(err) {
		// Try alternative path (session-specific)
		conversationPath = filepath.Join(home, ".claude", "sessions", sessionID, "conversation.jsonl")
		if _, err := os.Stat(conversationPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("conversation file not found for session %s", sessionID)
		}
	}

	// Read file and parse last system reminder
	data, err := os.ReadFile(conversationPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read conversation file: %w", err)
	}

	// Extract from last system reminder in file
	return d.extractFromSystemReminder(string(data), sessionID)
}

// DetectFromHeuristic estimates token usage when actual data is unavailable.
func (d *Detector) DetectFromHeuristic() (*Usage, error) {
	// Default to Claude-level context (200K)
	maxTokens := 200000

	// Estimate based on typical session (assume 50 messages)
	estimatedMessages := 50

	usage := EstimateFromMessageCount(estimatedMessages, maxTokens)
	usage.Source = "heuristic"
	usage.ModelID = "default"

	return usage, nil
}
