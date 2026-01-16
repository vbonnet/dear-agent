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

// Message represents a single conversation message
type Message struct {
	Timestamp time.Time `json:"timestamp"`
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
}

// ConvertHTMLToJSONL converts HTML conversation transcripts to JSONL format
// Each line in JSONL is a JSON-encoded Message
// Falls back to copying HTML as-is if parsing fails
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
		// Create empty JSONL file
		return os.WriteFile(jsonlPath, []byte{}, 0600)
	}

	// Write JSONL
	jsonlFile, err := os.OpenFile(jsonlPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create JSONL: %w", err)
	}
	defer jsonlFile.Close()

	writer := bufio.NewWriter(jsonlFile)
	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			continue // Skip malformed message
		}
		writer.Write(data)
		writer.WriteString("\n")
	}

	return writer.Flush()
}

// extractMessages parses HTML document and extracts conversation messages
// This is a simplified implementation - production version would use
// internal/history/parser.go if available
func extractMessages(n *html.Node) []Message {
	var messages []Message

	// Simplified extraction: find text content in conversation structure
	// In production, this would parse specific HTML structure from AGM
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
					messages = append(messages, Message{
						Timestamp: time.Now(), // Placeholder - real version extracts from HTML
						Role:      role,
						Content:   content,
					})
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

// extractText recursively extracts text content from HTML node
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

// copyFileAsFallback copies HTML as-is when parsing fails
func copyFileAsFallback(src, dst string, parseErr error) error {
	fmt.Fprintf(os.Stderr, "Warning: HTML parsing failed (%v), copying as-is: %s\n", parseErr, src)

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0600)
}
