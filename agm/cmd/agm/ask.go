package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

var (
	askContext string // --context flag for additional context
)

var askCmd = &cobra.Command{
	Use:   "ask <session> <question>",
	Short: "Send a question to the human operator",
	Long: `Send a question to the human operator for a given session.

The question is written to ~/.agm/questions/{session}-{timestamp}.md
so the human can review and answer it.

The human can answer by:
  1. Running: agm answer <session> "response text"
  2. Editing the question file directly (setting status to "answered")

Question file format:
  - YAML frontmatter with session, timestamp, status, sender
  - Markdown body with the question text and optional context

Examples:
  # Ask a simple question
  agm ask my-session "Should I use the v2 or v3 API?"

  # Ask with additional context
  agm ask my-session "Should I refactor this?" --context "The function is 200 lines"

  # Answer a pending question
  agm answer my-session "Use the v3 API, it has better error handling"`,
	Args: cobra.ExactArgs(2),
	RunE: runAsk,
}

func init() {
	askCmd.Flags().StringVar(
		&askContext,
		"context",
		"",
		"Additional context for the question",
	)

	rootCmd.AddCommand(askCmd)
}

// Question represents a pending question from an AI session to the human.
type Question struct {
	Session   string
	Sender    string
	Timestamp time.Time
	Text      string
	Context   string
	Status    string // "pending", "answered"
}

// QuestionsDir returns the path to the questions directory.
func QuestionsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".agm", "questions"), nil
}

// QuestionFilename returns the filename for a question.
func QuestionFilename(sessionName string, ts time.Time) string {
	return fmt.Sprintf("%s-%d.md", sessionName, ts.UnixMilli())
}

// WriteQuestion writes a question file to the questions directory.
func WriteQuestion(q *Question) (string, error) {
	dir, err := QuestionsDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create questions directory: %w", err)
	}

	filename := QuestionFilename(q.Session, q.Timestamp)
	filePath := filepath.Join(dir, filename)

	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "session: %s\n", q.Session)
	fmt.Fprintf(&sb, "sender: %s\n", q.Sender)
	fmt.Fprintf(&sb, "timestamp: %s\n", q.Timestamp.UTC().Format(time.RFC3339))
	sb.WriteString("status: pending\n")
	sb.WriteString("---\n\n")
	sb.WriteString("## Question\n\n")
	sb.WriteString(q.Text)
	sb.WriteString("\n")

	if q.Context != "" {
		sb.WriteString("\n## Context\n\n")
		sb.WriteString(q.Context)
		sb.WriteString("\n")
	}

	if err := os.WriteFile(filePath, []byte(sb.String()), 0o600); err != nil {
		return "", fmt.Errorf("failed to write question file: %w", err)
	}

	return filePath, nil
}

// FindPendingQuestion finds the most recent pending question for a session.
func FindPendingQuestion(sessionName string) (string, *Question, error) {
	dir, err := QuestionsDir()
	if err != nil {
		return "", nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, fmt.Errorf("no questions directory found")
		}
		return "", nil, fmt.Errorf("failed to read questions directory: %w", err)
	}

	// Find matching files for this session, pick the most recent
	prefix := sessionName + "-"
	var latestPath string
	var latestTime int64

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".md") {
			continue
		}

		// Read and check status
		filePath := filepath.Join(dir, name)
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		contentStr := string(content)
		if !strings.Contains(contentStr, "status: pending") {
			continue
		}

		// Extract timestamp from filename
		tsStr := strings.TrimPrefix(name, prefix)
		tsStr = strings.TrimSuffix(tsStr, ".md")
		var ts int64
		if _, err := fmt.Sscanf(tsStr, "%d", &ts); err != nil {
			continue
		}

		if ts > latestTime {
			latestTime = ts
			latestPath = filePath
		}
	}

	if latestPath == "" {
		return "", nil, fmt.Errorf("no pending questions found for session '%s'", sessionName)
	}

	// Parse the question file
	content, err := os.ReadFile(latestPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read question file: %w", err)
	}

	q, err := ParseQuestion(string(content))
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse question file: %w", err)
	}

	return latestPath, q, nil
}

// ParseQuestion parses a question file's content into a Question struct.
func ParseQuestion(content string) (*Question, error) {
	q := &Question{}

	// Split frontmatter from body
	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid question format: missing frontmatter")
	}

	frontmatter := parts[1]
	body := parts[2]

	// Parse frontmatter fields
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		switch key {
		case "session":
			q.Session = value
		case "sender":
			q.Sender = value
		case "timestamp":
			ts, err := time.Parse(time.RFC3339, value)
			if err == nil {
				q.Timestamp = ts
			}
		case "status":
			q.Status = value
		}
	}

	// Extract question text from body
	if idx := strings.Index(body, "## Question\n\n"); idx >= 0 {
		rest := body[idx+len("## Question\n\n"):]
		// Question text ends at next section or end of file
		if ctxIdx := strings.Index(rest, "\n## Context\n\n"); ctxIdx >= 0 {
			q.Text = strings.TrimSpace(rest[:ctxIdx])
			q.Context = strings.TrimSpace(rest[ctxIdx+len("\n## Context\n\n"):])
		} else {
			q.Text = strings.TrimSpace(rest)
		}
	}

	return q, nil
}

// MarkQuestionAnswered updates the status in a question file from "pending" to "answered".
// filePath is always produced by FindPendingQuestion which constrains it to ~/.agm/questions/.
func MarkQuestionAnswered(filePath, answer string) error {
	cleanPath := filepath.Clean(filePath)

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to read question file: %w", err)
	}

	contentStr := string(content)
	contentStr = strings.Replace(contentStr, "status: pending", "status: answered", 1)

	// Append answer to the file
	contentStr += "\n## Answer\n\n" + answer + "\n"

	//nolint:gosec // G703: filePath is produced by FindPendingQuestion, constrained to ~/.agm/questions/
	if err := os.WriteFile(cleanPath, []byte(contentStr), 0o600); err != nil {
		return fmt.Errorf("failed to update question file: %w", err)
	}

	return nil
}

func runAsk(_ *cobra.Command, args []string) error {
	sessionName := args[0]
	questionText := args[1]

	// Determine sender (auto-detect from current AGM session)
	adapter, _ := getStorage()
	if adapter != nil {
		defer adapter.Close()
	}

	senderName := "unknown"
	if detected, err := session.GetCurrentSessionName(cfg.SessionsDir, adapter); err == nil {
		senderName = detected
	}

	now := time.Now()
	q := &Question{
		Session:   sessionName,
		Sender:    senderName,
		Timestamp: now,
		Text:      questionText,
		Context:   askContext,
		Status:    "pending",
	}

	filePath, err := WriteQuestion(q)
	if err != nil {
		return err
	}

	fmt.Printf("? Question sent to human for session '%s'\n", sessionName)
	fmt.Printf("  File: %s\n", filePath)
	fmt.Printf("  Answer with: agm answer %s \"your response\"\n", sessionName)

	return nil
}
