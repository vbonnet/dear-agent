// Package retrieval provides a high-level service for searching and retrieving
// engrams, wrapping the ecphory retrieval system for CLI and API use.
//
// This package offers a simplified interface for engram search operations,
// handling both basic file-based retrieval and advanced semantic search through
// the ecphory 3-tier pipeline.
//
// Search capabilities:
//   - Query: Natural language search query
//   - Tags: Filter by hierarchical tags (OR logic)
//   - Type: Filter by engram type (pattern, strategy, workflow)
//   - API ranking: Optional semantic ranking via Anthropic API
//   - Limit: Maximum number of results
//
// Example usage:
//
//	service := retrieval.NewService()
//	results, err := service.Search(ctx, retrieval.SearchOptions{
//	    EngramPath: "/path/to/engrams",
//	    Query:      "error handling",
//	    Tags:       []string{"go"},
//	    UseAPI:     true,
//	    Limit:      5,
//	})
//
//	for _, result := range results {
//	    fmt.Printf("%s (score: %.2f)\n", result.Engram.Frontmatter.Title, result.Score)
//	}
//
// The service is used by:
//   - CLI commands: engram retrieve
//   - API server: /api/search endpoint
//   - Plugins: Search for relevant patterns
package retrieval

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/engram/ecphory"
	"github.com/vbonnet/dear-agent/internal/tracking"
	"github.com/vbonnet/dear-agent/pkg/engram"
)

// SearchOptions configures the search behavior
type SearchOptions struct {
	EngramPath string   // Path to engrams directory
	Query      string   // Search query
	SessionID  string   // Session identifier for telemetry tracking
	Transcript string   // Conversation transcript or context
	Tags       []string // Filter by tags (OR)
	Type       string   // Filter by type (pattern, strategy, etc.)
	Limit      int      // Maximum results to return
	UseAPI     bool     // Whether to use API ranking (requires ANTHROPIC_API_KEY)
}

// SearchResult represents a single search result
type SearchResult struct {
	Path    string         // Full path to engram file
	Engram  *engram.Engram // Parsed engram
	Score   float64        // Relevance score (if API ranking used)
	Ranking string         // Reasoning (if API ranking used)
}

// Service handles engram retrieval
type Service struct {
	parser  *engram.Parser
	tracker *tracking.Tracker
}

// NewService creates a new retrieval service
func NewService() *Service {
	updater := tracking.NewMetadataUpdater()
	tracker := tracking.NewTracker(updater)

	return &Service{
		parser:  engram.NewParser(),
		tracker: tracker,
	}
}

// Search performs engram retrieval with optional AI ranking
func (s *Service) Search(ctx context.Context, opts SearchOptions) ([]*SearchResult, error) {
	// Resolve engram path
	engramPath, err := s.resolveEngramPath(opts.EngramPath)
	if err != nil {
		return nil, err
	}

	// Build index
	index := ecphory.NewIndex()
	if err := index.Build(engramPath); err != nil {
		return nil, fmt.Errorf("failed to build index: %w", err)
	}

	// Tier 1: Fast filter
	candidates := s.filterCandidates(index, opts)
	if len(candidates) == 0 {
		return nil, nil // No results
	}

	// Tier 2: API ranking (optional)
	var resultPaths []string
	var rankings map[string]ecphory.RankingResult

	if opts.UseAPI {
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			// Fallback to index-only
			resultPaths = s.limitResults(candidates, opts.Limit)
		} else {
			// Use API ranking
			ranker, err := ecphory.NewRanker()
			if err != nil {
				return nil, fmt.Errorf("failed to create ranker: %w", err)
			}

			ranked, err := ranker.Rank(ctx, opts.Query, candidates)
			if err != nil {
				return nil, fmt.Errorf("failed to rank candidates: %w", err)
			}

			// Build rankings map and extract paths
			rankings = make(map[string]ecphory.RankingResult)
			limit := opts.Limit
			if limit > len(ranked) {
				limit = len(ranked)
			}

			for i := 0; i < limit; i++ {
				path := ranked[i].Path
				resultPaths = append(resultPaths, path)
				rankings[path] = ranked[i]
			}
		}
	} else {
		// Index-only search
		resultPaths = s.limitResults(candidates, opts.Limit)
	}

	// Load engrams and build results
	var results []*SearchResult
	for _, path := range resultPaths {
		eg, err := s.parser.Parse(path)
		if err != nil {
			// Skip unparseable engrams
			continue
		}

		result := &SearchResult{
			Path:   path,
			Engram: eg,
		}

		// Add ranking info if available
		if ranking, ok := rankings[path]; ok {
			result.Score = ranking.Relevance
			result.Ranking = ranking.Reasoning
		}

		results = append(results, result)

		// Track access for this engram
		s.tracker.RecordAccess(path, time.Now())
	}

	return results, nil
}

// Close flushes pending tracking updates and cleans up resources.
// Should be called when the service is no longer needed.
func (s *Service) Close() error {
	if err := s.tracker.Flush(); err != nil {
		log.Printf("retrieval: failed to flush tracking updates: %v", err)
		// Don't return error - this is best-effort
	}
	return nil
}

// resolveEngramPath resolves the engram directory path
func (s *Service) resolveEngramPath(path string) (string, error) {
	// If absolute, use as-is
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return "", fmt.Errorf("engrams directory not found: %s", path)
		}
		return path, nil
	}

	// Try ~/.engram/core/engrams first
	home, err := os.UserHomeDir()
	if err == nil {
		defaultPath := filepath.Join(home, ".engram/core/engrams")
		if _, err := os.Stat(defaultPath); err == nil {
			return defaultPath, nil
		}
	}

	// Try relative to cwd
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	fullPath := filepath.Join(cwd, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("engrams directory not found: %s", fullPath)
	}

	return fullPath, nil
}

// filterCandidates applies tag/type filters to get candidates
func (s *Service) filterCandidates(index *ecphory.Index, opts SearchOptions) []string {
	// Filter by tags (if specified)
	if len(opts.Tags) > 0 {
		return index.FilterByTags(opts.Tags)
	}

	// Filter by type (if specified)
	if opts.Type != "" {
		return index.FilterByType(opts.Type)
	}

	// No filters - return all
	return index.All()
}

// limitResults limits candidates to specified number
func (s *Service) limitResults(candidates []string, limit int) []string {
	if limit <= 0 || limit >= len(candidates) {
		return candidates
	}
	return candidates[:limit]
}
