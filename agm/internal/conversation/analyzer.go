package conversation

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// CountMessages counts messages in conversation.jsonl
// Returns: message count, error
func CountMessages(conversationPath string) (int, error) {
	// Check if file exists
	if _, err := os.Stat(conversationPath); os.IsNotExist(err) {
		// File not found: Return 0, nil (assume empty)
		return 0, nil
	}

	// Open conversation.jsonl file
	file, err := os.Open(conversationPath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	// Scan line-by-line (each line is JSON message)
	scanner := bufio.NewScanner(file)
	count := 0

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Try to parse as JSON to validate
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Parse error: Skip line, continue
			continue
		}

		// Count valid JSON objects
		count++
	}

	// Check for I/O errors during scanning
	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return count, nil
}

// IsTrivialSession checks if session has few messages (likely test)
// threshold: minimum messages to consider non-trivial (default: 5)
// Returns: true if trivial (< threshold messages), false otherwise
func IsTrivialSession(sessionID string, threshold int) (bool, error) {
	// Build conversation path: $HOME/.claude/sessions/{sessionID}/conversation.jsonl
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}

	conversationPath := filepath.Join(homeDir, ".claude", "sessions", sessionID, "conversation.jsonl")

	// Call CountMessages
	count, err := CountMessages(conversationPath)
	if err != nil {
		return false, err
	}

	// Return count < threshold
	return count < threshold, nil
}
