// Package memory provides memory-related functionality.
package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EpisodicMemory represents the "Narrative Arc" of a project session.
// It maintains DECISION_LOG.md as the long-term memory of the agent.
type EpisodicMemory struct {
	projectRoot   string
	logPath       string
	currentTokens int
	maxTokens     int
	mu            sync.RWMutex
}

// MemoryEntry represents a single decision or event in the episodic log.
type MemoryEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Session   string            `json:"session"` // Session ID or name
	Event     string            `json:"event"`   // "decision", "error", "learning"
	Summary   string            `json:"summary"` // Brief one-liner
	Details   string            `json:"details"` // Full context
	Tokens    int               `json:"tokens"`  // Estimated token count
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NewEpisodicMemory creates a new episodic memory service.
// maxTokens defines the threshold for triggering "Molt" behavior (default: 80% of context window).
func NewEpisodicMemory(projectRoot string, maxTokens int) (*EpisodicMemory, error) {
	logPath := filepath.Join(projectRoot, "DECISION_LOG.md")

	// Create DECISION_LOG.md if it doesn't exist
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		if err := initializeDecisionLog(logPath); err != nil {
			return nil, fmt.Errorf("failed to initialize DECISION_LOG.md: %w", err)
		}
	}

	return &EpisodicMemory{
		projectRoot:   projectRoot,
		logPath:       logPath,
		currentTokens: 0,
		maxTokens:     maxTokens,
	}, nil
}

// AppendEntry adds a new memory entry to DECISION_LOG.md.
// This is the core "Molt" operation - the agent updates its own long-term memory.
func (em *EpisodicMemory) AppendEntry(ctx context.Context, entry *MemoryEntry) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Format entry as markdown
	entryText := formatMemoryEntry(entry)

	// Append to DECISION_LOG.md
	f, err := os.OpenFile(em.logPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open DECISION_LOG.md: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(entryText); err != nil {
		return fmt.Errorf("failed to write entry: %w", err)
	}

	// Update token count
	em.currentTokens += entry.Tokens

	return nil
}

// ShouldMolt checks if current session has exceeded token threshold (80%).
// When true, agent should append a session summary to DECISION_LOG.md.
func (em *EpisodicMemory) ShouldMolt(currentSessionTokens int) bool {
	em.mu.RLock()
	defer em.mu.RUnlock()

	threshold := int(float64(em.maxTokens) * 0.8)
	return currentSessionTokens >= threshold
}

// GetTokenUsage returns current token count and threshold.
func (em *EpisodicMemory) GetTokenUsage() (current int, max int, percentage float64) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	percentage = float64(em.currentTokens) / float64(em.maxTokens) * 100
	return em.currentTokens, em.maxTokens, percentage
}

// MoltSession creates a session summary and appends it to DECISION_LOG.md.
// This is called when token threshold is exceeded or session ends.
func (em *EpisodicMemory) MoltSession(ctx context.Context, sessionID string, summary string, details string) error {
	entry := &MemoryEntry{
		Timestamp: time.Now(),
		Session:   sessionID,
		Event:     "molt",
		Summary:   summary,
		Details:   details,
		Tokens:    estimateTokens(summary + details),
		Metadata: map[string]string{
			"trigger": "token_threshold",
		},
	}

	return em.AppendEntry(ctx, entry)
}

// RecordDecision logs a decision made during the session.
func (em *EpisodicMemory) RecordDecision(ctx context.Context, sessionID string, decision string, rationale string) error {
	entry := &MemoryEntry{
		Timestamp: time.Now(),
		Session:   sessionID,
		Event:     "decision",
		Summary:   decision,
		Details:   rationale,
		Tokens:    estimateTokens(decision + rationale),
	}

	return em.AppendEntry(ctx, entry)
}

// RecordError logs an error or failure for future reference.
func (em *EpisodicMemory) RecordError(ctx context.Context, sessionID string, errorSummary string, resolution string) error {
	entry := &MemoryEntry{
		Timestamp: time.Now(),
		Session:   sessionID,
		Event:     "error",
		Summary:   errorSummary,
		Details:   resolution,
		Tokens:    estimateTokens(errorSummary + resolution),
	}

	return em.AppendEntry(ctx, entry)
}

// formatMemoryEntry converts a MemoryEntry to markdown format.
func formatMemoryEntry(entry *MemoryEntry) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "\n## %s - %s\n\n",
		entry.Timestamp.Format("2006-01-02 15:04:05"),
		entry.Event)

	fmt.Fprintf(&sb, "**Session**: %s\n\n", entry.Session)
	fmt.Fprintf(&sb, "**Summary**: %s\n\n", entry.Summary)

	if entry.Details != "" {
		fmt.Fprintf(&sb, "**Details**:\n%s\n\n", entry.Details)
	}

	if len(entry.Metadata) > 0 {
		sb.WriteString("**Metadata**:\n")
		for k, v := range entry.Metadata {
			fmt.Fprintf(&sb, "- %s: %s\n", k, v)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("---\n\n")

	return sb.String()
}

// initializeDecisionLog creates a new DECISION_LOG.md with template content.
func initializeDecisionLog(path string) error {
	template := `# Decision Log

This file contains the "Narrative Arc" of the project - a chronological record of decisions, learnings, and key events maintained by the agent.

## Purpose

The Decision Log serves as episodic memory, allowing the agent to:
- Remember past decisions and their rationale
- Learn from previous errors and avoid repeating them
- Maintain context across sessions
- Build a coherent narrative of the project's evolution

## Molt Behavior

When the agent's token usage exceeds 80% of the context window, it performs a "Molt" - appending a session summary to this log and continuing with fresh context.

---

`

	return os.WriteFile(path, []byte(template), 0o600)
}

// estimateTokens provides rough token count (4 chars = 1 token heuristic).
func estimateTokens(text string) int {
	return len(text) / 4
}
