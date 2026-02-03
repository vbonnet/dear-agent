package gemini

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TopicsResponse represents the JSON schema returned by Gemini
// Schema: {"topics": ["topic1", "topic2", "topic3"]}
type TopicsResponse struct {
	Topics []string `json:"topics"`
}

// StripMarkdownFences removes markdown code fences from JSON responses
// Handles various fence formats:
//   - ```json ... ```
//   - ``` ... ```
//   - ```JSON ... ```
//
// This is necessary because Gemini CLI may wrap JSON in markdown fences.
func StripMarkdownFences(response string) string {
	// Remove leading/trailing whitespace
	response = strings.TrimSpace(response)

	// First pass: check if there are any fences
	hasFence := strings.Contains(response, "```")

	if !hasFence {
		// No fences, return as-is
		return response
	}

	// Second pass: extract content from inside fences
	lines := strings.Split(response, "\n")
	var contentLines []string
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect fence start (```json, ```JSON, ```)
		if strings.HasPrefix(trimmed, "```") && !inFence {
			inFence = true
			continue
		}

		// Detect fence end (```)
		if strings.HasPrefix(trimmed, "```") && inFence {
			inFence = false
			continue
		}

		// Include content lines only if we're inside a fence
		if inFence {
			contentLines = append(contentLines, line)
		}
	}

	return strings.TrimSpace(strings.Join(contentLines, "\n"))
}

// ParseTopicsJSON parses the JSON response and validates the schema
func ParseTopicsJSON(rawJSON string) ([]string, error) {
	// Strip markdown fences first
	cleanJSON := StripMarkdownFences(rawJSON)

	// Parse JSON
	var response TopicsResponse
	if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
		return nil, NewInvalidJSONError(fmt.Sprintf("parse error: %v (raw: %s)", err, cleanJSON[:min(100, len(cleanJSON))]))
	}

	// Validate topics array exists and is not empty
	if response.Topics == nil {
		return nil, NewInvalidJSONError("missing 'topics' key in JSON response")
	}

	if len(response.Topics) == 0 {
		return nil, NewNoTopicsError()
	}

	// Validate each topic is non-empty string
	validTopics := make([]string, 0, len(response.Topics))
	for _, topic := range response.Topics {
		trimmed := strings.TrimSpace(topic)
		if trimmed == "" {
			continue // Skip empty topics
		}
		validTopics = append(validTopics, trimmed)
	}

	// Check if all topics were empty
	if len(validTopics) == 0 {
		return nil, NewNoTopicsError()
	}

	return validTopics, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
