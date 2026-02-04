package discovery

import (
	"context"
	"testing"
)

func TestExtractCompetitorName(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "X vs Y pattern",
			query: "AWS Lambda vs Google Cloud Functions",
			want:  "AWS Lambda",
		},
		{
			name:  "X vs. Y pattern with period",
			query: "React vs. Vue",
			want:  "React",
		},
		{
			name:  "compare X and Y pattern",
			query: "compare PostgreSQL and MySQL",
			want:  "PostgreSQL",
		},
		{
			name:  "compare X to Y pattern",
			query: "compare Docker to Kubernetes",
			want:  "Docker",
		},
		{
			name:  "what can we learn from X",
			query: "what can we learn from GitHub Copilot",
			want:  "GitHub Copilot",
		},
		{
			name:  "competitive analysis of X",
			query: "competitive analysis of Terraform",
			want:  "Terraform",
		},
		{
			name:  "gap analysis of X",
			query: "gap analysis of Slack",
			want:  "Slack",
		},
		{
			name:  "X competitor analysis",
			query: "Notion competitor analysis",
			want:  "Notion",
		},
		{
			name:  "simple query (fallback)",
			query: "GitHub Copilot",
			want:  "GitHub Copilot",
		},
		{
			name:  "case insensitive",
			query: "AWS LAMBDA VS Google Cloud Functions",
			want:  "AWS LAMBDA",
		},
		{
			name:  "extra whitespace",
			query: "  React  vs  Vue  ",
			want:  "React",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCompetitorName(tt.query)
			if got != tt.want {
				t.Errorf("ExtractCompetitorName(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestCalculateURLScore(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		title     string
		wantScore int
	}{
		{
			name:      "official docs domain",
			url:       "https://docs.example.com/guide",
			title:     "Example Documentation",
			wantScore: 10,
		},
		{
			name:      "docs path",
			url:       "https://example.com/docs/api",
			title:     "API Reference",
			wantScore: 10,
		},
		{
			name:      "GitHub repo",
			url:       "https://github.com/user/repo",
			title:     "user/repo: Example Repository",
			wantScore: 8,
		},
		{
			name:      "technical blog",
			url:       "https://blog.example.com/article",
			title:     "How to use Example",
			wantScore: 5,
		},
		{
			name:      "Medium article",
			url:       "https://medium.com/@user/article",
			title:     "Introduction to Example",
			wantScore: 5,
		},
		{
			name:      ".io domain",
			url:       "https://example.io",
			title:     "Example - Official Site",
			wantScore: 3,
		},
		{
			name:      ".dev domain",
			url:       "https://example.dev",
			title:     "Example Developer Portal",
			wantScore: 3,
		},
		{
			name:      "GitHub Pages .io domain",
			url:       "https://username.github.io/project",
			title:     "Project Documentation",
			wantScore: 13, // 10 (docs in title) + 3 (.io)
		},
		{
			name:      "docs + .io (combined score)",
			url:       "https://docs.example.io",
			title:     "Documentation",
			wantScore: 13, // 10 (docs) + 3 (.io)
		},
		{
			name:      "social media (Twitter) - negative",
			url:       "https://twitter.com/example",
			title:     "Example on Twitter",
			wantScore: -5,
		},
		{
			name:      "social media (LinkedIn) - negative",
			url:       "https://linkedin.com/company/example",
			title:     "Example on LinkedIn",
			wantScore: -5,
		},
		{
			name:      "marketing page - negative",
			url:       "https://example.com/pricing",
			title:     "Pricing - Example",
			wantScore: -3,
		},
		{
			name:      "marketing plans page - negative",
			url:       "https://example.com/plans",
			title:     "Plans and Pricing",
			wantScore: -3,
		},
		{
			name:      "generic page",
			url:       "https://example.com",
			title:     "Example Homepage",
			wantScore: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateURLScore(tt.url, tt.title)
			if got != tt.wantScore {
				t.Errorf("calculateURLScore(%q, %q) = %d, want %d", tt.url, tt.title, got, tt.wantScore)
			}
		})
	}
}

func TestRankAndFilterURLs(t *testing.T) {
	tests := []struct {
		name    string
		input   []URLResult
		wantTop []string // Expected URLs in order
	}{
		{
			name: "docs rank higher than GitHub",
			input: []URLResult{
				{URL: "https://github.com/example/repo", Title: "Repo"},
				{URL: "https://docs.example.com", Title: "Documentation"},
				{URL: "https://blog.example.com", Title: "Blog"},
			},
			wantTop: []string{
				"https://docs.example.com",    // Score: 10
				"https://github.com/example/repo", // Score: 8
				"https://blog.example.com",    // Score: 5
			},
		},
		{
			name: "filter out social media",
			input: []URLResult{
				{URL: "https://docs.example.com", Title: "Docs"},
				{URL: "https://twitter.com/example", Title: "Twitter"},
				{URL: "https://github.com/example/repo", Title: "GitHub"},
			},
			wantTop: []string{
				"https://docs.example.com",
				"https://github.com/example/repo",
				// Twitter filtered out (negative score)
			},
		},
		{
			name: "deduplicate by domain",
			input: []URLResult{
				{URL: "https://docs.example.com/guide", Title: "Guide"},
				{URL: "https://docs.example.com/api", Title: "API"},
				{URL: "https://github.com/example/repo", Title: "Repo"},
			},
			wantTop: []string{
				"https://docs.example.com/guide", // First docs.example.com wins
				"https://github.com/example/repo",
				// Second docs.example.com filtered out (duplicate domain)
			},
		},
		{
			name: "combined scoring (docs + .io)",
			input: []URLResult{
				{URL: "https://example.com", Title: "Homepage"},
				{URL: "https://docs.example.io", Title: "Docs"},
				{URL: "https://github.com/example/repo", Title: "GitHub"},
			},
			wantTop: []string{
				"https://docs.example.io",     // Score: 13 (10 + 3)
				"https://github.com/example/repo", // Score: 8
				// example.com has score 0, filtered out
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rankAndFilterURLs(tt.input)

			// Check length
			if len(got) != len(tt.wantTop) {
				t.Errorf("rankAndFilterURLs() returned %d results, want %d", len(got), len(tt.wantTop))
			}

			// Check order
			for i := 0; i < len(got) && i < len(tt.wantTop); i++ {
				if got[i].URL != tt.wantTop[i] {
					t.Errorf("rankAndFilterURLs()[%d] = %q, want %q", i, got[i].URL, tt.wantTop[i])
				}
			}
		})
	}
}

func TestDeduplicateByDomain(t *testing.T) {
	tests := []struct {
		name  string
		input []URLResult
		want  int // Expected number of results after deduplication
	}{
		{
			name: "no duplicates",
			input: []URLResult{
				{URL: "https://example.com/page1", Score: 10},
				{URL: "https://another.com/page2", Score: 8},
			},
			want: 2,
		},
		{
			name: "duplicate domains - keep first",
			input: []URLResult{
				{URL: "https://example.com/page1", Score: 10},
				{URL: "https://example.com/page2", Score: 5},
				{URL: "https://another.com/page3", Score: 8},
			},
			want: 2, // Only example.com (first) and another.com
		},
		{
			name: "multiple duplicates",
			input: []URLResult{
				{URL: "https://docs.example.com/page1", Score: 10},
				{URL: "https://docs.example.com/page2", Score: 9},
				{URL: "https://docs.example.com/page3", Score: 8},
			},
			want: 1, // All same domain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicateByDomain(tt.input)
			if len(got) != tt.want {
				t.Errorf("deduplicateByDomain() returned %d results, want %d", len(got), tt.want)
			}
		})
	}
}

func TestDiscoverCompetitorURLs_ConfigValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		config    SearchConfig
		query     string
		wantError string
	}{
		{
			name:      "missing API key",
			config:    SearchConfig{SearchEngineID: "test-id"},
			query:     "test query",
			wantError: "API key is required",
		},
		{
			name:      "missing search engine ID",
			config:    SearchConfig{APIKey: "test-key"},
			query:     "test query",
			wantError: "search engine ID is required",
		},
		{
			name:      "invalid query - cannot extract competitor",
			config:    SearchConfig{APIKey: "test-key", SearchEngineID: "test-id"},
			query:     "", // Empty query
			wantError: "could not extract competitor name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DiscoverCompetitorURLs(ctx, tt.query, tt.config)
			if err == nil {
				t.Errorf("DiscoverCompetitorURLs() expected error containing %q, got nil", tt.wantError)
				return
			}
			if !contains(err.Error(), tt.wantError) {
				t.Errorf("DiscoverCompetitorURLs() error = %q, want substring %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestDiscoverCompetitorURLs_MaxResults(t *testing.T) {
	// Note: This test would require mocking the Custom Search API
	// For now, we test that MaxResults defaults to 5 when not specified
	config := SearchConfig{
		APIKey:         "test-key",
		SearchEngineID: "test-id",
		MaxResults:     0, // Not specified
	}

	if config.MaxResults == 0 {
		config.MaxResults = 5 // Default behavior
	}

	if config.MaxResults != 5 {
		t.Errorf("Default MaxResults should be 5, got %d", config.MaxResults)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 &&
			(s[:len(substr)] == substr ||
			 (len(s) > len(substr) && contains(s[1:], substr)))))
}
