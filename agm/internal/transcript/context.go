package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Message represents a single conversation exchange
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Context represents extracted transcript context
type Context struct {
	Messages []Message
	UUID     string
}

// ExtractContext reads the HTML transcript and extracts the last N message exchanges
// For v1, this uses simple regex parsing of HTML. Future versions could use proper HTML parser.
func ExtractContext(projectRoot string, uuid string, numExchanges int) (*Context, error) {
	// Find transcript HTML file in orphan branch
	transcriptPath := filepath.Join(projectRoot, "transcripts", uuid, "index.html")

	// Check if transcript exists
	if _, err := os.Stat(transcriptPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("transcript not found: %s", transcriptPath)
	}

	// Read HTML file
	htmlBytes, err := os.ReadFile(transcriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript: %w", err)
	}

	html := string(htmlBytes)

	// Extract messages using regex (simple approach for v1)
	messages := extractMessages(html, numExchanges*2) // *2 for user+assistant pairs

	return &Context{
		Messages: messages,
		UUID:     uuid,
	}, nil
}

// extractMessages parses HTML to extract message blocks
// Uses simple regex - not production-grade, but sufficient for v1
func extractMessages(html string, maxMessages int) []Message {
	var messages []Message

	// Match message divs with class "user" or "assistant"
	// Pattern: <div class="message (user|assistant)">...</div>
	messageRegex := regexp.MustCompile(`(?s)<div class="message (user|assistant)"[^>]*>.*?<div class="message-content">(.*?)</div>`)

	matches := messageRegex.FindAllStringSubmatch(html, -1)

	// Take last N messages
	startIdx := 0
	if len(matches) > maxMessages {
		startIdx = len(matches) - maxMessages
	}

	for _, match := range matches[startIdx:] {
		if len(match) < 3 {
			continue
		}

		role := match[1]
		content := match[2]

		// Clean HTML tags from content (simple strip)
		content = stripHTML(content)
		content = strings.TrimSpace(content)

		// Truncate long messages (500 chars as per spec)
		if len(content) > 500 {
			content = content[:500] + "..."
		}

		messages = append(messages, Message{
			Role:    role,
			Content: content,
		})
	}

	return messages
}

// stripHTML removes HTML tags from content (simple version)
func stripHTML(html string) string {
	// Remove <script> and <style> tags and their content
	html = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`).ReplaceAllString(html, "")

	// Remove HTML tags
	html = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(html, "")

	// Decode common HTML entities
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")

	// Collapse multiple whitespace
	html = regexp.MustCompile(`\s+`).ReplaceAllString(html, " ")

	return html
}

// FormatForDisplay formats context messages for terminal display (boxed UI)
func (c *Context) FormatForDisplay() string {
	var sb strings.Builder

	sb.WriteString("┌─────────────────────────────────────────────────────────────────┐\n")
	sb.WriteString("│ 📝 Transcript Context (Last Session)                           │\n")
	sb.WriteString("├─────────────────────────────────────────────────────────────────┤\n")

	for i, msg := range c.Messages {
		icon := "👤"
		if msg.Role == "assistant" {
			icon = "🤖"
		}

		fmt.Fprintf(&sb, "│ %s %s:\n", icon, capitalizeFirst(msg.Role))

		// Word wrap content to fit in box (60 chars wide)
		wrapped := wordWrap(msg.Content, 58)
		for _, line := range strings.Split(wrapped, "\n") {
			fmt.Fprintf(&sb, "│   %s\n", line)
		}

		if i < len(c.Messages)-1 {
			sb.WriteString("│                                                                 │\n")
		}
	}

	sb.WriteString("└─────────────────────────────────────────────────────────────────┘\n")

	return sb.String()
}

// wordWrap wraps text to specified width
func wordWrap(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	var currentLine string

	for _, word := range words {
		if len(currentLine) == 0 {
			currentLine = word
		} else if len(currentLine)+1+len(word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}

	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}

	return strings.Join(lines, "\n")
}

// capitalizeFirst returns s with its first byte upper-cased.
// Roles ("user", "assistant") are ASCII, so this is sufficient.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
