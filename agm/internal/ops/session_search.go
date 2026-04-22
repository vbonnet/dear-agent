package ops

import (
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// SearchSessionsRequest defines input for searching sessions by name.
type SearchSessionsRequest struct {
	// Query is the search string (case-insensitive, matches against session name).
	Query string `json:"query"`

	// Status filters by lifecycle: "active" (default), "archived", "all".
	Status string `json:"status,omitempty"`

	// Limit caps results (default: 10, max: 50).
	Limit int `json:"limit,omitempty"`
}

// SearchMatch is a session that matched the search query.
type SearchMatch struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
	Harness        string  `json:"harness"`
	Project        string  `json:"project"`
	RelevanceScore float64 `json:"relevance_score"`
}

// SearchSessionsResult is the output of SearchSessions.
type SearchSessionsResult struct {
	Operation    string        `json:"operation"`
	Query        string        `json:"query"`
	Matches      []SearchMatch `json:"matches"`
	TotalMatches int           `json:"total_matches"`
}

// SearchSessions searches for sessions by name (case-insensitive).
func SearchSessions(ctx *OpContext, req *SearchSessionsRequest) (*SearchSessionsResult, error) {
	if req == nil || req.Query == "" {
		return nil, ErrInvalidInput("query", "Search query is required.")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		return nil, ErrInvalidInput("limit", "Search limit must be between 1 and 50.")
	}

	// List all sessions, then filter by name match
	listReq := &ListSessionsRequest{
		Status: req.Status,
		Limit:  1000, // Get all for searching
	}
	if listReq.Status == "" {
		listReq.Status = "active"
	}

	listResult, err := ListSessions(ctx, listReq)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(req.Query)
	matches := make([]SearchMatch, 0)

	for _, s := range listResult.Sessions {
		score := calculateRelevance(s.Name, queryLower)
		if score > 0 {
			matches = append(matches, SearchMatch{
				ID:             s.ID,
				Name:           s.Name,
				Status:         s.Status,
				Harness:        s.Harness,
				Project:        s.Project,
				RelevanceScore: score,
			})
		}
	}

	// Sort by relevance (highest first) — simple insertion sort for small N
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].RelevanceScore > matches[j-1].RelevanceScore; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	// Apply limit
	if len(matches) > limit {
		matches = matches[:limit]
	}

	return &SearchSessionsResult{
		Operation:    "search_sessions",
		Query:        req.Query,
		Matches:      matches,
		TotalMatches: len(matches),
	}, nil
}

func calculateRelevance(name, queryLower string) float64 {
	nameLower := strings.ToLower(name)

	if nameLower == queryLower {
		return 1.0
	}
	if strings.HasPrefix(nameLower, queryLower) {
		return 0.8
	}
	if strings.Contains(nameLower, queryLower) {
		return 0.5
	}
	return 0.0
}

// manifestMatchesQuery checks if a manifest matches a search query.
// Used internally; exported for testing.
func manifestMatchesQuery(m *manifest.Manifest, query string) bool {
	queryLower := strings.ToLower(query)
	return strings.Contains(strings.ToLower(m.Name), queryLower)
}
