package engram

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/engram/retrieval"
)

// EngramResult represents a single engram result from retrieval
type EngramResult struct {
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	Score   float64  `json:"score"`
	Tags    []string `json:"tags"`
	Content string   `json:"content"`
	Hash    string   `json:"hash"`
}

// Client provides interface for Engram CLI interaction
type Client interface {
	Query(query string, tags []string) ([]EngramResult, error)
	IsAvailable() bool
}

type libClient struct {
	config  EngramConfig
	service *retrieval.Service
}

// NewClient creates a new Engram library client
func NewClient(cfg EngramConfig) Client {
	return &libClient{
		config:  cfg,
		service: retrieval.NewService(),
	}
}

// IsAvailable checks if Engram retrieval is available
// With library integration, this always returns true
// (no binary dependency required)
func (c *libClient) IsAvailable() bool {
	return true // Library always available
}

// Query executes Engram retrieval with given query and tags
func (c *libClient) Query(query string, tags []string) ([]EngramResult, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()

	// Generate session ID for telemetry
	sessionID := uuid.New().String()

	// Build search options
	opts := retrieval.SearchOptions{
		EngramPath: c.resolveEngramPath(),
		Query:      query,
		SessionID:  sessionID,
		Transcript: query, // V1: use query as transcript
		Tags:       tags,
		Limit:      c.config.Limit,
		UseAPI:     true, // Enable API ranking for better results
	}

	// Perform search
	results, err := c.service.Search(ctx, opts)
	if err != nil {
		return []EngramResult{}, fmt.Errorf("engram search failed: %w", err)
	}

	// Convert retrieval.SearchResult → EngramResult
	engramResults := make([]EngramResult, 0, len(results))
	for _, r := range results {
		// Compute content hash (SHA-256)
		hasher := sha256.New()
		hasher.Write([]byte(r.Engram.Content))
		contentHash := fmt.Sprintf("%x", hasher.Sum(nil))

		result := EngramResult{
			Path:    r.Path,
			Title:   r.Engram.Frontmatter.Title,
			Score:   r.Score,
			Tags:    r.Engram.Frontmatter.Tags,
			Content: r.Engram.Content,
			Hash:    "sha256:" + contentHash,
		}

		engramResults = append(engramResults, result)
	}

	// Filter by score threshold
	filtered := filterByScore(engramResults, c.config.ScoreThreshold)
	return filtered, nil
}

// resolveEngramPath determines the engrams directory path
func (c *libClient) resolveEngramPath() string {
	// Use BinaryPath as EngramPath (config field name is legacy from subprocess version)
	if c.config.BinaryPath != "" {
		return c.config.BinaryPath
	}

	// Default: let retrieval.Service resolve path
	// (tries ~/.engram/core/engrams, then relative path)
	return "engrams"
}

// filterByScore filters results by minimum score threshold
func filterByScore(results []EngramResult, threshold float64) []EngramResult {
	filtered := []EngramResult{}
	for _, r := range results {
		if r.Score >= threshold {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
