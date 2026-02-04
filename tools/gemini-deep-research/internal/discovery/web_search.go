// Package discovery implements competitor URL discovery via web search.
package discovery

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"google.golang.org/api/customsearch/v1"
	"google.golang.org/api/option"
)

// SearchConfig holds configuration for web search
type SearchConfig struct {
	APIKey         string // Google Custom Search API key
	SearchEngineID string // Programmable Search Engine ID
	MaxResults     int    // Maximum URLs to return (default: 5)
}

// URLResult represents a discovered URL with its relevance score
type URLResult struct {
	URL   string
	Title string
	Score int
}

// DiscoverCompetitorURLs discovers relevant URLs for a competitor using Google Custom Search.
// Extracts competitor name from query (e.g., "X vs Y" → "X"), searches for official docs,
// GitHub repos, and technical resources, then ranks and filters results.
func DiscoverCompetitorURLs(ctx context.Context, query string, config SearchConfig) ([]string, error) {
	// Validate config
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required for web search")
	}
	if config.SearchEngineID == "" {
		return nil, fmt.Errorf("search engine ID is required for web search")
	}
	if config.MaxResults == 0 {
		config.MaxResults = 5 // Default to 5 URLs
	}

	// Extract competitor name from query
	competitor := ExtractCompetitorName(query)
	if competitor == "" {
		return nil, fmt.Errorf("could not extract competitor name from query: %s", query)
	}

	// Create Custom Search client
	service, err := customsearch.NewService(ctx, option.WithAPIKey(config.APIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Custom Search client: %w", err)
	}

	// Build search queries for different content types
	searches := []string{
		fmt.Sprintf("%s documentation", competitor),
		fmt.Sprintf("%s GitHub", competitor),
		fmt.Sprintf("%s official site", competitor),
	}

	// Collect all results
	var allResults []URLResult
	for _, searchQuery := range searches {
		results, err := performSearch(ctx, service, searchQuery, config.SearchEngineID)
		if err != nil {
			// Log error but continue with other searches
			fmt.Printf("Warning: Search failed for '%s': %v\n", searchQuery, err)
			continue
		}
		allResults = append(allResults, results...)
	}

	if len(allResults) == 0 {
		return nil, fmt.Errorf("no results found for competitor: %s", competitor)
	}

	// Rank and filter URLs
	rankedResults := rankAndFilterURLs(allResults)

	// Extract top N URLs
	maxResults := config.MaxResults
	if len(rankedResults) < maxResults {
		maxResults = len(rankedResults)
	}

	urls := make([]string, maxResults)
	for i := 0; i < maxResults; i++ {
		urls[i] = rankedResults[i].URL
	}

	return urls, nil
}

// ExtractCompetitorName extracts the first competitor name from a query.
// Examples:
//   - "X vs Y" → "X"
//   - "compare X and Y" → "X"
//   - "what can we learn from X" → "X"
//   - "competitive analysis of X" → "X"
func ExtractCompetitorName(query string) string {
	// Pattern 1: "X vs Y" or "X vs. Y"
	vsPattern := regexp.MustCompile(`(?i)^(.+?)\s+vs\.?\s+.+$`)
	if matches := vsPattern.FindStringSubmatch(query); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Pattern 2: "compare X and Y" or "compare X to Y"
	comparePattern := regexp.MustCompile(`(?i)compare\s+(.+?)\s+(?:and|to|with)\s+.+$`)
	if matches := comparePattern.FindStringSubmatch(query); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Pattern 3: "what can we learn from X"
	learnPattern := regexp.MustCompile(`(?i)what\s+can\s+we\s+learn\s+from\s+(.+)$`)
	if matches := learnPattern.FindStringSubmatch(query); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Pattern 4: "competitive analysis of X" or "gap analysis of X"
	analysisPattern := regexp.MustCompile(`(?i)(?:competitive|gap)\s+analysis\s+of\s+(.+)$`)
	if matches := analysisPattern.FindStringSubmatch(query); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Pattern 5: "X competitor analysis"
	competitorPattern := regexp.MustCompile(`(?i)^(.+?)\s+competitor\s+analysis$`)
	if matches := competitorPattern.FindStringSubmatch(query); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Fallback: use entire query (for simple queries like "GitHub Copilot")
	return strings.TrimSpace(query)
}

// performSearch executes a single Custom Search API query
func performSearch(ctx context.Context, service *customsearch.Service, query string, searchEngineID string) ([]URLResult, error) {
	// Execute search
	call := service.Cse.List().Cx(searchEngineID).Q(query).Num(10) // Request 10 results
	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("search API call failed: %w", err)
	}

	// Parse results
	var results []URLResult
	for _, item := range resp.Items {
		results = append(results, URLResult{
			URL:   item.Link,
			Title: item.Title,
			Score: 0, // Score will be calculated in rankAndFilterURLs
		})
	}

	return results, nil
}

// rankAndFilterURLs scores URLs based on domain patterns and filters out unwanted domains.
// Scoring (from S6 TD4):
//   - Official docs: +10 points
//   - GitHub repos: +8 points
//   - Technical blogs: +5 points
//   - Social media: -5 points (filtered out)
func rankAndFilterURLs(results []URLResult) []URLResult {
	// Score each URL
	for i := range results {
		results[i].Score = calculateURLScore(results[i].URL, results[i].Title)
	}

	// Filter out low-scoring URLs (social media, marketing sites)
	var filtered []URLResult
	for _, result := range results {
		if result.Score > 0 { // Keep only positive scores
			filtered = append(filtered, result)
		}
	}

	// Sort by score (descending)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	// Remove duplicates (same domain)
	deduped := deduplicateByDomain(filtered)

	return deduped
}

// calculateURLScore assigns a score to a URL based on domain patterns
func calculateURLScore(urlStr string, title string) int {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return 0
	}

	domain := strings.ToLower(parsedURL.Hostname())
	path := strings.ToLower(parsedURL.Path)
	titleLower := strings.ToLower(title)

	score := 0

	// Official documentation sites: +10
	if strings.Contains(domain, "docs.") ||
		strings.Contains(domain, "documentation") ||
		strings.Contains(path, "/docs/") ||
		strings.Contains(path, "/documentation/") ||
		strings.Contains(titleLower, "documentation") {
		score += 10
	}

	// GitHub repositories: +8
	if strings.Contains(domain, "github.com") {
		score += 8
	}

	// Technical blogs: +5
	if strings.Contains(domain, "blog") ||
		strings.Contains(domain, "medium.com") ||
		strings.Contains(domain, "dev.to") ||
		strings.Contains(domain, "hashnode") {
		score += 5
	}

	// Official sites (.io, .dev, .org): +3
	if strings.HasSuffix(domain, ".io") ||
		strings.HasSuffix(domain, ".dev") ||
		strings.HasSuffix(domain, ".org") {
		score += 3
	}

	// Social media: -5 (filtered out)
	if strings.Contains(domain, "twitter.com") ||
		strings.Contains(domain, "x.com") ||
		strings.Contains(domain, "facebook.com") ||
		strings.Contains(domain, "linkedin.com") ||
		strings.Contains(domain, "instagram.com") ||
		strings.Contains(domain, "reddit.com") {
		score -= 5
	}

	// Marketing/landing pages: -3
	if strings.Contains(path, "/pricing") ||
		strings.Contains(path, "/plans") ||
		strings.Contains(path, "/buy") {
		score -= 3
	}

	return score
}

// deduplicateByDomain removes duplicate URLs from the same domain, keeping highest scored
func deduplicateByDomain(results []URLResult) []URLResult {
	seen := make(map[string]bool)
	var deduped []URLResult

	for _, result := range results {
		parsedURL, err := url.Parse(result.URL)
		if err != nil {
			continue
		}

		domain := parsedURL.Hostname()
		if !seen[domain] {
			seen[domain] = true
			deduped = append(deduped, result)
		}
	}

	return deduped
}
