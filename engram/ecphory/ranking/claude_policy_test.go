package ranking

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildClaudeRankingPromptExact(t *testing.T) {
	candidates := []Candidate{
		{
			Name:        "oauth-pattern",
			Description: "OAuth 2.0 implementation pattern",
			Tags:        []string{"security", "authentication"},
		},
		{Name: "jwt-validation"},
	}
	want := `Query: implement OAuth

Candidates:
0. Name: oauth-pattern
   Description: OAuth 2.0 implementation pattern
   Tags: [security authentication]

1. Name: jwt-validation


Provide a ranked list of candidate indices (0-indexed) with scores and reasoning.`

	assert.Equal(t, want, buildClaudeRankingPrompt("implement OAuth", candidates))
}

func TestParseClaudeRankingResponse(t *testing.T) {
	candidates := []Candidate{
		{Name: "first"},
		{Name: "second"},
		{Name: "third"},
	}

	tests := []struct {
		name    string
		content []anthropic.ContentBlockUnion
		want    []RankedResult
		wantErr string
	}{
		{
			name: "structured results preserve response order and skip invalid indices",
			content: []anthropic.ContentBlockUnion{
				{Type: "thinking", Text: "ignored"},
				{Type: "text", Text: `[{"index":2,"score":0.9,"reasoning":"best"},`},
				{Type: "text", Text: `{"index":8,"score":0.7,"reasoning":"invalid"},{"index":0,"score":0.4,"reasoning":"last"}]`},
			},
			want: []RankedResult{
				{Candidate: candidates[2], Score: 0.9, Reasoning: "best"},
				{Candidate: candidates[0], Score: 0.4, Reasoning: "last"},
			},
		},
		{
			name:    "malformed structured output uses deterministic fallback",
			content: []anthropic.ContentBlockUnion{{Type: "text", Text: "not json"}},
			want: []RankedResult{
				{Candidate: candidates[0], Score: 1, Reasoning: "Fallback ranking (structured output parsing failed)"},
				{Candidate: candidates[1], Score: 0.5, Reasoning: "Fallback ranking (structured output parsing failed)"},
				{Candidate: candidates[2], Score: 1.0 / 3.0, Reasoning: "Fallback ranking (structured output parsing failed)"},
			},
		},
		{
			name:    "empty text response fails",
			content: []anthropic.ContentBlockUnion{{Type: "thinking", Text: "not visible response text"}},
			wantErr: "empty response from API",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseClaudeRankingResponse(&anthropic.Message{Content: tt.content}, candidates)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClaudeRankingSystemPromptDefinesStructuredContract(t *testing.T) {
	assert.Contains(t, claudeRankingSystemPrompt, `"index": the 0-based index`)
	assert.Contains(t, claudeRankingSystemPrompt, `"score": a float between 0.0`)
	assert.Contains(t, claudeRankingSystemPrompt, `"reasoning": a brief explanation`)
}
