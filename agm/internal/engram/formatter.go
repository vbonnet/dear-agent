// Package engram provides engram functionality.
package engram

import (
	"fmt"
	"strings"
)

const MaxContentLength = 2000

// FormatSystemMessage formats engram results as system message for Claude
func FormatSystemMessage(results []EngramResult) string {
	if len(results) == 0 {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("<system>\n")
	buf.WriteString("The following context from Engram may be relevant:\n\n")

	for _, r := range results {
		fmt.Fprintf(&buf, "<engram id=\"%s\" score=\"%.2f\" tags=\"%s\">\n",
			extractID(r.Hash), r.Score, strings.Join(r.Tags, ","))
		fmt.Fprintf(&buf, "%s\n\n", r.Title)

		content := truncateContent(r.Content, MaxContentLength)
		fmt.Fprintf(&buf, "%s\n", content)
		buf.WriteString("</engram>\n\n")
	}

	buf.WriteString("Note: This context was automatically loaded. Use it as reference.\n")
	buf.WriteString("</system>")

	return buf.String()
}

// truncateContent truncates content at maxLength with marker
func truncateContent(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}
	return content[:maxLength] + "\n... [truncated]"
}

// extractID extracts short ID from hash (first 8 chars after "sha256:")
func extractID(hash string) string {
	if len(hash) > 15 {
		return hash[7:15] // Skip "sha256:" prefix
	}
	return hash
}
