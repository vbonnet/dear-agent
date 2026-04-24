package scanners

import (
	"context"
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
)

// ConversationScanner detects frameworks/tools mentioned in recent conversation.
// CRITICAL: Runs UNCACHED (separated from WorkingDir-only cache key).
// Priority: 50 (highest, user intent is most important).
type ConversationScanner struct {
	name     string
	priority int
	patterns map[string]*regexp.Regexp
}

// NewConversationScanner creates a new ConversationScanner.
func NewConversationScanner() *ConversationScanner {
	// Framework/tool detection patterns
	patterns := map[string]*regexp.Regexp{
		"React":      regexp.MustCompile(`(?i)\breact\b`),
		"Vue":        regexp.MustCompile(`(?i)\bvue\b`),
		"Angular":    regexp.MustCompile(`(?i)\bangular\b`),
		"Next.js":    regexp.MustCompile(`(?i)\bnext\.?js\b`),
		"Django":     regexp.MustCompile(`(?i)\bdjango\b`),
		"Flask":      regexp.MustCompile(`(?i)\bflask\b`),
		"FastAPI":    regexp.MustCompile(`(?i)\bfastapi\b`),
		"Express":    regexp.MustCompile(`(?i)\bexpress\b`),
		"Docker":     regexp.MustCompile(`(?i)\bdocker\b`),
		"Kubernetes": regexp.MustCompile(`(?i)\b(kubernetes|k8s)\b`),
		"PostgreSQL": regexp.MustCompile(`(?i)\b(postgres|postgresql)\b`),
		"MongoDB":    regexp.MustCompile(`(?i)\bmongodb\b`),
		"Redis":      regexp.MustCompile(`(?i)\bredis\b`),
		"GraphQL":    regexp.MustCompile(`(?i)\bgraphql\b`),
		"gRPC":       regexp.MustCompile(`(?i)\bgrpc\b`),
	}

	return &ConversationScanner{
		name:     "conversation",
		priority: 50,
		patterns: patterns,
	}
}

func (s *ConversationScanner) Name() string {
	return s.name
}

func (s *ConversationScanner) Priority() int {
	return s.priority
}

// Scan analyzes recent conversation turns for framework/tool mentions.
// CRITICAL: This scanner runs SEPARATELY from cached scanners.
// Results are merged at runtime (Section 2.8 ConversationScanner Separation).
func (s *ConversationScanner) Scan(ctx context.Context, req *metacontext.AnalyzeRequest) ([]metacontext.Signal, error) {
	// Extract recent N turns (default 5)
	recentTurns := req.Conversation
	if len(recentTurns) > 5 {
		recentTurns = recentTurns[len(recentTurns)-5:]
	}

	// Concatenate recent turns
	conversationText := strings.Join(recentTurns, " ")

	signals := []metacontext.Signal{}

	// Match patterns, count occurrences
	for name, pattern := range s.patterns {
		matches := pattern.FindAllString(conversationText, -1)
		if len(matches) > 0 {
			// Confidence based on mention frequency (normalized to 0-1)
			confidence := float64(len(matches)) / 5.0
			if confidence > 1.0 {
				confidence = 1.0
			}

			signals = append(signals, metacontext.Signal{
				Name:       name,
				Confidence: confidence,
				Source:     "conversation",
				Metadata: map[string]string{
					"mentions": string(rune(len(matches))),
				},
			})
		}
	}

	return signals, nil
}
