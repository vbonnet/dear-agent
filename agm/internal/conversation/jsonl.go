// Package conversation provides conversation functionality.
package conversation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ParseJSONL reads a JSONL file and returns a Conversation.
// Format: first line is conversation header, subsequent lines are messages.
// Handles malformed lines gracefully (logs warning, skips line, continues).
func ParseJSONL(path string) (*Conversation, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Parse header (first line)
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty file")
	}

	var conv Conversation
	if err := json.Unmarshal(scanner.Bytes(), &conv); err != nil {
		return nil, fmt.Errorf("invalid header: %w", err)
	}

	// Validate schema version
	if conv.SchemaVersion != "1.0" {
		return nil, fmt.Errorf("unsupported schema version: %s (expected 1.0)", conv.SchemaVersion)
	}

	// Parse messages (subsequent lines)
	lineNum := 1
	for scanner.Scan() {
		lineNum++
		var msg Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			// Log warning, skip line, continue (graceful degradation)
			fmt.Fprintf(os.Stderr, "Warning: invalid message at line %d: %v\n", lineNum, err)
			continue
		}
		conv.Messages = append(conv.Messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan error: %w", err)
	}

	return &conv, nil
}

// WriteJSONL writes a Conversation to a JSONL file using atomic write pattern.
// Writes to temp file, then renames to prevent partial files on error.
func WriteJSONL(path string, conv *Conversation) error {
	// Create temp file for atomic write
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpPath) // Cleanup on error

	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)

	// Write header (first line)
	conv.TotalMessages = len(conv.Messages)
	if err := encoder.Encode(conv); err != nil {
		file.Close()
		return fmt.Errorf("encode header: %w", err)
	}

	// Write messages (subsequent lines)
	for i, msg := range conv.Messages {
		if err := encoder.Encode(msg); err != nil {
			file.Close()
			return fmt.Errorf("encode message %d: %w", i, err)
		}
	}

	if err := writer.Flush(); err != nil {
		file.Close()
		return fmt.Errorf("flush writer: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

// ConvertHTMLToJSONL converts HTML conversation transcripts to JSONL format.
// Enhanced version: creates Conversation header, extracts messages with agent field.
// Falls back to copying HTML as-is if parsing fails.
func ConvertHTMLToJSONL(htmlPath, jsonlPath string) error {
	// Read HTML file
	htmlFile, err := os.Open(htmlPath)
	if err != nil {
		return fmt.Errorf("failed to open HTML: %w", err)
	}
	defer htmlFile.Close()

	// Parse HTML
	doc, err := html.Parse(htmlFile)
	if err != nil {
		// Fallback: copy HTML as-is, log warning
		return copyFileAsFallback(htmlPath, jsonlPath, err)
	}

	// Extract messages from HTML
	messages := extractMessages(doc)
	if len(messages) == 0 {
		// Create empty JSONL file with header only
		conv := &Conversation{
			SchemaVersion: "1.0",
			CreatedAt:     time.Now(),
			Model:         "unknown",
			Harness:       "claude-code",
			Messages:      []Message{},
		}
		return WriteJSONL(jsonlPath, conv)
	}

	// Create Conversation with header
	conv := &Conversation{
		SchemaVersion: "1.0",
		CreatedAt:     time.Now(), // Use current time if no timestamp in HTML
		Model:         "claude",   // Assume Claude HTML export
		Harness:       "claude-code",
		Messages:      messages,
	}

	// Write using JSONL serializer
	return WriteJSONL(jsonlPath, conv)
}

// extractMessages parses HTML document and extracts conversation messages.
// Enhanced version: creates Message with Content blocks (TextBlock).
func extractMessages(n *html.Node) []Message {
	var messages []Message

	// Simplified extraction: find text content in conversation structure
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			// Look for role attribute or class
			var role string
			var content string

			for _, attr := range n.Attr {
				if attr.Key == "class" {
					if strings.Contains(attr.Val, "user-message") {
						role = "user"
					} else if strings.Contains(attr.Val, "assistant-message") {
						role = "assistant"
					}
				}
			}

			if role != "" {
				content = extractText(n)
				if content != "" {
					// Create message with TextBlock content
					msg := Message{
						Timestamp: time.Now(), // TODO: Extract from HTML if available
						Role:      role,
						Harness:   "claude-code",
						Content: []ContentBlock{
							TextBlock{
								Type: "text",
								Text: strings.TrimSpace(content),
							},
						},
					}
					messages = append(messages, msg)
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}

	extract(n)
	return messages
}

// extractText recursively extracts text content from HTML node.
func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}

	var text string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text += extractText(c)
	}
	return text
}

// copyFileAsFallback copies HTML as-is when parsing fails.
func copyFileAsFallback(src, dst string, parseErr error) error {
	fmt.Fprintf(os.Stderr, "Warning: HTML parsing failed (%v), copying as-is: %s\n", parseErr, src)

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0600)
}
