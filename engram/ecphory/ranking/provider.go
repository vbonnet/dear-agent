package ranking

import "context"

// Provider abstracts AI provider for semantic ranking
type Provider interface {
	// Name returns provider identifier (e.g., "anthropic", "vertexai-gemini")
	Name() string

	// Model returns model identifier (e.g., "claude-3-5-haiku-20241022")
	Model() string

	// Rank performs semantic ranking of candidates against query
	// Returns scores in range [0.0, 1.0] with reasoning
	Rank(ctx context.Context, query string, candidates []Candidate) ([]RankedResult, error)

	// Capabilities returns provider capabilities
	Capabilities() Capabilities
}

// Candidate represents an engram to be ranked
type Candidate struct {
	Name        string         // Engram name (e.g., "oauth-pattern")
	Description string         // T0 or T1 content for ranking
	Frontmatter map[string]any // Parsed YAML frontmatter
	Tags        []string       // For local provider matching
}

// RankedResult contains ranking score and reasoning
type RankedResult struct {
	Candidate Candidate
	Score     float64 // 0.0 to 1.0 (higher = more relevant)
	Reasoning string  // Provider's reasoning (if available)
}

// Capabilities describes provider features
type Capabilities struct {
	SupportsCaching          bool // Prompt caching available
	SupportsStructuredOutput bool // JSON mode available
	MaxConcurrentRequests    int  // Rate limit
	MaxTokensPerRequest      int  // Context window
}
