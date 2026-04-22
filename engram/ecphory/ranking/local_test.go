package ranking_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/engram/ecphory/ranking"
)

func TestLocalProvider_Name(t *testing.T) {
	provider := ranking.NewLocal()
	assert.Equal(t, "local", provider.Name())
}

func TestLocalProvider_Model(t *testing.T) {
	provider := ranking.NewLocal()
	assert.Equal(t, "local-jaccard-v1", provider.Model())
}

func TestLocalProvider_Capabilities(t *testing.T) {
	provider := ranking.NewLocal()
	caps := provider.Capabilities()

	assert.False(t, caps.SupportsCaching)
	assert.False(t, caps.SupportsStructuredOutput)
	assert.Equal(t, 0, caps.MaxConcurrentRequests)
	assert.Equal(t, 0, caps.MaxTokensPerRequest)
}

func TestLocalProvider_Rank_ExactMatch(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{
			Name:        "oauth-pattern",
			Description: "OAuth implementation",
			Tags:        []string{"oauth", "security", "authentication"},
		},
		{
			Name:        "jwt-pattern",
			Description: "JWT validation",
			Tags:        []string{"jwt", "security", "tokens"},
		},
	}

	results, err := provider.Rank(ctx, "oauth security", candidates)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// oauth-pattern should rank higher (2 matching tags vs 1)
	assert.Equal(t, "oauth-pattern", results[0].Candidate.Name)
	assert.Greater(t, results[0].Score, results[1].Score)
	assert.Contains(t, results[0].Reasoning, "oauth")
	assert.Contains(t, results[0].Reasoning, "security")
}

func TestLocalProvider_Rank_NoMatch(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{
			Name: "oauth-pattern",
			Tags: []string{"oauth", "security"},
		},
	}

	results, err := provider.Rank(ctx, "database postgres", candidates)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	assert.Equal(t, 0.0, results[0].Score)
	assert.Equal(t, "No matches", results[0].Reasoning)
}

func TestLocalProvider_Rank_PartialMatch(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{
			Name: "oauth-pattern",
			Tags: []string{"oauth", "security", "authentication", "authorization"},
		},
	}

	// Query has 2 keywords, candidate has 4 tags, 1 matches
	// Tag Jaccard = 1 / (2 + 4 - 1) = 1/5 = 0.2, blended = 0.2*0.7 = 0.14
	results, err := provider.Rank(ctx, "oauth database", candidates)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	assert.InDelta(t, 0.14, results[0].Score, 0.01)
	assert.Contains(t, results[0].Reasoning, "oauth")
}

func TestLocalProvider_Rank_CaseInsensitive(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{
			Name: "oauth-pattern",
			Tags: []string{"OAuth", "Security", "Authentication"},
		},
	}

	results, err := provider.Rank(ctx, "oauth security", candidates)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Should match despite case differences
	assert.Greater(t, results[0].Score, 0.0)
	assert.Contains(t, results[0].Reasoning, "oauth")
	assert.Contains(t, results[0].Reasoning, "security")
}

func TestLocalProvider_Rank_Sorting(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{
			Name: "low-match",
			Tags: []string{"database"},
		},
		{
			Name: "high-match",
			Tags: []string{"oauth", "security"},
		},
		{
			Name: "medium-match",
			Tags: []string{"oauth"},
		},
	}

	results, err := provider.Rank(ctx, "oauth security", candidates)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Should be sorted by score descending
	assert.Equal(t, "high-match", results[0].Candidate.Name)
	assert.Equal(t, "medium-match", results[1].Candidate.Name)
	assert.Equal(t, "low-match", results[2].Candidate.Name)

	// Verify scores are decreasing
	assert.GreaterOrEqual(t, results[0].Score, results[1].Score)
	assert.GreaterOrEqual(t, results[1].Score, results[2].Score)
}

func TestLocalProvider_Rank_EmptyQuery(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{
			Name: "oauth-pattern",
			Tags: []string{"oauth", "security"},
		},
	}

	results, err := provider.Rank(ctx, "", candidates)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Empty query should result in 0 score
	assert.Equal(t, 0.0, results[0].Score)
}

func TestLocalProvider_Rank_EmptyTags(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{
			Name: "no-tags",
			Tags: []string{},
		},
	}

	results, err := provider.Rank(ctx, "oauth security", candidates)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// No tags should result in 0 score
	assert.Equal(t, 0.0, results[0].Score)
	assert.Equal(t, "No matches", results[0].Reasoning)
}

func TestLocalProvider_Rank_MultipleCandidates(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{Name: "oauth", Tags: []string{"oauth", "security", "auth"}},
		{Name: "jwt", Tags: []string{"jwt", "security", "tokens"}},
		{Name: "database", Tags: []string{"database", "sql", "postgres"}},
		{Name: "api", Tags: []string{"api", "rest", "http"}},
		{Name: "auth", Tags: []string{"authentication", "security", "oauth"}},
	}

	results, err := provider.Rank(ctx, "oauth security authentication", candidates)
	require.NoError(t, err)
	assert.Len(t, results, 5)

	// Top results should be oauth and auth (most relevant)
	assert.Contains(t, []string{"oauth", "auth"}, results[0].Candidate.Name)
	assert.Contains(t, []string{"oauth", "auth"}, results[1].Candidate.Name)

	// All results should have scores in [0.0, 1.0]
	for _, result := range results {
		assert.GreaterOrEqual(t, result.Score, 0.0)
		assert.LessOrEqual(t, result.Score, 1.0)
	}
}

func TestLocalProvider_Rank_JaccardScore(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	// Test exact Jaccard calculation with blending
	// Query: {a, b} = 2 keywords
	// Tags: {a, b} = 2 tags → tag Jaccard = 2/2 = 1.0
	// Description: empty → desc Jaccard = 0.0
	// Blended: 1.0*0.7 + 0.0*0.3 = 0.7
	candidates := []ranking.Candidate{
		{
			Name: "perfect-match",
			Tags: []string{"a", "b"},
		},
	}

	results, err := provider.Rank(ctx, "a b", candidates)
	require.NoError(t, err)
	assert.InDelta(t, 0.7, results[0].Score, 0.01)
}

func TestLocalProvider_Rank_Performance(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	// Test with large number of candidates
	candidates := make([]ranking.Candidate, 100)
	for i := range 100 {
		candidates[i] = ranking.Candidate{
			Name: string(rune('a' + i%26)),
			Tags: []string{"tag1", "tag2", "tag3"},
		}
	}

	// Should complete quickly (< 1ms typically)
	results, err := provider.Rank(ctx, "tag1 tag2", candidates)
	require.NoError(t, err)
	assert.Len(t, results, 100)
}

func TestLocalProvider_Rank_DuplicateKeywords(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{
			Name: "oauth",
			Tags: []string{"oauth", "security"},
		},
	}

	// Duplicate keywords should be deduplicated
	results, err := provider.Rank(ctx, "oauth oauth security security", candidates)
	require.NoError(t, err)

	// Score should be same as "oauth security" (duplicates removed)
	assert.Greater(t, results[0].Score, 0.0)
}

func TestLocalProvider_Rank_WhitespaceHandling(t *testing.T) {
	provider := ranking.NewLocal()
	ctx := context.Background()

	candidates := []ranking.Candidate{
		{
			Name: "oauth",
			Tags: []string{"oauth", "security"},
		},
	}

	// Extra whitespace should be handled
	results, err := provider.Rank(ctx, "  oauth   security  ", candidates)
	require.NoError(t, err)

	assert.Greater(t, results[0].Score, 0.0)
	assert.Contains(t, results[0].Reasoning, "oauth")
}
