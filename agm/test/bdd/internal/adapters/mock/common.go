// Package mock provides mock functionality.
package mock

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// SessionContext holds conversation context for mock sessions
type SessionContext struct {
	// Attributes stores user-provided key-value pairs
	// Example: "name" → "Alice", "color" → "blue"
	Attributes map[string]string

	// Messages stores all user messages in order for recall
	// Example: ["message 1", "message 2", "recall first"]
	Messages []string
}

// Pattern represents a response generation pattern
type Pattern struct {
	Regex   *regexp.Regexp
	Handler func(session *Session, matches []string) string
}

// Patterns defines response generation rules (priority-ordered)
var Patterns = []Pattern{
	// Priority 1: Message recall "recall first"
	{
		Regex: regexp.MustCompile(`(?i)recall first`),
		Handler: func(s *Session, m []string) string {
			if len(s.Context.Messages) > 0 {
				return fmt.Sprintf("The first message was: %s", s.Context.Messages[0])
			}
			return "No messages yet."
		},
	},

	// Priority 2: Message recall "recall message N"
	{
		Regex: regexp.MustCompile(`(?i)recall message (\d+)`),
		Handler: func(s *Session, m []string) string {
			idx, _ := strconv.Atoi(m[1])
			if idx > 0 && idx <= len(s.Context.Messages) {
				return fmt.Sprintf("Message %d was: %s", idx, s.Context.Messages[idx-1])
			}
			return fmt.Sprintf("Message %d not found.", idx)
		},
	},

	// Priority 3: Attribute storage "My X is Y" (multi-word attributes)
	{
		Regex: regexp.MustCompile(`(?i)^[Mm]y ([\w\s]+) is (.+)$`),
		Handler: func(s *Session, m []string) string {
			key := strings.ToLower(strings.TrimSpace(m[1]))
			value := strings.TrimSpace(m[2])
			s.Context.Attributes[key] = value
			return fmt.Sprintf("Noted: your %s is %s.", key, value)
		},
	},

	// Priority 4: Attribute retrieval "What is my X?"
	{
		Regex: regexp.MustCompile(`(?i)^[Ww]hat is my ([\w\s]+)\??$`),
		Handler: func(s *Session, m []string) string {
			key := strings.ToLower(strings.TrimSpace(m[1]))
			if val, ok := s.Context.Attributes[key]; ok {
				return fmt.Sprintf("Your %s is %s.", key, val)
			}
			return fmt.Sprintf("I don't know your %s.", key)
		},
	},
}

// GenerateContextualResponse generates a response using pattern matching
// Returns (response, matched) where matched=true if a pattern was found
func GenerateContextualResponse(session *Session, message string) (string, bool) {
	// Try each pattern in priority order
	for _, pattern := range Patterns {
		if matches := pattern.Regex.FindStringSubmatch(message); matches != nil {
			return pattern.Handler(session, matches), true
		}
	}
	return "", false // No pattern matched
}
